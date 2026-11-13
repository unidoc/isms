package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Trust center certifications (#521): certificates, audit reports, penetration
// test summaries and attestations an organization shows its visitors. A
// certification is a record of its own and can exist unpublished; publishing it
// adds a trust_center_items row (kind certification, reference = its id) that
// owns the access level, the display order and the public id.
//
// valid_from, valid_until and verified_at are calendar days. They cross this
// package as YYYY-MM-DD strings (never Epoch, which shifts a day west of UTC),
// are read with to_char, and are compared against Postgres's CURRENT_DATE.

// TrustCertificationTypes is the allowed link_type set (matches the table
// CHECK). Unlike an external link it has no "contract".
var TrustCertificationTypes = []string{"certificate", "audit_report", "penetration_test", "attestation", "other"}

// TrustCertificationFileTypes is the set of stored file content types. The
// caller picks one from the sniffed bytes, never from the client.
var TrustCertificationFileTypes = []string{"application/pdf", "image/png", "image/jpeg"}

// ErrTrustCertificationNotFound: no certification with that id (or public id)
// in the organization, or it is not servable.
var ErrTrustCertificationNotFound = errors.New("trust center certification not found")

// TrustCertificationItem is a certification's publication: the item row that
// owns its access level, order and public id.
type TrustCertificationItem struct {
	ID          int64  `json:"id"`
	PublicID    string `json:"public_id"`
	AccessLevel string `json:"access_level"`
	SortOrder   int    `json:"sort_order"`
}

// TrustCertification is one certification. Item is nil while it is not
// published.
type TrustCertification struct {
	ID         int64   `json:"id"`
	Label      string  `json:"label"`
	LinkType   string  `json:"link_type"`
	Issuer     string  `json:"issuer"`
	ValidFrom  *string `json:"valid_from,omitempty"`
	ValidUntil *string `json:"valid_until,omitempty"`
	// URL is set for a link certification, the File fields for an uploaded one;
	// exactly one source exists. FileKey is the blob path and never leaves the
	// server.
	URL             string  `json:"url,omitempty"`
	FileKey         string  `json:"-"`
	FileName        string  `json:"file_name,omitempty"`
	FileContentType string  `json:"file_content_type,omitempty"`
	FileSize        int64   `json:"file_size,omitempty"`
	FileSHA256      string  `json:"file_sha256,omitempty"`
	VerifiedAt      *string `json:"verified_at,omitempty"`
	// VerifiedBy is the verifying user's email; empty when never verified or
	// the user has since been deleted.
	VerifiedBy string `json:"verified_by,omitempty"`
	// State is "expired", "expiring" (within 30 days) or "current", computed in
	// SQL like external links. Empty when ValidUntil is NULL.
	State     string                  `json:"state,omitempty"`
	CreatedBy string                  `json:"created_by"`
	CreatedAt Epoch                   `json:"created_at"`
	UpdatedAt Epoch                   `json:"updated_at"`
	Item      *TrustCertificationItem `json:"item,omitempty"`
}

// TrustCertificationFile describes an uploaded file already written to the
// blob store. Key, Name, ContentType, Size and SHA256 are all required.
type TrustCertificationFile struct {
	Key         string
	Name        string
	ContentType string
	Size        int64
	SHA256      string
}

// TrustCertificationInput is every editable field of a certification. A nil
// date is NULL. Dates must already be YYYY-MM-DD (ParseTrustDate); they are
// checked again here. On create exactly one of URL and File is set; on update
// File must be nil, URL is required for a link certification and must be empty
// for a file one (the source is fixed at creation).
type TrustCertificationInput struct {
	Label      string
	LinkType   string
	Issuer     string
	ValidFrom  *string
	ValidUntil *string
	VerifiedAt *string
	URL        string
	File       *TrustCertificationFile
}

// ParseTrustDate is the one strict parser for trust center dates: exactly
// YYYY-MM-DD, a real calendar day, nothing else (no time, zone, padding,
// whitespace or epoch number). It returns the date unchanged. An empty string
// is invalid here; "no date" is expressed by the caller passing nil (see
// ParseOptionalTrustDate).
func ParseTrustDate(s string) (string, error) {
	t, err := time.Parse("2006-01-02", s)
	// Postgres has no year 0; four digits keep every other year in its range.
	if err != nil || t.Format("2006-01-02") != s || t.Year() < 1 {
		return "", trustInvalid("date %q must be YYYY-MM-DD", s)
	}
	return s, nil
}

// ParseOptionalTrustDate maps a JSON date field: nil (absent or null) stays nil
// and means NULL; a non-nil value must pass ParseTrustDate, so an empty string
// is rejected rather than treated as null.
func ParseOptionalTrustDate(s *string) (*string, error) {
	if s == nil {
		return nil, nil
	}
	v, err := ParseTrustDate(*s)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func containsStr(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// ValidateTrustCertificationInput runs the checks CreateTrustCertification and
// UpdateTrustCertification run on the editable fields, so a caller can refuse a
// request before it stores a file. Errors wrap ErrTrustCenterInvalid.
func ValidateTrustCertificationInput(in TrustCertificationInput) error {
	return validateTrustCertification(in)
}

func validateTrustCertificationFile(f *TrustCertificationFile) error {
	if f.Key == "" || f.SHA256 == "" || f.Size <= 0 {
		return trustInvalid("file key, size and sha256 are required")
	}
	if n := utf8.RuneCountInString(f.Name); n < 1 || n > 255 {
		return trustInvalid("file name must be 1 to 255 characters")
	}
	if !containsStr(TrustCertificationFileTypes, f.ContentType) {
		return trustInvalid("file content type %q", f.ContentType)
	}
	return nil
}

// validateTrustCertification checks the editable fields and returns the input
// with its dates normalised.
func validateTrustCertification(in TrustCertificationInput) error {
	if n := utf8.RuneCountInString(in.Label); n < 1 || n > 200 || strings.TrimSpace(in.Label) == "" {
		return trustInvalid("label must be 1 to 200 characters")
	}
	if utf8.RuneCountInString(in.Issuer) > 200 {
		return trustInvalid("issuer is longer than 200 characters")
	}
	if !containsStr(TrustCertificationTypes, in.LinkType) {
		return trustInvalid("link_type %q", in.LinkType)
	}
	if utf8.RuneCountInString(in.URL) > 2048 {
		return trustInvalid("url is longer than 2048 characters")
	}
	for _, p := range []*string{in.ValidFrom, in.ValidUntil, in.VerifiedAt} {
		if _, err := ParseOptionalTrustDate(p); err != nil {
			return err
		}
	}
	// YYYY-MM-DD compares correctly as text.
	if in.ValidFrom != nil && in.ValidUntil != nil && *in.ValidFrom > *in.ValidUntil {
		return trustInvalid("valid_from is after valid_until")
	}
	if in.File != nil {
		return validateTrustCertificationFile(in.File)
	}
	return nil
}

const trustCertificationCols = `c.id, c.label, c.link_type, c.issuer,
	to_char(c.valid_from, 'YYYY-MM-DD'), to_char(c.valid_until, 'YYYY-MM-DD'),
	COALESCE(c.url, ''), COALESCE(c.file_key, ''), COALESCE(c.file_name, ''),
	COALESCE(c.file_content_type, ''), COALESCE(c.file_size, 0), COALESCE(c.file_sha256, ''),
	to_char(c.verified_at, 'YYYY-MM-DD'),
	COALESCE((SELECT email FROM users WHERE id = c.verified_by_id), ''),
	CASE WHEN c.valid_until IS NULL THEN ''
		WHEN (c.valid_until - CURRENT_DATE)::int < 0 THEN 'expired'
		WHEN (c.valid_until - CURRENT_DATE)::int <= 30 THEN 'expiring'
		ELSE 'current' END,
	c.created_by, c.created_at, c.updated_at,
	i.id, i.public_id::text, i.access_level, i.sort_order`

// trustCertificationFrom joins the publication, if any. The item is matched by
// (org, kind, reference) like every other lookup of an item's source.
const trustCertificationFrom = `
	FROM trust_center_certifications c
	LEFT JOIN trust_center_items i ON i.organization_id = c.organization_id
		AND i.kind = 'certification' AND i.reference = c.id::text`

func scanTrustCertification(row pgx.Row) (*TrustCertification, error) {
	var c TrustCertification
	var itemID *int64
	var publicID, level *string
	var order *int
	if err := row.Scan(&c.ID, &c.Label, &c.LinkType, &c.Issuer, &c.ValidFrom, &c.ValidUntil,
		&c.URL, &c.FileKey, &c.FileName, &c.FileContentType, &c.FileSize, &c.FileSHA256,
		&c.VerifiedAt, &c.VerifiedBy, &c.State, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		&itemID, &publicID, &level, &order); err != nil {
		return nil, err
	}
	if itemID != nil {
		c.Item = &TrustCertificationItem{ID: *itemID, PublicID: *publicID, AccessLevel: *level, SortOrder: *order}
	}
	return &c, nil
}

// trustCertQuerier is what both the pool and a transaction offer.
type trustCertQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// getTrustCertification reads one certification of the organization. With lock
// it also locks the certification row for the rest of the transaction.
func getTrustCertification(ctx context.Context, q trustCertQuerier, orgID int, id int64, lock bool) (*TrustCertification, error) {
	sql := `SELECT ` + trustCertificationCols + trustCertificationFrom + `
		WHERE c.id = $1 AND c.organization_id = $2`
	if lock {
		sql += ` FOR UPDATE OF c`
	}
	c, err := scanTrustCertification(q.QueryRow(ctx, sql, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrustCertificationNotFound
	}
	return c, err
}

// GetTrustCertification returns one of the organization's certifications,
// published or not.
func (d *DB) GetTrustCertification(ctx context.Context, orgID int, id int64) (*TrustCertification, error) {
	return getTrustCertification(ctx, d.pool, orgID, id, false)
}

// ListTrustCertifications returns every certification of the organization,
// published ones first in display order, then unpublished ones by label.
func (d *DB) ListTrustCertifications(ctx context.Context, orgID int) ([]TrustCertification, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+trustCertificationCols+trustCertificationFrom+`
		WHERE c.organization_id = $1
		ORDER BY (i.id IS NULL), i.sort_order, c.label, c.id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrustCertification{}
	for rows.Next() {
		c, err := scanTrustCertification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ListServableTrustCertifications returns what a visitor may be shown: published,
// not hidden and not past valid_until (judged by Postgres's CURRENT_DATE, so the
// certificate counts as valid through its valid_until day), in display order.
// It does not look at the access level: the caller decides per level what to
// reveal (only a public certification's url or file may be served anonymously).
func (d *DB) ListServableTrustCertifications(ctx context.Context, orgID int) ([]TrustCertification, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+trustCertificationCols+`
		FROM trust_center_certifications c
		JOIN trust_center_items i ON i.organization_id = c.organization_id
			AND i.kind = 'certification' AND i.reference = c.id::text
		WHERE c.organization_id = $1 AND i.hidden_at IS NULL
			AND (c.valid_until IS NULL OR c.valid_until >= CURRENT_DATE)
		ORDER BY i.sort_order, i.published_at, i.id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrustCertification{}
	for rows.Next() {
		c, err := scanTrustCertification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetServableTrustCertification returns the servable certification with that
// item public id, under the same rules as ListServableTrustCertifications. An
// id that is not a UUID, is of another organization or kind, or is unpublished,
// hidden or expired is ErrTrustCertificationNotFound. The caller enforces the
// access level: the file and url of anything but a public certification must
// not be served to an anonymous visitor.
func (d *DB) GetServableTrustCertification(ctx context.Context, orgID int, publicID string) (*TrustCertification, error) {
	if !trustUUID.MatchString(publicID) {
		return nil, ErrTrustCertificationNotFound
	}
	c, err := scanTrustCertification(d.pool.QueryRow(ctx, `SELECT `+trustCertificationCols+`
		FROM trust_center_certifications c
		JOIN trust_center_items i ON i.organization_id = c.organization_id
			AND i.kind = 'certification' AND i.reference = c.id::text
		WHERE c.organization_id = $1 AND i.public_id = $2::uuid AND i.hidden_at IS NULL
			AND (c.valid_until IS NULL OR c.valid_until >= CURRENT_DATE)`, orgID, publicID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrustCertificationNotFound
	}
	return c, err
}

// CreateTrustCertification inserts a certification (unpublished) and records it
// in the changelog in the same transaction. A set verified_at is stamped with
// actor as the verifier.
func (d *DB) CreateTrustCertification(ctx context.Context, orgID int, in TrustCertificationInput, actor string) (*TrustCertification, error) {
	if actor == "" {
		return nil, trustInvalid("actor is required")
	}
	if err := validateTrustCertification(in); err != nil {
		return nil, err
	}
	if (in.URL != "") == (in.File != nil) {
		return nil, trustInvalid("exactly one of url and file is required")
	}
	var c *TrustCertification
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		f := in.File
		if f == nil {
			f = &TrustCertificationFile{}
		}
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO trust_center_certifications
				(organization_id, label, link_type, issuer, valid_from, valid_until, url,
				 file_key, file_name, file_content_type, file_size, file_sha256,
				 verified_at, verified_by_id, created_by)
			VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13::date,
				CASE WHEN $13::date IS NULL THEN NULL ELSE (SELECT id FROM users WHERE lower(email) = lower($14)) END,
				$14)
			RETURNING id
		`, orgID, in.Label, in.LinkType, in.Issuer, in.ValidFrom, in.ValidUntil, nilIfEmpty(in.URL),
			nilIfEmpty(f.Key), nilIfEmpty(f.Name), nilIfEmpty(f.ContentType), nilIfZero(f.Size), nilIfEmpty(f.SHA256),
			in.VerifiedAt, actor).Scan(&id)
		if err != nil {
			return err
		}
		if err := LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_certification", EntityID: id, Action: "create",
			NewValue: &in.Label, ChangedBy: actor,
		}); err != nil {
			return err
		}
		c, err = getTrustCertification(ctx, tx, orgID, id, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func nilIfZero(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}

// ErrTrustCertificationSource: the change does not fit the certification's
// source (a url for a file certification). It wraps ErrTrustCenterInvalid.
var ErrTrustCertificationSource = fmt.Errorf("%w: the url applies to link certifications only", ErrTrustCenterInvalid)

// OptionalTrustDate is a date in a patch: Set false leaves the field alone,
// Set with a nil Value clears it, otherwise Value is the new YYYY-MM-DD day.
type OptionalTrustDate struct {
	Set   bool
	Value *string
}

// UpdateTrustCertificationPatch is a partial update. A nil pointer or an unset
// date leaves the field as it is; it is merged into the row under its lock, so
// two concurrent patches of different fields cannot undo each other.
type UpdateTrustCertificationPatch struct {
	Label, LinkType, Issuer, URL      *string
	ValidFrom, ValidUntil, VerifiedAt OptionalTrustDate
}

// PatchTrustCertification applies a partial update: the merge into the current
// values happens on the row locked FOR UPDATE, inside the transaction that
// writes. Otherwise it behaves like UpdateTrustCertification (validation, one
// changelog row per changed field, nothing written when nothing changes). A url
// for a file certification is ErrTrustCertificationSource.
func (d *DB) PatchTrustCertification(ctx context.Context, orgID int, id int64, p UpdateTrustCertificationPatch, actor string) (*TrustCertification, error) {
	return d.updateTrustCertification(ctx, orgID, id, actor, func(cur *TrustCertification) (TrustCertificationInput, error) {
		in := TrustCertificationInput{
			Label: cur.Label, LinkType: cur.LinkType, Issuer: cur.Issuer, URL: cur.URL,
			ValidFrom: cur.ValidFrom, ValidUntil: cur.ValidUntil, VerifiedAt: cur.VerifiedAt,
		}
		if p.Label != nil {
			in.Label = *p.Label
		}
		if p.LinkType != nil {
			in.LinkType = *p.LinkType
		}
		if p.Issuer != nil {
			in.Issuer = *p.Issuer
		}
		if p.URL != nil {
			if cur.FileKey != "" {
				return in, ErrTrustCertificationSource
			}
			in.URL = *p.URL
		}
		if p.ValidFrom.Set {
			in.ValidFrom = p.ValidFrom.Value
		}
		if p.ValidUntil.Set {
			in.ValidUntil = p.ValidUntil.Value
		}
		if p.VerifiedAt.Set {
			in.VerifiedAt = p.VerifiedAt.Value
		}
		return in, nil
	})
}

// UpdateTrustCertification replaces every editable field and records one
// changelog row per field that changed, in the same transaction. An update that
// changes nothing writes nothing. A changed verified_at is stamped with actor as
// the verifier (a cleared one clears it); an unchanged one keeps its verifier.
// Use PatchTrustCertification to change some fields without reading the rest.
func (d *DB) UpdateTrustCertification(ctx context.Context, orgID int, id int64, in TrustCertificationInput, actor string) (*TrustCertification, error) {
	if in.File != nil {
		return nil, trustInvalid("the file is replaced with ReplaceTrustCertificationFile")
	}
	return d.updateTrustCertification(ctx, orgID, id, actor, func(*TrustCertification) (TrustCertificationInput, error) {
		return in, nil
	})
}

// updateTrustCertification locks the row, lets merge produce the new values from
// it, validates them and writes the difference.
func (d *DB) updateTrustCertification(ctx context.Context, orgID int, id int64, actor string, merge func(cur *TrustCertification) (TrustCertificationInput, error)) (*TrustCertification, error) {
	if actor == "" {
		return nil, trustInvalid("actor is required")
	}
	var out *TrustCertification
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		cur, err := getTrustCertification(ctx, tx, orgID, id, true)
		if err != nil {
			return err
		}
		in, err := merge(cur)
		if err != nil {
			return err
		}
		if err := validateTrustCertification(in); err != nil {
			return err
		}
		isFile := cur.FileKey != ""
		if isFile && in.URL != "" {
			return ErrTrustCertificationSource
		}
		if !isFile && in.URL == "" {
			return trustInvalid("a link certification needs a url")
		}
		type change struct {
			field    string
			old, new *string
		}
		str := func(s string) *string { return &s }
		var changes []change
		diff := func(field string, old, new *string) {
			if (old == nil) != (new == nil) || (old != nil && *old != *new) {
				changes = append(changes, change{field, old, new})
			}
		}
		diff("label", str(cur.Label), str(in.Label))
		diff("link_type", str(cur.LinkType), str(in.LinkType))
		diff("issuer", str(cur.Issuer), str(in.Issuer))
		diff("valid_from", cur.ValidFrom, in.ValidFrom)
		diff("valid_until", cur.ValidUntil, in.ValidUntil)
		diff("verified_at", cur.VerifiedAt, in.VerifiedAt)
		if !isFile {
			diff("url", str(cur.URL), str(in.URL))
		}
		if len(changes) == 0 {
			out = cur
			return nil
		}
		verifiedChanged := false
		for _, ch := range changes {
			if ch.field == "verified_at" {
				verifiedChanged = true
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE trust_center_certifications SET label = $3, link_type = $4, issuer = $5,
				valid_from = $6::date, valid_until = $7::date, verified_at = $8::date,
				url = CASE WHEN file_key IS NULL THEN $9::text ELSE NULL END,
				verified_by_id = CASE
					WHEN NOT $10::boolean THEN verified_by_id
					WHEN $8::date IS NULL THEN NULL
					ELSE (SELECT id FROM users WHERE lower(email) = lower($11)) END,
				updated_at = now()
			WHERE id = $1 AND organization_id = $2
		`, id, orgID, in.Label, in.LinkType, in.Issuer, in.ValidFrom, in.ValidUntil, in.VerifiedAt,
			nilIfEmpty(in.URL), verifiedChanged, actor); err != nil {
			return err
		}
		for _, ch := range changes {
			if err := LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
				EntityType: "trust_center_certification", EntityID: id, Action: "update",
				Field: ch.field, OldValue: ch.old, NewValue: ch.new, ChangedBy: actor,
			}); err != nil {
				return err
			}
		}
		out, err = getTrustCertification(ctx, tx, orgID, id, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceTrustCertificationFile points a file certification at a new, already
// stored file and returns the previous blob key, which the caller deletes after
// the commit. A link certification has no file to replace.
func (d *DB) ReplaceTrustCertificationFile(ctx context.Context, orgID int, id int64, f TrustCertificationFile, actor string) (*TrustCertification, string, error) {
	if actor == "" {
		return nil, "", trustInvalid("actor is required")
	}
	if err := validateTrustCertificationFile(&f); err != nil {
		return nil, "", err
	}
	var out *TrustCertification
	var oldKey string
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		cur, err := getTrustCertification(ctx, tx, orgID, id, true)
		if err != nil {
			return err
		}
		if cur.FileKey == "" {
			return trustInvalid("a link certification has no file to replace")
		}
		oldKey = cur.FileKey
		if _, err := tx.Exec(ctx, `
			UPDATE trust_center_certifications SET file_key = $3, file_name = $4,
				file_content_type = $5, file_size = $6, file_sha256 = $7, updated_at = now()
			WHERE id = $1 AND organization_id = $2
		`, id, orgID, f.Key, f.Name, f.ContentType, f.Size, f.SHA256); err != nil {
			return err
		}
		for _, ch := range []struct{ field, old, new string }{
			{"file_name", cur.FileName, f.Name}, {"file_sha256", cur.FileSHA256, f.SHA256},
		} {
			old, nw := ch.old, ch.new
			if err := LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
				EntityType: "trust_center_certification", EntityID: id, Action: "update",
				Field: ch.field, OldValue: &old, NewValue: &nw, ChangedBy: actor,
			}); err != nil {
				return err
			}
		}
		out, err = getTrustCertification(ctx, tx, orgID, id, false)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return out, oldKey, nil
}

// VerifyTrustCertification stamps verified_at with Postgres's today and the
// verifier, and records the change.
func (d *DB) VerifyTrustCertification(ctx context.Context, orgID int, id int64, actor string) (*TrustCertification, error) {
	if actor == "" {
		return nil, trustInvalid("actor is required")
	}
	var out *TrustCertification
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		cur, err := getTrustCertification(ctx, tx, orgID, id, true)
		if err != nil {
			return err
		}
		var today string
		if err := tx.QueryRow(ctx, `
			UPDATE trust_center_certifications SET verified_at = CURRENT_DATE,
				verified_by_id = (SELECT id FROM users WHERE lower(email) = lower($3)),
				updated_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING to_char(verified_at, 'YYYY-MM-DD')
		`, id, orgID, actor).Scan(&today); err != nil {
			return err
		}
		if err := LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_certification", EntityID: id, Action: "update",
			Field: "verified_at", OldValue: cur.VerifiedAt, NewValue: &today, ChangedBy: actor,
		}); err != nil {
			return err
		}
		out, err = getTrustCertification(ctx, tx, orgID, id, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteTrustCertification removes a certification and its item row in one
// transaction, with changelog rows for both. It returns the file's blob key
// (empty for a link certification) so the caller can delete the blob after the
// commit; a failed blob delete must not undo the delete.
func (d *DB) DeleteTrustCertification(ctx context.Context, orgID int, id int64, actor string) (string, error) {
	var fileKey string
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		cur, err := getTrustCertification(ctx, tx, orgID, id, true)
		if err != nil {
			return err
		}
		fileKey = cur.FileKey
		// The item is deleted by its own statement, not taken from cur: a publish
		// that committed while this transaction waited for the row lock is not in
		// the snapshot cur was read with.
		var itemID int64
		err = tx.QueryRow(ctx, `DELETE FROM trust_center_items
			WHERE organization_id = $1 AND kind = 'certification' AND reference = $2
			RETURNING id`, orgID, strconv.FormatInt(id, 10)).Scan(&itemID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			ref := TrustKindCertification + ":" + strconv.FormatInt(id, 10)
			if err := LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
				EntityType: "trust_center_item", EntityID: itemID, Action: "delete",
				OldValue: &ref, ChangedBy: actor,
			}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM trust_center_certifications WHERE id = $1 AND organization_id = $2`,
			id, orgID); err != nil {
			return err
		}
		return LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_certification", EntityID: id, Action: "delete",
			OldValue: &cur.Label, ChangedBy: actor,
		})
	})
	if err != nil {
		return "", err
	}
	return fileKey, nil
}

// TrustCertificationFileReferenced reports whether any certification of the
// organization points at the blob key. A caller that cannot tell whether its
// write committed (an ambiguous commit error) uses it to decide whether the
// blob it just stored is in use.
func (d *DB) TrustCertificationFileReferenced(ctx context.Context, orgID int, key string) (bool, error) {
	var n int
	err := d.pool.QueryRow(ctx, `SELECT count(*) FROM trust_center_certifications
		WHERE organization_id = $1 AND file_key = $2`, orgID, key).Scan(&n)
	return n > 0, err
}

// PublishTrustCertification publishes an existing certification of the
// organization at the given level (PublishTrustCenterItem with kind
// certification and reference = id). A certification of another organization,
// or none, is ErrTrustCertificationNotFound; one already published is
// ErrTrustItemAlreadyPublished.
func (d *DB) PublishTrustCertification(ctx context.Context, orgID int, id int64, level, actor string) (*TrustCenterItem, error) {
	it := &TrustCenterItem{Kind: TrustKindCertification, Reference: strconv.FormatInt(id, 10), AccessLevel: level}
	if !trustAccessLevels[level] {
		return nil, trustInvalid("access_level %q", level)
	}
	if actor == "" {
		return nil, trustInvalid("actor is required")
	}
	// The certification row is locked FOR SHARE in the same transaction as the
	// item insert, so a delete that races this publish either committed first
	// (not found here) or waits for it and then removes the item row too.
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var one int
		err := tx.QueryRow(ctx, `SELECT 1 FROM trust_center_certifications
			WHERE id = $1 AND organization_id = $2 FOR SHARE`, id, orgID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustCertificationNotFound
		}
		if err != nil {
			return err
		}
		return publishTrustCenterItemTx(ctx, tx, orgID, it, actor)
	})
	if err := mapTrustAlreadyPublished(err); err != nil {
		return nil, err
	}
	return it, nil
}

// UnpublishTrustCertification removes a certification's item row, keeping the
// certification. A certification that is not published (or does not exist) is
// ErrTrustItemNotFound.
func (d *DB) UnpublishTrustCertification(ctx context.Context, orgID int, id int64, actor string) error {
	it, err := d.GetTrustCenterItemByReference(ctx, orgID, TrustKindCertification, strconv.FormatInt(id, 10))
	if err != nil {
		return err
	}
	return d.UnpublishTrustCenterItem(ctx, orgID, it.ID, actor)
}
