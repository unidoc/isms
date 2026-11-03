package api

// A single sign-on session and an org-scoped key must stay out of other
// organizations even after the user leaves their own org, including over git
// (which grants by URL uuid and membership). Requests go through AuthMiddleware.
// Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run OIDCScope

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-git/go-git/v5"
	"isms.sh/internal/isms/db"
)

// newRepoOrg creates an org backed by a real bare repo and returns it.
func newRepoOrg(t *testing.T, s *Server) *db.Organization {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
	slug := "oidcscope" + oidcLinkUniq()
	org := &db.Organization{Name: slug, Slug: slug, RepoPath: dir}
	if err := s.db.CreateOrganization(context.Background(), org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.DeleteOrganization(context.Background(), org.ID) })
	return org
}

func (f *containFixture) git(uuid, token string, basic bool) int {
	req := httptest.NewRequest(http.MethodGet, "/git/"+uuid+"/info/refs?service=git-upload-pack", nil)
	if basic {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("x:"+token)))
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return rec.Code
}

func (f *containFixture) newKey(t *testing.T, org *int) string {
	t.Helper()
	key, hash := generateAPIKey()
	if _, err := f.s.db.CreateAPIKey(context.Background(), "t", hash, f.user.Email, "read-write", org, nil); err != nil {
		t.Fatal(err)
	}
	return key
}

func (f *containFixture) leaveOrgA(t *testing.T) {
	t.Helper()
	if _, err := f.s.db.Pool().Exec(context.Background(),
		`DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2`, f.orgA.id, f.user.ID); err != nil {
		t.Fatal(err)
	}
}

func scopeFixture(t *testing.T) (*containFixture, *db.Organization) {
	t.Helper()
	f := newContainFixture(t)
	f.e.GET("/git/:uuid/info/refs", f.s.handleGitInfoRefs)
	f.e.GET("/api/v1/me/organizations", f.s.handleMyOrganizations)
	x := newRepoOrg(t, f.s)
	if err := f.s.db.AddOrgMember(context.Background(), x.ID, f.user.ID, "manager"); err != nil {
		t.Fatal(err)
	}
	return f, x
}

func TestOIDCScope_ContainedSessionFailsClosedAfterRemoval(t *testing.T) {
	f, x := scopeFixture(t)
	if code := f.git(x.UUID, f.sso, false); code != 403 {
		t.Errorf("contained session git on another org (still a member of its own) = %d, want 403", code)
	}
	f.leaveOrgA(t)
	for _, basic := range []bool{false, true} {
		if code := f.git(x.UUID, f.sso, basic); code != 401 {
			t.Errorf("contained session git on another org after removal (basic=%v) = %d, want 401", basic, code)
		}
	}
	if code, body := f.do("GET", "/me/organizations", f.sso, ""); code != 401 {
		t.Errorf("contained session data route after removal = %d, want 401: %s", code, body)
	}
}

func TestOIDCScope_ScopedKeyStaysInItsOrgAfterRemoval(t *testing.T) {
	f, x := scopeFixture(t)
	orgA := f.orgA.id
	keyA := f.newKey(t, &orgA)
	if code := f.git(x.UUID, keyA, false); code != 403 {
		t.Errorf("scoped key git on another org = %d, want 403", code)
	}
	f.leaveOrgA(t)
	if code := f.git(x.UUID, keyA, false); code != 403 {
		t.Errorf("scoped key git on another org after removal = %d, want 403", code)
	}
	if code, body := f.do("GET", "/me/organizations", keyA, ""); code != 403 {
		t.Errorf("scoped key data route after removal = %d, want 403: %s", code, body)
	}
}

func TestOIDCScope_Controls(t *testing.T) {
	f, x := scopeFixture(t)
	xid := x.ID
	keyX := f.newKey(t, &xid)
	keyGlobal := f.newKey(t, nil)
	f.leaveOrgA(t)
	// An uncontained session whose claim org it left keeps today's behaviour.
	if code := f.git(x.UUID, f.plain, false); code != 200 {
		t.Errorf("ordinary session git on an org it belongs to = %d, want 200", code)
	}
	if code := f.git(x.UUID, keyX, false); code != 200 {
		t.Errorf("scoped key git on its own org = %d, want 200", code)
	}
	if code := f.git(x.UUID, keyGlobal, false); code != 200 {
		t.Errorf("global key git = %d, want 200", code)
	}
}
