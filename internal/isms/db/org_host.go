package db

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// trustDomainRe is the same shape the organizations_trust_domain_check CHECK
// enforces: a bare lowercase hostname with at least two labels.
var trustDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// NormalizeTrustDomain validates a trust domain typed by the server operator and
// returns it in the stored form. apex is the host the app itself is served on
// ("" when unknown): the host resolver never reaches the custom-domain step for
// the apex, www.<apex> or anything under the apex, so those are refused.
func NormalizeTrustDomain(raw, apex string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(raw))
	h = strings.TrimSuffix(h, ".")
	if h == "" {
		return "", errors.New("the trust domain is empty")
	}
	if strings.Contains(h, "://") {
		return "", errors.New("give the host name only, without http:// or https://")
	}
	if strings.ContainsAny(h, "/?#") {
		return "", errors.New("give the host name only, without a path, query or fragment")
	}
	if strings.ContainsAny(h, " \t\r\n") {
		return "", errors.New("the trust domain must not contain whitespace")
	}
	if strings.Contains(h, "*") {
		return "", errors.New("wildcards are not supported; give one exact host name")
	}
	for _, r := range h {
		if r > 127 {
			return "", errors.New("the trust domain must be ASCII; give an internationalised name in punycode (xn--...)")
		}
	}
	if net.ParseIP(strings.Trim(h, "[]")) != nil {
		return "", errors.New("an IP address cannot be a trust domain; give a host name")
	}
	if strings.Contains(h, ":") {
		return "", errors.New("give the host name only, without a port")
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return "", errors.New("localhost cannot be a trust domain")
	}
	if !strings.Contains(h, ".") {
		return "", errors.New("the trust domain needs at least two labels, for example trust.example.com")
	}
	if len(h) > 253 {
		return "", errors.New("the trust domain is longer than 253 characters")
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) > 63 {
			return "", fmt.Errorf("the label %q is longer than 63 characters", label[:20]+"...")
		}
	}
	if !trustDomainRe.MatchString(h) {
		return "", errors.New("the trust domain is not a valid host name (letters, digits and hyphens in dot-separated labels; no label may start or end with a hyphen or be empty)")
	}
	if apex = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(apex), ".")); apex != "" {
		if h == apex || h == "www."+apex || strings.HasSuffix(h, "."+apex) {
			return "", fmt.Errorf("%s is the app's own address or under it (%s); use a host outside it", h, apex)
		}
	}
	return h, nil
}

// ApexHost returns the host the app is served on: ISMS_DOMAIN when set, else the
// host name of ISMS_BASE_URL (no port), lowercased; "" when neither gives one.
func ApexHost(ismsDomain, baseURL string) string {
	if d := strings.ToLower(strings.TrimSpace(ismsDomain)); d != "" {
		return d
	}
	if baseURL = strings.TrimSpace(baseURL); baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// OrgHostKind says which of an organization's two hosts matched a request host.
type OrgHostKind int

const (
	// OrgHostApp: the host is the organization's app domain (organizations.domain).
	OrgHostApp OrgHostKind = iota + 1
	// OrgHostTrust: the host is the organization's trust domain.
	OrgHostTrust
)

// isOrgHostViolation reports whether err is the unique violation raised for a
// host (the trigger, or the trust domain index) rather than for the slug.
func isOrgHostViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return pgErr.ConstraintName == "organizations_host_unique" || pgErr.ConstraintName == "uq_organizations_trust_domain"
}

// SetOrganizationTrustDomain sets (or, with nil, clears) the host an
// organization's trust center is served on. domain must already be normalised
// (NormalizeTrustDomain); it is lower-cased here as a last guard. It returns
// ErrOrgHostTaken when another live organization, or this one's app domain,
// already uses the host, and an error when no live organization has the id.
func (d *DB) SetOrganizationTrustDomain(ctx context.Context, orgID int, domain *string) error {
	var v *string
	if domain != nil {
		l := strings.ToLower(*domain)
		v = &l
	}
	tag, err := d.pool.Exec(ctx, `
		UPDATE organizations SET trust_domain = $2, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, orgID, v)
	if err != nil {
		if isOrgHostViolation(err) {
			return ErrOrgHostTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("organization %d not found", orgID)
	}
	return nil
}

// GetOrganizationByHost returns the live organization whose trust domain or app
// domain is host, and which of the two matched. host must already be lower-cased
// by the caller. Uniqueness across both columns means at most one row; no match
// is pgx.ErrNoRows.
func (d *DB) GetOrganizationByHost(ctx context.Context, host string) (*Organization, OrgHostKind, error) {
	var o Organization
	var trustMatch bool
	err := d.pool.QueryRow(ctx, `
		SELECT id, uuid, name, slug, repo_path, domain, trust_domain, created_at, updated_at,
		       COALESCE(lower(trust_domain) = $1, false)
		FROM organizations
		WHERE deleted_at IS NULL AND (lower(trust_domain) = $1 OR isms_org_host(domain) = $1)
		LIMIT 1
	`, host).Scan(&o.ID, &o.UUID, &o.Name, &o.Slug, &o.RepoPath, &o.Domain, &o.TrustDomain, &o.CreatedAt, &o.UpdatedAt, &trustMatch)
	if err != nil {
		return nil, 0, err
	}
	if trustMatch {
		return &o, OrgHostTrust, nil
	}
	return &o, OrgHostApp, nil
}
