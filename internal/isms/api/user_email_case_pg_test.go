package api

// Accounts are unique by lower(email), so every lookup by email must match the
// same way. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run EmailCase

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"isms.sh/internal/isms/db"
)

func TestEmailCase_GetUserByEmailIgnoresCase(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	email := fmt.Sprintf("case-%s@email-case.test", uuid.NewString()[:8])
	u := &db.User{Email: email, Name: "Case Person", Active: true}
	if _, err := s.db.EnsureUser(ctx, u); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	got, err := s.db.GetUserByEmail(ctx, strings.ToUpper(email))
	if err != nil {
		t.Fatalf("GetUserByEmail(%q) = %v, want the stored account", strings.ToUpper(email), err)
	}
	if got.ID != u.ID {
		t.Errorf("found user %d, want %d", got.ID, u.ID)
	}
}

func TestEmailCase_EnsureUserDoesNotOverwriteAcrossCase(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	email := fmt.Sprintf("case2-%s@email-case.test", uuid.NewString()[:8])
	u := &db.User{Email: email, Name: "Original", Active: true}
	if _, err := s.db.EnsureUser(ctx, u); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	other := &db.User{Email: strings.ToUpper(email), Name: "Attacker", Active: false}
	created, err := s.db.EnsureUser(ctx, other)
	if err != nil || created {
		t.Fatalf("EnsureUser on a differently-cased email: created %v err %v, want existing", created, err)
	}
	if other.ID != u.ID || other.Name != "Original" || !other.Active {
		t.Errorf("loaded %+v, want the untouched original account", other)
	}
}
