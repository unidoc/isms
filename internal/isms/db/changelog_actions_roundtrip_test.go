package db

import (
	"context"
	"testing"
)

// Requires a migrated Postgres — see notifications_roundtrip_test.go's header
// for how to stand one up. Skipped when ISMS_TEST_DATABASE_URL is unset.
//
// Regression for #295: the readings handlers log Action "reading", which
// entity_changelog_action_check did not allow, so every row was rejected
// (SQLSTATE 23514) and dropped by the API's best-effort logChange. This asserts
// the constraint accepts every action the API writes, on every entity type that
// records readings.
func TestChangelogAcceptsWrittenActions(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "changelogactions")

	// Every Action value a ChangelogEntry is built with in internal/isms/api.
	// Add to this list whenever a new action is introduced.
	actions := []string{"create", "update", "delete", "reading", "suggestion_applied"}
	for _, action := range actions {
		entry := &ChangelogEntry{EntityType: "risk", EntityID: 1, Action: action, ChangedBy: email}
		if err := d.LogChange(ctx, orgID, entry); err != nil {
			t.Errorf("LogChange(action %q): %v", action, err)
		}
	}

	// The five readings endpoints (api_readings.go) each log on their own type.
	for _, entityType := range []string{"risk", "asset", "legal_requirement", "supplier", "system"} {
		entry := &ChangelogEntry{EntityType: entityType, EntityID: 1, Action: "reading", ChangedBy: email, Reason: "Reading #1 recorded"}
		if err := d.LogChange(ctx, orgID, entry); err != nil {
			t.Errorf("LogChange(%s, reading): %v", entityType, err)
			continue
		}
		got, err := d.ListEntityChangelog(ctx, orgID, entityType, 1)
		if err != nil {
			t.Fatalf("ListEntityChangelog(%s): %v", entityType, err)
		}
		found := false
		for _, e := range got {
			if e.Action == "reading" && e.Reason == "Reading #1 recorded" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: reading row not read back (got %d rows)", entityType, len(got))
		}
	}
}
