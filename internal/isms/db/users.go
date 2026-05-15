package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// User represents an ISMS platform user.
type User struct {
	ID            int        `json:"id"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	PasswordHash  *string    `json:"-"`                    // bcrypt, nil = external auth only
	OTPSecret     *string    `json:"-"`                    // TOTP base32 secret, nil = OTP not enabled
	OTPVerified   bool       `json:"otp_verified"`         // true after first successful OTP
	EmailVerified bool       `json:"email_verified"`       // true after verification or CF login
	IsAgent       bool   `json:"is_agent"`
	Active        bool   `json:"active"`
	CreatedAt     Epoch  `json:"created_at"`
	LastSeen      *Epoch `json:"last_seen,omitempty"`
}

// UserWithRole is a User plus their role within a specific organization.
type UserWithRole struct {
	User
	Role string `json:"role"`
}

// Organization represents a tenant / customer org.
type Organization struct {
	ID        int       `json:"-"`                          // never exposed externally
	UUID     string    `json:"uuid"`                       // public identifier
	Name     string    `json:"name"`
	Slug     string    `json:"slug"`                       // URL-friendly (e.g. "unidoc")
	RepoPath string    `json:"repo_path,omitempty"`        // git repo path on server
	Domain   *string `json:"domain,omitempty"`           // custom domain
	CreatedAt Epoch  `json:"created_at"`
	UpdatedAt Epoch  `json:"updated_at"`
}

// OrgMember represents a user's membership and role within an organization.
type OrgMember struct {
	ID             int       `json:"id"`
	OrganizationID int       `json:"organization_id"`
	UserID         int       `json:"user_id"`
	Role           string `json:"role"`
	CreatedAt      Epoch  `json:"created_at"`
}

// HasPassword returns true if the user has a local password set.
func (u *User) HasPassword() bool {
	return u.PasswordHash != nil && *u.PasswordHash != ""
}

// decryptOTP decrypts the OTPSecret field if encrypted.
func (u *User) decryptOTP(key string) error {
	if u.OTPSecret != nil && *u.OTPSecret != "" {
		dec, err := DecryptSecret(*u.OTPSecret, key)
		if err != nil {
			return fmt.Errorf("decrypting OTP secret for user %d: %w", u.ID, err)
		}
		u.OTPSecret = &dec
	}
	return nil
}

// HasOTP returns true if the user has OTP enabled and verified.
func (u *User) HasOTP() bool {
	return u.OTPSecret != nil && *u.OTPSecret != "" && u.OTPVerified
}

// OTPPending returns true if OTP is set up but not yet verified.
func (u *User) OTPPending() bool {
	return u.OTPSecret != nil && *u.OTPSecret != "" && !u.OTPVerified
}

// ---------------------------------------------------------------------------
// User CRUD
// ---------------------------------------------------------------------------

// GetUserByEmail looks up a user. Returns nil if not found.
func (d *DB) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := d.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, otp_secret, otp_verified, email_verified, is_agent, active, created_at, last_seen
		FROM users WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.OTPSecret, &u.OTPVerified, &u.EmailVerified, &u.IsAgent, &u.Active, &u.CreatedAt, &u.LastSeen)
	if err != nil {
		return nil, err
	}
	if err := u.decryptOTP(d.encryptionKey); err != nil {
		return nil, err
	}
	return &u, nil
}

// UpsertUser creates or updates a user. Email is normalized to lowercase.
func (d *DB) UpsertUser(ctx context.Context, u *User) error {
	u.Email = strings.ToLower(u.Email)
	return d.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, is_agent, active)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT ((lower(email))) DO UPDATE SET
			name = EXCLUDED.name,
			is_agent = EXCLUDED.is_agent,
			active = EXCLUDED.active
		RETURNING id, created_at
	`, u.Email, u.Name, u.IsAgent, u.Active).Scan(&u.ID, &u.CreatedAt)
}

// SetPassword sets the password hash for a user.
func (d *DB) SetPassword(ctx context.Context, userID int, hash string) error {
	tag, err := d.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
}

// SetOTPSecret sets the TOTP secret for a user (pending verification). Encrypted at rest.
func (d *DB) SetOTPSecret(ctx context.Context, userID int, secret string) error {
	enc, err := EncryptSecret(secret, d.encryptionKey)
	if err != nil {
		return fmt.Errorf("encrypt otp_secret: %w", err)
	}
	_, err = d.pool.Exec(ctx, `UPDATE users SET otp_secret = $2, otp_verified = false WHERE id = $1`, userID, enc)
	return err
}

// VerifyOTP marks OTP as verified after first successful code check.
func (d *DB) VerifyOTP(ctx context.Context, userID int) error {
	_, err := d.pool.Exec(ctx, `UPDATE users SET otp_verified = true WHERE id = $1`, userID)
	return err
}

// ClearOTP removes OTP for a user.
func (d *DB) ClearOTP(ctx context.Context, userID int) error {
	_, err := d.pool.Exec(ctx, `UPDATE users SET otp_secret = NULL, otp_verified = false WHERE id = $1`, userID)
	return err
}

// DeleteUser soft-deletes and anonymizes a user. FK references and snapshot TEXT fields are preserved.
// The user record stays for referential integrity; PII is scrubbed.
func (d *DB) DeleteUser(ctx context.Context, userID int) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE users SET
			active = false,
			name = 'Deleted User',
			email = 'deleted-' || id::text || '@deleted.local',
			password_hash = NULL,
			otp_secret = NULL,
			otp_verified = false
		WHERE id = $1
	`, userID)
	if err != nil {
		return err
	}
	// Remove org memberships
	_, _ = d.pool.Exec(ctx, `DELETE FROM organization_members WHERE user_id = $1`, userID)
	// Remove external identities
	_, _ = d.pool.Exec(ctx, `DELETE FROM user_identities WHERE user_id = $1`, userID)
	// Revoke API keys
	_, _ = d.pool.Exec(ctx, `UPDATE api_keys SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return nil
}

// SetEmailVerified marks a user's email as verified.
func (d *DB) SetEmailVerified(ctx context.Context, userID int) error {
	_, err := d.pool.Exec(ctx, `UPDATE users SET email_verified = true WHERE id = $1`, userID)
	return err
}

// SetUserActive sets a user's active status.
func (d *DB) SetUserActive(ctx context.Context, userID int, active bool) error {
	_, err := d.pool.Exec(ctx, `UPDATE users SET active = $2 WHERE id = $1`, userID, active)
	return err
}

// UpdateName updates a user's display name.
func (d *DB) UpdateName(ctx context.Context, userID int, name string) error {
	_, err := d.pool.Exec(ctx, `UPDATE users SET name = $2 WHERE id = $1`, userID, name)
	return err
}

// ListUsers returns all users (global, no org filter).
func (d *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, email, name, password_hash, otp_secret, otp_verified, email_verified, is_agent, active, created_at, last_seen
		FROM users ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.OTPSecret, &u.OTPVerified, &u.EmailVerified, &u.IsAgent, &u.Active, &u.CreatedAt, &u.LastSeen); err != nil {
			return nil, err
		}
		if err := u.decryptOTP(d.encryptionKey); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// TouchUser updates last_seen for an existing user. Does NOT create new users.
func (d *DB) TouchUser(ctx context.Context, email, name string) (*User, error) {
	var u User
	err := d.pool.QueryRow(ctx, `
		UPDATE users SET last_seen = now()
		WHERE email = $1
		RETURNING id, email, name, password_hash, otp_secret, otp_verified, email_verified, is_agent, active, created_at, last_seen
	`, email).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.OTPSecret, &u.OTPVerified, &u.EmailVerified, &u.IsAgent, &u.Active, &u.CreatedAt, &u.LastSeen)
	if err != nil {
		return nil, err
	}
	if err := u.decryptOTP(d.encryptionKey); err != nil {
		return nil, err
	}
	return &u, nil
}

// ---------------------------------------------------------------------------
// User Identities (OIDC / external IdP links)
// ---------------------------------------------------------------------------

// UserIdentity represents a linked external identity (OIDC provider).
type UserIdentity struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Provider  string `json:"provider"`
	Subject   string `json:"subject"`
	Email     string `json:"email,omitempty"`
	CreatedAt Epoch  `json:"created_at"`
}

// GetUserByIdentity looks up a user by their external identity (provider + subject).
// Returns nil if not found.
func (d *DB) GetUserByIdentity(ctx context.Context, provider, subject string) (*User, error) {
	var u User
	err := d.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.name, u.password_hash, u.otp_secret, u.otp_verified, u.email_verified, u.is_agent, u.active, u.created_at, u.last_seen
		FROM users u
		JOIN user_identities ui ON ui.user_id = u.id
		WHERE ui.provider = $1 AND ui.subject = $2
	`, provider, subject).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.OTPSecret, &u.OTPVerified, &u.EmailVerified, &u.IsAgent, &u.Active, &u.CreatedAt, &u.LastSeen)
	if err != nil {
		return nil, err
	}
	if err := u.decryptOTP(d.encryptionKey); err != nil {
		return nil, err
	}
	return &u, nil
}

// LinkIdentity links an external identity to a user. If the identity already exists, it's a no-op.
func (d *DB) LinkIdentity(ctx context.Context, userID int, provider, subject, email string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO user_identities (user_id, provider, subject, email)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider, subject) DO NOTHING
	`, userID, provider, subject, nilIfEmpty(email))
	return err
}

// ListUserIdentities returns all linked identities for a user.
func (d *DB) ListUserIdentities(ctx context.Context, userID int) ([]UserIdentity, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, user_id, provider, subject, COALESCE(email, ''), created_at
		FROM user_identities WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var identities []UserIdentity
	for rows.Next() {
		var ui UserIdentity
		if err := rows.Scan(&ui.ID, &ui.UserID, &ui.Provider, &ui.Subject, &ui.Email, &ui.CreatedAt); err != nil {
			return nil, err
		}
		identities = append(identities, ui)
	}
	return identities, nil
}

// UnlinkIdentity removes a linked identity.
func (d *DB) UnlinkIdentity(ctx context.Context, userID, identityID int) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM user_identities WHERE id = $1 AND user_id = $2`, identityID, userID)
	return err
}

// CheckAndSetTOTPUsed checks if a TOTP code was already used in the current 30-second window.
// Returns true if TOTP was already used (replay). Sets the timestamp if not.
// Uses a single atomic query to prevent race conditions between concurrent requests.
func (d *DB) CheckAndSetTOTPUsed(ctx context.Context, userID int) (bool, error) {
	now := time.Now()
	windowStart := now.Truncate(30 * time.Second)

	// Atomic check-and-set: only updates if last_totp_at is before the current window.
	// If no row is returned, the TOTP was already used in this window.
	var id int
	err := d.pool.QueryRow(ctx, `
		UPDATE users SET last_totp_at = $2
		WHERE id = $1 AND (last_totp_at IS NULL OR last_totp_at < $3)
		RETURNING id
	`, userID, now, windowStart).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil // replay — no row updated
		}
		return false, err
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// Organizations
// ---------------------------------------------------------------------------

// CreateOrganization inserts a new organization. Slug is normalized to lowercase.
func (d *DB) CreateOrganization(ctx context.Context, org *Organization) error {
	org.Slug = strings.ToLower(org.Slug)
	return d.pool.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, repo_path, domain)
		VALUES ($1, $2, $3, $4)
		RETURNING id, uuid, created_at, updated_at
	`, org.Name, org.Slug, org.RepoPath, org.Domain).Scan(&org.ID, &org.UUID, &org.CreatedAt, &org.UpdatedAt)
}

// GetOrganization returns an organization by ID.
func (d *DB) GetOrganization(ctx context.Context, id int) (*Organization, error) {
	var o Organization
	err := d.pool.QueryRow(ctx, `
		SELECT id, uuid, name, slug, repo_path, domain, created_at, updated_at
		FROM organizations WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(&o.ID, &o.UUID, &o.Name, &o.Slug, &o.RepoPath, &o.Domain, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrganizationBySlug returns an organization by its URL slug.
func (d *DB) GetOrganizationBySlug(ctx context.Context, slug string) (*Organization, error) {
	var o Organization
	err := d.pool.QueryRow(ctx, `
		SELECT id, uuid, name, slug, repo_path, domain, created_at, updated_at
		FROM organizations WHERE slug = $1 AND deleted_at IS NULL
	`, slug).Scan(&o.ID, &o.UUID, &o.Name, &o.Slug, &o.RepoPath, &o.Domain, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrganizationByDomain returns an organization by its custom domain.
func (d *DB) GetOrganizationByDomain(ctx context.Context, domain string) (*Organization, error) {
	var o Organization
	err := d.pool.QueryRow(ctx, `
		SELECT id, uuid, name, slug, repo_path, domain, created_at, updated_at
		FROM organizations WHERE domain = $1 AND deleted_at IS NULL
	`, domain).Scan(&o.ID, &o.UUID, &o.Name, &o.Slug, &o.RepoPath, &o.Domain, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrganizationByUUID returns an organization by its public UUID.
func (d *DB) GetOrganizationByUUID(ctx context.Context, uuid string) (*Organization, error) {
	var o Organization
	err := d.pool.QueryRow(ctx, `
		SELECT id, uuid, name, slug, repo_path, domain, created_at, updated_at
		FROM organizations WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid).Scan(&o.ID, &o.UUID, &o.Name, &o.Slug, &o.RepoPath, &o.Domain, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrganizations returns all active (non-deleted) organizations ordered by name.
func (d *DB) ListOrganizations(ctx context.Context) ([]Organization, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, uuid, name, slug, repo_path, domain, created_at, updated_at
		FROM organizations WHERE deleted_at IS NULL ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []Organization
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.UUID, &o.Name, &o.Slug, &o.RepoPath, &o.Domain, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, nil
}

// ---------------------------------------------------------------------------
// Organization members
// ---------------------------------------------------------------------------

// AddOrgMember adds a user to an organization with a role.
// If the membership already exists, the role is updated.
func (d *DB) AddOrgMember(ctx context.Context, orgID, userID int, role string) error {
	_, err := d.pool.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, orgID, userID, role)
	return err
}

// GetOrgMember returns a single membership record.
func (d *DB) GetOrgMember(ctx context.Context, orgID, userID int) (*OrgMember, error) {
	var m OrgMember
	err := d.pool.QueryRow(ctx, `
		SELECT id, organization_id, user_id, role, created_at
		FROM organization_members
		WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID).Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// RemoveOrgMember removes a user from an organization.
func (d *DB) RemoveOrgMember(ctx context.Context, orgID, userID int) error {
	_, err := d.pool.Exec(ctx, `DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2`, orgID, userID)
	return err
}

// ListOrgMembers returns all members of an organization.
func (d *DB) ListOrgMembers(ctx context.Context, orgID int) ([]OrgMember, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT m.id, m.organization_id, m.user_id, m.role, m.created_at
		FROM organization_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.organization_id = $1
		ORDER BY u.name
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []OrgMember
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

// ListUserOrgs returns all organizations a user belongs to.
func (d *DB) ListUserOrgs(ctx context.Context, userID int) ([]Organization, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT o.id, o.uuid, o.name, o.slug, o.repo_path, o.domain, o.created_at, o.updated_at
		FROM organizations o
		JOIN organization_members m ON m.organization_id = o.id
		WHERE m.user_id = $1 AND o.deleted_at IS NULL
		ORDER BY o.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []Organization
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.UUID, &o.Name, &o.Slug, &o.RepoPath, &o.Domain, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, nil
}

// IsOrgMember returns true if userID is a member of the given org.
func (d *DB) IsOrgMember(ctx context.Context, orgID, userID int) bool {
	var exists bool
	_ = d.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id = $1 AND user_id = $2)
	`, orgID, userID).Scan(&exists)
	return exists
}

// ValidateOrgUser checks that a user email belongs to a member of the org.
// Returns the user ID, or error if not a member. Use for validating owner_id, assignee_id, etc.
func (d *DB) ValidateOrgUser(ctx context.Context, orgID int, email string) (int, error) {
	var userID int
	err := d.pool.QueryRow(ctx, `
		SELECT u.id FROM users u
		JOIN organization_members m ON m.user_id = u.id AND m.organization_id = $1
		WHERE u.email = $2
	`, orgID, email).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("user %s is not a member of this organization", email)
	}
	return userID, nil
}

// GetUserRole returns the role a user has in an organization.
func (d *DB) GetUserRole(ctx context.Context, orgID, userID int) (string, error) {
	var role string
	err := d.pool.QueryRow(ctx, `
		SELECT role FROM organization_members
		WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID).Scan(&role)
	if err != nil {
		return "", err
	}
	return role, nil
}

// ListOrgUsers returns all users in an organization with their org role.
func (d *DB) ListOrgUsers(ctx context.Context, orgID int) ([]UserWithRole, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT u.id, u.email, u.name, u.password_hash, u.otp_secret, u.otp_verified, u.email_verified, u.is_agent, u.active, u.created_at, u.last_seen, m.role
		FROM users u
		JOIN organization_members m ON m.user_id = u.id
		WHERE m.organization_id = $1
		ORDER BY u.name
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserWithRole
	for rows.Next() {
		var ur UserWithRole
		if err := rows.Scan(&ur.ID, &ur.Email, &ur.Name, &ur.PasswordHash, &ur.OTPSecret, &ur.OTPVerified, &ur.EmailVerified, &ur.IsAgent, &ur.Active, &ur.CreatedAt, &ur.LastSeen, &ur.Role); err != nil {
			return nil, err
		}
		if err := ur.decryptOTP(d.encryptionKey); err != nil {
			return nil, err
		}
		users = append(users, ur)
	}
	return users, nil
}
