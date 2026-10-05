package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"isms.sh/internal/isms/db"
)

// Regression for #194: a #RISK-1 mention in a comment was text only — no
// entity_references row, so the mentioned record's Links tab never showed it.
// A mention now saves a link with origin 'comment', and deleting the comment
// removes it unless another comment or a manual link still holds it.
//
// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays
// green without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...

const mentionActor = "admin@comment-mentions.test"

// postEntityComment calls POST /entity-comments and returns the new comment's id.
func postMentionComment(t *testing.T, s *Server, orgID int, entityType, entityID, body string) int64 {
	t.Helper()
	req, err := json.Marshal(map[string]string{"entity_type": entityType, "entity_id": entityID, "body": body})
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/entity-comments", string(req), "contributor", mentionActor)
	if err := s.handleCreateEntityComment(c); err != nil {
		t.Fatalf("handleCreateEntityComment(%q): %v", body, err)
	}
	var out db.EntityComment
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding response body %q: %v", rec.Body.String(), err)
	}
	return out.ID
}

func deleteEntityComment(t *testing.T, s *Server, orgID int, id int64) {
	t.Helper()
	c, _ := reviewCtxForPath(orgID, http.MethodDelete, "/api/v1/entity-comments/"+strconv.FormatInt(id, 10), "", "admin", mentionActor)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(id, 10))
	if err := s.handleDeleteEntityComment(c); err != nil {
		t.Fatalf("handleDeleteEntityComment(%d): %v", id, err)
	}
}

// pairOrigins returns the origin of each stored row between a and b, in both
// directions; an empty slice means the two are not linked.
func pairOrigins(t *testing.T, s *Server, orgID int, aType, aID, bType, bID string) []string {
	t.Helper()
	refs, err := s.db.ListAllReferencesForEntity(context.Background(), orgID, aType, aID)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity(%s %s): %v", aType, aID, err)
	}
	var origins []string
	for _, r := range refs {
		if (r.TargetType == bType && r.TargetID == bID) || (r.SourceType == bType && r.SourceID == bID) {
			origins = append(origins, r.Origin)
		}
	}
	return origins
}

func TestEntityCommentMentionSavesLink(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "comment-mentions-entity")
	seedReviewUser(t, s, orgID, mentionActor, "admin")

	subject := newRefTestRisk(t, s, orgID, "risk the comment is on")
	mentioned := newRefTestRisk(t, s, orgID, "risk the comment mentions")

	// A real mention, an unknown id, and the subject itself: only the first links.
	body := "see #" + mentioned.Identifier + ", #RISK-99999 and #" + subject.Identifier
	postMentionComment(t, s, orgID, "risk", subject.Identifier, body)

	if got := pairOrigins(t, s, orgID, "risk", subject.Identifier, "risk", mentioned.Identifier); len(got) != 2 || got[0] != "comment" || got[1] != "comment" {
		t.Fatalf("link %s <-> %s: origins %v, want [comment comment]", subject.Identifier, mentioned.Identifier, got)
	}
	refs, err := s.db.ListAllReferencesForEntity(context.Background(), orgID, "risk", subject.Identifier)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity: %v", err)
	}
	if len(refs) != 2 {
		t.Errorf("subject has %d reference rows, want 2 (unknown id and self-mention must not link)", len(refs))
	}

	// The mentioned record's Links tab lists the subject.
	if got := listReferences(t, s, orgID, "risk", mentioned.Identifier); len(got) != 1 {
		t.Errorf("?type=risk&id=%s: got %d rows, want 1", mentioned.Identifier, len(got))
	}
}

func TestDeletingCommentRemovesOnlyLinksNothingElseHolds(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "comment-mentions-delete")
	seedReviewUser(t, s, orgID, mentionActor, "admin")

	subject := newRefTestRisk(t, s, orgID, "risk with comments")
	shared := newRefTestRisk(t, s, orgID, "risk two comments mention")
	manual := newRefTestRisk(t, s, orgID, "risk also linked by hand")
	solo := newRefTestRisk(t, s, orgID, "risk one comment mentions")

	first := postMentionComment(t, s, orgID, "risk", subject.Identifier, "#"+shared.Identifier+" #"+manual.Identifier+" #"+solo.Identifier)
	second := postMentionComment(t, s, orgID, "risk", subject.Identifier, "again #"+shared.Identifier)

	// Someone links the manual risk on the Links tab too: the pair becomes manual.
	body, err := json.Marshal(map[string]string{"source_type": "risk", "source_id": subject.Identifier, "target_type": "risk", "target_id": manual.Identifier})
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	c, _ := ctxForCreateReference(orgID, string(body))
	if err := s.handleCreateReference(c); err != nil {
		t.Fatalf("handleCreateReference: %v", err)
	}

	deleteEntityComment(t, s, orgID, first)

	if got := pairOrigins(t, s, orgID, "risk", subject.Identifier, "risk", solo.Identifier); len(got) != 0 {
		t.Errorf("link to %s (only the deleted comment held it): origins %v, want removed", solo.Identifier, got)
	}
	if got := pairOrigins(t, s, orgID, "risk", subject.Identifier, "risk", shared.Identifier); len(got) != 2 {
		t.Errorf("link to %s (the second comment still mentions it): origins %v, want kept", shared.Identifier, got)
	}
	if got := pairOrigins(t, s, orgID, "risk", subject.Identifier, "risk", manual.Identifier); len(got) != 2 || got[0] != "manual" || got[1] != "manual" {
		t.Errorf("link to %s (also added by hand): origins %v, want kept as [manual manual]", manual.Identifier, got)
	}

	deleteEntityComment(t, s, orgID, second)
	if got := pairOrigins(t, s, orgID, "risk", subject.Identifier, "risk", shared.Identifier); len(got) != 0 {
		t.Errorf("link to %s after both comments are deleted: origins %v, want removed", shared.Identifier, got)
	}
}

func TestCommentMentionOfManualLinkKeepsItManual(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "comment-mentions-manual-first")
	seedReviewUser(t, s, orgID, mentionActor, "admin")

	subject := newRefTestRisk(t, s, orgID, "risk linked by hand first")
	target := newRefTestRisk(t, s, orgID, "risk mentioned after")
	for _, r := range []*db.EntityReference{
		{SourceType: "risk", SourceID: subject.Identifier, TargetType: "risk", TargetID: target.Identifier},
		{SourceType: "risk", SourceID: target.Identifier, TargetType: "risk", TargetID: subject.Identifier},
	} {
		if err := s.db.CreateReference(context.Background(), orgID, r); err != nil {
			t.Fatalf("seeding manual reference: %v", err)
		}
	}

	id := postMentionComment(t, s, orgID, "risk", subject.Identifier, "#"+target.Identifier)
	deleteEntityComment(t, s, orgID, id)

	if got := pairOrigins(t, s, orgID, "risk", subject.Identifier, "risk", target.Identifier); len(got) != 2 || got[0] != "manual" || got[1] != "manual" {
		t.Errorf("manual link after a comment mentioned it and was deleted: origins %v, want [manual manual]", got)
	}
}

func TestDocumentAndReviewCommentMentionsLinkTheDocument(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "comment-mentions-doc")
	seedRepoForOrg(t, s, orgID, "mention-doc-1")
	seedReviewUser(t, s, orgID, mentionActor, "admin")
	seedReviewUser(t, s, orgID, "reviewer@comment-mentions.test", "contributor")

	fromDoc := newRefTestRisk(t, s, orgID, "risk mentioned in a document comment")
	fromReview := newRefTestRisk(t, s, orgID, "risk mentioned in a review comment")

	// POST /comments: a plain document comment.
	body, err := json.Marshal(map[string]any{"document_id": "mention-doc-1", "body": "relates to #" + fromDoc.Identifier})
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	c, _ := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/comments", string(body), "reader", mentionActor)
	if err := s.handleAddCommentDB(c); err != nil {
		t.Fatalf("handleAddCommentDB: %v", err)
	}
	if got := pairOrigins(t, s, orgID, "document", "mention-doc-1", "risk", fromDoc.Identifier); len(got) != 2 {
		t.Errorf("document comment: link to %s has origins %v, want 2 rows", fromDoc.Identifier, got)
	}

	// POST /reviews/:id/comment on a review of the same document.
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews", `{"document_id":"mention-doc-1","reviewers":["reviewer@comment-mentions.test"]}`, "admin", mentionActor)
	if err := s.handleCreateReview(c); err != nil {
		t.Fatalf("handleCreateReview: %v", err)
	}
	var review db.Review
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decoding review: %v", err)
	}
	body, err = json.Marshal(map[string]any{"body": "see #" + fromReview.Identifier})
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	rid := strconv.Itoa(review.ID)
	c, _ = reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reviews/"+rid+"/comment", string(body), "admin", mentionActor)
	c.SetParamNames("id")
	c.SetParamValues(rid)
	if err := s.handleAddReviewComment(c); err != nil {
		t.Fatalf("handleAddReviewComment: %v", err)
	}
	if got := pairOrigins(t, s, orgID, "document", "mention-doc-1", "risk", fromReview.Identifier); len(got) != 2 {
		t.Errorf("review comment: link to %s has origins %v, want 2 rows", fromReview.Identifier, got)
	}
}
