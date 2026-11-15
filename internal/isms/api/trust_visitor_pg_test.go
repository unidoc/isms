package api

// Trust center visitor sign-in, driven through the real middleware chain with
// Host headers like trust_public_pg_test.go. Mail goes to the fake SMTP sink.
// Every test seeds its own organizations. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustVisitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"isms.sh/internal/isms/db"
)

type visitorEnv struct {
	s    *Server
	sink *smtpSink
	org  trustOrg
}

// purgeTestLinks removes the sign-in links earlier tests left for the reserved
// test domain. The per-address cap counts links across every organization for an
// hour, and the tests reuse a few addresses, so without this a second run on the
// same database (or a long run) would trip the cap and lose a mail.
func purgeTestLinks(t *testing.T) {
	t.Helper()
	rawExec(t, `DELETE FROM trust_visitor_links WHERE email LIKE '%@auditco.test'`)
}

// visitorSetup returns a server with a mailer on a fake SMTP sink and an
// enabled trust center. The base URL is http, in path mode.
func visitorSetup(t *testing.T) visitorEnv {
	t.Helper()
	purgeTestLinks(t)
	t.Setenv("ISMS_BASE_URL", "http://"+trustTestBase)
	t.Setenv("ISMS_RATE_LIMIT", "")
	t.Setenv("ISMS_TRUST_RATE_LIMIT", "100000") // the general limiter is not under test here
	s := trustServer(t)
	sink := newSMTPSink(t)
	s.mailer = sink.mailer()
	return visitorEnv{s: s, sink: sink, org: trustNewOrg(t, s, "vis", true)}
}

// rawExec runs one statement on a separate connection to the test database,
// for state no handler can set yet.
func rawExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), os.Getenv("ISMS_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (e visitorEnv) path(rest string) string {
	return "/api/v1/trust/" + e.org.slug + rest
}

// vdo sends a request to the path-mode host. A POST or a request with a body
// is JSON unless a mod says otherwise.
func vdo(s *Server, method, path, body string, mods ...func(*http.Request)) trustResp {
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	req := httptest.NewRequest(method, "http://"+trustTestBase+path, rd)
	if method == http.MethodPost || body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mods {
		m(req)
	}
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	return trustResp{code: rec.Code, body: rec.Body.String(), hdr: rec.Header()}
}

func withCookie(v string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "isms_trust_visitor", Value: v}) }
}
func withBearer(v string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+v) }
}
func withContentType(v string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Content-Type", v) }
}

var linkTokenRE = regexp.MustCompile(`#visitor_token=([0-9a-f]{64})`)

// waitSinkMail waits until the sink holds n messages and returns them.
func waitSinkMail(t *testing.T, k *smtpSink, n int) []sinkMsg {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m := k.messages(); len(m) >= n {
			return m
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("sink has %d messages, want %d", len(k.messages()), n)
	return nil
}

// assertNoMail checks that nothing more than want messages arrive.
func assertNoMail(t *testing.T, k *smtpSink, want int) {
	t.Helper()
	time.Sleep(1200 * time.Millisecond)
	if got := len(k.messages()); got != want {
		t.Errorf("sink has %d messages, want %d", got, want)
	}
}

func tokenFromMail(t *testing.T, m sinkMsg) string {
	t.Helper()
	g := linkTokenRE.FindStringSubmatch(m.Body)
	if g == nil {
		t.Fatalf("no sign-in token in mail:\n%s", m.Body)
	}
	return g[1]
}

// requestLink asks for a link and returns the token of the mail that follows.
func (e visitorEnv) requestLink(t *testing.T, email string) string {
	t.Helper()
	before := len(e.sink.messages())
	r := vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"`+email+`"}`)
	if r.code != http.StatusAccepted {
		t.Fatalf("link request = %d %s", r.code, r.body)
	}
	return tokenFromMail(t, waitSinkMail(t, e.sink, before+1)[before])
}

func visitorCookieOf(t *testing.T, r trustResp) *http.Cookie {
	t.Helper()
	resp := http.Response{Header: r.hdr}
	for _, c := range resp.Cookies() {
		if c.Name == "isms_trust_visitor" {
			return c
		}
	}
	return nil
}

func codeOfBody(r trustResp) string {
	var b struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(r.body), &b)
	return b.Code
}

type visitorJSON struct {
	Visitor *struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Company string `json:"company"`
	} `json:"visitor"`
}

func decodeVisitor(t *testing.T, r trustResp) visitorJSON {
	t.Helper()
	if r.code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", r.code, r.body)
	}
	var v visitorJSON
	if err := json.Unmarshal([]byte(r.body), &v); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
	return v
}

// signIn runs the whole flow and returns the cookie value.
func (e visitorEnv) signIn(t *testing.T, email string) string {
	t.Helper()
	tok := e.requestLink(t, email)
	r := vdo(e.s, http.MethodPost, e.path("/visitor/session"), `{"token":"`+tok+`","name":"Ada Auditor","company":"Audit Co"}`)
	c := visitorCookieOf(t, r)
	if r.code != http.StatusOK || c == nil {
		t.Fatalf("session = %d %s cookie=%v", r.code, r.body, c)
	}
	return c.Value
}

func TestTrustVisitorRoundTrip(t *testing.T) {
	e := visitorSetup(t)
	const email = "ada@auditco.test"

	r := vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"Ada@AuditCo.test"}`)
	if r.code != http.StatusAccepted || r.body != `{"status":"sent"}`+"\n" {
		t.Fatalf("link = %d %q", r.code, r.body)
	}
	msgs := waitSinkMail(t, e.sink, 1)
	if msgs[0].To != email {
		t.Errorf("mail to %q, want %q", msgs[0].To, email)
	}
	wantLink := "http://" + trustTestBase + "/" + e.org.slug + "/trust/sign-in#visitor_token="
	if !strings.Contains(msgs[0].Body, wantLink) || !strings.Contains(msgs[0].Body, "15 minutes") {
		t.Errorf("mail lacks the link prefix %q or the expiry note:\n%s", wantLink, msgs[0].Body)
	}
	tok := tokenFromMail(t, msgs[0])

	// inspect consumes nothing and says a profile is needed
	for i := 0; i < 2; i++ {
		r = vdo(e.s, http.MethodPost, e.path("/visitor/link/inspect"), `{"token":"`+tok+`"}`)
		var in struct {
			Email        string `json:"email"`
			NeedsProfile bool   `json:"needs_profile"`
		}
		if r.code != http.StatusOK || json.Unmarshal([]byte(r.body), &in) != nil || in.Email != email || !in.NeedsProfile {
			t.Fatalf("inspect %d = %d %s", i, r.code, r.body)
		}
	}

	// no profile: 409 and the link still works
	r = vdo(e.s, http.MethodPost, e.path("/visitor/session"), `{"token":"`+tok+`"}`)
	if r.code != http.StatusConflict || codeOfBody(r) != CodeTrustVisitorProfileRequired {
		t.Fatalf("no profile = %d %s", r.code, r.body)
	}
	if visitorCookieOf(t, r) != nil {
		t.Error("a cookie was set on the 409")
	}

	r = vdo(e.s, http.MethodPost, e.path("/visitor/session"), `{"token":"`+tok+`","name":" Ada Auditor ","company":"Audit Co"}`)
	v := decodeVisitor(t, r)
	if v.Visitor == nil || v.Visitor.Email != email || v.Visitor.Name != "Ada Auditor" || v.Visitor.Company != "Audit Co" {
		t.Fatalf("session body = %s", r.body)
	}
	c := visitorCookieOf(t, r)
	if c == nil || c.Value == "" {
		t.Fatalf("no visitor cookie; Set-Cookie = %v", r.hdr["Set-Cookie"])
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/v1/trust/"+e.org.slug ||
		c.MaxAge != 604800 || c.Secure {
		t.Errorf("cookie attributes = %+v", c)
	}
	if strings.Contains(r.body, c.Value) {
		t.Error("the session token is in the JSON body")
	}

	r = vdo(e.s, http.MethodGet, e.path("/visitor"), "", withCookie(c.Value))
	if v := decodeVisitor(t, r); v.Visitor == nil || v.Visitor.Email != email {
		t.Errorf("GET visitor = %s", r.body)
	}
	if r.hdr.Get("Cache-Control") != "no-store" {
		t.Error("GET visitor is cacheable")
	}

	// the link is single use
	r = vdo(e.s, http.MethodPost, e.path("/visitor/session"), `{"token":"`+tok+`","name":"x","company":"y"}`)
	if r.code != http.StatusBadRequest || codeOfBody(r) != CodeTrustLinkInvalid {
		t.Errorf("second redeem = %d %s", r.code, r.body)
	}
	r = vdo(e.s, http.MethodPost, e.path("/visitor/link/inspect"), `{"token":"`+tok+`"}`)
	if r.code != http.StatusBadRequest || codeOfBody(r) != CodeTrustLinkInvalid {
		t.Errorf("inspect after redeem = %d %s", r.code, r.body)
	}

	// sign-out
	r = vdo(e.s, http.MethodPost, e.path("/visitor/sign-out"), "", withCookie(c.Value))
	if v := decodeVisitor(t, r); v.Visitor != nil {
		t.Errorf("sign-out body = %s", r.body)
	}
	cleared := visitorCookieOf(t, r)
	if cleared == nil || cleared.Path != "/api/v1/trust/"+e.org.slug || cleared.MaxAge >= 0 {
		t.Errorf("cleared cookie = %+v", cleared)
	}
	for _, mods := range [][]func(*http.Request){nil, {withCookie(c.Value)}} {
		r = vdo(e.s, http.MethodGet, e.path("/visitor"), "", mods...)
		if v := decodeVisitor(t, r); v.Visitor != nil {
			t.Errorf("GET visitor after sign-out = %s", r.body)
		}
	}
	// sign-out with no session is fine
	if r = vdo(e.s, http.MethodPost, e.path("/visitor/sign-out"), ""); r.code != http.StatusOK {
		t.Errorf("sign-out without a cookie = %d", r.code)
	}

	// a returning visitor is not asked again
	tok2 := e.requestLink(t, email)
	r = vdo(e.s, http.MethodPost, e.path("/visitor/link/inspect"), `{"token":"`+tok2+`"}`)
	if !strings.Contains(r.body, `"needs_profile":false`) {
		t.Errorf("returning inspect = %s", r.body)
	}
	r = vdo(e.s, http.MethodPost, e.path("/visitor/session"), `{"token":"`+tok2+`"}`)
	if v := decodeVisitor(t, r); v.Visitor == nil || v.Visitor.Name != "Ada Auditor" {
		t.Errorf("returning sign-in = %s", r.body)
	}
}

func TestTrustVisitorSecureCookieOnHTTPSBase(t *testing.T) {
	e := visitorSetup(t)
	t.Setenv("ISMS_BASE_URL", "https://"+trustTestBase)
	tok := e.requestLink(t, "sec@auditco.test")
	r := vdo(e.s, http.MethodPost, e.path("/visitor/session"), `{"token":"`+tok+`","name":"A","company":"B"}`)
	if c := visitorCookieOf(t, r); c == nil || !c.Secure {
		t.Errorf("cookie = %+v, want Secure on an https base", c)
	}
}

func TestTrustVisitorLinkAnswerIsUniform(t *testing.T) {
	e := visitorSetup(t)
	ctx := context.Background()

	// A known visitor and a blocked one.
	e.signIn(t, "known@auditco.test")
	e.signIn(t, "blocked@auditco.test")
	blocked, err := e.s.db.GetTrustVisitorByEmail(ctx, e.org.id, "blocked@auditco.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.db.SetTrustVisitorBlocked(ctx, e.org.id, blocked.ID, true, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	sent := len(e.sink.messages()) // the two sign-in mails

	send := func(email string) trustResp {
		return vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"`+email+`"}`)
	}
	base := send("unknown@auditco.test")
	if base.code != http.StatusAccepted {
		t.Fatalf("unknown = %d %s", base.code, base.body)
	}
	waitSinkMail(t, e.sink, sent+1)
	known := send("known@auditco.test")
	waitSinkMail(t, e.sink, sent+2)
	sent += 2
	blockedResp := send("blocked@auditco.test")
	// per-email cap: three links in 15 minutes already exist for this address
	for i := 0; i < 3; i++ {
		_, h := newHashedToken()
		if err := e.s.db.CreateTrustVisitorLink(ctx, e.org.id, "capped@auditco.test", h, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	capped := send("capped@auditco.test")

	for name, r := range map[string]trustResp{"known": known, "blocked": blockedResp, "capped": capped} {
		if r.code != base.code || r.body != base.body {
			t.Errorf("%s answer = %d %q, want %d %q", name, r.code, r.body, base.code, base.body)
		}
	}
	assertNoMail(t, e.sink, sent)

	// Per-org hourly cap: silent too.
	t.Setenv("ISMS_TRUST_SIGNIN_ORG_HOURLY", "1")
	r := send("another@auditco.test")
	if r.code != base.code || r.body != base.body {
		t.Errorf("org-capped answer = %d %q", r.code, r.body)
	}
	assertNoMail(t, e.sink, sent)
}

func TestTrustVisitorBlockedDomain(t *testing.T) {
	e := visitorSetup(t)
	r := vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"someone@gmail.com"}`)
	if r.code != http.StatusBadRequest || codeOfBody(r) != CodeTrustEmailDomainBlocked || !strings.Contains(r.body, "gmail.com") {
		t.Fatalf("gmail with the default list = %d %s", r.code, r.body)
	}
	assertNoMail(t, e.sink, 0)

	// An explicit empty list allows every domain.
	rawExec(t, `UPDATE trust_center_settings SET blocked_email_domains = '{}' WHERE organization_id = $1`, e.org.id)
	r = vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"someone@gmail.com"}`)
	if r.code != http.StatusAccepted {
		t.Fatalf("gmail with an empty list = %d %s", r.code, r.body)
	}
	waitSinkMail(t, e.sink, 1)
}

func TestTrustVisitorEmailValidation(t *testing.T) {
	e := visitorSetup(t)
	long := strings.Repeat("a", 320) + "@auditco.test"
	for _, em := range []string{"", "not-an-email", "Ada <ada@auditco.test>", "a@b@c.test", "a@[127.0.0.1]", "a@[IPv6:::1]", "a@localhost", long} {
		r := vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":`+jsonString(em)+`}`)
		if r.code != http.StatusBadRequest {
			t.Errorf("email %q = %d %s, want 400", em, r.code, r.body)
		}
	}
	if r := vdo(e.s, http.MethodPost, e.path("/visitor/link"), `not json`); r.code != http.StatusBadRequest {
		t.Errorf("bad json = %d", r.code)
	}
	assertNoMail(t, e.sink, 0)
}

func TestTrustVisitorNoMailer(t *testing.T) {
	e := visitorSetup(t)
	e.s.mailer = nil

	r := vdo(e.s, http.MethodGet, e.path(""), "")
	var ov map[string]any
	if err := json.Unmarshal([]byte(r.body), &ov); err != nil || r.code != 200 {
		t.Fatalf("overview = %d %s", r.code, r.body)
	}
	if v, ok := ov["visitor_sign_in"]; !ok || v != false {
		t.Errorf("visitor_sign_in = %v (present %v), want false", v, ok)
	}
	if _, ok := ov["privacy_notice_url"]; !ok {
		t.Error("overview lacks privacy_notice_url")
	}
	r = vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"a@auditco.test"}`)
	if r.code != http.StatusServiceUnavailable || codeOfBody(r) != CodeTrustSignInUnavailable {
		t.Errorf("link without a mailer = %d %s", r.code, r.body)
	}

	// With a mailer the flag is true.
	e.s.mailer = e.sink.mailer()
	r = vdo(e.s, http.MethodGet, e.path(""), "")
	if !strings.Contains(r.body, `"visitor_sign_in":true`) {
		t.Errorf("overview with mailer = %s", r.body)
	}
}

func TestTrustVisitorContainment(t *testing.T) {
	e := visitorSetup(t)
	ctx := context.Background()
	orgB := trustNewOrg(t, e.s, "visb", true)
	const email = "contained@auditco.test"
	cookie := e.signIn(t, email)

	// The visitor cookie value is the session token, so it is exactly what an
	// attacker could try as a credential.
	for _, p := range []string{"/api/v1/risks", "/api/v1/users", "/api/v1/trust-center/settings"} {
		if r := vdo(e.s, http.MethodGet, p, "", withBearer(cookie)); r.code != http.StatusUnauthorized {
			t.Errorf("visitor token as Bearer on %s = %d, want 401", p, r.code)
		}
		if r := vdo(e.s, http.MethodGet, p, "", withCookie(cookie)); r.code != http.StatusUnauthorized {
			t.Errorf("visitor cookie on %s = %d, want 401", p, r.code)
		}
	}
	basic := func(r *http.Request) { r.SetBasicAuth("x-token-auth", cookie) }
	if r := vdo(e.s, http.MethodGet, "/git/00000000-0000-0000-0000-000000000000/info/refs", "", basic); r.code != http.StatusUnauthorized {
		t.Errorf("visitor token as Basic password on /git = %d, want 401", r.code)
	}

	// A member JWT on the trust prefix grants no visitor access.
	u := contractTestUser(t, e.s, e.org.id, "member@vis.test", "admin")
	jwt, err := e.s.createSessionJWT(u, e.org.id, "admin", e.org.slug, "Org")
	if err != nil {
		t.Fatal(err)
	}
	r := vdo(e.s, http.MethodGet, e.path("/visitor"), "", withBearer(jwt))
	if v := decodeVisitor(t, r); v.Visitor != nil {
		t.Errorf("member JWT as Bearer on /visitor = %s", r.body)
	}
	r = vdo(e.s, http.MethodGet, e.path("/visitor"), "", withCookie(jwt))
	if v := decodeVisitor(t, r); v.Visitor != nil {
		t.Errorf("member JWT as visitor cookie = %s", r.body)
	}
	// ...and a visitor token as Bearer on /visitor is not read either.
	r = vdo(e.s, http.MethodGet, e.path("/visitor"), "", withBearer(cookie))
	if v := decodeVisitor(t, r); v.Visitor != nil {
		t.Errorf("visitor token as Bearer on /visitor = %s", r.body)
	}

	// Org A's session is not valid in org B.
	r = vdo(e.s, http.MethodGet, "/api/v1/trust/"+orgB.slug+"/visitor", "", withCookie(cookie))
	if v := decodeVisitor(t, r); v.Visitor != nil {
		t.Errorf("org A cookie in org B = %s", r.body)
	}
	r = vdo(e.s, http.MethodGet, e.path("/visitor"), "", withCookie(cookie))
	if v := decodeVisitor(t, r); v.Visitor == nil {
		t.Error("org A cookie stopped working in org A")
	}

	// A visitor is never a user or a member.
	if _, err := e.s.db.GetUserByEmail(ctx, email); err == nil {
		t.Error("sign-in created a users row")
	}
	members, err := e.s.db.ListOrgUsers(ctx, e.org.id)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if strings.EqualFold(m.Email, email) {
			t.Errorf("visitor is an organization member: %+v", m)
		}
	}
}

func TestTrustVisitorLinkFromOtherOrgIsInvalid(t *testing.T) {
	e := visitorSetup(t)
	orgB := trustNewOrg(t, e.s, "visb2", true)
	tok := e.requestLink(t, "cross@auditco.test")
	for _, p := range []string{"/visitor/link/inspect", "/visitor/session"} {
		r := vdo(e.s, http.MethodPost, "/api/v1/trust/"+orgB.slug+p, `{"token":"`+tok+`","name":"A","company":"B"}`)
		if r.code != http.StatusBadRequest || codeOfBody(r) != CodeTrustLinkInvalid {
			t.Errorf("%s with org A's token in org B = %d %s", p, r.code, r.body)
		}
	}
}

func TestTrustVisitorBlockedVisitorCannotSignIn(t *testing.T) {
	e := visitorSetup(t)
	ctx := context.Background()
	cookie := e.signIn(t, "bv@auditco.test")
	v, _ := e.s.db.GetTrustVisitorByEmail(ctx, e.org.id, "bv@auditco.test")
	// A link issued before the block stays in the inbox.
	tok := e.requestLink(t, "bv@auditco.test")
	if _, err := e.s.db.SetTrustVisitorBlocked(ctx, e.org.id, v.ID, true, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	// The block revokes the session at once.
	if r := vdo(e.s, http.MethodGet, e.path("/visitor"), "", withCookie(cookie)); decodeVisitor(t, r).Visitor != nil {
		t.Error("blocked visitor's session still valid")
	}
	for _, p := range []string{"/visitor/link/inspect", "/visitor/session"} {
		r := vdo(e.s, http.MethodPost, e.path(p), `{"token":"`+tok+`"}`)
		if r.code != http.StatusForbidden || codeOfBody(r) != CodeTrustVisitorBlocked {
			t.Errorf("%s for a blocked visitor = %d %s", p, r.code, r.body)
		}
	}
}

func TestTrustVisitorMailIsEscapedAndBranded(t *testing.T) {
	e := visitorSetup(t)
	if err := e.s.db.SetOrgSetting(context.Background(), e.org.id, "branding_name", "<script>x</script>"); err != nil {
		t.Fatal(err)
	}
	e.requestLink(t, "esc@auditco.test")
	// The brand is also in the From and Subject headers, as text; only the
	// HTML body must have it escaped.
	m := e.sink.messages()[0]
	_, html, _ := strings.Cut(m.Body, "\n\n")
	m.Body = html
	if strings.Contains(m.Body, "<script>x</script>") || !strings.Contains(m.Body, "&lt;script&gt;x&lt;/script&gt;") {
		t.Errorf("brand name not escaped in the mail:\n%s", m.Body)
	}
}

func TestTrustVisitorMethodAndPathHygiene(t *testing.T) {
	e := visitorSetup(t)
	notFound := vdo(e.s, http.MethodGet, "/api/v1/trust/no-such-org-zz/visitor", "")
	if notFound.code != http.StatusNotFound {
		t.Fatalf("baseline = %d", notFound.code)
	}
	cases := []struct{ method, rest string }{
		{http.MethodPut, "/visitor"}, {http.MethodDelete, "/visitor"}, {http.MethodPost, "/visitor"},
		{http.MethodPut, "/visitor/link"}, {http.MethodDelete, "/visitor/session"},
		{http.MethodGet, "/visitor/link"}, {http.MethodGet, "/visitor/session"},
		{http.MethodGet, "/visitor/sign-out"}, {http.MethodGet, "/visitor/link/inspect"},
		{http.MethodPost, ""}, {http.MethodPost, "/items"}, {http.MethodPost, "/visitor/other"},
		{http.MethodPost, "/visitor/link/extra"}, {http.MethodPost, "/visitor/link/inspect/x"},
		{http.MethodPatch, "/visitor/sign-out"},
	}
	for _, c := range cases {
		r := vdo(e.s, c.method, e.path(c.rest), `{}`)
		if r.code != http.StatusNotFound || r.body != notFound.body {
			t.Errorf("%s %s = %d %s, want the single 404", c.method, c.rest, r.code, r.body)
		}
	}

	// non-JSON POSTs
	for _, rest := range []string{"/visitor/link", "/visitor/link/inspect", "/visitor/session", "/visitor/sign-out"} {
		for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", ""} {
			req := httptest.NewRequest(http.MethodPost, "http://"+trustTestBase+e.path(rest), strings.NewReader(`email=a%40b.test`))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			rec := httptest.NewRecorder()
			e.s.echo.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("POST %s as %q = %d, want 400", rest, ct, rec.Code)
			}
		}
	}
	assertNoMail(t, e.sink, 0)

	// Trust center off: every visitor route is the single 404.
	off := trustNewOrg(t, e.s, "visoff", false)
	for _, c := range []struct{ method, rest string }{
		{http.MethodGet, "/visitor"}, {http.MethodPost, "/visitor/link"}, {http.MethodPost, "/visitor/link/inspect"},
		{http.MethodPost, "/visitor/session"}, {http.MethodPost, "/visitor/sign-out"},
	} {
		r := vdo(e.s, c.method, "/api/v1/trust/"+off.slug+c.rest, `{"email":"a@auditco.test","token":"x"}`)
		if r.code != http.StatusNotFound || r.body != notFound.body {
			t.Errorf("trust center off: %s %s = %d %s", c.method, c.rest, r.code, r.body)
		}
	}
	// Host/slug mismatch: org B's custom domain with org A's slug.
	orgB := trustNewOrg(t, e.s, "vishost", true)
	req := httptest.NewRequest(http.MethodPost, "http://"+orgB.domain+e.path("/visitor/link"), strings.NewReader(`{"email":"a@auditco.test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.s.echo.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("host mismatch = %d, want 404", rec.Code)
	}
}

func TestTrustVisitorLinkRequestsArePerIPLimited(t *testing.T) {
	e := visitorSetup(t)
	post := func() trustResp {
		return vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"someone@gmail.com"}`) // refused fast, no mail
	}
	for i := 1; i <= 10; i++ {
		if r := post(); r.code != http.StatusBadRequest {
			t.Fatalf("request %d = %d, want 400", i, r.code)
		}
	}
	if r := post(); r.code != http.StatusTooManyRequests {
		t.Errorf("request 11 = %d, want 429", r.code)
	}
	// GETs and the other POST routes are not counted.
	if r := vdo(e.s, http.MethodGet, e.path("/visitor"), ""); r.code != http.StatusOK {
		t.Errorf("GET visitor after the limit = %d", r.code)
	}
	if r := vdo(e.s, http.MethodPost, e.path("/visitor/sign-out"), ""); r.code != http.StatusOK {
		t.Errorf("sign-out after the limit = %d", r.code)
	}
}

func TestTrustVisitorPreviewHasNewFieldsAndIgnoresTheCookie(t *testing.T) {
	e := visitorSetup(t)
	cookie := e.signIn(t, "prev@auditco.test")
	u := contractTestUser(t, e.s, e.org.id, "mgr@vis.test", "manager")
	jwt, _ := e.s.createSessionJWT(u, e.org.id, "manager", e.org.slug, "Org")
	r := vdo(e.s, http.MethodGet, "/api/v1/trust-center/preview", "", withBearer(jwt), withCookie(cookie))
	if r.code != http.StatusOK || !strings.Contains(r.body, `"visitor_sign_in":true`) || !strings.Contains(r.body, `"privacy_notice_url"`) {
		t.Errorf("preview = %d %s", r.code, r.body)
	}
}

var _ = db.TrustAccessGated

// R1: the caps hold under parallel requests, not just sequential ones.
func TestTrustVisitorLinkRequestCapsHoldInParallel(t *testing.T) {
	e := visitorSetup(t)
	e.s.trustLinkLimit = &trustLinkLimiter{} // the per-IP limiter is not under test; a spoofed IP bypasses it anyway
	const n = 20
	codes := make([]int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i] = vdo(e.s, http.MethodPost, e.path("/visitor/link"), `{"email":"burst@auditco.test"}`).code
		}(i)
	}
	close(start)
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusAccepted {
			t.Errorf("request %d = %d, want 202", i, c)
		}
	}
	waitSinkMail(t, e.sink, 3)
	assertNoMail(t, e.sink, 3)
}
