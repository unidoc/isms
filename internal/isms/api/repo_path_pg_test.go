package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// #462: an org's repo_path is an absolute path frozen at creation. After the
// data dir moves, the server must find the repo under ISMS_DATA_DIR instead of
// failing every document operation.
//
// Requires a migrated Postgres (see id_resolution_pg_test.go).

// newRepoPathOrg creates an org whose stored repo_path is storedPath.
func newRepoPathOrg(t *testing.T, s *Server, tag, storedPath string) *db.Organization {
	t.Helper()
	slug := fmt.Sprintf("repopath-%s-%d", tag, os.Getpid())
	org := &db.Organization{Name: slug, Slug: slug, RepoPath: storedPath}
	if err := s.db.CreateOrganization(context.Background(), org); err != nil {
		t.Fatalf("creating org %s: %v", slug, err)
	}
	t.Cleanup(func() { _ = s.db.DeleteOrganization(context.Background(), org.ID) })
	return org
}

// initBareRepo creates a usable bare repo (one commit) at path.
func initBareRepo(t *testing.T, path string) *store.Store {
	t.Helper()
	if _, err := git.PlainInit(path, true); err != nil {
		t.Fatalf("init %s: %v", path, err)
	}
	st, err := store.NewBare(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if _, err := st.CommitFile(filepath.Join(path, "README.md"), []byte("# repo\n"),
		"Test", "test@example.com", "init"); err != nil {
		t.Fatalf("commit into %s: %v", path, err)
	}
	return st
}

func TestStoreForOrgFallsBackToDataDirWhenRepoMoved(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "new")
	t.Setenv("ISMS_DATA_DIR", newDir)

	stored := filepath.Join(tmp, "old", "repos", "x.git") // never created
	org := newRepoPathOrg(t, s, "moved", stored)
	want := filepath.Join(newDir, "repos", org.Slug+".git")
	initBareRepo(t, want)

	st, err := s.storeForOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("storeForOrg with moved repo: %v", err)
	}
	if st.Root() != want {
		t.Errorf("store root = %q, want fallback %q", st.Root(), want)
	}
	got, err := s.db.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoPath != stored {
		t.Errorf("repo_path rewritten to %q, want unchanged %q", got.RepoPath, stored)
	}
}

func TestStoreForOrgPrefersStoredPathOverDataDir(t *testing.T) {
	s := testServer(t)
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "new")
	t.Setenv("ISMS_DATA_DIR", newDir)

	stored := filepath.Join(tmp, "custom", "repo.git")
	org := newRepoPathOrg(t, s, "stored", stored)
	initBareRepo(t, stored)
	initBareRepo(t, filepath.Join(newDir, "repos", org.Slug+".git"))

	st, err := s.storeForOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatalf("storeForOrg: %v", err)
	}
	if st.Root() != stored {
		t.Errorf("store root = %q, want stored path %q", st.Root(), stored)
	}
}

func TestStoreForOrgErrorNamesBothPathsWhenNeitherExists(t *testing.T) {
	s := testServer(t)
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "new")
	t.Setenv("ISMS_DATA_DIR", newDir)

	stored := filepath.Join(tmp, "old", "repos", "gone.git")
	org := newRepoPathOrg(t, s, "none", stored)

	_, err := s.storeForOrg(context.Background(), org.ID)
	if err == nil {
		t.Fatal("storeForOrg succeeded with no repo anywhere")
	}
	tried := filepath.Join(newDir, "repos", org.Slug+".git")
	for _, p := range []string{stored, tried} {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q does not name %q", err, p)
		}
	}
}

// Template removal builds its path from the store root, so a moved repo (store
// root != repo_path) deletes the right directory rather than a ../.. path.
func TestRemoveTemplateWorksWhenRepoMoved(t *testing.T) {
	s := testServer(t)
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "new")
	t.Setenv("ISMS_DATA_DIR", newDir)

	tplDir := filepath.Join(tmp, "templates")
	if err := os.MkdirAll(filepath.Join(tplDir, "tpl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "tpl", "meta.yaml"), []byte("id: tpl\nname: Tpl\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ISMS_TEMPLATE_PATH", tplDir)

	org := newRepoPathOrg(t, s, "tplrm", filepath.Join(tmp, "old", "repos", "x.git"))
	repo := filepath.Join(newDir, "repos", org.Slug+".git")
	st := initBareRepo(t, repo)
	doc := "---\ndocument_id: tpl-1\ntitle: T\nstatus: draft\n---\nbody\n"
	if _, err := st.CommitFile(filepath.Join(repo, "documents", "tpl", "a.md"), []byte(doc),
		"Test", "test@example.com", "add doc"); err != nil {
		t.Fatal(err)
	}

	c, rec := ctxFor(org.ID, http.MethodDelete, "tpl", "")
	c.SetParamNames("name")
	if err := s.handleRemoveTemplate(c); err != nil {
		t.Fatalf("handleRemoveTemplate: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	fresh, err := store.NewBare(repo)
	if err != nil {
		t.Fatal(err)
	}
	if path := fresh.FindDocumentByID("tpl-1"); path != "" {
		t.Errorf("template document still present after removal at %s", path)
	}
}
