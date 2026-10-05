package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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
	return createEntityComment(ctx, d.pool, orgID, c)
}

// CreateEntityCommentTx is CreateEntityComment inside a transaction, so the
// links its #mentions make are written with it (#194).
func CreateEntityCommentTx(ctx context.Context, tx pgx.Tx, orgID int, c *EntityComment) error {
	return createEntityComment(ctx, tx, orgID, c)
}

func createEntityComment(ctx context.Context, q rowQuerier, orgID int, c *EntityComment) error {
	c.OrganizationID = orgID
	return q.QueryRow(ctx, `
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

// GetEntityComment returns one comment in the org, or pgx.ErrNoRows.
func (d *DB) GetEntityComment(ctx context.Context, orgID int, id int64) (*EntityComment, error) {
	var c EntityComment
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, entity_type, entity_id, parent_id, author, body, status,
			COALESCE(resolved_by, ''), resolved_at, created_at, updated_at
		FROM entity_comments
		WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(&c.ID, &c.OrganizationID, &c.EntityType, &c.EntityID, &c.ParentID,
		&c.Author, &c.Body, &c.Status, &c.ResolvedBy, &c.ResolvedAt,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (d *DB) ResolveEntityComment(ctx context.Context, orgID int, id int64, resolvedBy string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE entity_comments SET status = 'resolved', resolved_by = $3, resolved_at = now(), updated_at = now()
		WHERE id = $1 AND organization_id = $2
	`, id, orgID, resolvedBy)
	return err
}

// CommentHasRepliesError is returned by DeleteEntityComment when the comment
// still has replies. Deleting it would orphan them, and cascading would
// silently remove other people's replies (#403), so the delete is refused.
type CommentHasRepliesError struct{ Count int }

func (e *CommentHasRepliesError) Error() string {
	return fmt.Sprintf("comment has %d replies", e.Count)
}

// DeleteEntityComment refuses with *CommentHasRepliesError while replies exist,
// returns pgx.ErrNoRows when the comment is not in the org, and otherwise
// deletes the comment, its reactions and the links its #mentions alone held
// (#194), in one transaction.
func (d *DB) DeleteEntityComment(ctx context.Context, orgID int, id int64) error {
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		// Inserting a reply takes a FOR KEY SHARE lock on the parent through the
		// foreign key, which conflicts with FOR UPDATE: once we hold this lock no
		// new reply can commit, so the count below cannot go stale.
		var lockedID int64
		if err := tx.QueryRow(ctx,
			`SELECT id FROM entity_comments WHERE id = $1 AND organization_id = $2 FOR UPDATE`,
			id, orgID).Scan(&lockedID); err != nil {
			return err
		}
		var replies int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM entity_comments WHERE organization_id = $1 AND parent_id = $2`,
			orgID, id).Scan(&replies); err != nil {
			return err
		}
		if replies > 0 {
			return &CommentHasRepliesError{Count: replies}
		}
		if err := RemoveCommentReferencesTx(ctx, tx, orgID, CommentTypeEntityComment, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM entity_reactions WHERE organization_id = $1 AND target_type = 'entity_comment' AND target_id = $2`,
			orgID, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM entity_comments WHERE id = $1 AND organization_id = $2`, id, orgID)
		return err
	})
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
