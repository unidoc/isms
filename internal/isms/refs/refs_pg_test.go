package refs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// These tests cover what the shared rule adds over the API's old behaviour:
// telling "the record is not there" apart from "the lookup failed". A caller
// that deletes broken references (the CLI) must never act on a failed lookup.
// The per-type matrix is exercised through the API's own tests.
//
// Requires a migrated Postgres; skipped otherwise:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/refs/...
func testDB(t *testing.T) *db.DB {
	t.Helper()
	url := os.Getenv("ISMS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ISMS_TEST_DATABASE_URL not set — skipping references database test")
	}
	d, err := db.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

// newTestOrg creates a throwaway organization; it is soft-deleted on cleanup so
// the suite can run repeatedly.
func newTestOrg(t *testing.T, d *db.DB, name string) int {
	t.Helper()
	slug := fmt.Sprintf("%s-%d", name, os.Getpid())
	org := &db.Organization{Name: slug, Slug: slug, RepoPath: "/tmp/none"}
	if err := d.CreateOrganization(context.Background(), org); err != nil {
		t.Fatalf("creating org %s: %v", name, err)
	}
	t.Cleanup(func() { _ = d.DeleteOrganization(context.Background(), org.ID) })
	return org.ID
}

func newBareStoreWithDoc(t *testing.T, docID, title string) *store.Store {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "org.git")
	if _, err := git.PlainInit(repoPath, true); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	st, err := store.NewBare(repoPath)
	if err != nil {
		t.Fatalf("NewBare: %v", err)
	}
	content := "---\ndocument_id: " + docID + "\ntitle: " + title + "\nstatus: draft\n---\nbody\n"
	abs := filepath.Join(st.Root(), "documents", "test", docID+".md")
	if _, err := st.CommitFile(abs, []byte(content), "Test User", "test@example.com", "add "+docID); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	return st
}

func TestLookupDocument(t *testing.T) {
	d := testDB(t)
	orgID := newTestOrg(t, d, "refs-doc")
	ctx := context.Background()
	viewer := db.TaskViewer{CanSeeAll: true}

	t.Run("store will not open is an error, not not-found", func(t *testing.T) {
		boom := errors.New("repo is not on this machine")
		r := &Resolver{DB: d, Store: func(context.Context, int) (*store.Store, error) { return nil, boom }}
		_, _, found, err := r.Lookup(ctx, orgID, viewer, "document", "iso27001-4-1")
		if err == nil {
			t.Fatal("Lookup returned a nil error for a store that would not open; want the error")
		}
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want it to wrap the opener's error", err)
		}
		if found {
			t.Error("found = true for a failed lookup")
		}
	})

	st := newBareStoreWithDoc(t, "iso27001-4-1", "Context of the organization")
	r := &Resolver{DB: d, Store: func(context.Context, int) (*store.Store, error) { return st, nil }}

	t.Run("document present", func(t *testing.T) {
		title, canonical, found, err := r.Lookup(ctx, orgID, viewer, "document", "iso27001-4-1")
		if err != nil || !found {
			t.Fatalf("found=%v err=%v, want found and no error", found, err)
		}
		if title != "Context of the organization" || canonical != "iso27001-4-1" {
			t.Errorf("title=%q canonical=%q", title, canonical)
		}
	})

	t.Run("document missing is not-found without error", func(t *testing.T) {
		_, _, found, err := r.Lookup(ctx, orgID, viewer, "document", "iso27001-9-9")
		if err != nil {
			t.Fatalf("err = %v, want nil for a document that is not in the repo", err)
		}
		if found {
			t.Error("found = true for a missing document")
		}
	})
}

func TestLookupDatabase(t *testing.T) {
	d := testDB(t)
	orgID := newTestOrg(t, d, "refs-db")
	viewer := db.TaskViewer{CanSeeAll: true}
	r := &Resolver{DB: d, Store: func(context.Context, int) (*store.Store, error) {
		t.Error("the store was opened for a database-backed type")
		return nil, errors.New("unexpected")
	}}

	// An id in the form each type's lookup accepts, so the query actually runs.
	ids := map[string]string{
		"risk": "RISK-99999", "task": "TASK-99999", "change_request": "CR-99999",
		"program": "NOPE", "objective": "NOPE-99999", "audit": "AUDIT-99999",
		"audit_finding": "FIND-99999", "audit_programme": "AP-99999",
	}

	t.Run("missing record is not-found without error", func(t *testing.T) {
		for typ, id := range ids {
			_, _, found, err := r.Lookup(context.Background(), orgID, viewer, typ, id)
			if err != nil || found {
				t.Errorf("%s %s: found=%v err=%v, want found=false err=nil", typ, id, found, err)
			}
		}
		// Not a record problem at all: an unknown type and an unparsable id.
		for _, c := range [][2]string{{"unknown_type", "X-1"}, {"audit", "AUDIT-x"}, {"task", "7x"}} {
			_, _, found, err := r.Lookup(context.Background(), orgID, viewer, c[0], c[1])
			if err != nil || found {
				t.Errorf("%s %s: found=%v err=%v, want found=false err=nil", c[0], c[1], found, err)
			}
		}
	})

	t.Run("database failure is an error, not not-found", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for typ, id := range ids {
			_, _, found, err := r.Lookup(ctx, orgID, viewer, typ, id)
			if err == nil {
				t.Errorf("%s: nil error for a cancelled context; want the failure", typ)
			}
			if found {
				t.Errorf("%s: found = true for a failed lookup", typ)
			}
		}
	})
}

func TestCanonicalBareNumberNeedsNoDatabase(t *testing.T) {
	// Nil DB and nil Store: the shape gate must answer before either is used.
	r := &Resolver{}
	for typ := range SequenceIdentifierTypes {
		id, found, err := r.Canonical(context.Background(), 1, db.TaskViewer{}, typ, "12")
		if err != nil || found || id != "12" {
			t.Errorf("%s: id=%q found=%v err=%v, want %q, false, nil", typ, id, found, err, "12")
		}
	}
}
