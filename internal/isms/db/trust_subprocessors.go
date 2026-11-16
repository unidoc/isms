package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Trust center subprocessors (#522) are published from the Suppliers register:
// an item of kind subprocessor whose reference is the supplier's id as decimal
// text, always public. A supplier can be published only while it is active and
// has data access; every writer that makes it ineligible hides the item in the
// same transaction (UpdateSupplierTx, DeleteSupplier), and the read paths check
// again, so a writer that bypassed the hook never leaks a row.

var (
	// ErrTrustSubprocessorNotFound: no such supplier in the organization (or it
	// was deleted).
	ErrTrustSubprocessorNotFound = errors.New("supplier not found")
)

// TrustSubprocessorIneligibleError: the supplier cannot be published now.
// Reason is TrustHiddenTerminated or TrustHiddenNoDataAccess.
type TrustSubprocessorIneligibleError struct{ Reason string }

func (e *TrustSubprocessorIneligibleError) Error() string {
	return "supplier cannot be published as a subprocessor: " + e.Reason
}

func subprocessorRef(supplierID int64) string { return strconv.FormatInt(supplierID, 10) }

// hideSubprocessorForSupplierTx hides the supplier's visible subprocessor item,
// if it has one, and logs it like HideTrustCenterItem does. No visible item
// writes nothing.
func hideSubprocessorForSupplierTx(ctx context.Context, tx pgx.Tx, orgID int, supplierID int64, reason, actor string) error {
	var itemID int64
	err := tx.QueryRow(ctx, `
		UPDATE trust_center_items SET hidden_at = now(), hidden_reason = $3, updated_at = now()
		WHERE organization_id = $1 AND kind = 'subprocessor' AND reference = $2 AND hidden_at IS NULL
		RETURNING id
	`, orgID, subprocessorRef(supplierID), reason).Scan(&itemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	empty := ""
	return LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
		EntityType: "trust_center_item", EntityID: itemID, Action: "update",
		Field: "hidden", OldValue: &empty, NewValue: &reason, ChangedBy: actor,
	})
}

// bumpSubprocessorsChangedTx moves trust_center_settings.subprocessors_changed_at
// to now and leaves every other settings column alone. A missing settings row is
// created with the defaults (disabled), which is what a missing row reads as.
func bumpSubprocessorsChangedTx(ctx context.Context, tx pgx.Tx, orgID int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO trust_center_settings (organization_id, subprocessors_changed_at)
		VALUES ($1, now())
		ON CONFLICT (organization_id) DO UPDATE SET subprocessors_changed_at = now()
	`, orgID)
	return err
}

// PublishTrustSubprocessor publishes a supplier as a subprocessor at level
// public, or republishes it when its item is hidden (the public id is kept).
// The supplier row is locked for the transaction, so a concurrent terminate
// either is refused here or hides the new item after it. Errors:
// ErrTrustSubprocessorNotFound, *TrustSubprocessorIneligibleError,
// ErrTrustItemAlreadyPublished.
func (d *DB) PublishTrustSubprocessor(ctx context.Context, orgID int, supplierID int64, actor string) (*TrustCenterItem, error) {
	if actor == "" {
		return nil, trustInvalid("actor is required")
	}
	var item *TrustCenterItem
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		sup := Supplier{}
		err := tx.QueryRow(ctx, `
			SELECT COALESCE(status, 'active'), data_access FROM suppliers
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL FOR UPDATE
		`, supplierID, orgID).Scan(&sup.Status, &sup.DataAccess)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustSubprocessorNotFound
		}
		if err != nil {
			return err
		}
		if reason := SupplierTrustIneligibleReason(&sup); reason != "" {
			return &TrustSubprocessorIneligibleError{Reason: reason}
		}
		existing, err := scanTrustItem(tx.QueryRow(ctx, `
			SELECT `+trustItemColumns+` FROM trust_center_items
			WHERE organization_id = $1 AND kind = 'subprocessor' AND reference = $2 FOR UPDATE
		`, orgID, subprocessorRef(supplierID)))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			item = &TrustCenterItem{Kind: TrustKindSubprocessor, Reference: subprocessorRef(supplierID), AccessLevel: TrustAccessPublic}
			return publishTrustCenterItemTx(ctx, tx, orgID, item, actor)
		case err != nil:
			return err
		case existing.HiddenAt == nil:
			return ErrTrustItemAlreadyPublished
		}
		if err := republishTrustCenterItemTx(ctx, tx, orgID, existing.ID, TrustAccessPublic, actor); err != nil {
			return err
		}
		item, err = scanTrustItem(tx.QueryRow(ctx, `SELECT `+trustItemColumns+` FROM trust_center_items WHERE id = $1`, existing.ID))
		return err
	})
	if err != nil {
		return nil, mapTrustAlreadyPublished(err)
	}
	return item, nil
}

// UnpublishTrustSubprocessor removes the supplier's item (hidden or not) and
// moves "last changed". A supplier that is not published is ErrTrustItemNotFound.
func (d *DB) UnpublishTrustSubprocessor(ctx context.Context, orgID int, supplierID int64, actor string) error {
	return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var itemID int64
		err := tx.QueryRow(ctx, `
			DELETE FROM trust_center_items
			WHERE organization_id = $1 AND kind = 'subprocessor' AND reference = $2
			RETURNING id
		`, orgID, subprocessorRef(supplierID)).Scan(&itemID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrustItemNotFound
		}
		if err != nil {
			return err
		}
		ref := TrustKindSubprocessor + ":" + subprocessorRef(supplierID)
		if err := LogChangeTx(ctx, tx, orgID, &ChangelogEntry{
			EntityType: "trust_center_item", EntityID: itemID, Action: "delete",
			OldValue: &ref, ChangedBy: actor,
		}); err != nil {
			return err
		}
		return bumpSubprocessorsChangedTx(ctx, tx, orgID)
	})
}

// TrustSubprocessorsChangedAt is when the visitor-visible subprocessor list last
// changed, or nil when it never did: the latest of the unpublish/reorder stamp,
// any subprocessor item's published_at or updated_at (hiding sets updated_at, so
// hidden items count), and trust_public_changed_at of the suppliers behind the
// items that are not hidden. GREATEST ignores NULLs.
func (d *DB) TrustSubprocessorsChangedAt(ctx context.Context, orgID int) (*time.Time, error) {
	var t *time.Time
	err := d.pool.QueryRow(ctx, `
		SELECT GREATEST(
			(SELECT subprocessors_changed_at FROM trust_center_settings WHERE organization_id = $1),
			(SELECT MAX(published_at) FROM trust_center_items WHERE organization_id = $1 AND kind = 'subprocessor'),
			(SELECT MAX(updated_at) FROM trust_center_items WHERE organization_id = $1 AND kind = 'subprocessor'),
			(SELECT MAX(s.trust_public_changed_at) FROM suppliers s
			   JOIN trust_center_items i ON i.organization_id = s.organization_id
			    AND i.kind = 'subprocessor' AND i.reference = s.id::text AND i.hidden_at IS NULL
			  WHERE s.organization_id = $1))
	`, orgID).Scan(&t)
	return t, err
}

// TrustSubprocessor is a supplier as the subprocessor list sees it: the public
// fields as stored, the state that decides eligibility, and its item if it has
// one.
type TrustSubprocessor struct {
	SupplierID         int64
	Identifier         string
	Name               string
	PublicName         string
	PublicPurpose      string
	ProcessingLocation string
	DataCategories     string
	Status             string
	DataAccess         bool
	// Deleted: the supplier is soft-deleted, or its row is gone.
	Deleted bool
	Item    *TrustCenterItem
}

// IneligibleReason is "" when the supplier may be served, else deleted,
// terminated or no_data_access.
func (t *TrustSubprocessor) IneligibleReason() string {
	if t.Deleted {
		return TrustHiddenDeleted
	}
	return SupplierTrustIneligibleReason(&Supplier{Status: t.Status, DataAccess: t.DataAccess})
}

// ListTrustSubprocessors returns every supplier that is a candidate or already
// published: active-in-register suppliers with data access, plus any supplier
// that has a subprocessor item, soft-deleted ones included (so a deleted one can
// be reported). Published items come first in display order, then the rest by
// name.
func (d *DB) ListTrustSubprocessors(ctx context.Context, orgID int) ([]TrustSubprocessor, error) {
	return d.listTrustSubprocessors(ctx, orgID, "")
}

// GetTrustSubprocessor is ListTrustSubprocessors for one supplier id: the entry
// the list would hold, or ErrTrustSubprocessorNotFound when it holds none (not a
// candidate and not published).
func (d *DB) GetTrustSubprocessor(ctx context.Context, orgID int, supplierID int64) (*TrustSubprocessor, error) {
	list, err := d.listTrustSubprocessors(ctx, orgID, subprocessorRef(supplierID))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrTrustSubprocessorNotFound
	}
	return &list[0], nil
}

// listTrustSubprocessors: ref "" lists everything, else only that supplier id.
func (d *DB) listTrustSubprocessors(ctx context.Context, orgID int, ref string) ([]TrustSubprocessor, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT s.id, COALESCE(s.identifier, ''), COALESCE(s.name, ''),
		       COALESCE(s.public_name, ''), COALESCE(s.public_purpose, ''),
		       COALESCE(s.processing_location, ''), COALESCE(s.data_categories, ''),
		       COALESCE(s.status, 'active'), COALESCE(s.data_access, false),
		       (s.id IS NULL OR s.deleted_at IS NOT NULL),
		       i.id, i.public_id::text, i.reference, i.access_level, i.section, i.sort_order,
		       i.published_by, i.published_at, i.updated_at, i.hidden_at, COALESCE(i.hidden_reason, '')
		FROM (SELECT * FROM suppliers WHERE organization_id = $1) s
		FULL OUTER JOIN (SELECT * FROM trust_center_items WHERE organization_id = $1 AND kind = 'subprocessor') i
		  ON i.reference = s.id::text
		WHERE ((s.deleted_at IS NULL AND s.data_access) OR i.id IS NOT NULL)
		  AND ($2 = '' OR COALESCE(s.id::text, i.reference) = $2)
		ORDER BY (i.id IS NULL), i.sort_order, i.id, lower(s.name), s.id
	`, orgID, ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrustSubprocessor{}
	for rows.Next() {
		var t TrustSubprocessor
		var sid *int64
		var itemID *int64
		var pub, ref, level, section, by *string
		var order *int
		var pubAt, updAt, hiddenAt *time.Time
		var hiddenReason string
		if err := rows.Scan(&sid, &t.Identifier, &t.Name, &t.PublicName, &t.PublicPurpose,
			&t.ProcessingLocation, &t.DataCategories, &t.Status, &t.DataAccess, &t.Deleted,
			&itemID, &pub, &ref, &level, &section, &order, &by, &pubAt, &updAt, &hiddenAt, &hiddenReason); err != nil {
			return nil, err
		}
		if sid != nil {
			t.SupplierID = *sid
		}
		if itemID != nil {
			it := &TrustCenterItem{
				ID: *itemID, PublicID: *pub, OrganizationID: orgID, Kind: TrustKindSubprocessor,
				Reference: *ref, AccessLevel: *level, Section: *section, SortOrder: *order,
				PublishedBy: *by, PublishedAt: Epoch{Time: *pubAt}, UpdatedAt: Epoch{Time: *updAt},
				HiddenReason: hiddenReason,
			}
			if hiddenAt != nil {
				it.HiddenAt = &Epoch{Time: *hiddenAt}
			}
			if sid == nil {
				// Supplier row gone: recover the id from the reference.
				if n, err := strconv.ParseInt(it.Reference, 10, 64); err == nil {
					t.SupplierID = n
				}
			}
			t.Item = it
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
