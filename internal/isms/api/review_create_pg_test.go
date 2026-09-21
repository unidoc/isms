package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// Postgres-gated regression tests for #299: POST /reviews silently discarded
// the reviewers field and returned 201 with zero review_assignments rows.
// Requires a migrated Postgres — see id_resolution_pg_test.go's testServer for
// setup. Skipped when ISMS_TEST_DATABASE_URL is unset.

// reviewCtxForPath is modelled on risk_custom_fields_pg_test.go's ctxForPath,
// but takes the actor email as an argument instead of hardcoding one.
// CreateReviewTx resolves requested_by_id via a NOT NULL lookup against
// users.email, so the context's user_email must be a seeded user or the
// handler fails with a 500 that looks like a bug in the code under test.
func reviewCtxForPath(orgID int, method, path, body, role, email string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	c.Set("user_role", role)
	c.Set("user_email", email)
	return c, rec
}

// seedRepoForOrg gives the org a bare repo containing one document and registers
// the store in the server's per-org cache, so storeForOrg finds it without
// touching Organization.RepoPath.
func seedRepoForOrg(t *testing.T, s *Server, orgID int, docID string) {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "org.git")
	if _, err := git.PlainInit(repoPath, true); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	st, err := store.NewBare(repoPath)
	if err != nil {
		t.Fatalf("NewBare: %v", err)
	}
	content := "---\ndocument_id: " + docID + "\ntitle: Test Doc\nversion: \"1.0\"\nstatus: draft\n---\nbody\n"
	abs := filepath.Join(st.Root(), "documents", "test", docID+".md")
	if _, err := st.CommitFile(abs, []byte(content), "Test", "test@example.com", "seed"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	s.stores.Store(orgID, st)
}

// seedReviewUser creates (or reuses) a user and makes them a member of orgID
// with the given role. CreateReviewTx and AddReviewAssignmentTx both resolve
// emails to user ids, so reviewers and the actor must exist and be members.
func seedReviewUser(t *testing.T, s *Server, orgID int, email, role string) {
	t.Helper()
	ctx := context.Background()
	u := &db.User{Email: email, Name: email, Active: true}
	if err := s.db.UpsertUser(ctx, u); err != nil {
		t.Fatalf("UpsertUser %s: %v", email, err)
	}
	if err := s.db.AddOrgMember(ctx, orgID, u.ID, role); err != nil {
		t.Fatalf("AddOrgMember %s: %v", email, err)
	}
}

func TestCreateReviewPersistsReviewers(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "review-create")
	seedRepoForOrg(t, s, orgID, "test-doc-1")
	seedReviewUser(t, s, orgID, "admin@review-create.test", "admin")
	seedReviewUser(t, s, orgID, "reviewer@review-create.test", "contributor")

	body := `{"document_id":"test-doc-1","reviewers":["reviewer@review-create.test"]}`
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", body, "admin", "admin@review-create.test")
	if err := s.handleCreateReview(c); err != nil {
		t.Fatalf("handleCreateReview: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var review db.Review
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	assignments, err := s.db.ListAssignmentsForReview(context.Background(), orgID, review.ID)
	if err != nil {
		t.Fatalf("ListAssignmentsForReview: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].Reviewer != "reviewer@review-create.test" {
		t.Errorf("expected reviewer reviewer@review-create.test, got %s", assignments[0].Reviewer)
	}
	if assignments[0].Status != "pending" {
		t.Errorf("expected status pending, got %s", assignments[0].Status)
	}
}

func TestCreateReviewAcceptsAssigneesAlias(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "review-alias")
	seedRepoForOrg(t, s, orgID, "test-doc-2")
	seedReviewUser(t, s, orgID, "admin@review-alias.test", "admin")
	seedReviewUser(t, s, orgID, "reviewer@review-alias.test", "contributor")

	body := `{"document_id":"test-doc-2","assignees":["reviewer@review-alias.test"]}`
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", body, "admin", "admin@review-alias.test")
	if err := s.handleCreateReview(c); err != nil {
		t.Fatalf("handleCreateReview: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var review db.Review
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	assignments, err := s.db.ListAssignmentsForReview(context.Background(), orgID, review.ID)
	if err != nil {
		t.Fatalf("ListAssignmentsForReview: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
}

func TestCreateReviewRejectsEmptyReviewers(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "review-empty")
	seedRepoForOrg(t, s, orgID, "test-doc-3")
	seedReviewUser(t, s, orgID, "admin@review-empty.test", "admin")

	body := `{"document_id":"test-doc-3"}`
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", body, "admin", "admin@review-empty.test")
	err := s.handleCreateReview(c)
	if err == nil {
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	} else {
		he, ok := err.(*echo.HTTPError)
		if !ok || he.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 echo.HTTPError, got %v", err)
		}
	}

	existing, lerr := s.db.GetOpenReviewForDocument(context.Background(), orgID, "test-doc-3")
	if lerr != nil {
		t.Fatalf("GetOpenReviewForDocument: %v", lerr)
	}
	if existing != nil {
		t.Fatalf("expected no review created, found review %d", existing.ID)
	}
}

func TestCreateReviewRejectsUnknownReviewer(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "review-unknown")
	seedRepoForOrg(t, s, orgID, "test-doc-4")
	seedReviewUser(t, s, orgID, "admin@review-unknown.test", "admin")

	body := `{"document_id":"test-doc-4","reviewers":["nobody@review-unknown.test"]}`
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", body, "admin", "admin@review-unknown.test")
	err := s.handleCreateReview(c)
	if err == nil {
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		return
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 echo.HTTPError, got %v", err)
	}
}

func TestCreateReviewConflictsWithOpenReview(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "review-conflict")
	seedRepoForOrg(t, s, orgID, "test-doc-5")
	seedReviewUser(t, s, orgID, "admin@review-conflict.test", "admin")
	seedReviewUser(t, s, orgID, "reviewer@review-conflict.test", "contributor")

	body := `{"document_id":"test-doc-5","reviewers":["reviewer@review-conflict.test"]}`

	c1, rec1 := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", body, "admin", "admin@review-conflict.test")
	if err := s.handleCreateReview(c1); err != nil {
		t.Fatalf("first handleCreateReview: %v", err)
	}
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected first call to return 201, got %d: %s", rec1.Code, rec1.Body.String())
	}
	var first db.Review
	if err := json.Unmarshal(rec1.Body.Bytes(), &first); err != nil {
		t.Fatalf("decoding first response: %v", err)
	}

	c2, rec2 := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", body, "admin", "admin@review-conflict.test")
	if err := s.handleCreateReview(c2); err != nil {
		t.Fatalf("second handleCreateReview: %v", err)
	}
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var conflict struct {
		ReviewID int    `json:"review_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &conflict); err != nil {
		t.Fatalf("decoding conflict response: %v", err)
	}
	if conflict.ReviewID != first.ID {
		t.Errorf("expected conflict review_id %d, got %d", first.ID, conflict.ReviewID)
	}

	existing, err := s.db.GetOpenReviewForDocument(context.Background(), orgID, "test-doc-5")
	if err != nil {
		t.Fatalf("GetOpenReviewForDocument: %v", err)
	}
	if existing == nil || existing.ID != first.ID {
		t.Fatalf("expected exactly the first review still open for the document")
	}
}

func TestCreateReviewUnknownDocument(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "review-nodoc")
	seedRepoForOrg(t, s, orgID, "test-doc-6")
	seedReviewUser(t, s, orgID, "admin@review-nodoc.test", "admin")
	seedReviewUser(t, s, orgID, "reviewer@review-nodoc.test", "contributor")

	body := `{"document_id":"does-not-exist","reviewers":["reviewer@review-nodoc.test"]}`
	c, _ := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", body, "admin", "admin@review-nodoc.test")
	err := s.handleCreateReview(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound {
		t.Fatalf("expected 404 echo.HTTPError, got %v", err)
	}
}
