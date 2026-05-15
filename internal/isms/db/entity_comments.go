package db

import (
	"context"
	"fmt"
)

// EntityComment is a comment on any operational entity.
type EntityComment struct {
	ID             int64  `json:"id"`
	OrganizationID int    `json:"organization_id"`
	EntityType     string `json:"entity_type"`
	EntityID       string `json:"entity_id"`
	ParentID       *int64 `json:"parent_id,omitempty"`
	Author         string `json:"author"`
	Body           string `json:"body"`
	Status         string `json:"status"`
	ResolvedBy     string `json:"resolved_by,omitempty"`
	ResolvedAt     *Epoch `json:"resolved_at,omitempty"`
	CreatedAt      Epoch  `json:"created_at"`
	UpdatedAt      Epoch  `json:"updated_at"`
	// Enriched
	Reactions []ReactionSummary `json:"reactions,omitempty"`
}

// ReactionSummary shows counts per emoji for a target.
type ReactionSummary struct {
	Emoji string   `json:"emoji"`
	Count int      `json:"count"`
	Users []string `json:"users"`
}

func (d *DB) CreateEntityComment(ctx context.Context, orgID int, c *EntityComment) error {
	c.OrganizationID = orgID
	return d.pool.QueryRow(ctx, `
		INSERT INTO entity_comments (organization_id, entity_type, entity_id, parent_id, author, author_user_id, body)
		VALUES ($1, $2, $3, $4, $5, (SELECT id FROM users WHERE email = $5), $6)
		RETURNING id, status, created_at, updated_at
	`, orgID, c.EntityType, c.EntityID, c.ParentID, c.Author, c.Body,
	).Scan(&c.ID, &c.Status, &c.CreatedAt, &c.UpdatedAt)
}

func (d *DB) ListEntityComments(ctx context.Context, orgID int, entityType, entityID string) ([]EntityComment, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, entity_type, entity_id, parent_id, author, body, status,
			COALESCE(resolved_by, ''), resolved_at, created_at, updated_at
		FROM entity_comments
		WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3
		ORDER BY created_at ASC
	`, orgID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []EntityComment
	for rows.Next() {
		var c EntityComment
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.EntityType, &c.EntityID, &c.ParentID,
			&c.Author, &c.Body, &c.Status, &c.ResolvedBy, &c.ResolvedAt,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (d *DB) ResolveEntityComment(ctx context.Context, orgID int, id int64, resolvedBy string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE entity_comments SET status = 'resolved', resolved_by = $3, resolved_at = now(), updated_at = now()
		WHERE id = $1 AND organization_id = $2
	`, id, orgID, resolvedBy)
	return err
}

func (d *DB) DeleteEntityComment(ctx context.Context, orgID int, id int64) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM entity_comments WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}

// ═══════════════════════════════════════════════════════════════════════
// REACTIONS
// ═══════════════════════════════════════════════════════════════════════

type EntityReaction struct {
	ID             int64  `json:"id"`
	OrganizationID int    `json:"organization_id"`
	TargetType     string `json:"target_type"`
	TargetID       int64  `json:"target_id"`
	Emoji          string `json:"emoji"`
	UserEmail      string `json:"user_email"`
	CreatedAt      Epoch  `json:"created_at"`
}

// ToggleReaction adds a reaction if not present, removes if already exists. Returns true if added.
func (d *DB) ToggleReaction(ctx context.Context, orgID int, targetType string, targetID int64, emoji, userEmail string) (bool, error) {
	// Try delete first
	tag, err := d.pool.Exec(ctx, `
		DELETE FROM entity_reactions
		WHERE organization_id = $1 AND target_type = $2 AND target_id = $3 AND emoji = $4 AND user_email = $5
	`, orgID, targetType, targetID, emoji, userEmail)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil // removed
	}
	// Add
	_, err = d.pool.Exec(ctx, `
		INSERT INTO entity_reactions (organization_id, target_type, target_id, emoji, user_email, user_id)
		VALUES ($1, $2, $3, $4, $5, (SELECT id FROM users WHERE email = $5))
	`, orgID, targetType, targetID, emoji, userEmail)
	if err != nil {
		return false, err
	}
	return true, nil // added
}

// ListReactions returns reaction summaries for a target.
func (d *DB) ListReactions(ctx context.Context, orgID int, targetType string, targetID int64) ([]ReactionSummary, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT emoji, COUNT(*), array_agg(user_email ORDER BY created_at)
		FROM entity_reactions
		WHERE organization_id = $1 AND target_type = $2 AND target_id = $3
		GROUP BY emoji ORDER BY MIN(created_at)
	`, orgID, targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reactions []ReactionSummary
	for rows.Next() {
		var r ReactionSummary
		if err := rows.Scan(&r.Emoji, &r.Count, &r.Users); err != nil {
			return nil, fmt.Errorf("scanning reaction: %w", err)
		}
		reactions = append(reactions, r)
	}
	return reactions, nil
}
