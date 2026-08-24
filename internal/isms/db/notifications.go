package db

import (
	"context"
	"encoding/json"
	"fmt"
)

// Notification represents an inbox item for a user.
//
// Title and Body are the English originals and are always populated. TitleKey,
// BodyKey and Params are the translatable form: the client renders the key with
// the params and falls back to Title when there is no key or no message for it.
// All three are omitempty so legacy rows and agent rows serialise exactly as
// they did before this existed.
type Notification struct {
	ID             int            `json:"id"`
	OrganizationID int            `json:"organization_id"`
	RecipientID    int            `json:"recipient_id"`
	Title          string         `json:"title"`
	Body           string         `json:"body,omitempty"`
	Link           string         `json:"link,omitempty"`
	TitleKey       string         `json:"title_key,omitempty"`
	BodyKey        string         `json:"body_key,omitempty"`
	Params         map[string]any `json:"params,omitempty"`
	Read           bool           `json:"read"`
	CreatedAt      Epoch          `json:"created_at"`
}

// NotificationContent is the write-side shape for a keyed notification.
//
// A struct rather than three more positional strings on four function
// signatures: the call sites migrate one at a time, and a positional
// (title, titleKey, body, bodyKey, params, link) list is exactly the kind of
// argument order that gets transposed silently.
type NotificationContent struct {
	// Title is the English original and MUST always be set — it is what every
	// existing client renders and the fallback for every future one.
	Title string
	// TitleKey is "" for rows that are deliberately not translatable: agent
	// notifications consumed over MCP, and anything whose title is not copy.
	TitleKey string
	Body     string
	// BodyKey is "" when Body is org-authored content rather than product copy
	// — an incident description, a mention snippet. Those must never be
	// translated, and "" is how that is expressed.
	BodyKey string
	// Params interpolate into the keyed frames. Keys are a closed set; see the
	// column comments on the migration for which ones are translated before
	// interpolation and which pass through verbatim.
	Params map[string]any
	Link   string
}

// paramsJSON marshals Params for the JSONB column, returning nil (SQL NULL) for
// an empty map. A marshal failure drops the params rather than the whole
// notification: the English Title still renders, so a lost translation beats a
// lost inbox item.
func (c NotificationContent) paramsJSON() []byte {
	if len(c.Params) == 0 {
		return nil
	}
	b, err := json.Marshal(c.Params)
	if err != nil {
		return nil
	}
	return b
}

func (d *DB) CreateNotification(ctx context.Context, orgID int, n *Notification) error {
	n.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO notifications (organization_id, recipient_id, title, body, link, title_key, body_key, params)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`, orgID, n.RecipientID, n.Title, nilIfEmpty(n.Body), nilIfEmpty(n.Link),
		nilIfEmpty(n.TitleKey), nilIfEmpty(n.BodyKey),
		NotificationContent{Params: n.Params}.paramsJSON(),
	).Scan(&n.ID, &n.CreatedAt)
}

// CreateNotificationByEmail creates a notification for a user identified by email.
// If the user is not found, the notification is silently dropped.
//
// Kept as-is so unconverted call sites keep compiling and behaving identically;
// it delegates with no keys, which is the same "English only, no translation"
// state every row was in before. New call sites should use
// CreateNotificationContentByEmail.
func (d *DB) CreateNotificationByEmail(ctx context.Context, orgID int, recipientEmail string, title, body, link string) error {
	return d.CreateNotificationContentByEmail(ctx, orgID, recipientEmail, NotificationContent{
		Title: title, Body: body, Link: link,
	})
}

// CreateNotificationContentByEmail creates a keyed notification for a user
// identified by email. If the user is not found the notification is silently
// dropped, matching CreateNotificationByEmail.
func (d *DB) CreateNotificationContentByEmail(ctx context.Context, orgID int, recipientEmail string, c NotificationContent) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO notifications (organization_id, recipient_id, title, body, link, title_key, body_key, params)
		SELECT $1, u.id, $3, $4, $5, $6, $7, $8
		FROM users u WHERE u.email = $2
	`, orgID, recipientEmail, c.Title, nilIfEmpty(c.Body), nilIfEmpty(c.Link),
		nilIfEmpty(c.TitleKey), nilIfEmpty(c.BodyKey), c.paramsJSON())
	return err
}

func (d *DB) ListNotifications(ctx context.Context, orgID int, userID int, unreadOnly bool, limit int) ([]Notification, error) {
	query := `SELECT id, organization_id, recipient_id, title, COALESCE(body, ''), COALESCE(link, ''),
			COALESCE(title_key, ''), COALESCE(body_key, ''), params, read, created_at
		FROM notifications WHERE organization_id = $1 AND recipient_id = $2`
	args := []interface{}{orgID, userID}
	if unreadOnly {
		query += ` AND read = false`
	}
	query += ` ORDER BY created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialise (not a nil slice) so an empty result serialises as JSON [] not
	// null — frontend/tests can iterate the result without a null guard.
	notifications := []Notification{}
	for rows.Next() {
		var n Notification
		// params is JSONB and NULL on every legacy row, so it scans into a
		// []byte that may be nil — not straight into the map.
		var params []byte
		if err := rows.Scan(&n.ID, &n.OrganizationID, &n.RecipientID, &n.Title, &n.Body, &n.Link,
			&n.TitleKey, &n.BodyKey, &params, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		if len(params) > 0 {
			// A row with unreadable params still renders from Title, so drop the
			// params rather than failing the whole inbox listing.
			_ = json.Unmarshal(params, &n.Params)
		}
		notifications = append(notifications, n)
	}
	return notifications, nil
}

func (d *DB) MarkRead(ctx context.Context, orgID int, id int, recipientID int) error {
	_, err := d.pool.Exec(ctx, `UPDATE notifications SET read = true WHERE id = $1 AND organization_id = $2 AND recipient_id = $3`, id, orgID, recipientID)
	return err
}

func (d *DB) MarkAllRead(ctx context.Context, orgID int, userID int) error {
	_, err := d.pool.Exec(ctx, `UPDATE notifications SET read = true WHERE organization_id = $1 AND recipient_id = $2 AND read = false`, orgID, userID)
	return err
}

func (d *DB) UnreadCount(ctx context.Context, orgID int, userID int) (int, error) {
	var count int
	err := d.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id = $1 AND recipient_id = $2 AND read = false`, orgID, userID).Scan(&count)
	return count, err
}
