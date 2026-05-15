package db

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

// OIDCProvider represents an OIDC/OAuth2 identity provider configured for an organization.
type OIDCProvider struct {
	ID             int       `json:"id"`
	OrganizationID int       `json:"organization_id"`
	ProviderName   string    `json:"provider_name"`
	DisplayName    string    `json:"display_name"`
	ClientID       string    `json:"client_id"`
	ClientSecret   string    `json:"client_secret,omitempty"`
	DiscoveryURL   string    `json:"discovery_url"`
	Scopes         string    `json:"scopes"`
	AutoAddMembers bool      `json:"auto_add_members"`
	DefaultRole    string    `json:"default_role"`
	Enabled        bool  `json:"enabled"`
	CreatedAt      Epoch `json:"created_at"`
}

// EncryptSecret encrypts plaintext using AES-GCM with a key derived from ISMS_SECRET.
// Returns a base64-encoded ciphertext string. If key is empty, returns plaintext unchanged.
func EncryptSecret(plaintext, key string) (string, error) {
	if key == "" {
		return plaintext, nil // no encryption key configured
	}
	keyHash := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSecret decrypts a base64-encoded AES-GCM ciphertext.
// If key is empty or data isn't encrypted, returns as-is.
func DecryptSecret(ciphertext, key string) (string, error) {
	if key == "" {
		return ciphertext, nil // no encryption key configured
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		// Not encrypted — return as-is
		return ciphertext, nil
	}
	keyHash := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		// Too short to be encrypted — return as-is
		return ciphertext, nil
	}
	nonce, encrypted := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (authentication error): %w", err)
	}
	return string(plaintext), nil
}

// OIDCSession stores ephemeral state for an in-flight OIDC authorization flow.
type OIDCSession struct {
	ID             int
	State          string
	Nonce          string
	ProviderID     int
	OrganizationID int
	RedirectURI    string
	ExpiresAt      time.Time
}

// CreateOIDCProvider inserts a new OIDC provider for an organization.
// The client_secret is encrypted at rest using the DB encryption key.
func (d *DB) CreateOIDCProvider(ctx context.Context, p *OIDCProvider) error {
	encSecret, err := EncryptSecret(p.ClientSecret, d.encryptionKey)
	if err != nil {
		return fmt.Errorf("encrypting client secret: %w", err)
	}
	return d.pool.QueryRow(ctx, `
		INSERT INTO oidc_providers (organization_id, provider_name, display_name, client_id, client_secret, discovery_url, scopes, auto_add_members, default_role, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`, p.OrganizationID, p.ProviderName, p.DisplayName, p.ClientID, encSecret, p.DiscoveryURL, p.Scopes, p.AutoAddMembers, p.DefaultRole, p.Enabled,
	).Scan(&p.ID, &p.CreatedAt)
}

// GetOIDCProvider returns an OIDC provider by org ID and provider name.
// The client_secret is decrypted from its at-rest encrypted form.
func (d *DB) GetOIDCProvider(ctx context.Context, orgID int, providerName string) (*OIDCProvider, error) {
	var p OIDCProvider
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, provider_name, display_name, client_id, client_secret, discovery_url, scopes, auto_add_members, default_role, enabled, created_at
		FROM oidc_providers WHERE organization_id = $1 AND provider_name = $2
	`, orgID, providerName).Scan(&p.ID, &p.OrganizationID, &p.ProviderName, &p.DisplayName, &p.ClientID, &p.ClientSecret, &p.DiscoveryURL, &p.Scopes, &p.AutoAddMembers, &p.DefaultRole, &p.Enabled, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	dec, decErr := DecryptSecret(p.ClientSecret, d.encryptionKey)
	if decErr != nil {
		return nil, fmt.Errorf("decrypting client secret for %s: %w", providerName, decErr)
	}
	p.ClientSecret = dec
	return &p, nil
}

// GetOIDCProviderByID returns an OIDC provider by primary key.
func (d *DB) GetOIDCProviderByID(ctx context.Context, id int) (*OIDCProvider, error) {
	var p OIDCProvider
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, provider_name, display_name, client_id, client_secret, discovery_url, scopes, auto_add_members, default_role, enabled, created_at
		FROM oidc_providers WHERE id = $1
	`, id).Scan(&p.ID, &p.OrganizationID, &p.ProviderName, &p.DisplayName, &p.ClientID, &p.ClientSecret, &p.DiscoveryURL, &p.Scopes, &p.AutoAddMembers, &p.DefaultRole, &p.Enabled, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	dec, decErr := DecryptSecret(p.ClientSecret, d.encryptionKey)
	if decErr != nil {
		return nil, fmt.Errorf("decrypting client secret for provider %d: %w", id, decErr)
	}
	p.ClientSecret = dec
	return &p, nil
}

// ListOIDCProviders returns all OIDC providers for an organization.
// Client secrets are masked in listing responses for security.
func (d *DB) ListOIDCProviders(ctx context.Context, orgID int) ([]OIDCProvider, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, provider_name, display_name, client_id, client_secret, discovery_url, scopes, auto_add_members, default_role, enabled, created_at
		FROM oidc_providers WHERE organization_id = $1
		ORDER BY provider_name
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []OIDCProvider
	for rows.Next() {
		var p OIDCProvider
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.ProviderName, &p.DisplayName, &p.ClientID, &p.ClientSecret, &p.DiscoveryURL, &p.Scopes, &p.AutoAddMembers, &p.DefaultRole, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		// Mask client secret in list view
		if p.ClientSecret != "" {
			p.ClientSecret = "********"
		}
		providers = append(providers, p)
	}
	return providers, nil
}

// ListEnabledOIDCProviders returns only enabled OIDC providers for an organization.
// Client secrets are decrypted for use in OIDC authorization flows.
func (d *DB) ListEnabledOIDCProviders(ctx context.Context, orgID int) ([]OIDCProvider, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, provider_name, display_name, client_id, client_secret, discovery_url, scopes, auto_add_members, default_role, enabled, created_at
		FROM oidc_providers WHERE organization_id = $1 AND enabled = true
		ORDER BY provider_name
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []OIDCProvider
	for rows.Next() {
		var p OIDCProvider
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.ProviderName, &p.DisplayName, &p.ClientID, &p.ClientSecret, &p.DiscoveryURL, &p.Scopes, &p.AutoAddMembers, &p.DefaultRole, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		dec, decErr := DecryptSecret(p.ClientSecret, d.encryptionKey)
		if decErr != nil {
			return nil, fmt.Errorf("decrypting client secret for provider %s: %w", p.ProviderName, decErr)
		}
		p.ClientSecret = dec
		providers = append(providers, p)
	}
	return providers, nil
}

// UpdateOIDCProvider updates an existing OIDC provider, scoped to organization.
// The client_secret is encrypted at rest.
func (d *DB) UpdateOIDCProvider(ctx context.Context, p *OIDCProvider) error {
	encSecret, err := EncryptSecret(p.ClientSecret, d.encryptionKey)
	if err != nil {
		return fmt.Errorf("encrypting client secret: %w", err)
	}
	_, err = d.pool.Exec(ctx, `
		UPDATE oidc_providers SET
			provider_name = $2, display_name = $3, client_id = $4, client_secret = $5,
			discovery_url = $6, scopes = $7, auto_add_members = $8, default_role = $9, enabled = $10,
			updated_at = now()
		WHERE id = $1 AND organization_id = $11
	`, p.ID, p.ProviderName, p.DisplayName, p.ClientID, encSecret, p.DiscoveryURL, p.Scopes, p.AutoAddMembers, p.DefaultRole, p.Enabled, p.OrganizationID)
	return err
}

// DeleteOIDCProvider removes an OIDC provider by ID, scoped to organization.
func (d *DB) DeleteOIDCProvider(ctx context.Context, orgID, id int) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM oidc_providers WHERE id = $1 AND organization_id = $2`, id, orgID)
	return err
}

// CreateOIDCSession stores an OIDC session for the authorization code flow.
func (d *DB) CreateOIDCSession(ctx context.Context, s *OIDCSession) error {
	return d.pool.QueryRow(ctx, `
		INSERT INTO oidc_sessions (state, nonce, provider_id, organization_id, redirect_uri, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, s.State, s.Nonce, s.ProviderID, s.OrganizationID, s.RedirectURI, s.ExpiresAt).Scan(&s.ID)
}

// LookupOIDCSession finds and deletes an OIDC session by state (single-use).
func (d *DB) LookupOIDCSession(ctx context.Context, state string) (*OIDCSession, error) {
	var s OIDCSession
	err := d.pool.QueryRow(ctx, `
		DELETE FROM oidc_sessions WHERE state = $1
		RETURNING id, state, nonce, provider_id, organization_id, redirect_uri, expires_at
	`, state).Scan(&s.ID, &s.State, &s.Nonce, &s.ProviderID, &s.OrganizationID, &s.RedirectURI, &s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteExpiredOIDCSessions removes all expired OIDC sessions.
func (d *DB) DeleteExpiredOIDCSessions(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM oidc_sessions WHERE expires_at < now()`)
	return err
}
