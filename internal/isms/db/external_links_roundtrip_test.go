package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// addLink creates a supplier link whose valid_until is CURRENT_DATE + days,
// computed by Postgres (days == nil leaves it NULL).
func addLink(t *testing.T, d *DB, orgID int, supplierID int64, label string, days *int) *ExternalLink {
	t.Helper()
	ctx := context.Background()
	var link *ExternalLink
	err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		link, err = CreateExternalLinkTx(ctx, tx, orgID, "supplier", supplierID, ExternalLinkInput{
			URL: "https://example.com/" + label, Label: label, LinkType: "certificate",
		}, "creator@external-links.test")
		return err
	})
	if err != nil {
		t.Fatalf("creating link %q: %v", label, err)
	}
	if days != nil {
		if _, err := d.pool.Exec(ctx, `UPDATE external_links SET valid_until = CURRENT_DATE + $1::int WHERE id = $2`, *days, link.ID); err != nil {
			t.Fatalf("setting valid_until: %v", err)
		}
	}
	return link
}

func intp(n int) *int { return &n }

func listLabels(t *testing.T, d *DB, orgID int, supplierID int64) []string {
	t.Helper()
	links, err := d.ListExternalLinks(context.Background(), orgID, "supplier", supplierID)
	if err != nil {
		t.Fatalf("ListExternalLinks: %v", err)
	}
	var out []string
	for _, l := range links {
		out = append(out, l.Label)
	}
	return out
}

func TestExternalLinkCRUDAndState(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "extlink-crud")
	sup := newTestSupplier(t, d, orgID, "Link Co")

	expired := addLink(t, d, orgID, sup.ID, "b-expired", intp(-3))
	_ = addLink(t, d, orgID, sup.ID, "c-expiring", intp(10))
	current := addLink(t, d, orgID, sup.ID, "d-current", intp(31))
	nodate := addLink(t, d, orgID, sup.ID, "a-nodate", nil)

	links, err := d.ListExternalLinks(ctx, orgID, "supplier", sup.ID)
	if err != nil {
		t.Fatalf("ListExternalLinks: %v", err)
	}
	got := map[string]ExternalLink{}
	var order []string
	for _, l := range links {
		got[l.Label] = l
		order = append(order, l.Label)
	}
	for label, want := range map[string]string{"b-expired": "expired", "c-expiring": "expiring", "d-current": "current", "a-nodate": ""} {
		if got[label].State != want {
			t.Errorf("%s: state = %q, want %q", label, got[label].State, want)
		}
	}
	if strings.Join(order, ",") != "b-expired,c-expiring,d-current,a-nodate" {
		t.Errorf("order = %v, want soonest expiry first, undated last", order)
	}
	if nodate.CreatedBy != "creator@external-links.test" || nodate.VerifiedBy != "" || nodate.VerifiedAt != nil {
		t.Errorf("unexpected fresh link: %+v", nodate)
	}

	// Update (full replace) with a verified date stamps the verifier.
	day := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	var updated *ExternalLink
	err = d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		updated, err = UpdateExternalLinkTx(ctx, tx, orgID, current.ID, ExternalLinkInput{
			URL: "https://example.com/new", Label: "renamed", LinkType: "audit_report",
			ValidUntil: &day, VerifiedAt: &day,
		}, false, email)
		return err
	})
	if err != nil {
		t.Fatalf("UpdateExternalLinkTx: %v", err)
	}
	if updated.Label != "renamed" || updated.LinkType != "audit_report" || updated.VerifiedBy != email ||
		updated.ValidUntil == nil || updated.ValidUntil.UTC().Format("2006-01-02") != "2030-01-02" {
		t.Errorf("update not applied: %+v", updated)
	}
	if updated.State != "current" {
		t.Errorf("state after update = %q, want current", updated.State)
	}
	if want := "renamed (audit_report) https://example.com/new; valid until 2030-01-02; verified 2030-01-02"; updated.ChangeSummary() != want {
		t.Errorf("ChangeSummary = %q, want %q", updated.ChangeSummary(), want)
	}

	// Replace with nil dates clears them and the verifier.
	err = d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		updated, err = UpdateExternalLinkTx(ctx, tx, orgID, current.ID, ExternalLinkInput{
			URL: "https://example.com/new", Label: "renamed", LinkType: "audit_report",
		}, false, email)
		return err
	})
	if err != nil {
		t.Fatalf("UpdateExternalLinkTx: %v", err)
	}
	if updated.ValidUntil != nil || updated.VerifiedAt != nil || updated.VerifiedBy != "" || updated.State != "" {
		t.Errorf("nil dates did not clear: %+v", updated)
	}
	if updated.ChangeSummary() != "renamed (audit_report) https://example.com/new; valid until -; verified -" {
		t.Errorf("ChangeSummary with no dates = %q", updated.ChangeSummary())
	}

	// Verify stamps today (Postgres's date) and the user.
	var verified *ExternalLink
	err = d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		verified, err = VerifyExternalLinkTx(ctx, tx, orgID, nodate.ID, email)
		return err
	})
	if err != nil {
		t.Fatalf("VerifyExternalLinkTx: %v", err)
	}
	var sameDay bool
	if err := d.pool.QueryRow(ctx, `SELECT verified_at = CURRENT_DATE FROM external_links WHERE id = $1`, nodate.ID).Scan(&sameDay); err != nil || !sameDay {
		t.Errorf("verified_at is not CURRENT_DATE (err %v)", err)
	}
	if verified.VerifiedBy != email {
		t.Errorf("verified_by = %q, want %q", verified.VerifiedBy, email)
	}

	// Soft delete: gone from list and from get, row kept.
	err = d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return DeleteExternalLinkTx(ctx, tx, orgID, expired.ID)
	})
	if err != nil {
		t.Fatalf("DeleteExternalLinkTx: %v", err)
	}
	if strings.Contains(strings.Join(listLabels(t, d, orgID, sup.ID), ","), "b-expired") {
		t.Errorf("deleted link still listed")
	}
	var kept int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM external_links WHERE id = $1 AND deleted_at IS NOT NULL`, expired.ID).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("soft-deleted row not kept (count %d, err %v)", kept, err)
	}
	err = d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := GetExternalLinkTx(ctx, tx, orgID, expired.ID)
		return err
	})
	if err != pgx.ErrNoRows {
		t.Errorf("GetExternalLinkTx on a deleted link: err = %v, want ErrNoRows", err)
	}
}

func TestGetOverdueSummarySurfacesExternalLinks(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "extlink-overdue")
	sup := newTestSupplier(t, d, orgID, "Evidence Co")

	expired := addLink(t, d, orgID, sup.ID, "expired-cert", intp(-5))
	expiring := addLink(t, d, orgID, sup.ID, "expiring-cert", intp(10))
	far := addLink(t, d, orgID, sup.ID, "far-cert", intp(31))
	nodate := addLink(t, d, orgID, sup.ID, "undated-cert", nil)
	deleted := addLink(t, d, orgID, sup.ID, "deleted-cert", intp(-1))
	if err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return DeleteExternalLinkTx(ctx, tx, orgID, deleted.ID)
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	term := newTestSupplier(t, d, orgID, "Terminated Co")
	termLink := addLink(t, d, orgID, term.ID, "terminated-cert", intp(-5))
	if _, err := d.pool.Exec(ctx, `UPDATE suppliers SET status = 'terminated' WHERE id = $1`, term.ID); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	gone := newTestSupplier(t, d, orgID, "Deleted Co")
	goneLink := addLink(t, d, orgID, gone.ID, "deleted-supplier-cert", intp(-5))
	if _, err := d.pool.Exec(ctx, `UPDATE suppliers SET deleted_at = now() WHERE id = $1`, gone.ID); err != nil {
		t.Fatalf("delete supplier: %v", err)
	}

	summary, err := d.GetOverdueSummary(ctx, orgID, TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("GetOverdueSummary: %v", err)
	}
	byID := map[int64]OverdueItem{}
	for _, it := range summary.ExternalLinks {
		byID[it.LinkID] = it
	}

	if it, ok := byID[expired.ID]; !ok {
		t.Errorf("expired link absent")
	} else {
		if it.State != "expired" || it.DaysLate != 5 || it.EntityType != "external_link" ||
			it.EntityID != sup.Identifier || it.Title != "expired-cert" || it.ParentTitle != "Evidence Co" ||
			it.Criticality != "medium" || it.NextReview == nil {
			t.Errorf("expired item wrong: %+v", it)
		}
	}
	if it, ok := byID[expiring.ID]; !ok || it.State != "expiring" || it.DaysLate != -10 {
		t.Errorf("expiring item wrong: %+v (present %v)", it, ok)
	}
	for name, l := range map[string]*ExternalLink{"31-day": far, "undated": nodate, "deleted": deleted, "terminated supplier": termLink, "deleted supplier": goneLink} {
		if _, ok := byID[l.ID]; ok {
			t.Errorf("%s link must be absent from the overdue summary", name)
		}
	}
	if len(summary.ExternalLinks) != 2 {
		t.Errorf("ExternalLinks = %d items, want 2", len(summary.ExternalLinks))
	}
	if summary.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2 (both links, nothing else seeded)", summary.TotalCount)
	}
}

func TestExternalLinksAreOrgScoped(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA, _ := newTestOrgUser(t, d, "extlink-org-a")
	orgB, _ := newTestOrgUser(t, d, "extlink-org-b")
	supA := newTestSupplier(t, d, orgA, "A Co")
	linkA := addLink(t, d, orgA, supA.ID, "a-cert", intp(-2))

	// Same row id asked for as org B: nothing.
	if got := listLabels(t, d, orgB, supA.ID); len(got) != 0 {
		t.Errorf("org B lists org A's links: %v", got)
	}
	sumB, err := d.GetOverdueSummary(ctx, orgB, TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("GetOverdueSummary: %v", err)
	}
	if len(sumB.ExternalLinks) != 0 || sumB.TotalCount != 0 {
		t.Errorf("org B summary shows org A's link: %+v", sumB.ExternalLinks)
	}
	sumA, _ := d.GetOverdueSummary(ctx, orgA, TaskViewer{CanSeeAll: true})
	if len(sumA.ExternalLinks) != 1 {
		t.Errorf("org A summary lost its link: %d", len(sumA.ExternalLinks))
	}

	// Writes and reads by id from org B hit ErrNoRows.
	err = d.WithOrgTx(ctx, orgB, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := GetExternalLinkTx(ctx, tx, orgB, linkA.ID); err != pgx.ErrNoRows {
			t.Errorf("GetExternalLinkTx cross-org: %v, want ErrNoRows", err)
		}
		if err := DeleteExternalLinkTx(ctx, tx, orgB, linkA.ID); err != pgx.ErrNoRows {
			t.Errorf("DeleteExternalLinkTx cross-org: %v, want ErrNoRows", err)
		}
		if _, err := VerifyExternalLinkTx(ctx, tx, orgB, linkA.ID, "x@y.test"); err != pgx.ErrNoRows {
			t.Errorf("VerifyExternalLinkTx cross-org: %v, want ErrNoRows", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
