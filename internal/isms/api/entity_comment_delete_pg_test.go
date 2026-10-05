package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// Postgres-gated tests for DELETE /entity-comments/:id (#403): a comment that
// still has replies is refused with 409 instead of a raw foreign-key 500, a
// missing id is a 404, and a delete takes the comment's reactions with it.
// Skipped when ISMS_TEST_DATABASE_URL is unset.

// callDeleteEntityComment returns the handler's error instead of failing the
// test, unlike deleteEntityComment in comment_mentions_pg_test.go.
func callDeleteEntityComment(s *Server, orgID int, id int64, role, email string) (error, *httptest.ResponseRecorder) {
	sid := strconv.FormatInt(id, 10)
	c, rec := reviewCtxForPath(orgID, http.MethodDelete, "/api/v1/entity-comments/"+sid, "", role, email)
	c.SetParamNames("id")
	c.SetParamValues(sid)
	return s.handleDeleteEntityComment(c), rec
}

func mustCreateEntityComment(t *testing.T, s *Server, orgID int, entityType, entityID, author, body string, parent *int64) *db.EntityComment {
	t.Helper()
	c := &db.EntityComment{EntityType: entityType, EntityID: entityID, Author: author, Body: body, ParentID: parent}
	if err := s.db.CreateEntityComment(context.Background(), orgID, c); err != nil {
		t.Fatalf("CreateEntityComment(%q): %v", body, err)
	}
	return c
}

func mustListEntityComments(t *testing.T, s *Server, orgID int, entityType, entityID string) []db.EntityComment {
	t.Helper()
	list, err := s.db.ListEntityComments(context.Background(), orgID, entityType, entityID)
	if err != nil {
		t.Fatalf("ListEntityComments: %v", err)
	}
	return list
}

func TestDeleteEntityCommentWithRepliesIs409(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-del-replies")
	const admin = "admin@ec-del-replies.test"
	seedReviewUser(t, s, orgID, admin, "admin")

	parent := mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "parent", nil)
	mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "reply 1", &parent.ID)
	mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "reply 2", &parent.ID)

	err, _ := callDeleteEntityComment(s, orgID, parent.ID, "admin", admin)
	he := wantHTTPStatus(t, err, http.StatusConflict)
	eb, ok := he.Message.(errorBody)
	if !ok {
		t.Fatalf("message is %T, want errorBody", he.Message)
	}
	if eb.Code != CodeCommentHasReplies || eb.Params["count"] != "2" {
		t.Fatalf("got code=%q params=%v, want %q count=2", eb.Code, eb.Params, CodeCommentHasReplies)
	}
	low := strings.ToLower(eb.Message)
	for _, bad := range []string{"foreign key", "violates", "constraint", "sqlstate"} {
		if strings.Contains(low, bad) {
			t.Fatalf("message leaks SQL text %q: %s", bad, eb.Message)
		}
	}
	if n := len(mustListEntityComments(t, s, orgID, "risk", "RISK-1")); n != 3 {
		t.Fatalf("after refused delete: %d comments, want 3", n)
	}
}

func TestDeleteEntityCommentAfterRepliesAreGone(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-del-after")
	const admin = "admin@ec-del-after.test"
	seedReviewUser(t, s, orgID, admin, "admin")

	parent := mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "parent", nil)
	reply := mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "reply", &parent.ID)

	if err, rec := callDeleteEntityComment(s, orgID, reply.ID, "admin", admin); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("delete reply: err=%v code=%d", err, rec.Code)
	}
	if err, rec := callDeleteEntityComment(s, orgID, parent.ID, "admin", admin); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("delete parent: err=%v code=%d", err, rec.Code)
	}
	if n := len(mustListEntityComments(t, s, orgID, "risk", "RISK-1")); n != 0 {
		t.Fatalf("%d comments remain, want 0", n)
	}
}

func TestDeleteEntityCommentMissingIs404(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-del-404")
	otherOrg := newTestOrg(t, s, "ec-del-404-other")
	const admin = "admin@ec-del-404.test"
	seedReviewUser(t, s, orgID, admin, "admin")

	err, _ := callDeleteEntityComment(s, orgID, 999999, "admin", admin)
	he := wantHTTPStatus(t, err, http.StatusNotFound)
	if eb, ok := he.Message.(errorBody); !ok || eb.Code != CodeNotFound {
		t.Fatalf("message = %#v, want code %q", he.Message, CodeNotFound)
	}

	foreign := mustCreateEntityComment(t, s, otherOrg, "risk", "RISK-1", "someone@other.test", "other org", nil)
	err, _ = callDeleteEntityComment(s, orgID, foreign.ID, "admin", admin)
	wantHTTPStatus(t, err, http.StatusNotFound)
	if n := len(mustListEntityComments(t, s, otherOrg, "risk", "RISK-1")); n != 1 {
		t.Fatalf("other org's comment gone: %d rows, want 1", n)
	}
}

func TestDeleteEntityCommentRemovesReactions(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-del-react")
	const admin = "admin@ec-del-react.test"
	seedReviewUser(t, s, orgID, admin, "admin")
	ctx := context.Background()

	cm := mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "react to me", nil)
	if _, err := s.db.ToggleReaction(ctx, orgID, "entity_comment", cm.ID, "👍", admin); err != nil {
		t.Fatalf("ToggleReaction: %v", err)
	}
	if err, rec := callDeleteEntityComment(s, orgID, cm.ID, "admin", admin); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("delete: err=%v code=%d", err, rec.Code)
	}
	got, err := s.db.ListReactions(ctx, orgID, "entity_comment", cm.ID)
	if err != nil || len(got) != 0 {
		t.Fatalf("reactions after delete: err=%v rows=%v", err, got)
	}
}

func TestDeleteEntityCommentRefusedKeepsLinks(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-del-links")
	seedReviewUser(t, s, orgID, mentionActor, "admin")

	subject := newRefTestRisk(t, s, orgID, "risk the comment is on")
	mentioned := newRefTestRisk(t, s, orgID, "risk the comment mentions")

	parentID := postMentionComment(t, s, orgID, "risk", subject.Identifier, "see #"+mentioned.Identifier)
	reply := mustCreateEntityComment(t, s, orgID, "risk", subject.Identifier, mentionActor, "a reply", &parentID)

	origins := func() []string {
		return pairOrigins(t, s, orgID, "risk", subject.Identifier, "risk", mentioned.Identifier)
	}
	if got := origins(); len(got) != 2 || got[0] != "comment" || got[1] != "comment" {
		t.Fatalf("link before delete = %v, want [comment comment] (one row per direction)", got)
	}

	err, _ := callDeleteEntityComment(s, orgID, parentID, "admin", mentionActor)
	wantHTTPStatus(t, err, http.StatusConflict)
	if got := origins(); len(got) != 2 || got[0] != "comment" || got[1] != "comment" {
		t.Fatalf("refused delete stripped the link: %v", got)
	}

	if err, _ := callDeleteEntityComment(s, orgID, reply.ID, "admin", mentionActor); err != nil {
		t.Fatalf("delete reply: %v", err)
	}
	if err, _ := callDeleteEntityComment(s, orgID, parentID, "admin", mentionActor); err != nil {
		t.Fatalf("delete parent: %v", err)
	}
	if got := origins(); len(got) != 0 {
		t.Fatalf("link survived the delete: %v", got)
	}
}

func TestDeleteEntityCommentRequiresManager(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-del-role")
	const contrib = "contrib@ec-del-role.test"
	seedReviewUser(t, s, orgID, contrib, "contributor")

	cm := mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", contrib, "mine", nil)
	err, _ := callDeleteEntityComment(s, orgID, cm.ID, "contributor", contrib)
	wantHTTPStatus(t, err, http.StatusForbidden)
	if n := len(mustListEntityComments(t, s, orgID, "risk", "RISK-1")); n != 1 {
		t.Fatalf("%d comments, want 1", n)
	}
}
