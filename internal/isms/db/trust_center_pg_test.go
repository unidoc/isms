package db

// Round-trip tests for the trust center tables. They run only when
// ISMS_TEST_DATABASE_URL points at a migrated database:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/db/ -run TrustCenter

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func trustTestOrg(t *testing.T, d *DB, tag string) int {
	t.Helper()
	slug := fmt.Sprintf("tc-%s-%d", tag, time.Now().UnixNano())
	org := &Organization{Name: slug, Slug: slug, RepoPath: "/tmp/none"}
	if err := d.CreateOrganization(context.Background(), org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, org.ID)
	})
	return org.ID
}

func trustPublish(t *testing.T, d *DB, orgID int, kind, ref, level string) *TrustCenterItem {
	t.Helper()
	it := &TrustCenterItem{Kind: kind, Reference: ref, AccessLevel: level, Section: "docs", SortOrder: 1}
	if err := d.PublishTrustCenterItem(context.Background(), orgID, it, "mgr@example.com"); err != nil {
		t.Fatalf("publish %s/%s: %v", kind, ref, err)
	}
	return it
}

func TestTrustCenterSettingsDefaultsAndUpsert(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "set")

	s, err := d.GetTrustCenterSettings(ctx, orgID)
	if err != nil {
		t.Fatalf("get with no row: %v", err)
	}
	if s.Enabled || s.DefaultAccessLevel != TrustAccessGated {
		t.Errorf("defaults = %+v, want disabled/gated", s)
	}

	in := &TrustCenterSettings{Enabled: true, Title: "Acme Trust", SecurityContact: "sec@acme.test", DefaultAccessLevel: TrustAccessPublic}
	if err := d.UpsertTrustCenterSettings(ctx, orgID, in, "mgr@example.com"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := d.GetTrustCenterSettings(ctx, orgID)
	if err != nil || !got.Enabled || got.Title != "Acme Trust" || got.DefaultAccessLevel != TrustAccessPublic {
		t.Fatalf("after upsert = %+v, %v", got, err)
	}
	in.Enabled = false
	if err := d.UpsertTrustCenterSettings(ctx, orgID, in, "mgr@example.com"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if got, _ := d.GetTrustCenterSettings(ctx, orgID); got.Enabled {
		t.Error("second upsert did not disable")
	}

	bad := &TrustCenterSettings{DefaultAccessLevel: "secret"}
	if err := d.UpsertTrustCenterSettings(ctx, orgID, bad, "x"); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("bad level err = %v, want ErrTrustCenterInvalid", err)
	}
}

func TestTrustCenterPublishDuplicateAndValidation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "dup")

	trustPublish(t, d, orgID, TrustKindDocument, "iso27001-4-1", TrustAccessPublic)
	dup := &TrustCenterItem{Kind: TrustKindDocument, Reference: "iso27001-4-1", AccessLevel: TrustAccessGated}
	if err := d.PublishTrustCenterItem(ctx, orgID, dup, "mgr@example.com"); !errors.Is(err, ErrTrustItemAlreadyPublished) {
		t.Fatalf("duplicate err = %v, want ErrTrustItemAlreadyPublished", err)
	}
	// Same reference under another kind is a different item.
	trustPublish(t, d, orgID, TrustKindFAQ, "iso27001-4-1", TrustAccessPublic)

	for name, it := range map[string]*TrustCenterItem{
		"kind":      {Kind: "nope", Reference: "r", AccessLevel: TrustAccessPublic},
		"level":     {Kind: TrustKindFAQ, Reference: "r", AccessLevel: "nope"},
		"reference": {Kind: TrustKindFAQ, Reference: "", AccessLevel: TrustAccessPublic},
	} {
		if err := d.PublishTrustCenterItem(ctx, orgID, it, "mgr@example.com"); !errors.Is(err, ErrTrustCenterInvalid) {
			t.Errorf("%s: err = %v, want ErrTrustCenterInvalid", name, err)
		}
	}
	items, _ := d.ListPublishedTrustCenterItems(ctx, orgID)
	if len(items) != 2 {
		t.Errorf("items = %d, want 2 (failed publishes must leave nothing)", len(items))
	}
}

func TestTrustCenterListIsolatedPerOrg(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	a := trustTestOrg(t, d, "a")
	b := trustTestOrg(t, d, "b")

	ia := trustPublish(t, d, a, TrustKindDocument, "doc-a", TrustAccessPublic)
	ib := trustPublish(t, d, b, TrustKindDocument, "doc-b", TrustAccessPublic)

	got, err := d.ListPublishedTrustCenterItems(ctx, a)
	if err != nil || len(got) != 1 || got[0].ID != ia.ID {
		t.Fatalf("list A = %+v, %v; want only A's item", got, err)
	}

	// An id from org B used with org A is not found and B's row is untouched.
	if err := d.UnpublishTrustCenterItem(ctx, a, ib.ID, "mgr@example.com"); !errors.Is(err, ErrTrustItemNotFound) {
		t.Errorf("cross-org unpublish err = %v, want not found", err)
	}
	if err := d.SetTrustCenterItemAccessLevel(ctx, a, ib.ID, TrustAccessRestricted, "mgr@example.com"); !errors.Is(err, ErrTrustItemNotFound) {
		t.Errorf("cross-org set level err = %v, want not found", err)
	}
	gotB, _ := d.ListPublishedTrustCenterItems(ctx, b)
	if len(gotB) != 1 || gotB[0].AccessLevel != TrustAccessPublic {
		t.Errorf("org B item changed: %+v", gotB)
	}
	for _, orgID := range []int{a, b} {
		cl, _ := d.ListChangelog(ctx, orgID, "trust_center_item", 50)
		if len(cl) != 1 {
			t.Errorf("org %d changelog rows = %d, want 1 (create only)", orgID, len(cl))
		}
	}
}

func TestTrustCenterListOrder(t *testing.T) {
	d := testDB(t)
	orgID := trustTestOrg(t, d, "ord")
	mk := func(ref, section string, order int) {
		it := &TrustCenterItem{Kind: TrustKindDocument, Reference: ref, AccessLevel: TrustAccessPublic, Section: section, SortOrder: order}
		if err := d.PublishTrustCenterItem(context.Background(), orgID, it, "m@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	mk("c", "b", 1)
	mk("b", "a", 2)
	mk("a", "a", 1)
	got, _ := d.ListPublishedTrustCenterItems(context.Background(), orgID)
	var refs string
	for _, it := range got {
		refs += it.Reference
	}
	if refs != "abc" {
		t.Errorf("order = %q, want abc", refs)
	}
}

func TestTrustCenterChangelog(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "log")
	it := trustPublish(t, d, orgID, TrustKindDocument, "doc-1", TrustAccessGated)

	if err := d.SetTrustCenterItemAccessLevel(ctx, orgID, it.ID, TrustAccessGated, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetTrustCenterItemAccessLevel(ctx, orgID, it.ID, "bogus", "mgr@example.com"); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("bogus level err = %v", err)
	}
	if err := d.SetTrustCenterItemAccessLevel(ctx, orgID, it.ID, TrustAccessPublic, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := d.UnpublishTrustCenterItem(ctx, orgID, it.ID, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := d.UnpublishTrustCenterItem(ctx, orgID, it.ID, "mgr@example.com"); !errors.Is(err, ErrTrustItemNotFound) {
		t.Errorf("second unpublish err = %v, want not found", err)
	}

	cl, err := d.ListEntityChangelog(ctx, orgID, "trust_center_item", it.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Newest first: delete, update, create. The unchanged level wrote nothing.
	if len(cl) != 3 {
		t.Fatalf("changelog rows = %d, want 3: %+v", len(cl), cl)
	}
	if cl[0].Action != "delete" || cl[1].Action != "update" || cl[2].Action != "create" {
		t.Errorf("actions = %s,%s,%s", cl[0].Action, cl[1].Action, cl[2].Action)
	}
	up := cl[1]
	if up.Field != "access_level" || up.OldValue == nil || *up.OldValue != "gated" || up.NewValue == nil || *up.NewValue != "public" {
		t.Errorf("update row = %+v", up)
	}
	if cl[2].ChangedBy != "mgr@example.com" {
		t.Errorf("changed_by = %q", cl[2].ChangedBy)
	}
}
