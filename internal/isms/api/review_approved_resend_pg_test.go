package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/store"
)

// Postgres-gated tests for #479: a review that is approved but not yet
// published still holds the one-active-review-per-document slot
// (idx_reviews_active_doc), so every "is there an active review?" check has to
// see it. Requires a migrated Postgres — see id_resolution_pg_test.go.

type approvedFixture struct {
	t        *testing.T
	s        *Server
	st       *store.Store
	orgID    int
	docID    string
	reviewID int
	admin    string
	rev1     string
	rev2     string
}

// newApprovedFixture seeds a document, sends it to reviewer 1 and has reviewer 1
// approve it, leaving an approved, unpublished review.
func newApprovedFixture(t *testing.T, slug string) *approvedFixture {
	t.Helper()
	f := newOpenFixture(t, slug)
	f.approve(f.rev1)
	got, err := f.s.db.GetReview(context.Background(), f.orgID, f.reviewID)
	if err != nil || got.Status != "approved" {
		t.Fatalf("fixture review not approved: %v %+v", err, got)
	}
	return f
}

// newOpenFixture is newApprovedFixture without the approval: one open review
// sent to reviewer 1.
func newOpenFixture(t *testing.T, slug string) *approvedFixture {
	t.Helper()
	s := testServer(t)
	orgID := newTestOrg(t, s, slug)
	docID := "doc-" + slug
	seedRepoForOrg(t, s, orgID, docID)
	f := &approvedFixture{
		t: t, s: s, orgID: orgID, docID: docID,
		admin: "admin@" + slug + ".test",
		rev1:  "rev1@" + slug + ".test",
		rev2:  "rev2@" + slug + ".test",
	}
	seedReviewUser(t, s, orgID, f.admin, "admin")
	seedReviewUser(t, s, orgID, f.rev1, "contributor")
	seedReviewUser(t, s, orgID, f.rev2, "contributor")
	st, err := s.storeForOrg(context.Background(), orgID)
	if err != nil {
		t.Fatalf("storeForOrg: %v", err)
	}
	f.st = st

	rec, herr := f.send(f.rev1)
	if herr != nil || rec.Code != http.StatusCreated {
		t.Fatalf("initial send: err=%v code=%d body=%s", herr, rec.Code, rec.Body.String())
	}
	var resp struct {
		ReviewID int `json:"review_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.ReviewID == 0 {
		t.Fatalf("decoding initial send: %v %s", err, rec.Body.String())
	}
	f.reviewID = resp.ReviewID
	return f
}

func (f *approvedFixture) send(reviewers ...string) (*httptest.ResponseRecorder, error) {
	body, _ := json.Marshal(map[string]any{"reviewers": reviewers})
	c, rec := reviewCtxForPath(f.orgID, http.MethodPost, "/api/v1/documents/"+f.docID+"/reviews", string(body), "admin", f.admin)
	c.SetParamNames("docId")
	c.SetParamValues(f.docID)
	err := f.s.handleReviewSend(c)
	return rec, err
}

func (f *approvedFixture) decide(who, decision string) {
	f.t.Helper()
	id := strconv.Itoa(f.reviewID)
	c, rec := reviewCtxForPath(f.orgID, http.MethodPost, "/api/v1/reviews/"+id+"/approve", `{"decision":"`+decision+`","comment":"ok"}`, "contributor", who)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := f.s.handleReviewApprove(c); err != nil || rec.Code != http.StatusOK {
		f.t.Fatalf("decision %s by %s: err=%v code=%d body=%s", decision, who, err, rec.Code, rec.Body.String())
	}
}

func (f *approvedFixture) approve(who string) { f.t.Helper(); f.decide(who, "approved") }

func (f *approvedFixture) head() string {
	f.t.Helper()
	h, err := f.st.HeadHash()
	if err != nil {
		f.t.Fatalf("HeadHash: %v", err)
	}
	return h
}

func (f *approvedFixture) reviewCount() int {
	f.t.Helper()
	rs, err := f.s.db.ListReviews(context.Background(), f.orgID, "", 100)
	if err != nil {
		f.t.Fatalf("ListReviews: %v", err)
	}
	return len(rs)
}

// outcome flattens a handler result (returned echo error or written response)
// into a status code and the machine code, when there is one.
func outcome(t *testing.T, rec *httptest.ResponseRecorder, err error) (int, string) {
	t.Helper()
	if err != nil {
		he, ok := err.(*echo.HTTPError)
		if !ok {
			t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
		}
		if eb, ok := he.Message.(errorBody); ok {
			return he.Code, eb.Code
		}
		return he.Code, ""
	}
	return rec.Code, ""
}

// a. approved + unchanged: Send refuses with a coded 409 and changes nothing.
func TestReviewApprovedSendUnchangedIsCoded409(t *testing.T) {
	f := newApprovedFixture(t, "apv-send")
	headBefore := f.head()

	rec, err := f.send(f.rev2)
	status, code := outcome(t, rec, err)
	if status != http.StatusConflict || code != CodeReviewAwaitingPublication {
		t.Fatalf("expected 409 %q, got %d %q (err=%v body=%s)", CodeReviewAwaitingPublication, status, code, err, rec.Body.String())
	}
	if n := f.reviewCount(); n != 1 {
		t.Errorf("expected exactly 1 review row, got %d", n)
	}
	if h := f.head(); h != headBefore {
		t.Errorf("a refused send must not commit: HEAD moved %s -> %s", headBefore, h)
	}
	r, _ := f.s.db.GetReview(context.Background(), f.orgID, f.reviewID)
	if r.Status != "approved" {
		t.Errorf("review should stay approved, got %q", r.Status)
	}
	as, _ := f.s.db.ListAssignmentsForReview(context.Background(), f.orgID, f.reviewID)
	if len(as) != 1 || as[0].Reviewer != f.rev1 || as[0].Status != "approved" {
		t.Errorf("assignments must be untouched, got %+v", as)
	}
}

// b. approved + unchanged: POST /reviews keeps its hand-rolled 409 body.
func TestReviewApprovedCreateReviewIs409(t *testing.T) {
	f := newApprovedFixture(t, "apv-create")
	headBefore := f.head()

	body := `{"document_id":"` + f.docID + `","reviewers":["` + f.rev2 + `"]}`
	c, rec := reviewCtxForPath(f.orgID, http.MethodPost, "/api/v1/reviews", body, "admin", f.admin)
	err := f.s.handleCreateReview(c)
	status, _ := outcome(t, rec, err)
	if status != http.StatusConflict {
		t.Fatalf("expected 409, got %d (err=%v body=%s)", status, err, rec.Body.String())
	}
	var got struct {
		Error    string `json:"error"`
		ReviewID int    `json:"review_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ReviewID != f.reviewID || got.Status != "approved" {
		t.Errorf("expected body with review_id %d status approved, got %s", f.reviewID, rec.Body.String())
	}
	if h := f.head(); h != headBefore {
		t.Errorf("a refused create must not commit: HEAD moved %s -> %s", headBefore, h)
	}
}

// c. the open-review probe sees the approved review.
func TestReviewApprovedOpenProbeFindsIt(t *testing.T) {
	f := newApprovedFixture(t, "apv-probe")
	c, rec := reviewCtxForPath(f.orgID, http.MethodGet, "/api/v1/documents/"+f.docID+"/reviews/open", "", "admin", f.admin)
	c.SetParamNames("docId")
	c.SetParamValues(f.docID)
	err := f.s.handleGetOpenReview(c)
	status, _ := outcome(t, rec, err)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (err=%v)", status, err)
	}
	var got struct {
		ReviewID int    `json:"review_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ReviewID != f.reviewID || got.Status != "approved" {
		t.Errorf("expected review %d approved, got %s", f.reviewID, rec.Body.String())
	}
}

// d. both document responses carry the active review for an approved review.
func TestReviewApprovedDocumentResponsesCarryActiveReview(t *testing.T) {
	f := newApprovedFixture(t, "apv-docs")
	for name, h := range map[string]func(echo.Context) error{
		"handleGetDocument":     f.s.handleGetDocument,
		"handleGetDocumentBody": f.s.handleGetDocumentBody,
	} {
		c, rec := reviewCtxForPath(f.orgID, http.MethodGet, "/api/v1/documents/"+f.docID, "", "admin", f.admin)
		// handleGetDocument resolves by folder + id, the body handler by docId.
		c.SetParamNames("docId", "folder", "id")
		c.SetParamValues(f.docID, "test", f.docID)
		if err := h(c); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: decoding: %v", name, err)
		}
		if id, _ := got["active_review_id"].(float64); int(id) != f.reviewID {
			t.Errorf("%s: active_review_id = %v, want %d", name, got["active_review_id"], f.reviewID)
		}
		if got["active_review_status"] != "approved" {
			t.Errorf("%s: active_review_status = %v, want approved", name, got["active_review_status"])
		}
	}
}

// e. controls: the paths that already worked must keep working.
func TestReviewSendControlsOpenAndChangesRequested(t *testing.T) {
	t.Run("open adds the reviewer", func(t *testing.T) {
		f := newOpenFixture(t, "ctl-open")
		rec, err := f.send(f.rev2)
		if status, _ := outcome(t, rec, err); status != http.StatusOK {
			t.Fatalf("expected 200, got %d (err=%v body=%s)", status, err, rec.Body.String())
		}
		as, _ := f.s.db.ListAssignmentsForReview(context.Background(), f.orgID, f.reviewID)
		if len(as) != 2 {
			t.Errorf("expected 2 assignments, got %+v", as)
		}
	})
	t.Run("changes_requested resubmits", func(t *testing.T) {
		f := newOpenFixture(t, "ctl-cr")
		f.decide(f.rev1, "changes_requested")
		rec, err := f.send(f.rev2)
		if status, _ := outcome(t, rec, err); status != http.StatusOK {
			t.Fatalf("expected 200, got %d (err=%v body=%s)", status, err, rec.Body.String())
		}
		r, _ := f.s.db.GetReview(context.Background(), f.orgID, f.reviewID)
		if r.Status != "open" || r.Round != 2 {
			t.Errorf("expected open round 2, got %q round %d", r.Status, r.Round)
		}
	})
}

// Race residual: when the lookups pass but a concurrent send takes the slot,
// the insert hits idx_reviews_active_doc. Calling the shared creator directly
// reproduces that (the lookup is bypassed); it must surface as a coded 409.
func TestReviewCreateSlotTakenIsCoded409(t *testing.T) {
	f := newApprovedFixture(t, "apv-race")
	_, err := f.s.createReviewWithAssignments(context.Background(), f.st, f.orgID, f.admin, f.docID, "Test Doc", "1.0", "", "", []string{f.rev2})
	if err == nil {
		t.Fatal("expected an error")
	}
	status, code := outcome(t, nil, reviewCreateHTTPError(err))
	if status != http.StatusConflict || code != CodeReviewAlreadyActive {
		t.Fatalf("expected 409 %q, got %d %q (%v)", CodeReviewAlreadyActive, status, code, err)
	}
}
