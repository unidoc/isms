package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

// callToggleReaction calls POST /reactions and returns the handler's error.
func callToggleReaction(s *Server, orgID int, targetID int64, email string) (error, *httptest.ResponseRecorder) {
	body := `{"target_type":"entity_comment","target_id":` + strconv.FormatInt(targetID, 10) + `,"emoji":"👍"}`
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/reactions", body, "contributor", email)
	return s.handleToggleReaction(c), rec
}

// A reaction on a register comment that is gone, or never existed, is refused:
// entity_reactions has no foreign key, so nothing else stops an orphan row.
func TestReactionOnMissingEntityCommentIs404(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-react-404")
	const admin = "admin@ec-react-404.test"
	seedReviewUser(t, s, orgID, admin, "admin")
	ctx := context.Background()

	err, _ := callToggleReaction(s, orgID, 999999, admin)
	wantHTTPStatus(t, err, http.StatusNotFound)

	cm := mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "react to me", nil)
	if err, rec := callToggleReaction(s, orgID, cm.ID, admin); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("reaction on a live comment: err=%v code=%d", err, rec.Code)
	}
	if err, rec := callDeleteEntityComment(s, orgID, cm.ID, "admin", admin); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("delete: err=%v code=%d", err, rec.Code)
	}
	err, _ = callToggleReaction(s, orgID, cm.ID, admin)
	wantHTTPStatus(t, err, http.StatusNotFound)
	if got, err := s.db.ListReactions(ctx, orgID, "entity_comment", cm.ID); err != nil || len(got) != 0 {
		t.Fatalf("reactions on the deleted comment: err=%v rows=%v", err, got)
	}
}

// A reaction that arrives while the comment is being deleted waits for the
// delete's row lock and is then refused, rather than landing after the
// delete's reaction cleanup and outliving the comment.
func TestReactionRacingEntityCommentDeleteLeavesNoOrphan(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-react-race")
	const admin = "admin@ec-react-race.test"
	seedReviewUser(t, s, orgID, admin, "admin")
	ctx := context.Background()

	cm := mustCreateEntityComment(t, s, orgID, "risk", "RISK-1", admin, "racing", nil)

	// Hold the same lock DeleteEntityComment takes, start the reaction, give it
	// time to reach the lock, then delete and commit.
	toggled := make(chan error, 1)
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM entity_comments WHERE id = $1 FOR UPDATE`, cm.ID); err != nil {
			return err
		}
		go func() {
			_, err := s.db.ToggleReaction(context.Background(), orgID, "entity_comment", cm.ID, "👍", admin)
			toggled <- err
		}()
		time.Sleep(300 * time.Millisecond)
		_, err := tx.Exec(ctx, `DELETE FROM entity_comments WHERE id = $1`, cm.ID)
		return err
	}); err != nil {
		t.Fatalf("delete tx: %v", err)
	}

	select {
	case err := <-toggled:
		if !errors.Is(err, db.ErrReactionTargetNotFound) {
			t.Fatalf("ToggleReaction after a racing delete = %v, want ErrReactionTargetNotFound", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ToggleReaction did not return")
	}
	if got, err := s.db.ListReactions(ctx, orgID, "entity_comment", cm.ID); err != nil || len(got) != 0 {
		t.Fatalf("orphan reaction left behind: err=%v rows=%v", err, got)
	}
}

// The Documents page's Discussion tab stores its comments in the same table
// with entity_type "document", so the same rules apply there.
func TestDeleteDocumentDiscussionComment(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ec-del-doc")
	const admin = "admin@ec-del-doc.test"
	seedReviewUser(t, s, orgID, admin, "admin")

	parent := mustCreateEntityComment(t, s, orgID, "document", "iso27001-4-1", admin, "parent", nil)
	reply := mustCreateEntityComment(t, s, orgID, "document", "iso27001-4-1", admin, "reply", &parent.ID)

	err, _ := callDeleteEntityComment(s, orgID, parent.ID, "admin", admin)
	wantHTTPStatus(t, err, http.StatusConflict)
	for _, id := range []int64{reply.ID, parent.ID} {
		if err, rec := callDeleteEntityComment(s, orgID, id, "admin", admin); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("delete %d: err=%v code=%d", id, err, rec.Code)
		}
	}
	if n := len(mustListEntityComments(t, s, orgID, "document", "iso27001-4-1")); n != 0 {
		t.Fatalf("document discussion comments left: %d, want 0", n)
	}
}
