package db

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSupplierActiveAndIneligibleReason(t *testing.T) {
	for status, want := range map[string]bool{"active": true, "under_review": true, "suspended": true, "terminated": false, "": true} {
		if got := SupplierActive(status); got != want {
			t.Errorf("SupplierActive(%q) = %v, want %v", status, got, want)
		}
	}
	for _, tc := range []struct {
		status string
		access bool
		want   string
	}{
		{"active", true, ""},
		{"suspended", true, ""},
		{"under_review", true, ""},
		{"active", false, "no_data_access"},
		{"terminated", true, "terminated"},
		{"terminated", false, "terminated"},
	} {
		if got := SupplierTrustIneligibleReason(&Supplier{Status: tc.status, DataAccess: tc.access}); got != tc.want {
			t.Errorf("reason(%s, access=%v) = %q, want %q", tc.status, tc.access, got, tc.want)
		}
	}
}

// Every supplier writer hides the published item in its own transaction.
func TestSubprocessorHiddenInTheWritersTransaction(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "subproc-hide-tx")

	newPublished := func(name string) *Supplier {
		s := &Supplier{Name: name, SupplierType: "cloud", Criticality: "low", Status: "active", DataAccess: true}
		if err := d.CreateSupplier(ctx, orgID, s); err != nil {
			t.Fatal(err)
		}
		if _, err := d.PublishTrustSubprocessor(ctx, orgID, s.ID, email); err != nil {
			t.Fatalf("publish: %v", err)
		}
		return s
	}
	hidden := func(s *Supplier) (string, bool) {
		var reason string
		var isHidden bool
		if err := d.pool.QueryRow(ctx, `
			SELECT COALESCE(hidden_reason, ''), hidden_at IS NOT NULL FROM trust_center_items
			WHERE organization_id = $1 AND kind = 'subprocessor' AND reference = $2`,
			orgID, fmt.Sprint(s.ID)).Scan(&reason, &isHidden); err != nil {
			t.Fatal(err)
		}
		return reason, isHidden
	}
	hideLogs := func(s *Supplier) int {
		var n int
		_ = d.pool.QueryRow(ctx, `
			SELECT count(*) FROM entity_changelog c JOIN trust_center_items i ON i.id = c.entity_id
			WHERE c.organization_id = $1 AND c.entity_type = 'trust_center_item' AND c.field = 'hidden'
			  AND i.reference = $2`, orgID, fmt.Sprint(s.ID)).Scan(&n)
		return n
	}

	t.Run("UpdateSupplier", func(t *testing.T) {
		s := newPublished("via UpdateSupplier")
		s.Status = "terminated"
		if err := d.UpdateSupplier(ctx, orgID, s, nil); err != nil {
			t.Fatal(err)
		}
		if r, h := hidden(s); !h || r != "terminated" || hideLogs(s) != 1 {
			t.Fatalf("hidden=%v reason=%q logs=%d", h, r, hideLogs(s))
		}
	})
	t.Run("UpdateSupplierTx", func(t *testing.T) {
		s := newPublished("via UpdateSupplierTx")
		s.DataAccess = false
		err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
			return UpdateSupplierTx(ctx, tx, orgID, s, d.SupplierReviewCycles(ctx, orgID), nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		if r, h := hidden(s); !h || r != "no_data_access" {
			t.Fatalf("hidden=%v reason=%q", h, r)
		}
	})
	t.Run("rolled back transaction leaves it visible", func(t *testing.T) {
		s := newPublished("rolled back")
		s.Status = "terminated"
		boom := errors.New("boom")
		err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
			if err := UpdateSupplierTx(ctx, tx, orgID, s, d.SupplierReviewCycles(ctx, orgID), nil); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatal(err)
		}
		if _, h := hidden(s); h {
			t.Fatal("the hide survived a rolled-back update")
		}
	})
	t.Run("DeleteSupplier", func(t *testing.T) {
		s := newPublished("via DeleteSupplier")
		if err := d.DeleteSupplier(ctx, orgID, s.ID); err != nil {
			t.Fatal(err)
		}
		if r, h := hidden(s); !h || r != "deleted" {
			t.Fatalf("hidden=%v reason=%q", h, r)
		}
	})
	t.Run("an eligible update and an unpublished supplier write nothing", func(t *testing.T) {
		s := newPublished("stays visible")
		s.Notes = "edit"
		if err := d.UpdateSupplier(ctx, orgID, s, nil); err != nil {
			t.Fatal(err)
		}
		if _, h := hidden(s); h || hideLogs(s) != 0 {
			t.Fatalf("hidden=%v logs=%d", h, hideLogs(s))
		}
		u := &Supplier{Name: "unpublished", SupplierType: "cloud", Criticality: "low", Status: "terminated"}
		if err := d.CreateSupplier(ctx, orgID, u); err != nil {
			t.Fatal(err)
		}
		if err := d.UpdateSupplier(ctx, orgID, u, nil); err != nil {
			t.Fatal(err)
		}
	})
}
