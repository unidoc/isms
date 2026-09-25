package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Regression for #346: applying an incident `link` suggestion stored
// references without checking that the targets exist, so an agent-proposed
// link to a raw row id or a made-up identifier applied cleanly and left a
// broken chip. #345 added this check for POST /references but not for this
// apply path.
//
// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays
// green without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...

func newLinkTestIncident(t *testing.T, s *Server, orgID int, title string) *db.Incident {
	t.Helper()
	inc := &db.Incident{
		Title:             title,
		Description:       "x",
		Severity:          "low",
		Status:            "open",
		IncidentType:      "event",
		DetectedAt:        db.Epoch{Time: time.Now()},
		AuthorityNotified: "not_required",
		SubjectsNotified:  "not_required",
		Source:            "internal",
		Reporter:          "reporter@link-test.test",
	}
	if err := s.db.CreateIncident(context.Background(), orgID, inc); err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}
	return inc
}

func newLinkTestLegal(t *testing.T, s *Server, orgID int, title string) *db.LegalRequirement {
	t.Helper()
	lr := &db.LegalRequirement{
		Title:        title,
		Jurisdiction: "EU",
		Category:     "privacy",
		Status:       "open",
	}
	if err := s.db.CreateLegalRequirement(context.Background(), orgID, lr); err != nil {
		t.Fatalf("CreateLegalRequirement: %v", err)
	}
	return lr
}

// newIncidentLinkSuggestion creates a pending incident "link" suggestion whose
// EntityID and payload links are as given.
func newIncidentLinkSuggestion(t *testing.T, s *Server, orgID int, incidentEntityID string, links []map[string]string) *db.Suggestion {
	t.Helper()
	payload, err := json.Marshal(map[string]interface{}{"links": links})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return newTestSuggestion(t, s, orgID, &db.Suggestion{
		EntityType:      "incident",
		EntityID:        incidentEntityID,
		SuggestionType:  "link",
		Title:           "link",
		Payload:         json.RawMessage(payload),
		Status:          "open",
		SuggestedBy:     "agent@test",
		SuggestedByType: "agent",
	})
}

func applyIncidentLinkSuggestion(s *Server, orgID int, sg *db.Suggestion) error {
	c, _ := suggestionCtx(orgID, http.MethodPost, sg.ID)
	return s.handleApplyEntitySuggestion(c)
}

func TestApplyIncidentLinkRejectsUnresolvableTarget(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "suggestion-link-reject")

	inc := newLinkTestIncident(t, s, orgID, "incident for link rejection test")
	legal := newLinkTestLegal(t, s, orgID, "legal for link rejection test")

	cases := []struct {
		name    string
		links   []map[string]string
		wantErr bool
	}{
		{
			name:    "valid identifier",
			links:   []map[string]string{{"type": "legal_requirement", "id": legal.Identifier}},
			wantErr: false,
		},
		{
			name:    "raw row id",
			links:   []map[string]string{{"type": "legal_requirement", "id": strconv.FormatInt(legal.ID, 10)}},
			wantErr: true,
		},
		{
			name:    "unknown identifier",
			links:   []map[string]string{{"type": "legal_requirement", "id": "LEGAL-99999"}},
			wantErr: true,
		},
		{
			name: "mixed valid and invalid",
			links: []map[string]string{
				{"type": "legal_requirement", "id": legal.Identifier},
				{"type": "legal_requirement", "id": "LEGAL-99999"},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sg := newIncidentLinkSuggestion(t, s, orgID, inc.Identifier, tc.links)
			err := applyIncidentLinkSuggestion(s, orgID, sg)

			if !tc.wantErr {
				if err != nil {
					t.Fatalf("apply: %v", err)
				}
				refs, lerr := s.db.ListAllReferencesForEntity(ctx, orgID, "incident", inc.Identifier)
				if lerr != nil {
					t.Fatalf("ListAllReferencesForEntity: %v", lerr)
				}
				if len(refs) != 1 {
					t.Fatalf("want 1 reference row for the valid case, got %d", len(refs))
				}
				if refs[0].TargetType != "legal_requirement" || refs[0].TargetID != legal.Identifier {
					t.Fatalf("want reference targeting %s, got %s %s", legal.Identifier, refs[0].TargetType, refs[0].TargetID)
				}
				return
			}

			var he *echo.HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("want *echo.HTTPError, got %#v", err)
			}
			if he.Code != http.StatusBadRequest {
				t.Fatalf("want status 400, got %d", he.Code)
			}

			// The rejected suggestion stays open, not applied.
			got, gerr := s.db.GetSuggestion(ctx, orgID, sg.ID)
			if gerr != nil {
				t.Fatalf("GetSuggestion: %v", gerr)
			}
			if got.Status != "open" {
				t.Fatalf("want suggestion to stay open after a rejected apply, got %q", got.Status)
			}

			// And, for the mixed case in particular, nothing was written for the
			// valid link either: the whole apply rolled back.
			refs, lerr := s.db.ListAllReferencesForEntity(ctx, orgID, "incident", inc.Identifier)
			if lerr != nil {
				t.Fatalf("ListAllReferencesForEntity: %v", lerr)
			}
			// Reference count must equal the valid case's alone (1), from the
			// first subtest — this rejected case must not have added any rows.
			if len(refs) != 1 {
				t.Fatalf("want reference count to still equal the valid case's alone (1), got %d", len(refs))
			}
		})
	}
}

func TestApplyIncidentLinkStoresCanonicalSource(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "suggestion-link-canonical")

	inc := newLinkTestIncident(t, s, orgID, "incident for canonical source test")
	legal := newLinkTestLegal(t, s, orgID, "legal for canonical source test")

	// The suggestion's EntityID is the incident's numeric row id, which
	// resolveIncidentID accepts but which is not a valid reference source.
	sg := newIncidentLinkSuggestion(t, s, orgID, strconv.FormatInt(inc.ID, 10), []map[string]string{
		{"type": "legal_requirement", "id": legal.Identifier},
	})

	if err := applyIncidentLinkSuggestion(s, orgID, sg); err != nil {
		t.Fatalf("apply: %v", err)
	}

	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "incident", inc.Identifier)
	if err != nil {
		t.Fatalf("ListAllReferencesForEntity: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("want 1 reference row, got %d", len(refs))
	}
	if refs[0].SourceID != inc.Identifier {
		t.Fatalf("want stored source %s (the incident's identifier), got %s", inc.Identifier, refs[0].SourceID)
	}
	if refs[0].SourceType != "incident" {
		t.Fatalf("want source type incident, got %s", refs[0].SourceType)
	}
}
