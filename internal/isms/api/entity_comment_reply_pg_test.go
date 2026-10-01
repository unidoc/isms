package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

// Postgres-gated tests for replies on POST /entity-comments: a reply must be on
// the same record as its parent. Without the check a reply could name a
// different entity_type/entity_id than its parent, so a thread rendered under one
// record held a comment that was filed under another. Reuses the harness from
// review_create_pg_test.go. Skipped when ISMS_TEST_DATABASE_URL is unset.

func postEntityComment(t *testing.T, s *Server, orgID int, email, body string) (error, int, string) {
	t.Helper()
	c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/entity-comments", body, "contributor", email)
	err := s.handleCreateEntityComment(c)
	return err, rec.Code, rec.Body.String()
}

func TestEntityCommentReplyMustStayOnParentsRecord(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "entity-reply")
	seedReviewUser(t, s, orgID, "dee@entity-reply.test", "contributor")

	parent := &db.EntityComment{EntityType: "incident", EntityID: "INC-1", Author: "dee@entity-reply.test", Body: "parent"}
	if err := s.db.CreateEntityComment(context.Background(), orgID, parent); err != nil {
		t.Fatalf("CreateEntityComment: %v", err)
	}
	pid := itoa(int(parent.ID))

	// Same record: allowed, and stored under the parent.
	err, code, body := postEntityComment(t, s, orgID, "dee@entity-reply.test",
		`{"entity_type":"incident","entity_id":"INC-1","parent_id":`+pid+`,"body":"reply"}`)
	if err != nil || code != http.StatusCreated {
		t.Fatalf("same-entity reply: expected 201, got err=%v code=%d: %s", err, code, body)
	}
	var got db.EntityComment
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Fatalf("stored parent_id = %v, want %d", got.ParentID, parent.ID)
	}

	// Another entity type, or another entity of the same type: 400.
	for _, payload := range map[string]string{
		"other type": `{"entity_type":"change_request","entity_id":"CH-1","parent_id":` + pid + `,"body":"x"}`,
		"other id":   `{"entity_type":"incident","entity_id":"INC-2","parent_id":` + pid + `,"body":"x"}`,
	} {
		err, _, _ := postEntityComment(t, s, orgID, "dee@entity-reply.test", payload)
		wantHTTPStatus(t, err, http.StatusBadRequest)
	}
	list, lerr := s.db.ListEntityComments(context.Background(), orgID, "change_request", "CH-1")
	if lerr != nil || len(list) != 0 {
		t.Fatalf("cross-entity reply was stored: err=%v rows=%d", lerr, len(list))
	}
}

func TestEntityCommentReplyToMissingParentIs404(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "entity-reply-404")
	seedReviewUser(t, s, orgID, "dee@entity-reply-404.test", "contributor")

	err, _, _ := postEntityComment(t, s, orgID, "dee@entity-reply-404.test",
		`{"entity_type":"incident","entity_id":"INC-1","parent_id":999999,"body":"x"}`)
	he := wantHTTPStatus(t, err, http.StatusNotFound)
	msg := strings.ToLower(jsonString(he.Message))
	for _, leak := range []string{"foreign key", "violates", "constraint", "sqlstate"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("response leaks database text (%q): %s", leak, msg)
		}
	}
}
