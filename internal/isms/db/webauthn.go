package db

import (
	"context"
)

// WebAuthnCredential represents a WebAuthn/FIDO2 passkey credential for a user.
type WebAuthnCredential struct {
	ID              int        `json:"id"`
	UserID          int        `json:"user_id"`
	CredentialID    []byte     `json:"-"`
	PublicKey       []byte     `json:"-"`
	AttestationType string     `json:"attestation_type"`
	Transport       []string   `json:"transport"`
	SignCount       int        `json:"sign_count"`
	Name            string `json:"name"`
	CreatedAt       Epoch  `json:"created_at"`
	LastUsedAt      *Epoch `json:"last_used_at,omitempty"`
}

// CreateWebAuthnCredential stores a new WebAuthn credential.
func (d *DB) CreateWebAuthnCredential(ctx context.Context, cred *WebAuthnCredential) error {
	return d.pool.QueryRow(ctx, `
		INSERT INTO webauthn_credentials (user_id, credential_id, public_key, attestation_type, transport, sign_count, name)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, cred.UserID, cred.CredentialID, cred.PublicKey, cred.AttestationType, cred.Transport, cred.SignCount, cred.Name,
	).Scan(&cred.ID, &cred.CreatedAt)
}

// ListWebAuthnCredentials returns all WebAuthn credentials for a user.
func (d *DB) ListWebAuthnCredentials(ctx context.Context, userID int) ([]WebAuthnCredential, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, user_id, credential_id, public_key, attestation_type, transport, sign_count, name, created_at, last_used_at
		FROM webauthn_credentials WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []WebAuthnCredential
	for rows.Next() {
		var c WebAuthnCredential
		if err := rows.Scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.AttestationType, &c.Transport, &c.SignCount, &c.Name, &c.CreatedAt, &c.LastUsedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, nil
}

// GetWebAuthnCredentialByCredID looks up a credential by its raw credential ID.
func (d *DB) GetWebAuthnCredentialByCredID(ctx context.Context, credentialID []byte) (*WebAuthnCredential, error) {
	var c WebAuthnCredential
	err := d.pool.QueryRow(ctx, `
		SELECT id, user_id, credential_id, public_key, attestation_type, transport, sign_count, name, created_at, last_used_at
		FROM webauthn_credentials WHERE credential_id = $1
	`, credentialID).Scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.AttestationType, &c.Transport, &c.SignCount, &c.Name, &c.CreatedAt, &c.LastUsedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateWebAuthnSignCount updates the signature counter and last_used_at for a credential.
func (d *DB) UpdateWebAuthnSignCount(ctx context.Context, id, count int) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE webauthn_credentials SET sign_count = $2, last_used_at = now() WHERE id = $1
	`, id, count)
	return err
}

// DeleteWebAuthnCredential removes a WebAuthn credential by ID, scoped to user.
func (d *DB) DeleteWebAuthnCredential(ctx context.Context, id, userID int) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// RenameWebAuthnCredential updates the display name of a credential, scoped to user.
func (d *DB) RenameWebAuthnCredential(ctx context.Context, id, userID int, name string) error {
	_, err := d.pool.Exec(ctx, `UPDATE webauthn_credentials SET name = $2 WHERE id = $1 AND user_id = $3`, id, name, userID)
	return err
}

// GetWebAuthnCredentialByID looks up a credential by its primary key.
func (d *DB) GetWebAuthnCredentialByID(ctx context.Context, id int) (*WebAuthnCredential, error) {
	var c WebAuthnCredential
	err := d.pool.QueryRow(ctx, `
		SELECT id, user_id, credential_id, public_key, attestation_type, transport, sign_count, name, created_at, last_used_at
		FROM webauthn_credentials WHERE id = $1
	`, id).Scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.AttestationType, &c.Transport, &c.SignCount, &c.Name, &c.CreatedAt, &c.LastUsedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
