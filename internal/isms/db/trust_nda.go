package db

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/publicsuffix"
)

// Click-through NDA for gated trust center items (#525). The settings name the
// NDA document and a list of email domains that need not accept it; every
// acceptance of a version is a row in trust_nda_acceptances.

// How a visitor satisfied the NDA.
const (
	TrustNDAAccepted   = "accepted"
	TrustNDADomainSkip = "domain_skip"
)

// maxNDASkipDomains caps the skip list, like the blocked-domain list.
const maxNDASkipDomains = 500

// nonNilStrings is l, or an empty slice: pgx writes a nil slice as NULL.
func nonNilStrings(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// NormalizeNDASkipDomains trims, lower-cases, strips a leading "@", de-duplicates
// and sorts the entries. An entry is a domain ("example.com", that domain only)
// or a wildcard ("*.example.com", any subdomain at any depth, never the domain
// itself); a wildcard over a public suffix (`*.co.uk`, `*.github.io`) is
// refused. It refuses an empty entry, one with an "@", whitespace, "/" or a "*"
// anywhere but the "*." prefix, one whose domain part has no dot (so "*.com" is
// refused) or is longer than 253 characters, and a list of more than 500. The
// result is never nil.
func NormalizeNDASkipDomains(in []string) ([]string, error) {
	if len(in) > maxNDASkipDomains {
		return nil, trustInvalid("more than %d NDA skip domains", maxNDASkipDomains)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range in {
		d := strings.ToLower(strings.TrimSpace(raw))
		d = strings.TrimPrefix(d, "@")
		base := strings.TrimPrefix(d, "*.")
		if base == "" || len(d) > 253 || !strings.Contains(base, ".") || strings.ContainsAny(base, "@/*") ||
			strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".") || strings.Contains(base, "..") ||
			strings.IndexFunc(d, unicode.IsSpace) >= 0 {
			return nil, trustInvalid("NDA skip domain %q", raw)
		}
		// A wildcard over a public suffix (*.co.uk, *.github.io) would cover
		// unrelated organizations, so it is refused. *.example.co.uk is fine.
		if base != d {
			if suffix, _ := publicsuffix.PublicSuffix(base); suffix == base {
				return nil, trustInvalid("NDA skip domain %q covers a public suffix", raw)
			}
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out, nil
}

// NDASkipMatches reports whether the email's domain is on the skip list. The
// match is case-insensitive; "example.com" matches only that domain and
// "*.example.com" any subdomain of it, never "example.com" itself.
func NDASkipMatches(list []string, email string) bool {
	email = normalizeVisitorEmail(email)
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, e := range list {
		e = strings.ToLower(strings.TrimSpace(e))
		if suffix, ok := strings.CutPrefix(e, "*"); ok && strings.HasPrefix(suffix, ".") {
			if len(domain) > len(suffix) && strings.HasSuffix(domain, suffix) {
				return true
			}
		} else if e == domain {
			return true
		}
	}
	return false
}

// NDASkips reports whether the settings' current skip list covers the email.
func (s *TrustCenterSettings) NDASkips(email string) bool {
	return NDASkipMatches(s.NDASkipDomains, email)
}

// TrustNDAAcceptance is one NDA acceptance (or domain skip) of one visitor.
type TrustNDAAcceptance struct {
	ID             int64
	OrganizationID int
	VisitorID      int64
	DocumentID     string
	// VersionID is the document_versions row the visitor saw; Version is its
	// label (which can be empty) and ContentHash the hash of the served file.
	VersionID   int64
	Version     string
	ContentHash string
	// CommitHash and FilePath locate the accepted bytes in git: the
	// document_versions row can be overwritten later (same document and label),
	// the commit cannot, so the text a visitor accepted stays provable.
	CommitHash string
	FilePath   string
	Method     string // TrustNDAAccepted | TrustNDADomainSkip
	Company    string // the visitor's company when they accepted
	IP         string
	UserAgent  string
	CreatedAt  Epoch
}

// Bounds on the text stored with an acceptance.
const (
	maxNDAUserAgent = 512
	maxNDAIP        = 64
)

// clampText makes s valid UTF-8 (Postgres refuses anything else in a TEXT
// column, which would fail the whole acceptance) and at most max bytes.
func clampText(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	if len(s) > max {
		s = strings.ToValidUTF8(s[:max], "")
	}
	return s
}

// RecordTrustNDAAcceptance stores a.Method for the visitor and version unless
// the same row exists already, and reports whether it wrote one. A row without
// a content hash is refused: it could never be told apart from "not accepted".
func (d *DB) RecordTrustNDAAcceptance(ctx context.Context, orgID int, a *TrustNDAAcceptance) (bool, error) {
	if a.ContentHash == "" {
		return false, trustInvalid("an NDA acceptance needs a content hash")
	}
	if a.Method != TrustNDAAccepted && a.Method != TrustNDADomainSkip {
		return false, trustInvalid("NDA method %q", a.Method)
	}
	ua, ip := clampText(a.UserAgent, maxNDAUserAgent), clampText(a.IP, maxNDAIP)
	var inserted bool
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO trust_nda_acceptances
				(organization_id, visitor_id, document_id, version_id, version, content_hash,
				 commit_hash, file_path, method, company, ip, user_agent)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (organization_id, visitor_id, content_hash, method) DO NOTHING
		`, orgID, a.VisitorID, a.DocumentID, a.VersionID, a.Version, a.ContentHash,
			a.CommitHash, a.FilePath, a.Method, a.Company, ip, ua)
		inserted = err == nil && tag.RowsAffected() == 1
		return err
	})
	return inserted, err
}

// HasTrustNDAAccepted reports whether the visitor clicked through the NDA
// version with this content hash. A domain-skip row does not count, and an
// empty hash never matches.
func (d *DB) HasTrustNDAAccepted(ctx context.Context, orgID int, visitorID int64, contentHash string) (bool, error) {
	if contentHash == "" {
		return false, nil
	}
	var ok bool
	err := d.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM trust_nda_acceptances
		               WHERE organization_id = $1 AND visitor_id = $2
		                 AND content_hash = $3 AND method = 'accepted')
	`, orgID, visitorID, contentHash).Scan(&ok)
	return ok, err
}

// ListTrustNDAAccepted returns every click-through acceptance of the
// organization (domain-skip log rows are not included), oldest first, for the
// visitors list: one query for all visitors.
func (d *DB) ListTrustNDAAccepted(ctx context.Context, orgID int) ([]TrustNDAAcceptance, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, organization_id, visitor_id, document_id, version_id, version, content_hash,
		       commit_hash, file_path, method, company, ip, user_agent, created_at
		FROM trust_nda_acceptances WHERE organization_id = $1 AND method = 'accepted'
		ORDER BY created_at, id
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrustNDAAcceptance{}
	for rows.Next() {
		var a TrustNDAAcceptance
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.VisitorID, &a.DocumentID, &a.VersionID, &a.Version,
			&a.ContentHash, &a.CommitHash, &a.FilePath, &a.Method, &a.Company, &a.IP, &a.UserAgent, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
