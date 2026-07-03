package db

import (
	"context"
	"fmt"
)

// Notification represents an inbox item for a user.
type Notification struct {
	ID             int    `json:"id"`
	OrganizationID int    `json:"organization_id"`
	RecipientID    int    `json:"recipient_id"`
	Title          string `json:"title"`
	Body           string `json:"body,omitempty"`
	Link           string `json:"link,omitempty"`
	Read           bool   `json:"read"`
	CreatedAt      Epoch  `json:"created_at"`
}

func (d *DB) CreateNotification(ctx context.Context, orgID int, n *Notification) error {
	n.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO notifications (organization_id, recipient_id, title, body, link)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, orgID, n.RecipientID, n.Title, nilIfEmpty(n.Body), nilIfEmpty(n.Link),
	).Scan(&n.ID, &n.CreatedAt)
}

// CreateNotificationByEmail creates a notification for a user identified by email.
// If the user is not found, the notification is silently dropped.
func (d *DB) CreateNotificationByEmail(ctx context.Context, orgID int, recipientEmail string, title, body, link string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO notifications (organization_id, recipient_id, title, body, link)
		SELECT $1, u.id, $3, $4, $5
		FROM users u WHERE u.email = $2
	`, orgID, recipientEmail, title, nilIfEmpty(body), nilIfEmpty(link))
	return err
}

func (d *DB) ListNotifications(ctx context.Context, orgID int, userID int, unreadOnly bool, limit int) ([]Notification, error) {
	query := `SELECT id, organization_id, recipient_id, title, COALESCE(body, ''), COALESCE(link, ''), read, created_at
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
		if err := rows.Scan(&n.ID, &n.OrganizationID, &n.RecipientID, &n.Title, &n.Body, &n.Link, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
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
