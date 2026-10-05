package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Comment tables a comment_references row can point into (#194).
const (
	CommentTypeComment       = "comment"        // comments: document and review comments
	CommentTypeEntityComment = "entity_comment" // entity_comments: register records
)

// CommentReference is one #IDENT mention in a comment: a link between the
// comment's subject (source) and the mentioned record (target).
type CommentReference struct {
	SourceType string
	SourceID   string
	TargetType string
	TargetID   string
}

// AddCommentReferencesTx saves each mention as a bidirectional link with
// origin 'comment' (an existing pair keeps its origin) and records that the
// comment holds it, so RemoveCommentReferencesTx can undo it.
func AddCommentReferencesTx(ctx context.Context, tx pgx.Tx, orgID int, commentType string, commentID int64, actor string, refs []CommentReference) error {
	for _, r := range refs {
		fwd := &EntityReference{SourceType: r.SourceType, SourceID: r.SourceID, TargetType: r.TargetType, TargetID: r.TargetID, CreatedBy: actor, Origin: ReferenceOriginComment}
		if err := upsertReference(ctx, tx, orgID, fwd); err != nil {
			return fmt.Errorf("linking %s %s to %s %s: %w", r.SourceType, r.SourceID, r.TargetType, r.TargetID, err)
		}
		rev := &EntityReference{SourceType: r.TargetType, SourceID: r.TargetID, TargetType: r.SourceType, TargetID: r.SourceID, CreatedBy: actor, Origin: ReferenceOriginComment}
		if err := upsertReference(ctx, tx, orgID, rev); err != nil {
			return fmt.Errorf("linking %s %s to %s %s: %w", r.TargetType, r.TargetID, r.SourceType, r.SourceID, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO comment_references (organization_id, comment_type, comment_id, source_type, source_id, target_type, target_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT DO NOTHING
		`, orgID, commentType, commentID, r.SourceType, r.SourceID, r.TargetType, r.TargetID); err != nil {
			return fmt.Errorf("recording mention of %s %s: %w", r.TargetType, r.TargetID, err)
		}
	}
	return nil
}

// RemoveCommentReferencesTx drops a comment's mentions and deletes each link
// they made that nothing else still holds: no other comment mentions the pair
// (in either direction) and neither row of the pair is manual.
func RemoveCommentReferencesTx(ctx context.Context, tx pgx.Tx, orgID int, commentType string, commentID int64) error {
	rows, err := tx.Query(ctx, `
		DELETE FROM comment_references
		WHERE organization_id = $1 AND comment_type = $2 AND comment_id = $3
		RETURNING source_type, source_id, target_type, target_id
	`, orgID, commentType, commentID)
	if err != nil {
		return err
	}
	var pairs []CommentReference
	for rows.Next() {
		var r CommentReference
		if err := rows.Scan(&r.SourceType, &r.SourceID, &r.TargetType, &r.TargetID); err != nil {
			rows.Close()
			return err
		}
		pairs = append(pairs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range pairs {
		if _, err := tx.Exec(ctx, `
			DELETE FROM entity_references
			WHERE organization_id = $1
			  AND ((source_type = $2 AND source_id = $3 AND target_type = $4 AND target_id = $5)
			    OR (source_type = $4 AND source_id = $5 AND target_type = $2 AND target_id = $3))
			  AND NOT EXISTS (
			    SELECT 1 FROM comment_references c
			    WHERE c.organization_id = $1
			      AND ((c.source_type = $2 AND c.source_id = $3 AND c.target_type = $4 AND c.target_id = $5)
			        OR (c.source_type = $4 AND c.source_id = $5 AND c.target_type = $2 AND c.target_id = $3)))
			  AND NOT EXISTS (
			    SELECT 1 FROM entity_references m
			    WHERE m.organization_id = $1 AND m.origin = 'manual'
			      AND ((m.source_type = $2 AND m.source_id = $3 AND m.target_type = $4 AND m.target_id = $5)
			        OR (m.source_type = $4 AND m.source_id = $5 AND m.target_type = $2 AND m.target_id = $3)))
		`, orgID, p.SourceType, p.SourceID, p.TargetType, p.TargetID); err != nil {
			return fmt.Errorf("unlinking %s %s from %s %s: %w", p.SourceType, p.SourceID, p.TargetType, p.TargetID, err)
		}
	}
	return nil
}
