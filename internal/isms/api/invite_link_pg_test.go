package api

// Invitations without email (#322): an admin can get the verification link
// back instead of having it mailed. A link hands over whatever account its
// token belongs to, so it is only ever issued for a brand-new account or one
// that was never activated and belongs to no other organization. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run InviteLink

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

type inviteLinkResp struct {
	Status    string `json:"status"`
	Delivery  string `json:"delivery"`
	Email     string `json:"email"`
	InviteURL string `json:"invite_url"`
	ExpiresAt int64  `json:"expires_at"`
}

func linkEmail() string {
	return fmt.Sprintf("il-%s@invite-link.test", uuid.NewString()[:8])
}

func decodeInviteResp(t *testing.T, body string) inviteLinkResp {
	t.Helper()
	var r inviteLinkResp
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return r
}

// tokenFromInviteURL returns the token of an invite_url after checking its shape.
func tokenFromInviteURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil || u.Path != "/verify-email" || u.Query().Get("token") == "" {
		t.Fatalf("invite_url %q is not <base>/verify-email?token=...", raw)
	}
	return u.Query().Get("token")
}

func wantInviteStatus(t *testing.T, code int, err error, want int) {
	t.Helper()
	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
	} else if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if code != want {
		t.Fatalf("status = %d (err %v), want %d", code, err, want)
	}
}

func userExists(t *testing.T, s *Server, email string) bool {
	t.Helper()
	_, err := s.db.GetUserByEmail(context.Background(), email)
	return err == nil
}

func verifyCall(t *testing.T, s *Server, token, password string) (int, error, string) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"token": token, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-email", strings.NewReader(string(b)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	err := s.handleVerifyEmail(echo.New().NewContext(req, rec))
	return rec.Code, err, rec.Body.String()
}

func tokenLive(t *testing.T, s *Server, token string) bool {
	t.Helper()
	h := sha256.Sum256([]byte(token))
	_, err := s.db.LookupEmailVerification(context.Background(), hex.EncodeToString(h[:]))
	return err == nil
}

func TestInviteLink_EmailWithoutMailerCreatesNothing(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-nomail")
	email := linkEmail()

	code, err, body := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email, "role": "reader"})
	wantInviteStatus(t, code, err, http.StatusServiceUnavailable)
	if he, ok := err.(*echo.HTTPError); ok && !strings.Contains(fmt.Sprint(he.Message), "create an invite link instead") {
		t.Errorf("message = %v", he.Message)
	}
	if userExists(t, s, email) {
		t.Errorf("a user row was left behind (body %s)", body)
	}
	members, _ := s.db.ListOrgUsers(context.Background(), org)
	if len(members) != 0 {
		t.Errorf("membership was created: %d members", len(members))
	}
}

func TestInviteLink_InvalidDelivery(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-baddelivery")
	email := linkEmail()
	code, err, _ := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email, "delivery": "pigeon"})
	wantInviteStatus(t, code, err, http.StatusBadRequest)
	if userExists(t, s, email) {
		t.Errorf("user created for an invalid delivery")
	}
}

func TestInviteLink_NewPersonActivatesThroughLink(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-new")
	email := linkEmail()

	code, err, body := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email, "name": "Ext Auditor", "role": "reader", "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusOK)
	r := decodeInviteResp(t, body)
	if r.Status != "invited" || r.Delivery != "link" || r.Email != email || r.ExpiresAt == 0 {
		t.Fatalf("response = %s", body)
	}
	token := tokenFromInviteURL(t, r.InviteURL)

	u, err := s.db.GetUserByEmail(context.Background(), email)
	if err != nil || u.Active {
		t.Fatalf("want an inactive user, got %+v err %v", u, err)
	}
	assertRole(t, s, org, u.ID, "reader")

	code, err, vbody := verifyCall(t, s, token, "A-long-password-1")
	wantInviteStatus(t, code, err, http.StatusOK)
	u, _ = s.db.GetUserByEmail(context.Background(), email)
	if !u.Active || !u.HasPassword() {
		t.Errorf("account not activated by the link: active=%v pw=%v body %s", u.Active, u.HasPassword(), vbody)
	}
	if tokenLive(t, s, token) {
		t.Errorf("token is still usable after use")
	}
}

func TestInviteLink_TokenNotInActivity(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-activity")
	email := linkEmail()
	code, err, body := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email, "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusOK)
	token := tokenFromInviteURL(t, decodeInviteResp(t, body).InviteURL)

	var detail string
	if err := s.db.Pool().QueryRow(context.Background(),
		`SELECT detail FROM activity WHERE organization_id = $1 AND action = 'user_invited' ORDER BY id DESC LIMIT 1`, org).Scan(&detail); err != nil {
		t.Fatalf("reading activity: %v", err)
	}
	if strings.Contains(detail, token) || strings.Contains(detail, "verify-email") {
		t.Errorf("activity detail leaks the link: %q", detail)
	}
	if !strings.Contains(detail, email) {
		t.Errorf("activity detail %q does not name the invitee", detail)
	}
}

func TestInviteLink_RefusedForAccountsThatAreSomeoneElses(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgA := newTestOrg(t, s, "il-refuse-a")
	orgB := newTestOrg(t, s, "il-refuse-b")

	// Inactive account WITH a password, not a member of orgB.
	withPw := orgUserAddAccount(t, s, orgA, "Has Password", false, false)
	if err := s.db.SetPassword(ctx, withPw.ID, "x-not-a-real-hash"); err != nil {
		t.Fatal(err)
	}
	// Inactive, no password, but a member of another org.
	elsewhere := orgUserAddAccount(t, s, orgA, "Elsewhere", false, false)

	for name, u := range map[string]*db.User{"password": withPw, "other org": elsewhere} {
		code, err, body := orgUserAddCall(t, s.handleInviteUser, orgB, "/api/v1/auth/invite",
			map[string]string{"email": u.Email, "role": "reader", "delivery": "link"})
		wantInviteStatus(t, code, err, http.StatusConflict)
		if strings.Contains(body, "invite_url") {
			t.Errorf("%s: link issued", name)
		}
		if _, err := s.db.GetOrgMember(ctx, orgB, u.ID); err == nil {
			t.Errorf("%s: membership written despite the refusal", name)
		}
		var n int
		_ = s.db.Pool().QueryRow(ctx, `SELECT count(*) FROM email_verifications WHERE user_id = $1`, u.ID).Scan(&n)
		if n != 0 {
			t.Errorf("%s: %d token(s) written", name, n)
		}
	}
}

func TestInviteLink_ExistingActiveAccountIsAddedWithoutLink(t *testing.T) {
	s := testServer(t)
	orgA := newTestOrg(t, s, "il-active-a")
	orgB := newTestOrg(t, s, "il-active-b")
	u := orgUserAddAccount(t, s, orgA, "Active Person", false, true)

	code, err, body := orgUserAddCall(t, s.handleInviteUser, orgB, "/api/v1/auth/invite",
		map[string]string{"email": u.Email, "role": "reader", "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusOK)
	r := decodeInviteResp(t, body)
	if r.Status != "added" || r.InviteURL != "" {
		t.Fatalf("response = %s", body)
	}
	assertRole(t, s, orgB, u.ID, "reader")
}

func TestInviteLink_ExistingMemberIsConflict(t *testing.T) {
	s := testServer(t)
	orgA := newTestOrg(t, s, "il-member")
	u := orgUserAddAccount(t, s, orgA, "Pending Member", false, false)
	_, err, _ := orgUserAddCall(t, s.handleInviteUser, orgA, "/api/v1/auth/invite",
		map[string]string{"email": u.Email, "delivery": "link"})
	assertConflict(t, err)
}

func TestInviteLink_ResendLinkReplacesOldToken(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-resend")
	email := linkEmail()
	_, err, body := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email, "delivery": "link"})
	if err != nil {
		t.Fatal(err)
	}
	old := tokenFromInviteURL(t, decodeInviteResp(t, body).InviteURL)

	code, err, body := orgUserAddCall(t, s.handleResendInvite, org, "/api/v1/auth/resend-invite",
		map[string]string{"email": email, "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusOK)
	r := decodeInviteResp(t, body)
	fresh := tokenFromInviteURL(t, r.InviteURL)
	if r.Delivery != "link" || r.ExpiresAt == 0 || fresh == old {
		t.Fatalf("response = %s", body)
	}
	if tokenLive(t, s, old) {
		t.Errorf("old token still valid after a resend")
	}
	if !tokenLive(t, s, fresh) {
		t.Errorf("new token is not valid")
	}
}

func TestInviteLink_ResendLinkRefusedForOtherOrgMember(t *testing.T) {
	s := testServer(t)
	orgA := newTestOrg(t, s, "il-resend-a")
	orgB := newTestOrg(t, s, "il-resend-b")
	u := orgUserAddAccount(t, s, orgA, "Two Orgs", false, false)
	if err := s.db.AddOrgMember(context.Background(), orgB, u.ID, "reader"); err != nil {
		t.Fatal(err)
	}
	code, err, body := orgUserAddCall(t, s.handleResendInvite, orgB, "/api/v1/auth/resend-invite",
		map[string]string{"email": u.Email, "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusConflict)
	if strings.Contains(body, "invite_url") {
		t.Errorf("link issued")
	}
}

func TestInviteLink_ResendLinkRefusedWithPassword(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-resend-pw")
	u := orgUserAddAccount(t, s, org, "Has Password", false, false)
	if err := s.db.SetPassword(context.Background(), u.ID, "x-not-a-real-hash"); err != nil {
		t.Fatal(err)
	}
	code, err, _ := orgUserAddCall(t, s.handleResendInvite, org, "/api/v1/auth/resend-invite",
		map[string]string{"email": u.Email, "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusConflict)
}

func TestInviteLink_ResendEmailWithoutMailerKeepsOldToken(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-resend-nomail")
	email := linkEmail()
	_, err, body := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email, "delivery": "link"})
	if err != nil {
		t.Fatal(err)
	}
	old := tokenFromInviteURL(t, decodeInviteResp(t, body).InviteURL)

	code, err, _ := orgUserAddCall(t, s.handleResendInvite, org, "/api/v1/auth/resend-invite",
		map[string]string{"email": email})
	wantInviteStatus(t, code, err, http.StatusServiceUnavailable)
	if !tokenLive(t, s, old) {
		t.Errorf("the old token was invalidated by a refused resend")
	}
}

func TestInviteLink_EmailPathStillSends(t *testing.T) {
	s := testServer(t)
	sink := newSMTPSink(t)
	s.mailer = sink.mailer()
	org := newTestOrg(t, s, "il-email")
	email := linkEmail()

	code, err, body := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email, "delivery": "email", "role": "reader"})
	wantInviteStatus(t, code, err, http.StatusOK)
	r := decodeInviteResp(t, body)
	if r.Status != "invited" || r.InviteURL != "" || strings.Contains(body, "invite_url") {
		t.Fatalf("response = %s", body)
	}
	if got := sink.to(); len(got) != 1 || !strings.EqualFold(got[0], email) {
		t.Errorf("mail recipients = %v, want exactly %s", got, email)
	}

	// Default delivery is email too.
	email2 := linkEmail()
	code, err, body = orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": email2})
	wantInviteStatus(t, code, err, http.StatusOK)
	if len(sink.to()) != 2 {
		t.Errorf("default delivery did not send: %v", sink.to())
	}

	// Link delivery sends nothing.
	code, err, _ = orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": linkEmail(), "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusOK)
	if len(sink.to()) != 2 {
		t.Errorf("link delivery sent mail: %v", sink.to())
	}
}

func TestInviteLink_Options(t *testing.T) {
	s := testServer(t)
	org := newTestOrg(t, s, "il-options")

	call := func(role string) (int, error, string) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/invite-options", nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("org_id", org)
		c.Set("user_role", role)
		err := s.handleInviteOptions(c)
		return rec.Code, err, rec.Body.String()
	}
	for _, role := range []string{"admin", "manager"} {
		code, err, body := call(role)
		wantInviteStatus(t, code, err, http.StatusOK)
		if !strings.Contains(body, `"email_enabled":false`) || !strings.Contains(body, `"link_valid_hours":72`) {
			t.Errorf("%s without mailer: %s", role, body)
		}
	}
	for _, role := range []string{"reader", "contributor"} {
		code, err, _ := call(role)
		wantInviteStatus(t, code, err, http.StatusForbidden)
	}
	s.mailer = newSMTPSink(t).mailer()
	code, err, body := call("manager")
	wantInviteStatus(t, code, err, http.StatusOK)
	if !strings.Contains(body, `"email_enabled":true`) {
		t.Errorf("with mailer: %s", body)
	}
}

func TestInviteLink_NewLinkSupersedesEarlierTokenOfNeverActivatedAccount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	org := newTestOrg(t, s, "il-supersede")
	u := &db.User{Email: linkEmail(), Name: "Left Over", Active: false}
	if _, err := s.db.EnsureUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	old := "old-token-" + uuid.NewString()
	h := sha256.Sum256([]byte(old))
	if err := s.db.CreateEmailVerification(ctx, u.ID, hex.EncodeToString(h[:])); err != nil {
		t.Fatal(err)
	}
	code, err, body := orgUserAddCall(t, s.handleInviteUser, org, "/api/v1/auth/invite",
		map[string]string{"email": u.Email, "delivery": "link"})
	wantInviteStatus(t, code, err, http.StatusOK)
	if tokenLive(t, s, old) {
		t.Errorf("earlier token still valid next to the new link")
	}
	if !tokenLive(t, s, tokenFromInviteURL(t, decodeInviteResp(t, body).InviteURL)) {
		t.Errorf("new token not valid")
	}
}
