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

func TestTrustCenterGetItemByPublicIDAndReference(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	a := trustTestOrg(t, d, "ga")
	b := trustTestOrg(t, d, "gb")
	ia := trustPublish(t, d, a, TrustKindDocument, "doc-a", TrustAccessPublic)
	ib := trustPublish(t, d, b, TrustKindDocument, "doc-b", TrustAccessPublic)

	got, err := d.GetTrustCenterItemByPublicID(ctx, a, TrustKindDocument, ia.PublicID)
	if err != nil || got.ID != ia.ID || got.Reference != "doc-a" || got.HiddenAt != nil {
		t.Fatalf("by public id = %+v, %v", got, err)
	}
	got, err = d.GetTrustCenterItemByReference(ctx, a, TrustKindDocument, "doc-a")
	if err != nil || got.ID != ia.ID {
		t.Fatalf("by reference = %+v, %v", got, err)
	}
	// Other org's id, wrong kind, unknown reference and a malformed uuid are
	// all plain not-found, never an error that would become a 500.
	for name, fn := range map[string]func() error{
		"cross-org id": func() error { _, e := d.GetTrustCenterItemByPublicID(ctx, a, TrustKindDocument, ib.PublicID); return e },
		"wrong kind":   func() error { _, e := d.GetTrustCenterItemByPublicID(ctx, a, TrustKindFAQ, ia.PublicID); return e },
		"malformed uuid": func() error {
			_, e := d.GetTrustCenterItemByPublicID(ctx, a, TrustKindDocument, "not-a-uuid")
			return e
		},
		"cross-org ref": func() error { _, e := d.GetTrustCenterItemByReference(ctx, a, TrustKindDocument, "doc-b"); return e },
	} {
		if err := fn(); !errors.Is(err, ErrTrustItemNotFound) {
			t.Errorf("%s: err = %v, want ErrTrustItemNotFound", name, err)
		}
	}
}

func TestTrustCenterHideAndRepublish(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "hide")
	other := trustTestOrg(t, d, "hideb")
	it := trustPublish(t, d, orgID, TrustKindDocument, "doc-1", TrustAccessPublic)
	foreign := trustPublish(t, d, other, TrustKindDocument, "doc-x", TrustAccessPublic)

	// Org A cannot hide org B's item.
	if err := d.HideTrustCenterItem(ctx, orgID, foreign.ID, "retired", "system"); !errors.Is(err, ErrTrustItemNotFound) {
		t.Errorf("cross-org hide err = %v, want not found", err)
	}
	if got, _ := d.GetTrustCenterItemByReference(ctx, other, TrustKindDocument, "doc-x"); got == nil || got.HiddenAt != nil {
		t.Errorf("org B item was hidden: %+v", got)
	}
	if err := d.HideTrustCenterItem(ctx, orgID, it.ID, "bogus", "system"); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("bogus reason err = %v, want invalid", err)
	}

	if err := d.HideTrustCenterItem(ctx, orgID, it.ID, "retired", "system"); err != nil {
		t.Fatal(err)
	}
	// Hiding again is a no-op, even with another reason.
	if err := d.HideTrustCenterItem(ctx, orgID, it.ID, "deleted", "system"); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetTrustCenterItemByPublicID(ctx, orgID, TrustKindDocument, it.PublicID)
	if got.HiddenAt == nil || got.HiddenReason != "retired" {
		t.Fatalf("after hide = %+v", got)
	}
	list, _ := d.ListPublishedTrustCenterItems(ctx, orgID)
	if len(list) != 1 || list[0].HiddenAt == nil || list[0].HiddenReason != "retired" {
		t.Errorf("list must expose hidden state: %+v", list)
	}
	cl, _ := d.ListEntityChangelog(ctx, orgID, "trust_center_item", it.ID)
	if len(cl) != 2 || cl[0].Action != "update" || cl[0].Field != "hidden" ||
		cl[0].NewValue == nil || *cl[0].NewValue != "retired" || cl[0].ChangedBy != "system" {
		t.Fatalf("changelog after double hide = %+v", cl)
	}

	if err := d.RepublishTrustCenterItem(ctx, orgID, it.ID, TrustAccessGated, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetTrustCenterItemByPublicID(ctx, orgID, TrustKindDocument, it.PublicID)
	if got.HiddenAt != nil || got.HiddenReason != "" || got.AccessLevel != TrustAccessGated || got.PublishedBy != "mgr@example.com" {
		t.Errorf("after republish = %+v", got)
	}
	if got.PublicID != it.PublicID {
		t.Errorf("public id changed on republish: %s -> %s", it.PublicID, got.PublicID)
	}
	cl, _ = d.ListEntityChangelog(ctx, orgID, "trust_center_item", it.ID)
	if len(cl) < 3 || cl[0].Action != "update" {
		t.Errorf("republish must be logged: %+v", cl)
	}
	if err := d.RepublishTrustCenterItem(ctx, other, it.ID, TrustAccessPublic, "x"); !errors.Is(err, ErrTrustItemNotFound) {
		t.Errorf("cross-org republish err = %v, want not found", err)
	}
	if err := d.RepublishTrustCenterItem(ctx, orgID, it.ID, "bogus", "x"); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("bogus level err = %v, want invalid", err)
	}
}
