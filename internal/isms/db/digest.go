package db

import (
	"context"
	"fmt"
)

// GetDigestEnabled reports whether the user wants the weekly digest for the
// organization (#232). A user who is not a member is an error: there is no
// preference to read.
func (d *DB) GetDigestEnabled(ctx context.Context, orgID, userID int) (bool, error) {
	var enabled bool
	err := d.pool.QueryRow(ctx,
		`SELECT digest_enabled FROM organization_members WHERE organization_id = $1 AND user_id = $2`,
		orgID, userID).Scan(&enabled)
	return enabled, err
}

// SetDigestEnabled stores the member's digest preference for the organization.
func (d *DB) SetDigestEnabled(ctx context.Context, orgID, userID int, enabled bool) error {
	tag, err := d.pool.Exec(ctx,
		`UPDATE organization_members SET digest_enabled = $3 WHERE organization_id = $1 AND user_id = $2`,
		orgID, userID, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %d is not a member of organization %d", userID, orgID)
	}
	return nil
}

// ListDigestOptOuts returns the ids of the organization's members who turned
// the digest off.
func (d *DB) ListDigestOptOuts(ctx context.Context, orgID int) (map[int]bool, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT user_id FROM organization_members WHERE organization_id = $1 AND NOT digest_enabled`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ClaimDigest records that the (organization, user, period) digest is handled
// with the given outcome ("sent" or "empty"). It reports whether this caller
// won the claim: false means another run already holds it, and the caller must
// not send.
func (d *DB) ClaimDigest(ctx context.Context, orgID, userID int, period, outcome string) (bool, error) {
	tag, err := d.pool.Exec(ctx, `
		INSERT INTO digest_sends (organization_id, user_id, period, outcome)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, user_id, period) DO NOTHING`,
		orgID, userID, period, outcome)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ReleaseDigestClaim deletes a claim so the next run retries: used when the
// mail could not be sent after the claim was won.
func (d *DB) ReleaseDigestClaim(ctx context.Context, orgID, userID int, period string) error {
	_, err := d.pool.Exec(ctx,
		`DELETE FROM digest_sends WHERE organization_id = $1 AND user_id = $2 AND period = $3`,
		orgID, userID, period)
	return err
}

// ListDigestClaims returns the ids of the members whose digest for the period
// is already handled in the organization, so a run can skip them without
// building anything.
func (d *DB) ListDigestClaims(ctx context.Context, orgID int, period string) (map[int]bool, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT user_id FROM digest_sends WHERE organization_id = $1 AND period = $2`, orgID, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
