package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Postgres-gated regression tests for the reply gap in POST /comments: a reply
// (parent_id set) that left review_id out skipped authorizeReviewComment
// entirely — no participant check, no merged/closed lock — and, because the
// client also chose document_id, could land on any document. The reply now
// inherits review_id and document_id from its parent before the gate runs.
//
// Reuses the harness from review_create_pg_test.go (testServer, newTestOrg,
// seedReviewUser, reviewCtxForPath). Skipped when ISMS_TEST_DATABASE_URL is unset.

type replyFixture struct {
	s      *Server
	orgID  int
	review *db.Review // open review on doc-a, participant: author + assigned
	parent *db.Comment
	suffix string
}

// postComment drives handleAddCommentDB as the given user and returns the
// handler error (if any) and the recorded response.
func (f *replyFixture) postComment(t *testing.T, role, email, body string) (error, int, string) {
	t.Helper()
	c, rec := reviewCtxForPath(f.orgID, http.MethodPost, "/api/v1/comments", body, role, email)
	err := f.s.handleAddCommentDB(c)
	return err, rec.Code, rec.Body.String()
}

func (f *replyFixture) addReview(t *testing.T, docID, status string) *db.Review {
	t.Helper()
	r := &db.Review{
		DocumentID:  docID,
		Title:       "T",
		Version:     "1.0",
		Status:      status,
		RequestedBy: "author@" + f.suffix + ".test",
	}
	if err := f.s.db.CreateReview(context.Background(), f.orgID, r); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	return r
}

func (f *replyFixture) addTopLevel(t *testing.T, reviewID *int, docID string) *db.Comment {
	t.Helper()
	cm := &db.Comment{ReviewID: reviewID, DocumentID: docID, Author: "author@" + f.suffix + ".test", Body: "parent"}
	if err := f.s.db.AddComment(context.Background(), f.orgID, cm); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	return cm
}

func newReplyFixture(t *testing.T, name string) *replyFixture {
	t.Helper()
	s := testServer(t)
	orgID := newTestOrg(t, s, name)
	f := &replyFixture{s: s, orgID: orgID, suffix: name}
	seedReviewUser(t, s, orgID, "author@"+name+".test", "contributor")
	seedReviewUser(t, s, orgID, "assigned@"+name+".test", "reader")
	seedReviewUser(t, s, orgID, "outsider-c@"+name+".test", "contributor")
	seedReviewUser(t, s, orgID, "outsider-r@"+name+".test", "reader")
	seedReviewUser(t, s, orgID, "mgr@"+name+".test", "manager")

	f.review = f.addReview(t, "doc-a", "open")
	a := &db.ReviewAssignment{ReviewID: f.review.ID, Reviewer: "assigned@" + name + ".test", Status: "pending"}
	if err := s.db.AddReviewAssignment(context.Background(), orgID, a); err != nil {
		t.Fatalf("AddReviewAssignment: %v", err)
	}
	f.parent = f.addTopLevel(t, &f.review.ID, "doc-a")
	return f
}

func replyBody(parentID int, extra string) string {
	b := `{"parent_id":` + itoa(parentID) + `,"body":"reply"`
	if extra != "" {
		b += "," + extra
	}
	return b + "}"
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func wantHTTPStatus(t *testing.T, err error, want int) *echo.HTTPError {
	t.Helper()
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError with %d, got %T: %v", want, err, err)
	}
	if he.Code != want {
		t.Fatalf("expected %d, got %d: %v", want, he.Code, he.Message)
	}
	return he
}

// A non-participant contributor must not slip a reply into a review thread by
// leaving review_id out.
func TestReplyWithoutReviewIDRefusesNonParticipantContributor(t *testing.T) {
	f := newReplyFixture(t, "reply-nonpart-c")
	err, _, _ := f.postComment(t, "contributor", "outsider-c@reply-nonpart-c.test", replyBody(f.parent.ID, ""))
	wantHTTPStatus(t, err, http.StatusForbidden)
}

func TestReplyWithoutReviewIDRefusesNonParticipantReader(t *testing.T) {
	f := newReplyFixture(t, "reply-nonpart-r")
	err, _, _ := f.postComment(t, "reader", "outsider-r@reply-nonpart-r.test", replyBody(f.parent.ID, ""))
	wantHTTPStatus(t, err, http.StatusForbidden)

	// Nothing was stored under the review.
	list, lerr := f.s.db.CommentsForDocument(context.Background(), f.orgID, "doc-a", f.review.ID)
	if lerr != nil {
		t.Fatalf("CommentsForDocument: %v", lerr)
	}
	if len(list) != 1 {
		t.Fatalf("expected only the parent comment, got %d rows", len(list))
	}
}

// A participant's reply inherits the parent's review and document, whatever
// the client claims for document_id.
func TestReplyInheritsParentReviewAndDocument(t *testing.T) {
	f := newReplyFixture(t, "reply-inherit")
	err, code, body := f.postComment(t, "reader", "assigned@reply-inherit.test",
		replyBody(f.parent.ID, `"document_id":"doc-elsewhere"`))
	if err != nil {
		t.Fatalf("handleAddCommentDB: %v", err)
	}
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", code, body)
	}
	var got db.Comment
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	stored, serr := f.s.db.GetComment(context.Background(), f.orgID, got.ID)
	if serr != nil {
		t.Fatalf("GetComment: %v", serr)
	}
	if stored.ReviewID == nil || *stored.ReviewID != f.review.ID {
		t.Fatalf("stored review_id = %v, want %d", stored.ReviewID, f.review.ID)
	}
	if stored.DocumentID != "doc-a" {
		t.Fatalf("stored document_id = %q, want doc-a", stored.DocumentID)
	}
	if stored.ParentID == nil || *stored.ParentID != f.parent.ID {
		t.Fatalf("stored parent_id = %v, want %d", stored.ParentID, f.parent.ID)
	}
}

// Echoing the parent's own review_id (what existing UI clients send) stays valid.
func TestReplyEchoingParentReviewIDIsAccepted(t *testing.T) {
	f := newReplyFixture(t, "reply-echo")
	err, code, body := f.postComment(t, "reader", "assigned@reply-echo.test",
		replyBody(f.parent.ID, `"review_id":`+itoa(f.review.ID)))
	if err != nil || code != http.StatusCreated {
		t.Fatalf("expected 201, got err=%v code=%d: %s", err, code, body)
	}
}

// Naming a different review is a client error — it must not move the reply to
// another thread, and a manager (who passes the participant rule everywhere)
// is no exception.
func TestReplyWithMismatchedReviewIDIsRejected(t *testing.T) {
	f := newReplyFixture(t, "reply-mismatch")
	other := f.addReview(t, "doc-c", "open")

	err, _, _ := f.postComment(t, "manager", "mgr@reply-mismatch.test",
		replyBody(f.parent.ID, `"review_id":`+itoa(other.ID)+`,"document_id":"doc-c"`))
	wantHTTPStatus(t, err, http.StatusBadRequest)

	// A review_id on a reply whose parent has none is a mismatch too.
	docParent := f.addTopLevel(t, nil, "doc-a")
	err, _, _ = f.postComment(t, "manager", "mgr@reply-mismatch.test",
		replyBody(docParent.ID, `"review_id":`+itoa(f.review.ID)))
	wantHTTPStatus(t, err, http.StatusBadRequest)
}

// The closed-record lock applies to replies that omit review_id.
func TestReplyOnMergedOrClosedReviewIsRejected(t *testing.T) {
	for _, status := range []string{"merged", "closed"} {
		t.Run(status, func(t *testing.T) {
			f := newReplyFixture(t, "reply-"+status)
			if err := f.s.db.UpdateReviewStatus(context.Background(), f.orgID, f.review.ID, status); err != nil {
				t.Fatalf("UpdateReviewStatus: %v", err)
			}
			err, _, _ := f.postComment(t, "reader", "assigned@reply-"+status+".test", replyBody(f.parent.ID, ""))
			he := wantHTTPStatus(t, err, http.StatusBadRequest)
			if eb, ok := he.Message.(errorBody); !ok || eb.Code != CodeReviewWrongStatus {
				t.Fatalf("expected %s, got %#v", CodeReviewWrongStatus, he.Message)
			}
		})
	}
}

func jsonString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// A missing parent is a 404, not the raw foreign-key error as a 500.
func TestReplyToMissingParentIs404WithoutDBText(t *testing.T) {
	f := newReplyFixture(t, "reply-missing")
	err, _, _ := f.postComment(t, "contributor", "author@reply-missing.test", replyBody(999999, ""))
	he := wantHTTPStatus(t, err, http.StatusNotFound)
	msg := strings.ToLower(jsonString(he.Message))
	for _, leak := range []string{"foreign key", "violates", "constraint", "sqlstate", "comments_"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("response leaks database text (%q): %s", leak, msg)
		}
	}
}

// Replies to document-level comments stay open to every role; the reply stays
// document-level (review_id nil) and lands on the parent's document.
func TestReplyToDocumentCommentStaysOpenToReader(t *testing.T) {
	f := newReplyFixture(t, "reply-doc")
	docParent := f.addTopLevel(t, nil, "doc-b")
	err, code, body := f.postComment(t, "reader", "outsider-r@reply-doc.test",
		replyBody(docParent.ID, `"document_id":"doc-elsewhere"`))
	if err != nil || code != http.StatusCreated {
		t.Fatalf("expected 201, got err=%v code=%d: %s", err, code, body)
	}
	var got db.Comment
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	stored, serr := f.s.db.GetComment(context.Background(), f.orgID, got.ID)
	if serr != nil {
		t.Fatalf("GetComment: %v", serr)
	}
	if stored.ReviewID != nil {
		t.Fatalf("stored review_id = %d, want nil", *stored.ReviewID)
	}
	if stored.DocumentID != "doc-b" {
		t.Fatalf("stored document_id = %q, want doc-b", stored.DocumentID)
	}
}
