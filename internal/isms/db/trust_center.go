package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Trust center access levels. public items are readable by anyone, gated ones
// after the visitor identifies themselves, restricted ones only with an
// explicit grant. This layer stores the level; enforcement belongs to the
// handlers that serve item content.
const (
	TrustAccessPublic     = "public"
	TrustAccessGated      = "gated"
	TrustAccessRestricted = "restricted"
)

// Trust center item kinds.
const (
	TrustKindDocument      = "document"
	TrustKindCertification = "certification"
	TrustKindSubprocessor  = "subprocessor"
	TrustKindCard          = "card"
	TrustKindFAQ           = "faq"
	TrustKindUpdate        = "update"
)

var trustAccessLevels = map[string]bool{
	TrustAccessPublic: true, TrustAccessGated: true, TrustAccessRestricted: true,
}

var trustKinds = map[string]bool{
	TrustKindDocument: true, TrustKindCertification: true, TrustKindSubprocessor: true,
	TrustKindCard: true, TrustKindFAQ: true, TrustKindUpdate: true,
}

var (
	// ErrTrustItemAlreadyPublished: the (kind, reference) pair is already
	// published in the organization.
	ErrTrustItemAlreadyPublished = errors.New("this item is already published in the trust center")
	// ErrTrustItemNotFound: no item with that id in the organization.
	ErrTrustItemNotFound = errors.New("trust center item not found")
	// ErrTrustCenterInvalid wraps every validation failure, so callers can map
	// it to a 400 with errors.Is.
	ErrTrustCenterInvalid = errors.New("invalid trust center value")
)

// TrustCenterSettings is an organization's trust center configuration. The
// zero value for an organization that never configured it is disabled.
type TrustCenterSettings struct {
	OrganizationID     int
	Enabled            bool
	Title              string
	OverviewDocumentID *string
	SecurityContact    string
	DefaultAccessLevel string
	UpdatedAt          Epoch
	UpdatedBy          *string
}

// TrustCenterItem is one published item. Reference is internal (a document_id,
// a register id, ...) and must not be shown to anonymous visitors; PublicID is
// the identifier safe to expose.
type TrustCenterItem struct {
	ID             int64
	PublicID       string
	OrganizationID int
	Kind           string
	Reference      string
	AccessLevel    string
	Section        string
	SortOrder      int
	PublishedBy    string
	PublishedAt    Epoch
	UpdatedAt      Epoch
	// HiddenAt is set when the item's source was retired or deleted; a hidden
	// item is not served until it is published again.
	HiddenAt     *Epoch
	HiddenReason string
}

// Reasons an item can be hidden.
const (
	TrustHiddenRetired = "retired"
	TrustHiddenDeleted = "deleted"
)

const trustItemColumns = `id, public_id::text, organization_id, kind, reference, access_level,
		       section, sort_order, published_by, published_at, updated_at,
		       hidden_at, COALESCE(hidden_reason, '')`

func scanTrustItem(row pgx.Row) (*TrustCenterItem, error) {
	var it TrustCenterItem
	var hidden *time.Time
	if err := row.Scan(&it.ID, &it.PublicID, &it.OrganizationID, &it.Kind, &it.Reference,
		&it.AccessLevel, &it.Section, &it.SortOrder, &it.PublishedBy, &it.PublishedAt, &it.UpdatedAt,
		&hidden, &it.HiddenReason); err != nil {
		return nil, err
	}
	if hidden != nil {
		it.HiddenAt = &Epoch{Time: *hidden}
	}
	return &it, nil
}

func trustInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrTrustCenterInvalid, fmt.Sprintf(format, args...))
}

// GetTrustCenterSettings returns the organization's settings. An organization
// with no row gets the defaults (disabled), not an error.
func (d *DB) GetTrustCenterSettings(ctx context.Context, orgID int) (*TrustCenterSettings, error) {
	s := &TrustCenterSettings{OrganizationID: orgID}
	err := d.pool.QueryRow(ctx, `
		SELECT enabled, title, overview_document_id, security_contact,
		       default_access_level, updated_at, updated_by
		FROM trust_center_settings WHERE organization_id = $1
	`, orgID).Scan(&s.Enabled, &s.Title, &s.OverviewDocumentID, &s.SecurityContact,
		&s.DefaultAccessLevel, &s.UpdatedAt, &s.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return &TrustCenterSettings{OrganizationID: orgID, DefaultAccessLevel: TrustAccessGated}, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpsertTrustCenterSettings creates or replaces the organization's settings.
func (d *DB) UpsertTrustCenterSettings(ctx context.Context, orgID int, s *TrustCenterSettings, actor string) error {
	if !trustAccessLevels[s.DefaultAccessLevel] {
		return trustInvalid("default_access_level %q", s.DefaultAccessLevel)
	}
	if len(s.Title) > 200 {
		return trustInvalid("title is longer than 200 characters")
	}
	if len(s.SecurityContact) > 320 {
		return trustInvalid("security_contact is longer than 320 characters")
	}
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO trust_center_settings
				(organization_id, enabled, title, overview_document_id, security_contact,
				 default_access_level, updated_at, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, now(), $7)
			ON CONFLICT (organization_id) DO UPDATE SET
				enabled = EXCLUDED.enabled, title = EXCLUDED.title,
				overview_document_id = EXCLUDED.overview_document_id,
				security_contact = EXCLUDED.security_contact,
				default_access_level = EXCLUDED.default_access_level,
				updated_at = now(), updated_by = EXCLUDED.updated_by
			RETURNING updated_at
		`, orgID, s.Enabled, s.Title, s.OverviewDocumentID, s.SecurityContact,
			s.DefaultAccessLevel, nilIfEmpty(actor)).Scan(&s.UpdatedAt)
	})
}

// PublishTrustCenterItem publishes an item and records it in the changelog in
// the same transaction. Publishing a (kind, reference) pair that is already
// published returns ErrTrustItemAlreadyPublished.
func (d *DB) PublishTrustCenterItem(ctx context.Context, orgID int, item *TrustCenterItem, actor string) error {
	if !trustKinds[item.Kind] {
		return trustInvalid("kind %q", item.Kind)
	}
	if !trustAccessLevels[item.AccessLevel] {
		return trustInvalid("access_level %q", item.AccessLevel)
	}
	if n := len(item.Reference); n < 1 || n > 200 {
		return trustInvalid("reference must be 1 to 200 characters")
	}
	if len(item.Section) > 100 {
		return trustInvalid("section is longer than 100 characters")
	}
	if actor == "" {
		return trustInvalid("actor is required")
	}
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO trust_center_items
				(organization_id, kind, reference, access_level, section, sort_order, published_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, public_id::text, published_at, updated_at
		`, orgID, item.Kind, item.Reference, item.AccessLevel, item.Section,
			item.SortOrder, actor).Scan(&item.ID, &item.PublicID, &item.PublishedAt, &item.UpdatedAt)
		if err != nil {
			return err
		}
		item.OrganizationID = orgID
		item.PublishedBy = actor
		ref := item.Kind + ":" + item.Reference
		return LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_item", EntityID: item.ID, Action: "create",
			NewValue: &ref, ChangedBy: actor,
		})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "trust_center_items_organization_id_kind_reference_key" {
		return ErrTrustItemAlreadyPublished
	}
	return err
}

// UnpublishTrustCenterItem removes an item and records it in the changelog in
// the same transaction. An id that does not belong to orgID is not found.
func (d *DB) UnpublishTrustCenterItem(ctx context.Context, orgID int, itemID int64, actor string) error {
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var kind, reference string
		err := tx.QueryRow(ctx, `
			DELETE FROM trust_center_items WHERE id = $1 AND organization_id = $2
			RETURNING kind, reference
		`, itemID, orgID).Scan(&kind, &reference)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustItemNotFound
		}
		if err != nil {
			return err
		}
		ref := kind + ":" + reference
		return LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_item", EntityID: itemID, Action: "delete",
			OldValue: &ref, ChangedBy: actor,
		})
	})
}

// SetTrustCenterItemAccessLevel changes an item's access level and records the
// change in the same transaction. Setting the level it already has writes
// nothing, not even a changelog row.
func (d *DB) SetTrustCenterItemAccessLevel(ctx context.Context, orgID int, itemID int64, level, actor string) error {
	if !trustAccessLevels[level] {
		return trustInvalid("access_level %q", level)
	}
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var old string
		err := tx.QueryRow(ctx, `
			SELECT access_level FROM trust_center_items
			WHERE id = $1 AND organization_id = $2 FOR UPDATE
		`, itemID, orgID).Scan(&old)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustItemNotFound
		}
		if err != nil {
			return err
		}
		if old == level {
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE trust_center_items SET access_level = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2
		`, itemID, orgID, level); err != nil {
			return err
		}
		return LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_item", EntityID: itemID, Action: "update",
			Field: "access_level", OldValue: &old, NewValue: &level, ChangedBy: actor,
		})
	})
}

// ListPublishedTrustCenterItems returns every item the organization has
// published, whatever its access level, in display order. Hidden items are
// included with their hidden state set; callers that serve visitors skip them.
func (d *DB) ListPublishedTrustCenterItems(ctx context.Context, orgID int) ([]TrustCenterItem, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+trustItemColumns+`
		FROM trust_center_items
		WHERE organization_id = $1
		ORDER BY section, sort_order, published_at, id
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TrustCenterItem{}
	for rows.Next() {
		it, err := scanTrustItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	return items, rows.Err()
}

var trustUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// GetTrustCenterItemByPublicID returns the item of the given kind with that
// public id in the organization. An id that is not a UUID, or that belongs to
// another organization, is ErrTrustItemNotFound.
func (d *DB) GetTrustCenterItemByPublicID(ctx context.Context, orgID int, kind, publicID string) (*TrustCenterItem, error) {
	if !trustUUID.MatchString(publicID) {
		return nil, ErrTrustItemNotFound
	}
	it, err := scanTrustItem(d.pool.QueryRow(ctx, `
		SELECT `+trustItemColumns+`
		FROM trust_center_items
		WHERE organization_id = $1 AND kind = $2 AND public_id = $3::uuid
	`, orgID, kind, publicID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrustItemNotFound
	}
	return it, err
}

// GetTrustCenterItemByReference returns the item of the given kind published
// for reference (for a document, its lowercase document_id).
func (d *DB) GetTrustCenterItemByReference(ctx context.Context, orgID int, kind, reference string) (*TrustCenterItem, error) {
	it, err := scanTrustItem(d.pool.QueryRow(ctx, `
		SELECT `+trustItemColumns+`
		FROM trust_center_items
		WHERE organization_id = $1 AND kind = $2 AND reference = $3
	`, orgID, kind, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTrustItemNotFound
	}
	return it, err
}

// HideTrustCenterItem marks an item hidden (reason retired or deleted) and
// records it in the changelog in the same transaction. An item that is already
// hidden is left alone and writes nothing. actor may be "system" when a read
// path notices the source is gone.
func (d *DB) HideTrustCenterItem(ctx context.Context, orgID int, itemID int64, reason, actor string) error {
	if reason != TrustHiddenRetired && reason != TrustHiddenDeleted {
		return trustInvalid("hidden reason %q", reason)
	}
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT true FROM trust_center_items WHERE id = $1 AND organization_id = $2 FOR UPDATE
		`, itemID, orgID).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustItemNotFound
		} else if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE trust_center_items SET hidden_at = now(), hidden_reason = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND hidden_at IS NULL
		`, itemID, orgID, reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		empty := ""
		return LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_item", EntityID: itemID, Action: "update",
			Field: "hidden", OldValue: &empty, NewValue: &reason, ChangedBy: actor,
		})
	})
}

// RepublishTrustCenterItem clears an item's hidden state and makes it live
// again at level, as published now by actor. The public id is kept. The
// changelog records the fields that changed.
func (d *DB) RepublishTrustCenterItem(ctx context.Context, orgID int, itemID int64, level, actor string) error {
	if !trustAccessLevels[level] {
		return trustInvalid("access_level %q", level)
	}
	if actor == "" {
		return trustInvalid("actor is required")
	}
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var oldLevel, oldHidden string
		err := tx.QueryRow(ctx, `
			SELECT access_level, COALESCE(hidden_reason, '') FROM trust_center_items
			WHERE id = $1 AND organization_id = $2 FOR UPDATE
		`, itemID, orgID).Scan(&oldLevel, &oldHidden)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustItemNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE trust_center_items
			SET hidden_at = NULL, hidden_reason = NULL, access_level = $3,
			    published_at = now(), published_by = $4, updated_at = now()
			WHERE id = $1 AND organization_id = $2
		`, itemID, orgID, level, actor); err != nil {
			return err
		}
		if oldHidden != "" {
			empty := ""
			if err := LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
				EntityType: "trust_center_item", EntityID: itemID, Action: "update",
				Field: "hidden", OldValue: &oldHidden, NewValue: &empty, ChangedBy: actor,
			}); err != nil {
				return err
			}
		}
		if oldLevel != level {
			return LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
				EntityType: "trust_center_item", EntityID: itemID, Action: "update",
				Field: "access_level", OldValue: &oldLevel, NewValue: &level, ChangedBy: actor,
			})
		}
		return nil
	})
}
