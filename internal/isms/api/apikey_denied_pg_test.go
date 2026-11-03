package api

// An API key is a data credential: it must not mint sessions or keys or change
// how the account signs in. Requests go through AuthMiddleware with a real key;
// an ordinary session JWT is the control. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run APIKeyDenied

import (
	"context"
	"testing"
)

func TestAPIKeyDenied_SessionAndCredentialRoutes(t *testing.T) {
	f := newContainFixture(t)
	key, hash := generateAPIKey()
	orgA := f.orgA.id
	if _, err := f.s.db.CreateAPIKey(context.Background(), "t", hash, f.user.Email, "read-write", &orgA, nil); err != nil {
		t.Fatalf("creating key: %v", err)
	}

	routes := []struct{ method, path, body string }{
		{"POST", "/auth/refresh", ""},
		{"POST", "/auth/switch-org", `{"slug":"` + f.orgA.slug + `"}`},
		{"POST", "/auth/api-keys", `{"name":"k"}`},
		{"POST", "/auth/api-keys", `{"name":"k","scope":"global"}`},
		{"PUT", "/auth/password", `{"new_password":"correct-horse-battery"}`},
		{"PUT", "/auth/email", `{"new_email":"x@example.com","current_password":"pw"}`},
		{"DELETE", "/auth/email", ""},
		{"POST", "/auth/otp/setup", ""},
		{"POST", "/auth/otp/verify", `{"code":"123456"}`},
		{"DELETE", "/auth/otp", `{"code":"123456"}`},
		{"POST", "/auth/passkey/register/begin", ""},
		{"POST", "/auth/passkey/register/complete", ""},
		{"PUT", "/auth/passkeys/2000000000", `{"name":"x"}`},
		{"DELETE", "/auth/passkeys/2000000000", ""},
	}
	f.e.DELETE("/api/v1/auth/email", f.s.handleCancelEmailChange)
	for _, r := range routes {
		if code, body := f.do(r.method, r.path, key, r.body); code != 403 {
			t.Errorf("key %s %s %s = %d, want 403: %s", r.method, r.path, r.body, code, body)
		}
		if code, body := f.do(r.method, r.path, f.plain, r.body); code == 403 {
			t.Errorf("control %s %s %s = 403, want anything else: %s", r.method, r.path, r.body, body)
		}
	}
}

func TestAPIKeyDenied_DataRoutesStillWork(t *testing.T) {
	f := newContainFixture(t)
	key, hash := generateAPIKey()
	orgA := f.orgA.id
	if _, err := f.s.db.CreateAPIKey(context.Background(), "t", hash, f.user.Email, "read-write", &orgA, nil); err != nil {
		t.Fatalf("creating key: %v", err)
	}
	f.e.GET("/api/v1/me/organizations", f.s.handleMyOrganizations)
	if code, body := f.do("GET", "/me/organizations", key, ""); code != 200 {
		t.Errorf("key on a data route = %d, want 200: %s", code, body)
	}
	// Revoking a key stays allowed.
	f.e.DELETE("/api/v1/auth/api-keys/:id", f.s.handleRevokeMyAPIKey)
	if code, body := f.do("DELETE", "/auth/api-keys/2000000000", key, ""); code == 403 {
		t.Errorf("key revoke = 403, want allowed: %s", body)
	}
}
