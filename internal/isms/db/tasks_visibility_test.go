package db

import (
	"strings"
	"testing"
)

// TestTaskViewerVisibilityClause is the core of task privacy on the read side:
// managers/admins (CanSeeAll) get no restriction, everyone else gets a predicate
// that only lets through public tasks plus their own (assigned or created). A bug
// here is a data-leak (too permissive) or a black-hole (too restrictive).
func TestTaskViewerVisibilityClause(t *testing.T) {
	// CanSeeAll → no predicate, no args → existing queries are byte-for-byte
	// unchanged, so managers/admins see every task.
	if clause, args := (TaskViewer{Email: "m@x.io", CanSeeAll: true}).visibilityClause(3); clause != "" || args != nil {
		t.Errorf("CanSeeAll viewer: got clause=%q args=%v, want empty/nil", clause, args)
	}

	// Restricted viewer → predicate on the given placeholder, email bound once.
	clause, args := TaskViewer{Email: "u@x.io"}.visibilityClause(4)
	if clause == "" {
		t.Fatal("restricted viewer: expected a predicate, got empty")
	}
	for _, want := range []string{"t.private = false", "t.created_by = $4", "email = $4"} {
		if !strings.Contains(clause, want) {
			t.Errorf("predicate missing %q: %s", want, clause)
		}
	}
	if len(args) != 1 || args[0] != "u@x.io" {
		t.Errorf("args = %v, want [u@x.io] (email bound exactly once)", args)
	}
}
