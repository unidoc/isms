package db

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Trust center visitors sign in with a one-time email link. A visitor is not
// an ISMS user: nothing here ever touches users or organization_members. A
// visitor belongs to one organization, so the same email at two organizations
// is two visitors. Every query filters by organization_id; row-level security
// is inert for the owner role, so the WHERE clause is the real isolation.
//
// Only token hashes (sha256 hex) reach this layer, never the tokens.

var (
	// ErrTrustLinkInvalid: the link is unknown, already used, expired, or
	// belongs to another organization. The cases are deliberately not told apart.
	ErrTrustLinkInvalid = errors.New("this sign-in link is not valid")
	// ErrTrustVisitorProfileRequired: a first sign-in needs a name and a company.
	// The link is not consumed, so the visitor can retry with both.
	ErrTrustVisitorProfileRequired = errors.New("name and company are required at the first sign-in")
	// ErrTrustVisitorBlocked: a manager blocked this visitor. The link is not consumed.
	ErrTrustVisitorBlocked = errors.New("this visitor is blocked")
	// ErrTrustVisitorNotFound is the one not-found answer of this file: no such
	// visitor in the organization, or a session that is unknown, expired,
	// revoked, of another organization, or whose visitor is blocked.
	ErrTrustVisitorNotFound = errors.New("trust center visitor not found")
)

// trustVisitorSeenEvery is how often a lookup refreshes the last-seen stamps.
const trustVisitorSeenEvery = 5 * time.Minute

// TrustVisitor is a person who signed in to an organization's trust center.
type TrustVisitor struct {
	ID             int64
	OrganizationID int
	Email          string
	Name           string
	Company        string
	VerifiedAt     *Epoch
	FirstSeen      Epoch
	LastSeen       Epoch
	Blocked        bool
	BlockedAt      *Epoch
	BlockedBy      *string
	// ActiveSessions counts sessions that are neither revoked nor expired.
	ActiveSessions int
}

const trustVisitorColumns = `v.id, v.organization_id, v.email, v.name, v.company, v.verified_at,
		       v.first_seen, v.last_seen, v.blocked, v.blocked_at, v.blocked_by,
		       (SELECT count(*) FROM trust_visitor_sessions s
		         WHERE s.visitor_id = v.id AND s.organization_id = v.organization_id
		           AND s.revoked_at IS NULL AND s.expires_at > now())`

func scanTrustVisitor(row pgx.Row) (*TrustVisitor, error) {
	var v TrustVisitor
	var verified, blockedAt *time.Time
	if err := row.Scan(&v.ID, &v.OrganizationID, &v.Email, &v.Name, &v.Company, &verified,
		&v.FirstSeen, &v.LastSeen, &v.Blocked, &blockedAt, &v.BlockedBy, &v.ActiveSessions); err != nil {
		return nil, err
	}
	if verified != nil {
		v.VerifiedAt = &Epoch{Time: *verified}
	}
	if blockedAt != nil {
		v.BlockedAt = &Epoch{Time: *blockedAt}
	}
	return &v, nil
}

func normalizeVisitorEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// CreateTrustVisitorLink stores a sign-in link (by hash) valid for ttl. It
// does not invalidate older links of the same email: otherwise anyone who
// knows an address could keep killing that person's link. The send caps
// (CountTrustVisitorLinks) bound how many exist.
func (d *DB) CreateTrustVisitorLink(ctx context.Context, orgID int, email, tokenHash string, ttl time.Duration) error {
	email = normalizeVisitorEmail(email)
	if email == "" || len(email) > 320 {
		return trustInvalid("email is empty or longer than 320 characters")
	}
	if tokenHash == "" {
		return trustInvalid("token hash is empty")
	}
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO trust_visitor_links (organization_id, email, token_hash, expires_at)
			VALUES ($1, $2, $3, now() + make_interval(secs => $4))
		`, orgID, email, tokenHash, ttl.Seconds())
		return err
	})
}

// CountTrustVisitorLinks counts the links created since the given time, for
// one email or, with an empty email, for the whole organization. Used and
// expired links count: the caps limit mail sent, not links still valid.
func (d *DB) CountTrustVisitorLinks(ctx context.Context, orgID int, email string, since time.Time) (int, error) {
	email = normalizeVisitorEmail(email)
	var n int
	err := d.pool.QueryRow(ctx, `
		SELECT count(*) FROM trust_visitor_links
		WHERE organization_id = $1 AND ($2 = '' OR email = $2) AND created_at >= $3
	`, orgID, email, since).Scan(&n)
	return n, err
}

// PeekTrustVisitorLink reads a valid link (unused, unexpired, this
// organization) without consuming it. It returns the link's email and the
// visitor already known for it, or nil for a new one. ErrTrustLinkInvalid
// otherwise.
func (d *DB) PeekTrustVisitorLink(ctx context.Context, orgID int, tokenHash string) (string, *TrustVisitor, error) {
	var email string
	err := d.pool.QueryRow(ctx, `
		SELECT email FROM trust_visitor_links
		WHERE organization_id = $1 AND token_hash = $2 AND used_at IS NULL AND expires_at > now()
	`, orgID, tokenHash).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrTrustLinkInvalid
	}
	if err != nil {
		return "", nil, err
	}
	v, err := d.GetTrustVisitorByEmail(ctx, orgID, email)
	if errors.Is(err, ErrTrustVisitorNotFound) {
		return email, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return email, v, nil
}

// GetTrustVisitor returns the organization's visitor by id, or
// ErrTrustVisitorNotFound (also for another organization's visitor).
func (d *DB) GetTrustVisitor(ctx context.Context, orgID int, id int64) (*TrustVisitor, error) {
	v, err := scanTrustVisitor(d.pool.QueryRow(ctx, `
		SELECT `+trustVisitorColumns+`
		FROM trust_visitors v WHERE v.organization_id = $1 AND v.id = $2
	`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrustVisitorNotFound
	}
	return v, err
}

// GetTrustVisitorByEmail returns the organization's visitor for an email, or
// ErrTrustVisitorNotFound.
func (d *DB) GetTrustVisitorByEmail(ctx context.Context, orgID int, email string) (*TrustVisitor, error) {
	v, err := scanTrustVisitor(d.pool.QueryRow(ctx, `
		SELECT `+trustVisitorColumns+`
		FROM trust_visitors v WHERE v.organization_id = $1 AND v.email = $2
	`, orgID, normalizeVisitorEmail(email)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrustVisitorNotFound
	}
	return v, err
}

// RedeemTrustVisitorLink consumes a link and opens a session, in one
// transaction. The link is consumed by a single atomic UPDATE, so two
// concurrent redeems of one token yield exactly one session. A refusal
// (blocked visitor, missing profile, invalid profile) rolls everything back,
// leaving the link usable. The profile is written only when the visitor has
// none yet (asked once); a returning visitor's name and company are never
// overwritten.
func (d *DB) RedeemTrustVisitorLink(ctx context.Context, orgID int, tokenHash, name, company, sessionHash string, sessionTTL time.Duration) (*TrustVisitor, error) {
	name, company = strings.TrimSpace(name), strings.TrimSpace(company)
	if utf8.RuneCountInString(name) > 200 {
		return nil, trustInvalid("name is longer than 200 characters")
	}
	if utf8.RuneCountInString(company) > 200 {
		return nil, trustInvalid("company is longer than 200 characters")
	}
	if sessionHash == "" {
		return nil, trustInvalid("session hash is empty")
	}
	var out *TrustVisitor
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var email string
		err := tx.QueryRow(ctx, `
			UPDATE trust_visitor_links SET used_at = now()
			WHERE organization_id = $1 AND token_hash = $2 AND used_at IS NULL AND expires_at > now()
			RETURNING email
		`, orgID, tokenHash).Scan(&email)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustLinkInvalid
		}
		if err != nil {
			return err
		}

		var id int64
		var blocked bool
		var haveName string
		load := func() error {
			return tx.QueryRow(ctx, `
				SELECT id, blocked, name FROM trust_visitors
				WHERE organization_id = $1 AND email = $2 FOR UPDATE
			`, orgID, email).Scan(&id, &blocked, &haveName)
		}
		err = load()
		if errors.Is(err, pgx.ErrNoRows) {
			if name == "" || company == "" {
				return ErrTrustVisitorProfileRequired
			}
			// Two different links for one new visitor may race to here; the
			// loser's insert is a no-op and it loads the winner's row.
			if _, err = tx.Exec(ctx, `
				INSERT INTO trust_visitors (organization_id, email, name, company)
				VALUES ($1, $2, $3, $4) ON CONFLICT (organization_id, email) DO NOTHING
			`, orgID, email, name, company); err != nil {
				return err
			}
			err = load()
		}
		if err != nil {
			return err
		}
		if blocked {
			return ErrTrustVisitorBlocked
		}
		if haveName == "" {
			if name == "" || company == "" {
				return ErrTrustVisitorProfileRequired
			}
			if _, err = tx.Exec(ctx, `
				UPDATE trust_visitors SET name = $3, company = $4
				WHERE organization_id = $1 AND id = $2
			`, orgID, id, name, company); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `
			UPDATE trust_visitors
			SET verified_at = COALESCE(verified_at, now()), last_seen = now()
			WHERE organization_id = $1 AND id = $2
		`, orgID, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO trust_visitor_sessions (organization_id, visitor_id, token_hash, expires_at, last_seen_at)
			VALUES ($1, $2, $3, now() + make_interval(secs => $4), now())
		`, orgID, id, sessionHash, sessionTTL.Seconds()); err != nil {
			return err
		}
		out, err = scanTrustVisitor(tx.QueryRow(ctx, `
			SELECT `+trustVisitorColumns+` FROM trust_visitors v
			WHERE v.organization_id = $1 AND v.id = $2
		`, orgID, id))
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetTrustVisitorSession resolves a session token hash to its visitor. The
// session must be unrevoked, unexpired, of this organization, and its visitor
// not blocked; otherwise ErrTrustVisitorNotFound. The last-seen stamps are
// refreshed at most once every five minutes.
func (d *DB) GetTrustVisitorSession(ctx context.Context, orgID int, tokenHash string) (*TrustVisitor, error) {
	var sessionID int64
	var seen *time.Time
	var v TrustVisitor
	var verified, blockedAt *time.Time
	err := d.pool.QueryRow(ctx, `
		SELECT s.id, s.last_seen_at,
		       v.id, v.organization_id, v.email, v.name, v.company, v.verified_at,
		       v.first_seen, v.last_seen, v.blocked, v.blocked_at, v.blocked_by
		FROM trust_visitor_sessions s
		JOIN trust_visitors v ON v.id = s.visitor_id AND v.organization_id = s.organization_id
		WHERE s.organization_id = $1 AND s.token_hash = $2
		  AND s.revoked_at IS NULL AND s.expires_at > now() AND NOT v.blocked
	`, orgID, tokenHash).Scan(&sessionID, &seen,
		&v.ID, &v.OrganizationID, &v.Email, &v.Name, &v.Company, &verified,
		&v.FirstSeen, &v.LastSeen, &v.Blocked, &blockedAt, &v.BlockedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrustVisitorNotFound
	}
	if err != nil {
		return nil, err
	}
	if verified != nil {
		v.VerifiedAt = &Epoch{Time: *verified}
	}
	if blockedAt != nil {
		v.BlockedAt = &Epoch{Time: *blockedAt}
	}
	if seen == nil || time.Since(*seen) >= trustVisitorSeenEvery {
		err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `
				UPDATE trust_visitor_sessions SET last_seen_at = now()
				WHERE organization_id = $1 AND id = $2
			`, orgID, sessionID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				UPDATE trust_visitors SET last_seen = now()
				WHERE organization_id = $1 AND id = $2
				  AND last_seen < now() - make_interval(secs => $3)
			`, orgID, v.ID, trustVisitorSeenEvery.Seconds())
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	return &v, nil
}

// RevokeTrustVisitorSession ends one session (sign-out). An unknown, foreign
// or already revoked token is not an error.
func (d *DB) RevokeTrustVisitorSession(ctx context.Context, orgID int, tokenHash string) error {
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE trust_visitor_sessions SET revoked_at = now()
			WHERE organization_id = $1 AND token_hash = $2 AND revoked_at IS NULL
		`, orgID, tokenHash)
		return err
	})
}

// RevokeTrustVisitorSessions ends every active session of a visitor and
// returns how many it ended.
func (d *DB) RevokeTrustVisitorSessions(ctx context.Context, orgID int, visitorID int64) (int, error) {
	var n int
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE trust_visitor_sessions SET revoked_at = now()
			WHERE organization_id = $1 AND visitor_id = $2 AND revoked_at IS NULL
		`, orgID, visitorID)
		n = int(tag.RowsAffected())
		return err
	})
	return n, err
}

// SetTrustVisitorBlocked blocks or unblocks a visitor. Blocking also revokes
// every session; unblocking does not bring them back. ErrTrustVisitorNotFound
// for an unknown id or another organization's visitor.
func (d *DB) SetTrustVisitorBlocked(ctx context.Context, orgID int, visitorID int64, blocked bool, actor string) (*TrustVisitor, error) {
	var out *TrustVisitor
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var tag int64
		if blocked {
			t, err := tx.Exec(ctx, `
				UPDATE trust_visitors SET blocked = true, blocked_at = now(), blocked_by = $3
				WHERE organization_id = $1 AND id = $2
			`, orgID, visitorID, actor)
			if err != nil {
				return err
			}
			tag = t.RowsAffected()
		} else {
			t, err := tx.Exec(ctx, `
				UPDATE trust_visitors SET blocked = false, blocked_at = NULL, blocked_by = NULL
				WHERE organization_id = $1 AND id = $2
			`, orgID, visitorID)
			if err != nil {
				return err
			}
			tag = t.RowsAffected()
		}
		if tag == 0 {
			return ErrTrustVisitorNotFound
		}
		if blocked {
			if _, err := tx.Exec(ctx, `
				UPDATE trust_visitor_sessions SET revoked_at = now()
				WHERE organization_id = $1 AND visitor_id = $2 AND revoked_at IS NULL
			`, orgID, visitorID); err != nil {
				return err
			}
		}
		v, err := scanTrustVisitor(tx.QueryRow(ctx, `
			SELECT `+trustVisitorColumns+` FROM trust_visitors v
			WHERE v.organization_id = $1 AND v.id = $2
		`, orgID, visitorID))
		out = v
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListTrustVisitors returns the organization's visitors, most recently seen
// first, each with its count of active sessions.
func (d *DB) ListTrustVisitors(ctx context.Context, orgID int) ([]TrustVisitor, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+trustVisitorColumns+`
		FROM trust_visitors v WHERE v.organization_id = $1
		ORDER BY v.last_seen DESC, v.id DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrustVisitor{}
	for rows.Next() {
		v, err := scanTrustVisitor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// trustBuiltinBlockedEmailDomains are the personal mail providers refused when
// an organization has not set its own list. Exact domain match only.
var trustBuiltinBlockedEmailDomains = []string{
	"gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com", "msn.com",
	"yahoo.com", "ymail.com", "icloud.com", "me.com", "mac.com", "aol.com",
	"proton.me", "protonmail.com", "gmx.com", "gmx.de", "gmx.net", "mail.com",
	"yandex.com", "yandex.ru", "zoho.com",
}

// TrustBuiltinBlockedEmailDomains returns a copy of the built-in list.
func TrustBuiltinBlockedEmailDomains() []string {
	return append([]string(nil), trustBuiltinBlockedEmailDomains...)
}

// EffectiveBlockedEmailDomains is the list that applies: the built-in one when
// the organization never set its own (nil), otherwise its list as stored (an
// empty list allows every domain).
func (s *TrustCenterSettings) EffectiveBlockedEmailDomains() []string {
	if s.BlockedEmailDomains == nil {
		return TrustBuiltinBlockedEmailDomains()
	}
	return s.BlockedEmailDomains
}

// DomainBlocked reports whether the email's domain is on the effective list.
// The match is exact and case-insensitive; a subdomain is a different domain.
func (s *TrustCenterSettings) DomainBlocked(email string) bool {
	email = normalizeVisitorEmail(email)
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, b := range s.EffectiveBlockedEmailDomains() {
		if strings.ToLower(strings.TrimSpace(b)) == domain {
			return true
		}
	}
	return false
}
