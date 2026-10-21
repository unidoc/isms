package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
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

// --- Part B: approved, then edited after sending -> a new round ---

func (f *approvedFixture) putBody(body string) {
	f.t.Helper()
	payload, _ := json.Marshal(map[string]string{"content": body})
	c, rec := reviewCtxForPath(f.orgID, http.MethodPut, "/api/v1/documents/"+f.docID+"/content", string(payload), "admin", f.admin)
	c.SetParamNames("docId")
	c.SetParamValues(f.docID)
	if err := f.s.handleUpdateDocumentContent(c); err != nil || rec.Code != http.StatusOK {
		f.t.Fatalf("content edit: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
	}
}

func (f *approvedFixture) putMetadata(field, value string) {
	f.t.Helper()
	payload, _ := json.Marshal(map[string]any{"fields": map[string]string{field: value}})
	c, rec := reviewCtxForPath(f.orgID, http.MethodPut, "/api/v1/documents/"+f.docID+"/metadata", string(payload), "admin", f.admin)
	c.SetParamNames("docId")
	c.SetParamValues(f.docID)
	if err := f.s.handleUpdateDocumentMetadata(c); err != nil || rec.Code != http.StatusOK {
		f.t.Fatalf("metadata edit: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
	}
}

func (f *approvedFixture) merge() (int, string) {
	f.t.Helper()
	id := strconv.Itoa(f.reviewID)
	c, rec := reviewCtxForPath(f.orgID, http.MethodPost, "/api/v1/reviews/"+id+"/merge", "", "admin", f.admin)
	c.SetParamNames("id")
	c.SetParamValues(id)
	err := f.s.handleMergeReview(c)
	status, _ := outcome(f.t, rec, err)
	return status, rec.Body.String() + "|" + errString(err)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// expectNewRound asserts the 200 resubmit outcome: same review, round 2, open,
// every assignment pending and reviewer 2 added, comments outdated, sent_head
// at the current HEAD.
func (f *approvedFixture) expectNewRound(rec *httptest.ResponseRecorder, err error) {
	f.t.Helper()
	if status, code := outcome(f.t, rec, err); status != http.StatusOK {
		f.t.Fatalf("expected 200, got %d %q (err=%v body=%s)", status, code, err, rec.Body.String())
	}
	var resp struct {
		ReviewID int `json:"review_id"`
		Round    int `json:"round"`
	}
	if jerr := json.Unmarshal(rec.Body.Bytes(), &resp); jerr != nil || resp.ReviewID != f.reviewID || resp.Round != 2 {
		f.t.Errorf("expected review %d round 2, got %s", f.reviewID, rec.Body.String())
	}
	ctx := context.Background()
	r, _ := f.s.db.GetReview(ctx, f.orgID, f.reviewID)
	if r.Status != "open" || r.Round != 2 {
		f.t.Errorf("expected open round 2, got %q round %d", r.Status, r.Round)
	}
	if r.SentHead != f.head() {
		f.t.Errorf("sent_head %s should be the new HEAD %s", r.SentHead, f.head())
	}
	as, _ := f.s.db.ListAssignmentsForReview(ctx, f.orgID, f.reviewID)
	got := map[string]string{}
	for _, a := range as {
		got[a.Reviewer] = a.Status
	}
	if len(got) != 2 || got[f.rev1] != "pending" || got[f.rev2] != "pending" {
		f.t.Errorf("expected both reviewers pending, got %+v", got)
	}
	if n := f.reviewCount(); n != 1 {
		f.t.Errorf("expected still exactly 1 review row, got %d", n)
	}
}

// f. body edited after approval: Send starts a new round, and the document can
// then be approved again and published (the "unstuck" proof).
func TestReviewApprovedEditedBodyResendsAsNewRound(t *testing.T) {
	f := newApprovedFixture(t, "apv-body")
	ctx := context.Background()
	if err := f.s.db.AddComment(ctx, f.orgID, &db.Comment{ReviewID: &f.reviewID, DocumentID: f.docID, Author: f.rev1, Body: "round one note", Status: "open"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	f.putBody("edited after approval\n")
	rec, err := f.send(f.rev2)
	f.expectNewRound(rec, err)

	cs, _ := f.s.db.CommentsForReview(ctx, f.orgID, f.reviewID)
	if len(cs) != 1 || !cs[0].IsOutdated {
		t.Errorf("earlier comments should be outdated, got %+v", cs)
	}

	f.approve(f.rev1)
	f.approve(f.rev2)
	if status, body := f.merge(); status != http.StatusOK {
		t.Fatalf("publish after the new round: expected 200, got %d %s", status, body)
	}
	r, _ := f.s.db.GetReview(ctx, f.orgID, f.reviewID)
	if r.Status != "merged" {
		t.Errorf("expected merged, got %q", r.Status)
	}
}

// g. a frontmatter-only edit also blocks publish (whole-file compare), so it
// must also allow the new round, or the document stays stuck.
func TestReviewApprovedEditedFrontmatterResendsAsNewRound(t *testing.T) {
	f := newApprovedFixture(t, "apv-fm")
	f.putMetadata("classification", "internal")

	// Prove the premise on this base: publish refuses a frontmatter-only edit.
	if status, body := f.merge(); status != http.StatusConflict {
		t.Fatalf("premise: expected publish to refuse with 409, got %d %s", status, body)
	}

	rec, err := f.send(f.rev2)
	f.expectNewRound(rec, err)
}

// h. approved + unchanged still refuses, even after unrelated churn elsewhere
// in the repo (another document is not this file).
func TestReviewApprovedUnchangedStillRefusesAfterOtherCommits(t *testing.T) {
	f := newApprovedFixture(t, "apv-other")
	if _, err := f.st.CommitFile(f.st.Root()+"/documents/test/other.md", []byte("---\ndocument_id: other\ntitle: O\nstatus: draft\n---\nx\n"), "T", "t@example.com", "other doc"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	rec, err := f.send(f.rev2)
	if status, code := outcome(t, rec, err); status != http.StatusConflict || code != CodeReviewAwaitingPublication {
		t.Fatalf("expected 409 %q, got %d %q", CodeReviewAwaitingPublication, status, code)
	}
}

// --- Part C: needs-review for an approved, unpublished review ---

// needsReviewEntry runs handleNeedsReview and returns the fixture document's
// entry, or nil when it is not listed.
func (f *approvedFixture) needsReviewEntry() map[string]any {
	f.t.Helper()
	c, rec := reviewCtxForPath(f.orgID, http.MethodGet, "/api/v1/documents/needs-review", "", "admin", f.admin)
	if err := f.s.handleNeedsReview(c); err != nil || rec.Code != http.StatusOK {
		f.t.Fatalf("handleNeedsReview: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		f.t.Fatalf("decoding needs-review: %v", err)
	}
	for _, e := range resp.Data {
		if e["document_id"] == f.docID {
			return e
		}
	}
	return nil
}

// sendNew sends the document for a fresh review (no active review exists) and
// makes it the fixture's current review.
func (f *approvedFixture) sendNew(reviewers ...string) {
	f.t.Helper()
	rec, err := f.send(reviewers...)
	if err != nil || rec.Code != http.StatusCreated {
		f.t.Fatalf("send: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
	}
	var resp struct {
		ReviewID int `json:"review_id"`
	}
	if jerr := json.Unmarshal(rec.Body.Bytes(), &resp); jerr != nil || resp.ReviewID == 0 {
		f.t.Fatalf("decoding send: %v %s", jerr, rec.Body.String())
	}
	f.reviewID = resp.ReviewID
}

func (f *approvedFixture) mustMerge() {
	f.t.Helper()
	if status, body := f.merge(); status != http.StatusOK {
		f.t.Fatalf("merge: %d %s", status, body)
	}
}

func TestNeedsReviewApprovedUnpublished(t *testing.T) {
	t.Run("first review approved and unchanged is not listed", func(t *testing.T) {
		f := newApprovedFixture(t, "nr-first")
		if e := f.needsReviewEntry(); e != nil {
			t.Fatalf("an approved, unchanged document must not be listed, got %+v", e)
		}
	})
	t.Run("later review approved and unchanged since sending is not listed", func(t *testing.T) {
		f := newOpenFixture(t, "nr-later")
		f.approve(f.rev1)
		f.mustMerge()
		f.putBody("second round body\n")
		f.sendNew(f.rev1)
		f.approve(f.rev1)
		if e := f.needsReviewEntry(); e != nil {
			t.Fatalf("approved and unchanged since the second send must not be listed, got %+v", e)
		}
	})
	t.Run("body edited after approval is listed", func(t *testing.T) {
		f := newApprovedFixture(t, "nr-body")
		f.putBody("edited after approval\n")
		e := f.needsReviewEntry()
		if e == nil {
			t.Fatal("a body edit after approval must be listed")
		}
		if e["never_approved"] != false {
			t.Errorf("never_approved = %v, want false", e["never_approved"])
		}
	})
	t.Run("frontmatter-only edit after approval is not listed", func(t *testing.T) {
		f := newApprovedFixture(t, "nr-fm")
		f.putMetadata("classification", "internal")
		if e := f.needsReviewEntry(); e != nil {
			t.Fatalf("a frontmatter-only edit must not be listed, got %+v", e)
		}
	})
}

// Controls: behaviour that must not change.
func TestNeedsReviewControls(t *testing.T) {
	t.Run("never reviewed is listed as never approved", func(t *testing.T) {
		s := testServer(t)
		orgID := newTestOrg(t, s, "nr-never")
		seedRepoForOrg(t, s, orgID, "doc-nr-never")
		seedReviewUser(t, s, orgID, "admin@nr-never.test", "admin")
		st, _ := s.storeForOrg(context.Background(), orgID)
		f := &approvedFixture{t: t, s: s, st: st, orgID: orgID, docID: "doc-nr-never", admin: "admin@nr-never.test"}
		e := f.needsReviewEntry()
		if e == nil || e["never_approved"] != true {
			t.Fatalf("expected listed with never_approved true, got %+v", e)
		}
	})
	t.Run("merged with a later body edit is listed", func(t *testing.T) {
		f := newOpenFixture(t, "nr-merged-edit")
		f.approve(f.rev1)
		f.mustMerge()
		f.putBody("edited after publish\n")
		e := f.needsReviewEntry()
		if e == nil || e["never_approved"] != false {
			t.Fatalf("expected listed with never_approved false, got %+v", e)
		}
	})
	t.Run("merged with no later edit is not listed", func(t *testing.T) {
		f := newOpenFixture(t, "nr-merged-clean")
		f.approve(f.rev1)
		f.mustMerge()
		if e := f.needsReviewEntry(); e != nil {
			t.Fatalf("expected not listed, got %+v", e)
		}
	})
}

// --- Part B2: an approved review carrying reviewer edits is not resent ---

// reviewerEdit saves content to the review branch as the given reviewer, the
// way PUT /reviews/:id/content does.
func (f *approvedFixture) reviewerEdit(who, content string) {
	f.t.Helper()
	id := strconv.Itoa(f.reviewID)
	payload, _ := json.Marshal(map[string]string{"content": content})
	c, rec := reviewCtxForPath(f.orgID, http.MethodPut, "/api/v1/reviews/"+id+"/content", string(payload), "contributor", who)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := f.s.handleUpdateReviewContent(c); err != nil || rec.Code != http.StatusOK {
		f.t.Fatalf("reviewer edit: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
	}
}

func (f *approvedFixture) publishedFile() string {
	f.t.Helper()
	raw, err := f.st.ReadFile(f.st.Root() + "/documents/test/" + f.docID + ".md")
	if err != nil {
		f.t.Fatalf("ReadFile: %v", err)
	}
	return string(raw)
}

func TestReviewApprovedWithReviewerEditsAndMainEditIsRefused(t *testing.T) {
	f := newOpenFixture(t, "rve-refuse")
	f.reviewerEdit(f.rev1, "reviewer text\n")
	f.approve(f.rev1)
	f.putBody("author text\n")
	headBefore := f.head()

	rec, err := f.send(f.rev2)
	status, code := outcome(t, rec, err)
	if status != http.StatusConflict || code != CodeReviewHasReviewerEdits {
		t.Fatalf("expected 409 %q, got %d %q (err=%v body=%s)", CodeReviewHasReviewerEdits, status, code, err, rec.Body.String())
	}
	ctx := context.Background()
	r, _ := f.s.db.GetReview(ctx, f.orgID, f.reviewID)
	if r.Status != "approved" || r.Round != 1 {
		t.Errorf("review should stay approved round 1, got %q round %d", r.Status, r.Round)
	}
	as, _ := f.s.db.ListAssignmentsForReview(ctx, f.orgID, f.reviewID)
	if len(as) != 1 || as[0].Reviewer != f.rev1 || as[0].Status != "approved" {
		t.Errorf("assignments must be untouched, got %+v", as)
	}
	if h := f.head(); h != headBefore {
		t.Errorf("a refused send must not commit: HEAD moved %s -> %s", headBefore, h)
	}
}

func TestReviewApprovedWithReviewerEditsUnchangedPublishesTheEdit(t *testing.T) {
	f := newOpenFixture(t, "rve-publish")
	f.reviewerEdit(f.rev1, "reviewer text\n")
	f.approve(f.rev1)

	rec, err := f.send(f.rev2)
	if status, code := outcome(t, rec, err); status != http.StatusConflict || code != CodeReviewAwaitingPublication {
		t.Fatalf("expected 409 %q, got %d %q", CodeReviewAwaitingPublication, status, code)
	}
	f.mustMerge()
	if got := f.publishedFile(); !strings.Contains(got, "reviewer text") {
		t.Errorf("published file should carry the reviewer edit, got %q", got)
	}
}

func TestReviewApprovedNoReviewerEditMainEditedResends(t *testing.T) {
	f := newApprovedFixture(t, "rve-none")
	f.putBody("author text\n")
	rec, err := f.send(f.rev2)
	f.expectNewRound(rec, err)
}

// A reviewer "edit" that saves the content unchanged leaves the branch file
// equal to the base: no reviewer edit, so a main edit still starts a new round.
func TestReviewApprovedIdenticalReviewerSaveDoesNotBlockResend(t *testing.T) {
	f := newOpenFixture(t, "rve-same")
	f.reviewerEdit(f.rev1, "body\n")
	f.approve(f.rev1)
	f.putBody("author text\n")
	rec, err := f.send(f.rev2)
	f.expectNewRound(rec, err)
}

// --- Part D: document responses say which approved state applies ---

// docResponses returns both document responses for the fixture document.
func (f *approvedFixture) docResponses() map[string]map[string]any {
	f.t.Helper()
	out := map[string]map[string]any{}
	for name, h := range map[string]func(echo.Context) error{
		"handleGetDocument":     f.s.handleGetDocument,
		"handleGetDocumentBody": f.s.handleGetDocumentBody,
	} {
		c, rec := reviewCtxForPath(f.orgID, http.MethodGet, "/api/v1/documents/"+f.docID, "", "admin", f.admin)
		c.SetParamNames("docId", "folder", "id")
		c.SetParamValues(f.docID, "test", f.docID)
		if err := h(c); err != nil {
			f.t.Fatalf("%s: %v", name, err)
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			f.t.Fatalf("%s: decoding: %v", name, err)
		}
		out[name] = got
	}
	return out
}

func TestDocumentResponsesCarryApprovedReviewState(t *testing.T) {
	cases := []struct {
		name           string
		setup          func(f *approvedFixture)
		changed, edits bool
	}{
		{"unchanged", func(f *approvedFixture) {}, false, false},
		{"changed without reviewer edits", func(f *approvedFixture) { f.putBody("author text\n") }, true, false},
		{"changed with reviewer edits", func(f *approvedFixture) { f.putBody("author text\n") }, true, true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slug := "dstate-" + strconv.Itoa(i)
			var f *approvedFixture
			if tc.edits {
				f = newOpenFixture(t, slug)
				f.reviewerEdit(f.rev1, "reviewer text\n")
				f.approve(f.rev1)
			} else {
				f = newApprovedFixture(t, slug)
			}
			tc.setup(f)
			for name, got := range f.docResponses() {
				if got["active_review_status"] != "approved" {
					t.Fatalf("%s: active_review_status = %v", name, got["active_review_status"])
				}
				if got["active_review_changed_since_sent"] != tc.changed {
					t.Errorf("%s: active_review_changed_since_sent = %v, want %v", name, got["active_review_changed_since_sent"], tc.changed)
				}
				if got["active_review_has_reviewer_edits"] != tc.edits {
					t.Errorf("%s: active_review_has_reviewer_edits = %v, want %v", name, got["active_review_has_reviewer_edits"], tc.edits)
				}
			}
		})
	}
}

// Control: the pair is only added for an approved review.
func TestDocumentResponsesOmitApprovedStateForOpenReview(t *testing.T) {
	f := newOpenFixture(t, "dstate-open")
	for name, got := range f.docResponses() {
		if got["active_review_status"] != "open" {
			t.Fatalf("%s: active_review_status = %v", name, got["active_review_status"])
		}
		for _, k := range []string{"active_review_changed_since_sent", "active_review_has_reviewer_edits"} {
			if _, ok := got[k]; ok {
				t.Errorf("%s: %s must be absent for an open review", name, k)
			}
		}
	}
}
