package db

import (
	"context"
)

// EmailVerification represents a pending email verification or password reset token.
type EmailVerification struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Purpose   string `json:"purpose"` // "verify" or "reset"
	ExpiresAt Epoch  `json:"expires_at"`
	UsedAt    *Epoch `json:"used_at,omitempty"`
	CreatedAt Epoch  `json:"created_at"`
}

// InvalidateEmailVerifications marks every live (unused) token of the given
// purpose for a user as used. Call it before issuing a fresh token so a resend
// doesn't leave older invite/reset links usable in parallel.
func (d *DB) InvalidateEmailVerifications(ctx context.Context, userID int, purpose string) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE email_verifications
		SET used_at = now()
		WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL
	`, userID, purpose)
	return err
}

// CreateEmailVerification stores a new verification token (72h expiry).
func (d *DB) CreateEmailVerification(ctx context.Context, userID int, tokenHash string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO email_verifications (user_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, 'verify', now() + interval '72 hours')
	`, userID, tokenHash)
	return err
}

// CreatePasswordResetToken stores a new password reset token (1h expiry).
func (d *DB) CreatePasswordResetToken(ctx context.Context, userID int, tokenHash string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO email_verifications (user_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, 'reset', now() + interval '1 hour')
	`, userID, tokenHash)
	return err
}

// CreateEmailChangeToken stores a new email-change verification token (2h expiry).
// The token is delivered to the *new* address; verifying it swaps the account's
// email to the pending address (verify-before-swap).
func (d *DB) CreateEmailChangeToken(ctx context.Context, userID int, tokenHash string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO email_verifications (user_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, 'email_change', now() + interval '2 hours')
	`, userID, tokenHash)
	return err
}

// LookupEmailVerification finds a valid (unused, not expired) verification token.
func (d *DB) LookupEmailVerification(ctx context.Context, tokenHash string) (*EmailVerification, error) {
	var v EmailVerification
	err := d.pool.QueryRow(ctx, `
		SELECT id, user_id, purpose, expires_at, used_at, created_at
		FROM email_verifications
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
	`, tokenHash).Scan(&v.ID, &v.UserID, &v.Purpose, &v.ExpiresAt, &v.UsedAt, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UseEmailVerification marks a verification token as used.
func (d *DB) UseEmailVerification(ctx context.Context, id int) error {
	_, err := d.pool.Exec(ctx, `UPDATE email_verifications SET used_at = now() WHERE id = $1`, id)
	return err
}
