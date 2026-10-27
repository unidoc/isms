package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ExternalLinkTypes is the allowed link_type set (matches the table CHECK).
var ExternalLinkTypes = []string{"certificate", "audit_report", "penetration_test", "attestation", "contract", "other"}

// ExternalLink is a URL to evidence hosted elsewhere, attached to a register
// record (#316). EntityID is the parent's row id. The two dates are DATE
// columns: they are written from, and compared against, Postgres's own idea of
// today, never a Go-side time.Now().
type ExternalLink struct {
	ID         int64  `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
	URL        string `json:"url"`
	Label      string `json:"label"`
	LinkType   string `json:"link_type"`
	ValidUntil *Epoch `json:"valid_until,omitempty"`
	VerifiedAt *Epoch `json:"verified_at,omitempty"`
	// VerifiedBy is the verifying user's email; empty when never verified or
	// the user has since been deleted.
	VerifiedBy string `json:"verified_by,omitempty"`
	// State is "expired", "expiring" (within 30 days) or "current", computed in
	// SQL like the supplier-contract overdue block. Empty when ValidUntil is NULL.
	State     string `json:"state,omitempty"`
	CreatedBy string `json:"created_by"`
	CreatedAt Epoch  `json:"created_at"`
	UpdatedAt Epoch  `json:"updated_at"`
}

// ExternalLinkInput is every editable field of a link. A nil date is NULL.
type ExternalLinkInput struct {
	URL        string
	Label      string
	LinkType   string
	ValidUntil *time.Time
	VerifiedAt *time.Time
}

const externalLinkCols = `l.id, l.entity_type, l.entity_id, l.url, l.label, l.link_type,
	l.valid_until, l.verified_at,
	COALESCE((SELECT email FROM users WHERE id = l.verified_by_id), ''),
	CASE WHEN l.valid_until IS NULL THEN ''
		WHEN (l.valid_until - CURRENT_DATE)::int < 0 THEN 'expired'
		WHEN (l.valid_until - CURRENT_DATE)::int <= 30 THEN 'expiring'
		ELSE 'current' END,
	l.created_by, l.created_at, l.updated_at`

func scanExternalLink(row pgx.Row, l *ExternalLink) error {
	return row.Scan(&l.ID, &l.EntityType, &l.EntityID, &l.URL, &l.Label, &l.LinkType,
		&l.ValidUntil, &l.VerifiedAt, &l.VerifiedBy, &l.State,
		&l.CreatedBy, &l.CreatedAt, &l.UpdatedAt)
}

// ChangeSummary is the one-line form of a link written to the parent's
// changelog: "<label> (<link_type>) <url>; valid until <date|->; verified <date|->".
func (l *ExternalLink) ChangeSummary() string {
	day := func(e *Epoch) string {
		if e == nil || e.IsZero() {
			return "-"
		}
		return e.UTC().Format("2006-01-02")
	}
	return fmt.Sprintf("%s (%s) %s; valid until %s; verified %s",
		l.Label, l.LinkType, l.URL, day(l.ValidUntil), day(l.VerifiedAt))
}

// ListExternalLinks returns the live links of one record, soonest expiry
// first, links without a date last.
func (d *DB) ListExternalLinks(ctx context.Context, orgID int, entityType string, entityID int64) ([]ExternalLink, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+externalLinkCols+`
		FROM external_links l
		WHERE l.organization_id = $1 AND l.entity_type = $2 AND l.entity_id = $3
			AND l.deleted_at IS NULL
		ORDER BY l.valid_until ASC NULLS LAST, l.label ASC, l.id ASC
	`, orgID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []ExternalLink{}
	for rows.Next() {
		var l ExternalLink
		if err := scanExternalLink(rows, &l); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// GetExternalLinkTx reads a live link and locks it for the rest of the
// transaction. pgx.ErrNoRows when it does not exist, is deleted, belongs to
// another organization, or hangs off a soft-deleted supplier, so every write
// path that starts here refuses those.
func GetExternalLinkTx(ctx context.Context, tx pgx.Tx, orgID int, id int64) (*ExternalLink, error) {
	var l ExternalLink
	row := tx.QueryRow(ctx, `
		SELECT `+externalLinkCols+`
		FROM external_links l
		WHERE l.id = $1 AND l.organization_id = $2 AND l.deleted_at IS NULL
			AND (l.entity_type <> 'supplier' OR EXISTS (
				SELECT 1 FROM suppliers s
				WHERE s.id = l.entity_id AND s.organization_id = l.organization_id AND s.deleted_at IS NULL))
		FOR UPDATE OF l
	`, id, orgID)
	if err := scanExternalLink(row, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// LockLiveSupplierTx takes a shared lock on a live supplier for the rest of the
// transaction. A concurrent soft delete either committed first (pgx.ErrNoRows
// here) or waits for this transaction, so a link can never be added to a
// supplier that is deleted underneath it.
func LockLiveSupplierTx(ctx context.Context, tx pgx.Tx, orgID int, supplierID int64) error {
	var one int
	return tx.QueryRow(ctx, `
		SELECT 1 FROM suppliers
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		FOR SHARE
	`, supplierID, orgID).Scan(&one)
}

// CreateExternalLinkTx inserts a link. A set verified_at is stamped with
// verifier (the actor's email).
func CreateExternalLinkTx(ctx context.Context, tx pgx.Tx, orgID int, entityType string, entityID int64, in ExternalLinkInput, createdBy string) (*ExternalLink, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO external_links (organization_id, entity_type, entity_id, url, label, link_type,
			valid_until, verified_at, verified_by_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7::date, $8::date,
			CASE WHEN $8::date IS NULL THEN NULL ELSE (SELECT id FROM users WHERE lower(email) = lower($9)) END,
			$9)
		RETURNING id
	`, orgID, entityType, entityID, in.URL, in.Label, in.LinkType, in.ValidUntil, in.VerifiedAt, createdBy).Scan(&id)
	if err != nil {
		return nil, err
	}
	return getExternalLinkAfterWrite(ctx, tx, orgID, id)
}

// UpdateExternalLinkTx replaces every editable field. keepVerifier leaves
// verified_by_id as it is (verified_at unchanged); otherwise a non-null
// verified_at is stamped with verifier and a null one clears the verifier.
func UpdateExternalLinkTx(ctx context.Context, tx pgx.Tx, orgID int, id int64, in ExternalLinkInput, keepVerifier bool, verifier string) (*ExternalLink, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE external_links SET url = $3, label = $4, link_type = $5,
			valid_until = $6::date, verified_at = $7::date,
			verified_by_id = CASE
				WHEN $8::boolean THEN verified_by_id
				WHEN $7::date IS NULL THEN NULL
				ELSE (SELECT id FROM users WHERE lower(email) = lower($9)) END,
			updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID, in.URL, in.Label, in.LinkType, in.ValidUntil, in.VerifiedAt, keepVerifier, verifier)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	return getExternalLinkAfterWrite(ctx, tx, orgID, id)
}

// VerifyExternalLinkTx stamps verified_at = today (Postgres's date) and the verifier.
func VerifyExternalLinkTx(ctx context.Context, tx pgx.Tx, orgID int, id int64, verifier string) (*ExternalLink, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE external_links SET verified_at = CURRENT_DATE,
			verified_by_id = (SELECT id FROM users WHERE lower(email) = lower($3)),
			updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID, verifier)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	return getExternalLinkAfterWrite(ctx, tx, orgID, id)
}

// DeleteExternalLinkTx soft-deletes a link.
func DeleteExternalLinkTx(ctx context.Context, tx pgx.Tx, orgID int, id int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE external_links SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func getExternalLinkAfterWrite(ctx context.Context, tx pgx.Tx, orgID int, id int64) (*ExternalLink, error) {
	var l ExternalLink
	row := tx.QueryRow(ctx, `
		SELECT `+externalLinkCols+`
		FROM external_links l WHERE l.id = $1 AND l.organization_id = $2
	`, id, orgID)
	if err := scanExternalLink(row, &l); err != nil {
		return nil, err
	}
	return &l, nil
}
