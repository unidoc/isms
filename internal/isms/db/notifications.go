package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

// The closed set of interpolation param names, split by how the client must
// treat each one before it reaches a translated frame.
// Mirrors the `params` column comment on
// migrations/20260824000000_v0.8.0.sql — that comment is the normative
// statement of the split; this is its executable form.
//
// The split is load-bearing, not cosmetic: splicing an untranslated enum value
// into a translated sentence yields half-translated output, so the translatable
// names resolve through common.enum.* / common.entity.* BEFORE interpolation
// (see resolveParams in web/src/composables/useNotificationRender.js, which
// keys off these same names) while the verbatim ones are proper nouns, numbers
// or the user's own words and pass through untouched.
var (
	// NotificationEnumParams resolve through common.enum.<param>.<value>. Each
	// name IS its enum group — the groups were named after the params in #229
	// precisely so there is no second mapping to keep in sync.
	NotificationEnumParams = []string{"status", "severity", "action", "suggestion_type"}

	// NotificationEntityParam is the one translatable param that is a noun the
	// whole app reuses rather than an enum member, so it resolves through
	// common.entity.* instead.
	NotificationEntityParam = "entity"

	// NotificationVerbatimParams interpolate as-is.
	NotificationVerbatimParams = []string{"actor", "title", "doc_id", "version", "round", "id", "note", "reason"}
)

// notificationParamAllowed reports whether name is in the closed set.
func notificationParamAllowed(name string) bool {
	if name == NotificationEntityParam {
		return true
	}
	for _, n := range NotificationEnumParams {
		if n == name {
			return true
		}
	}
	for _, n := range NotificationVerbatimParams {
		if n == name {
			return true
		}
	}
	return false
}

// Validate reports an unknown param name.
//
// An unlisted name means one of two things, and both are silent in production:
// an enum spliced into a frame with no group to translate it through, or a
// param no frame interpolates at all (a typo, or a rename on one side only).
// Neither produces an error, a warning, or a diff that reads wrong to an
// English-speaking reviewer — the frame simply renders with the placeholder
// unfilled or the value untranslated, and only for a non-English user.
//
// Note the closed set here is NOT the one plan 81 §1 defines for API errors
// (entity/field/status/count). Different surface, different set; they are easy
// to conflate.
func (c NotificationContent) Validate() error {
	for name := range c.Params {
		if !notificationParamAllowed(name) {
			return fmt.Errorf("notification param %q is not in the closed set (see the Notification*Params vars)", name)
		}
	}
	return nil
}

// paramsJSON marshals Params for the JSONB column, returning nil (SQL NULL) for
// an empty map. ok is false only when a non-empty map failed to marshal — the
// caller must then store the row English-only (see keyedColumns): the params a
// keyed frame needs are gone, and a key without them renders with unfilled
// placeholders, which is worse than the English original.
func (c NotificationContent) paramsJSON() (b []byte, ok bool) {
	if len(c.Params) == 0 {
		return nil, true
	}
	b, err := json.Marshal(c.Params)
	if err != nil {
		return nil, false
	}
	return b, true
}

// keyedColumns returns the three translation columns as they should be written.
// Keys and params travel together: if the params could not be marshalled the
// keys are dropped too, so the row is unambiguously English-only and the client
// takes the Title/Body fallback instead of rendering a half-filled frame. A
// lost translation beats a lost inbox item; a broken translation beats neither.
//
// A param outside the closed set degrades the same way, for the same reason.
// Plan 82 step 7 proposed failing the write on an unknown name, to make the
// guard a runtime guarantee as well as a test-time one — but a failed write
// here IS a lost inbox item, which the line above already decided against, and
// a param that fails validation is by definition one no frame renders
// correctly anyway. So the row goes in English-only.
//
// But degrading quietly would make a call-site typo LESS visible than it was
// before this check existed: an unknown param used to be stored, and the frame
// then rendered with an unfilled slot that every user saw, English included.
// Dropping the keys renders the English title perfectly and hides the bug from
// exactly the reviewer most likely to catch it. So this branch logs, and that
// log is the call-site half of the guarantee — the Go tests can only check the
// catalogue side, because params are inline map literals at each call site and
// no amount of constant-declaring makes them statically enumerable.
//
// The marshal branch below stays silent by contrast: a map[string]any failing
// to marshal is not a realistic programmer error, while a param typo is.
func (c NotificationContent) keyedColumns() (titleKey, bodyKey *string, params []byte) {
	if err := c.Validate(); err != nil {
		log.Printf("notification %q: %v (storing English-only)", c.TitleKey, err)
		return nil, nil, nil
	}
	params, ok := c.paramsJSON()
	if !ok {
		return nil, nil, nil
	}
	return nilIfEmpty(c.TitleKey), nilIfEmpty(c.BodyKey), params
}

func (d *DB) CreateNotification(ctx context.Context, orgID int, n *Notification) error {
	n.OrganizationID = orgID
	titleKey, bodyKey, params := NotificationContent{
		TitleKey: n.TitleKey, BodyKey: n.BodyKey, Params: n.Params,
	}.keyedColumns()
	return d.pool.QueryRow(ctx, `
		INSERT INTO notifications (organization_id, recipient_id, title, body, link, title_key, body_key, params)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`, orgID, n.RecipientID, n.Title, nilIfEmpty(n.Body), nilIfEmpty(n.Link),
		titleKey, bodyKey, params,
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
	titleKey, bodyKey, params := c.keyedColumns()
	_, err := d.pool.Exec(ctx, `
		INSERT INTO notifications (organization_id, recipient_id, title, body, link, title_key, body_key, params)
		SELECT $1, u.id, $3, $4, $5, $6, $7, $8
		FROM users u WHERE u.email = $2
	`, orgID, recipientEmail, c.Title, nilIfEmpty(c.Body), nilIfEmpty(c.Link),
		titleKey, bodyKey, params)
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
			// params rather than failing the whole inbox listing — and drop the
			// keys with them, for the same reason the write side does: a key
			// without its params renders unfilled placeholders, while no key
			// falls back cleanly to the English Title/Body.
			if err := json.Unmarshal(params, &n.Params); err != nil {
				n.Params = nil
				n.TitleKey, n.BodyKey = "", ""
			}
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
