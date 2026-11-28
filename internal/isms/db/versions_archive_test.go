package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Versions of a document that is gone are archived: no reader sees them, a new
// document with the same id (and the same version label) gets its own rows, and
// nothing is ever deleted. Requires a migrated Postgres, like the other tests here.

func recordV(t *testing.T, d *DB, orgID int, doc, ver, commit string) *DocumentVersion {
	t.Helper()
	v := &DocumentVersion{DocumentID: doc, Version: ver, CommitHash: commit, FilePath: "documents/" + doc + ".md", CreatedBy: "archive-test@example.test"}
	if err := d.RecordVersion(context.Background(), orgID, v); err != nil {
		t.Fatalf("RecordVersion: %v", err)
	}
	return v
}

func countVersions(t *testing.T, d *DB, orgID int, doc string, archived bool) int {
	t.Helper()
	var n int
	if err := d.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM document_versions WHERE organization_id = $1 AND document_id = $2 AND (archived_at IS NOT NULL) = $3`,
		orgID, doc, archived).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestArchiveDocumentVersionsAndTheReaders(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "archive-readers")
	old := recordV(t, d, orgID, "hr-pol", "1.0", "c1")
	recordV(t, d, orgID, "hr-pol", "2.0", "c2")
	keep := recordV(t, d, orgID, "other-pol", "1.0", "c3")

	// Case-insensitive, scoped to the id, idempotent.
	n, err := d.ArchiveDocumentVersions(ctx, orgID, "HR-Pol")
	if err != nil || n != 2 {
		t.Fatalf("archive = %d, %v; want 2", n, err)
	}
	if n, err := d.ArchiveDocumentVersions(ctx, orgID, "hr-pol"); err != nil || n != 0 {
		t.Errorf("second archive = %d, %v; want 0", n, err)
	}
	if got := countVersions(t, d, orgID, "hr-pol", true); got != 2 {
		t.Errorf("archived rows = %d: nothing may be deleted", got)
	}

	// Every reader ignores archived rows.
	if vs, err := d.ListVersions(ctx, orgID, "hr-pol"); err != nil || len(vs) != 0 {
		t.Errorf("ListVersions = %v, %v", vs, err)
	}
	if _, err := d.GetVersion(ctx, orgID, "hr-pol", "1.0"); err == nil {
		t.Error("GetVersion found an archived version")
	}
	if _, err := d.GetVersionByID(ctx, orgID, "hr-pol", old.ID); err == nil || !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetVersionByID = %v, want not found", err)
	}
	if _, err := d.LatestVersion(ctx, orgID, "hr-pol"); err == nil {
		t.Error("LatestVersion found an archived version")
	}
	// Another document is untouched.
	if vs, _ := d.ListVersions(ctx, orgID, "other-pol"); len(vs) != 1 || vs[0].ID != keep.ID {
		t.Errorf("other document's versions = %v", vs)
	}
}

// A document that reuses an id and a version label gets a new active row; the
// archived row is not overwritten, through RecordVersion and RecordVersionTx.
func TestRecordVersionNeverTouchesAnArchivedRow(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "archive-record")
	old := recordV(t, d, orgID, "hr-pol", "1.0", "old-commit")
	if _, err := d.ArchiveDocumentVersions(ctx, orgID, "hr-pol"); err != nil {
		t.Fatal(err)
	}

	fresh := recordV(t, d, orgID, "hr-pol", "1.0", "new-commit")
	if fresh.ID == old.ID {
		t.Fatal("RecordVersion overwrote the archived row")
	}
	// The same label again updates the ACTIVE row, as before.
	again := recordV(t, d, orgID, "hr-pol", "1.0", "newer-commit")
	if again.ID != fresh.ID {
		t.Errorf("an active row is no longer upserted: %d vs %d", again.ID, fresh.ID)
	}

	// The transaction variant (suggestion apply, merge).
	if _, err := d.ArchiveDocumentVersions(ctx, orgID, "hr-pol"); err != nil {
		t.Fatal(err)
	}
	var txID int
	if err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		v := &DocumentVersion{DocumentID: "hr-pol", Version: "1.0", CommitHash: "tx-commit", FilePath: "documents/hr-pol.md", CreatedBy: "archive-test@example.test"}
		if err := RecordVersionTx(ctx, tx, orgID, v); err != nil {
			return err
		}
		txID = v.ID
		return nil
	}); err != nil {
		t.Fatalf("RecordVersionTx: %v", err)
	}
	if txID == old.ID || txID == fresh.ID {
		t.Errorf("RecordVersionTx reused an archived row: %d (archived %d, %d)", txID, old.ID, fresh.ID)
	}
	var commit string
	if err := d.pool.QueryRow(ctx, `SELECT commit_hash FROM document_versions WHERE id = $1`, old.ID).Scan(&commit); err != nil || commit != "old-commit" {
		t.Errorf("the archived row changed: %q %v", commit, err)
	}
	if got := countVersions(t, d, orgID, "hr-pol", true); got != 2 {
		t.Errorf("archived rows = %d, want 2", got)
	}
	if vs, _ := d.ListVersions(ctx, orgID, "hr-pol"); len(vs) != 1 || vs[0].ID != txID {
		t.Errorf("active versions = %v", vs)
	}
}

func TestOverdueDocumentReviewsIgnoresArchivedVersions(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "archive-overdue")
	for _, doc := range []string{"gone-pol", "live-pol"} {
		v := recordV(t, d, orgID, doc, "1.0", "c")
		if _, err := d.pool.Exec(ctx, `UPDATE document_versions SET created_at = now() - interval '5 years' WHERE id = $1`, v.ID); err != nil {
			t.Fatal(err)
		}
	}
	names := func() map[string]bool {
		items, err := d.OverdueDocumentReviews(ctx, orgID)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, it := range items {
			m[it.EntityID] = true
		}
		return m
	}
	if m := names(); !m["gone-pol"] || !m["live-pol"] {
		t.Fatalf("precondition: both overdue, got %v", m)
	}
	if _, err := d.ArchiveDocumentVersions(ctx, orgID, "gone-pol"); err != nil {
		t.Fatal(err)
	}
	if m := names(); m["gone-pol"] || !m["live-pol"] {
		t.Errorf("after archiving: %v", m)
	}
}

func TestArchivedVersionsSinceAndTheTransaction(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "archive-since")
	recordV(t, d, orgID, "since-pol", "1.0", "c1")
	before := time.Now().Add(-time.Minute)
	if has, err := d.ArchivedVersionsSince(ctx, orgID, "since-pol", before); err != nil || has {
		t.Fatalf("nothing archived yet: %v %v", has, err)
	}
	// A live review of the document, and one that is finished.
	live := &Review{DocumentID: "since-pol", DocumentType: "policy", Title: "t", Version: "1.0", RequestedBy: email, Status: "approved"}
	done := &Review{DocumentID: "since-pol", DocumentType: "policy", Title: "t", Version: "0.9", RequestedBy: email, Status: "merged"}
	other := &Review{DocumentID: "other-pol", DocumentType: "policy", Title: "t", Version: "1.0", RequestedBy: email, Status: "open"}
	for _, r := range []*Review{live, done, other} {
		if err := d.CreateReview(ctx, orgID, r); err != nil {
			t.Fatal(err)
		}
	}
	n, closed, err := d.ArchiveDocumentAndCloseReviews(ctx, orgID, "SINCE-pol")
	if err != nil || n != 1 || len(closed) != 1 || closed[0] != live.ID {
		t.Fatalf("archive = %d %v %v", n, closed, err)
	}
	status := func(id int) string {
		var st string
		if err := d.pool.QueryRow(ctx, `SELECT status FROM reviews WHERE id = $1`, id).Scan(&st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	if status(live.ID) != "closed" || status(done.ID) != "merged" || status(other.ID) != "open" {
		t.Errorf("statuses: live %s, finished %s, other %s", status(live.ID), status(done.ID), status(other.ID))
	}
	if has, err := d.ArchivedVersionsSince(ctx, orgID, "since-pol", before); err != nil || !has {
		t.Errorf("archived since the review: %v %v", has, err)
	}
	if has, err := d.ArchivedVersionsSince(ctx, orgID, "since-pol", time.Now().Add(time.Minute)); err != nil || has {
		t.Errorf("an archive before the review counts: %v %v", has, err)
	}
	if has, _ := d.ArchivedVersionsSince(ctx, orgID, "other-pol", before); has {
		t.Error("another document counts")
	}
}
