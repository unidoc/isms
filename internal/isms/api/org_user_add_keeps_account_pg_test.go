package api

// Adding a person to an organization must only touch that organization's
// membership. Users are global: the account's name, agent flag and active flag
// belong to the person, not to whichever org adds them. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run OrgUserAdd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// orgUserAddCall runs a handler as the admin of orgID with a JSON body.
func orgUserAddCall(t *testing.T, h echo.HandlerFunc, orgID int, path string, body map[string]string) (int, error, string) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", "admin@org-user-add.test")
	err := h(c)
	return rec.Code, err, rec.Body.String()
}

// orgUserAddAccount creates a global account in orgA with the given flags.
func orgUserAddAccount(t *testing.T, s *Server, orgA int, name string, agent, active bool) *db.User {
	t.Helper()
	ctx := context.Background()
	u := &db.User{
		Email:   fmt.Sprintf("ouak-%s@org-user-add.test", uuid.NewString()[:8]),
		Name:    name,
		IsAgent: agent,
		Active:  active,
	}
	if err := s.db.UpsertUser(ctx, u); err != nil {
		t.Fatalf("creating account: %v", err)
	}
	if err := s.db.AddOrgMember(ctx, orgA, u.ID, "reader"); err != nil {
		t.Fatalf("adding account to org A: %v", err)
	}
	return u
}

// assertAccountUnchanged fails if the stored account differs from want.
func assertAccountUnchanged(t *testing.T, s *Server, want *db.User) {
	t.Helper()
	got, err := s.db.GetUserByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("loading account: %v", err)
	}
	if got.IsAgent != want.IsAgent {
		t.Errorf("is_agent = %v, want %v", got.IsAgent, want.IsAgent)
	}
	if got.Active != want.Active {
		t.Errorf("active = %v, want %v", got.Active, want.Active)
	}
	if got.Name != want.Name {
		t.Errorf("name = %q, want %q", got.Name, want.Name)
	}
}

func assertRole(t *testing.T, s *Server, orgID, userID int, want string) {
	t.Helper()
	role, err := s.db.GetUserRole(context.Background(), orgID, userID)
	if err != nil || role != want {
		t.Errorf("role in org %d = %q (err %v), want %q", orgID, role, err, want)
	}
}

func TestOrgUserAdd_UpsertKeepsAgentFlag(t *testing.T) {
	s := testServer(t)
	orgA := newTestOrg(t, s, "ouak-agent-a")
	orgB := newTestOrg(t, s, "ouak-agent-b")
	agent := orgUserAddAccount(t, s, orgA, "Review Bot", true, true)

	code, err, body := orgUserAddCall(t, s.handleUpsertUser, orgB, "/api/v1/users",
		map[string]string{"email": strings.ToUpper(agent.Email), "name": "Someone Else", "role": "contributor"})
	if err != nil || code != http.StatusOK {
		t.Fatalf("POST /users: code %d err %v body %s", code, err, body)
	}
	assertAccountUnchanged(t, s, agent)
	assertRole(t, s, orgB, agent.ID, "contributor")
	assertRole(t, s, orgA, agent.ID, "reader")
	if !strings.Contains(body, `"name":"Review Bot"`) {
		t.Errorf("response should report the account's actual name: %s", body)
	}
}

func TestOrgUserAdd_UpsertKeepsInactiveAccount(t *testing.T) {
	s := testServer(t)
	orgA := newTestOrg(t, s, "ouak-inactive-a")
	orgB := newTestOrg(t, s, "ouak-inactive-b")
	u := orgUserAddAccount(t, s, orgA, "Deactivated Person", false, false)
	if err := s.db.SetPassword(context.Background(), u.ID, "x-not-a-real-hash"); err != nil {
		t.Fatalf("setting password: %v", err)
	}

	code, err, body := orgUserAddCall(t, s.handleUpsertUser, orgB, "/api/v1/users",
		map[string]string{"email": u.Email, "name": "Renamed", "role": "reader"})
	if err != nil || code != http.StatusOK {
		t.Fatalf("POST /users: code %d err %v body %s", code, err, body)
	}
	assertAccountUnchanged(t, s, u)
	assertRole(t, s, orgB, u.ID, "reader")
}

func TestOrgUserAdd_InviteKeepsActiveAccount(t *testing.T) {
	s := testServer(t)
	orgA := newTestOrg(t, s, "ouak-invite-a")
	orgB := newTestOrg(t, s, "ouak-invite-b")
	u := orgUserAddAccount(t, s, orgA, "Active Person", false, true)

	// Cross-org invite.
	code, err, body := orgUserAddCall(t, s.handleInviteUser, orgB, "/api/v1/auth/invite",
		map[string]string{"email": u.Email, "name": "Renamed", "role": "contributor"})
	if err != nil || code != http.StatusOK || !strings.Contains(body, `"status":"added"`) {
		t.Fatalf("cross-org invite: code %d err %v body %s", code, err, body)
	}
	assertAccountUnchanged(t, s, u)
	assertRole(t, s, orgB, u.ID, "contributor")

	// Same-org re-invite of an existing member.
	code, err, body = orgUserAddCall(t, s.handleInviteUser, orgA, "/api/v1/auth/invite",
		map[string]string{"email": u.Email, "name": "Renamed", "role": "manager"})
	if err != nil || code != http.StatusOK || !strings.Contains(body, `"status":"added"`) {
		t.Fatalf("same-org re-invite: code %d err %v body %s", code, err, body)
	}
	assertAccountUnchanged(t, s, u)
	assertRole(t, s, orgA, u.ID, "manager")
}

func TestOrgUserAdd_NewEmailCreatedAsBefore(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "ouak-new")
	ctx := context.Background()

	email := fmt.Sprintf("ouak-new-%s@org-user-add.test", uuid.NewString()[:8])
	code, err, body := orgUserAddCall(t, s.handleUpsertUser, orgID, "/api/v1/users",
		map[string]string{"email": email, "name": "Brand New", "role": "reader"})
	if err != nil || code != http.StatusOK {
		t.Fatalf("POST /users: code %d err %v body %s", code, err, body)
	}
	u, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("loading created user: %v", err)
	}
	if !u.Active || u.IsAgent || u.Name != "Brand New" {
		t.Errorf("created user = active %v agent %v name %q, want active human 'Brand New'", u.Active, u.IsAgent, u.Name)
	}
	assertRole(t, s, orgID, u.ID, "reader")

	// Invite of a new email: created inactive with the membership. The test
	// server has no mailer, so the handler stops at the email step after
	// writing the user, as it always has.
	inv := fmt.Sprintf("ouak-inv-%s@org-user-add.test", uuid.NewString()[:8])
	_, _, _ = orgUserAddCall(t, s.handleInviteUser, orgID, "/api/v1/auth/invite",
		map[string]string{"email": inv, "name": "Invitee", "role": "reader"})
	iu, err := s.db.GetUserByEmail(ctx, inv)
	if err != nil {
		t.Fatalf("loading invited user: %v", err)
	}
	if iu.Active || iu.Name != "Invitee" {
		t.Errorf("invited user = active %v name %q, want inactive 'Invitee'", iu.Active, iu.Name)
	}
	assertRole(t, s, orgID, iu.ID, "reader")
}
