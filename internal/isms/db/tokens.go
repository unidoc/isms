package db

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// APIKey represents a Personal Access Token linked to a user.
// When OrganizationID is set, the token is scoped to that org only.
// NULL OrganizationID means legacy/global token (access to all user's orgs).
// Role is determined per-request from org membership, not stored on the token.
type APIKey struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	UserID         int    `json:"user_id"`
	UserEmail      string `json:"user_email"`  // from users table
	OrganizationID *int   `json:"organization_id,omitempty"` // NULL = all orgs, set = org-scoped
	Permissions    string `json:"permissions"` // read, write, read-write
	CreatedAt      Epoch  `json:"created_at"`
	RevokedAt      *Epoch `json:"revoked_at,omitempty"`
	LastUsedAt     *Epoch `json:"last_used_at,omitempty"`
	ExpiresAt      *Epoch `json:"expires_at,omitempty"`
}

// CreateAPIKey stores a new personal access token linked to a user by email.
// The user must already exist in the users table.
// organizationID scopes the token to a specific org (nil = global/legacy).
// expiresAt is optional — pass nil for long-lived CLI API keys, or a time for web sessions.
func (d *DB) CreateAPIKey(ctx context.Context, name, tokenHash, userEmail string, permissions string, organizationID *int, expiresAt *time.Time) (*APIKey, error) {
	// Look up user — must exist
	user, err := d.GetUserByEmail(ctx, userEmail)
	if err != nil {
		return nil, fmt.Errorf("user %q not found — create the user first with: isms serve + login, or insert into users table", userEmail)
	}

	if permissions == "" {
		permissions = "read-write"
	}

	var t APIKey
	err = d.pool.QueryRow(ctx, `
		INSERT INTO api_keys (name, token_hash, user_id, organization_id, permissions, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, user_id, organization_id, permissions, created_at, expires_at
	`, name, tokenHash, user.ID, organizationID, permissions, expiresAt).Scan(&t.ID, &t.Name, &t.UserID, &t.OrganizationID, &t.Permissions, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, err
	}
	t.UserEmail = user.Email
	return &t, nil
}

// LookupAPIKey finds a non-revoked API key by its hash, updates last_used_at,
// and returns the key with user email and org scope. Role is NOT included — it
// is determined per-request from org membership by the middleware.
func (d *DB) LookupAPIKey(ctx context.Context, tokenHash string) (*APIKey, error) {
	var t APIKey
	err := d.pool.QueryRow(ctx, `
		UPDATE api_keys SET last_used_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
		RETURNING id, name, user_id, organization_id, permissions, created_at, revoked_at, last_used_at, expires_at
	`, tokenHash).Scan(&t.ID, &t.Name, &t.UserID, &t.OrganizationID, &t.Permissions, &t.CreatedAt, &t.RevokedAt, &t.LastUsedAt, &t.ExpiresAt)
	if err != nil {
		return nil, err
	}

	// Get user email from users table
	user, err := d.GetUserByID(ctx, t.UserID)
	if err != nil {
		return nil, fmt.Errorf("API key user not found: %w", err)
	}
	t.UserEmail = user.Email
	return &t, nil
}

// RevokeAPIKey revokes an API key by ID, scoped to the owning user.
// Users can only revoke their own tokens.
func (d *DB) RevokeAPIKey(ctx context.Context, userID, id int) error {
	_, err := d.pool.Exec(ctx, `UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// ListUserAPIKeys returns all API keys for a specific user (including revoked).
func (d *DB) ListUserAPIKeys(ctx context.Context, userID int) ([]APIKey, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT t.id, t.name, t.user_id, u.email, t.organization_id, t.permissions, t.created_at, t.revoked_at, t.last_used_at, t.expires_at
		FROM api_keys t
		JOIN users u ON u.id = t.user_id
		WHERE t.user_id = $1
		ORDER BY t.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var t APIKey
		if err := rows.Scan(&t.ID, &t.Name, &t.UserID, &t.UserEmail, &t.OrganizationID, &t.Permissions, &t.CreatedAt, &t.RevokedAt, &t.LastUsedAt, &t.ExpiresAt); err != nil {
			return nil, err
		}
		keys = append(keys, t)
	}
	return keys, nil
}

// ListAllAPIKeysForOrg returns all API keys from users who are members of this org.
// Used for admin audit view — read-only visibility into all tokens from org members.
func (d *DB) ListAllAPIKeysForOrg(ctx context.Context, orgID int) ([]APIKey, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT t.id, t.name, t.user_id, u.email, t.organization_id, t.permissions, t.created_at, t.revoked_at, t.last_used_at, t.expires_at
		FROM api_keys t
		JOIN users u ON u.id = t.user_id
		JOIN organization_members m ON m.user_id = t.user_id AND m.organization_id = $1
		ORDER BY t.created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var t APIKey
		if err := rows.Scan(&t.ID, &t.Name, &t.UserID, &t.UserEmail, &t.OrganizationID, &t.Permissions, &t.CreatedAt, &t.RevokedAt, &t.LastUsedAt, &t.ExpiresAt); err != nil {
			return nil, err
		}
		keys = append(keys, t)
	}
	return keys, nil
}

// ListAPIKeys returns all API keys with user info (including revoked).
// Used by CLI admin command for global visibility.
func (d *DB) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT t.id, t.name, t.user_id, u.email, t.organization_id, t.permissions, t.created_at, t.revoked_at, t.last_used_at, t.expires_at
		FROM api_keys t
		JOIN users u ON u.id = t.user_id
		ORDER BY t.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var t APIKey
		if err := rows.Scan(&t.ID, &t.Name, &t.UserID, &t.UserEmail, &t.OrganizationID, &t.Permissions, &t.CreatedAt, &t.RevokedAt, &t.LastUsedAt, &t.ExpiresAt); err != nil {
			return nil, err
		}
		keys = append(keys, t)
	}
	return keys, nil
}

// --- JWT Blocklist ---

// BlockJWT adds a JWT token hash to the blocklist so it is rejected on future requests.
func (d *DB) BlockJWT(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO jwt_blocklist (token_hash, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (token_hash) DO NOTHING
	`, tokenHash, expiresAt)
	return err
}

// IsJWTBlocked returns true if the token hash is in the blocklist.
func (d *DB) IsJWTBlocked(ctx context.Context, tokenHash string) bool {
	var exists bool
	err := d.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM jwt_blocklist WHERE token_hash = $1)
	`, tokenHash).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

// CleanExpiredBlockedJWTs removes expired entries from the blocklist.
func (d *DB) CleanExpiredBlockedJWTs(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM jwt_blocklist WHERE expires_at < now()`)
	return err
}

// --- Login Attempts (DB-backed brute-force protection) ---

// RecordLoginAttempt records a failed login attempt and returns the count in the last 15 minutes.
//
// ipAddress is normalized to a bare IP before insert: IPv6 loopback arrives
// from echo's RealIP as "[::1]" (bracketed), which the inet column rejects —
// and since callers ignore this error, a failing insert silently disables
// brute-force protection for IPv6 clients. Unparseable values become NULL.
func (d *DB) RecordLoginAttempt(ctx context.Context, email, ipAddress string) (int, error) {
	if ip := net.ParseIP(strings.Trim(ipAddress, "[]")); ip != nil {
		ipAddress = ip.String()
	} else {
		ipAddress = ""
	}
	_, err := d.pool.Exec(ctx, `INSERT INTO login_attempts (email, ip_address) VALUES ($1, $2)`, email, nilIfEmpty(ipAddress))
	if err != nil {
		return 0, err
	}
	var count int
	err = d.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM login_attempts
		WHERE email = $1 AND attempted_at > now() - INTERVAL '15 minutes'
	`, email).Scan(&count)
	return count, err
}

// CountRecentLoginAttempts returns the number of login attempts in the last 15 minutes.
func (d *DB) CountRecentLoginAttempts(ctx context.Context, email string) (int, error) {
	var count int
	err := d.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM login_attempts
		WHERE email = $1 AND attempted_at > now() - INTERVAL '15 minutes'
	`, email).Scan(&count)
	return count, err
}

// CountRecentLoginAttemptsByIP returns the number of failed login attempts
// from a given IP in the last 15 minutes. Used for per-IP rate limiting on
// auth endpoints — complements the per-email check by catching one attacker
// trying many accounts from one source.
func (d *DB) CountRecentLoginAttemptsByIP(ctx context.Context, ip string) (int, error) {
	// Same normalization as RecordLoginAttempt — "[::1]" would fail the
	// ::inet cast and error out the rate-limit check entirely.
	if parsed := net.ParseIP(strings.Trim(ip, "[]")); parsed != nil {
		ip = parsed.String()
	} else {
		ip = ""
	}
	if ip == "" {
		return 0, nil
	}
	var count int
	err := d.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM login_attempts
		WHERE ip_address = $1::inet AND attempted_at > now() - INTERVAL '15 minutes'
	`, ip).Scan(&count)
	return count, err
}

// ClearLoginAttempts removes all login attempts for an email (on successful login).
func (d *DB) ClearLoginAttempts(ctx context.Context, email string) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM login_attempts WHERE email = $1`, email)
	return err
}

// CleanOldLoginAttempts removes login attempts older than 24 hours.
func (d *DB) CleanOldLoginAttempts(ctx context.Context) {
	d.pool.Exec(ctx, `DELETE FROM login_attempts WHERE attempted_at < now() - INTERVAL '24 hours'`)
}

// GetUserByID looks up a user by primary key.
func (d *DB) GetUserByID(ctx context.Context, id int) (*User, error) {
	var u User
	err := d.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, otp_secret, otp_verified, email_verified, is_agent, active, created_at, last_seen
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.OTPSecret, &u.OTPVerified, &u.EmailVerified, &u.IsAgent, &u.Active, &u.CreatedAt, &u.LastSeen)
	if err != nil {
		return nil, err
	}
	if err := u.decryptOTP(d.encryptionKey); err != nil {
		return nil, err
	}
	return &u, nil
}
