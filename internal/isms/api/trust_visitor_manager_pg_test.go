package api

// Trust center manager API for visitors and the sign-in settings (#524),
// through the real middleware chain with member JWTs. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustVisitorMgr

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type mgrEnv struct {
	visitorEnv
	jwt map[string]string // role -> member token
}

func mgrSetup(t *testing.T) mgrEnv {
	t.Helper()
	m := mgrEnv{visitorEnv: visitorSetup(t), jwt: map[string]string{}}
	for _, role := range []string{"admin", "manager", "contributor", "reader"} {
		u := contractTestUser(t, m.s, m.org.id, role+"@vmgr.test", role)
		tok, err := m.s.createSessionJWT(u, m.org.id, role, m.org.slug, "Org")
		if err != nil {
			t.Fatal(err)
		}
		m.jwt[role] = tok
	}
	return m
}

const tcAPI = "/api/v1/trust-center"

func (m mgrEnv) do(role, method, path, body string) trustResp {
	return vdo(m.s, method, tcAPI+path, body, withBearer(m.jwt[role]))
}

type settingsView struct {
	Domains        []string `json:"blocked_email_domains"`
	DomainsDefault bool     `json:"blocked_email_domains_default"`
	BuiltIn        []string `json:"default_blocked_email_domains"`
	PrivacyURL     string   `json:"privacy_notice_url"`
	EmailConfig    bool     `json:"email_configured"`
}

func decodeVSettings(t *testing.T, r trustResp) settingsView {
	t.Helper()
	if r.code != http.StatusOK {
		t.Fatalf("settings = %d %s", r.code, r.body)
	}
	var v settingsView
	if err := json.Unmarshal([]byte(r.body), &v); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.body, `"blocked_email_domains":null`) {
		t.Errorf("effective list is null: %s", r.body)
	}
	return v
}

func TestTrustVisitorMgrDomainSettings(t *testing.T) {
	m := mgrSetup(t)
	st := decodeVSettings(t, m.do("manager", http.MethodGet, "/settings", ""))
	if !st.DomainsDefault || len(st.Domains) == 0 || strings.Join(st.Domains, ",") != strings.Join(st.BuiltIn, ",") ||
		!contains(st.Domains, "gmail.com") {
		t.Fatalf("default settings = %+v", st)
	}

	st = decodeVSettings(t, m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":["Rival.TEST","@Example.com","example.com"," zeta.test ","alpha.test"]}`))
	if st.DomainsDefault || strings.Join(st.Domains, ",") != "alpha.test,example.com,rival.test,zeta.test" {
		t.Fatalf("normalised = %+v", st)
	}
	if got := decodeVSettings(t, m.do("manager", http.MethodGet, "/settings", "")); strings.Join(got.Domains, ",") != strings.Join(st.Domains, ",") {
		t.Errorf("GET after PUT = %+v", got)
	}
	// an unrelated PUT leaves the list alone
	st = decodeVSettings(t, m.do("manager", http.MethodPut, "/settings", `{"title":"T"}`))
	if st.DomainsDefault || len(st.Domains) != 4 {
		t.Errorf("list changed by an unrelated update: %+v", st)
	}

	// [] allows every domain, and gmail can then ask for a link
	st = decodeVSettings(t, m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":[]}`))
	if st.DomainsDefault || len(st.Domains) != 0 {
		t.Fatalf("empty list = %+v", st)
	}
	r := vdo(m.s, http.MethodPost, m.path("/visitor/link"), `{"email":"`+newGmail()+`"}`)
	if r.code != http.StatusAccepted {
		t.Errorf("gmail with an empty list = %d %s", r.code, r.body)
	}
	waitSinkMail(t, m.sink, 1)

	// null goes back to the built-in list
	st = decodeVSettings(t, m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":null}`))
	if !st.DomainsDefault || !contains(st.Domains, "gmail.com") {
		t.Errorf("null = %+v", st)
	}
	if r := vdo(m.s, http.MethodPost, m.path("/visitor/link"), `{"email":"`+newGmail()+`"}`); r.code != http.StatusBadRequest {
		t.Errorf("gmail with the default list again = %d", r.code)
	}

	// invalid entries
	many := make([]string, 501)
	for i := range many {
		many[i] = "d" + strconv.Itoa(i) + ".test"
	}
	manyJSON, _ := json.Marshal(many)
	for name, body := range map[string]string{
		"at sign": `["a@b.com"]`, "no dot": `["nodot"]`, "blank": `[" "]`, "empty": `[""]`, "slash": `["x/y.com"]`,
		"inner space": `["a b.com"]`, "too long": `["` + strings.Repeat("a", 250) + `.com"]`, "501 entries": string(manyJSON),
		"not a list": `"gmail.com"`, "not strings": `[1]`,
	} {
		if r := m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":`+body+`}`); r.code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, r.code, r.body)
		}
	}
	if st := decodeVSettings(t, m.do("manager", http.MethodGet, "/settings", "")); !st.DomainsDefault {
		t.Error("a rejected update changed the list")
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestTrustVisitorMgrPrivacyNoticeURL(t *testing.T) {
	m := mgrSetup(t)
	r := m.do("manager", http.MethodPut, "/settings", `{"privacy_notice_url":"https://acme.test/privacy?x=1"}`)
	if st := decodeVSettings(t, r); st.PrivacyURL != "https://acme.test/privacy?x=1" {
		t.Fatalf("set = %+v", st)
	}
	if ov := vdo(m.s, http.MethodGet, m.path(""), ""); !strings.Contains(ov.body, `"privacy_notice_url":"https://acme.test/privacy?x=1"`) {
		t.Errorf("public overview = %s", ov.body)
	}
	if st := decodeVSettings(t, m.do("manager", http.MethodPut, "/settings", `{"title":"x"}`)); st.PrivacyURL == "" {
		t.Error("unrelated update cleared the URL")
	}
	if st := decodeVSettings(t, m.do("manager", http.MethodPut, "/settings", `{"privacy_notice_url":""}`)); st.PrivacyURL != "" {
		t.Errorf("clear = %+v", st)
	}
	if ov := vdo(m.s, http.MethodGet, m.path(""), ""); !strings.Contains(ov.body, `"privacy_notice_url":""`) {
		t.Errorf("overview after clear = %s", ov.body)
	}
	for _, bad := range []string{"javascript:alert(1)", "ftp://x.test/p", "/relative", "https://", "http:///nohost", "acme.test/privacy", "https://a b.test", strings.Repeat("a", 2001)} {
		r := m.do("manager", http.MethodPut, "/settings", `{"privacy_notice_url":`+jsonString(bad)+`}`)
		if r.code != http.StatusBadRequest {
			t.Errorf("%.40q = %d, want 400", bad, r.code)
		}
	}
	if r := m.do("manager", http.MethodPut, "/settings", `{"privacy_notice_url":"http://acme.test/p"}`); r.code != 200 {
		t.Errorf("http URL = %d", r.code)
	}
}

func TestTrustVisitorMgrSettingsChangelog(t *testing.T) {
	m := mgrSetup(t)
	ctx := context.Background()
	rows := func() map[string][]string {
		cl, err := m.s.db.ListEntityChangelog(ctx, m.org.id, "trust_center_settings", int64(m.org.id))
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][]string{}
		for i := len(cl) - 1; i >= 0; i-- { // the log lists newest first; read it oldest first
			c := cl[i]
			if c.Field == "blocked_email_domains" || c.Field == "privacy_notice_url" {
				o, n := "", ""
				if c.OldValue != nil {
					o = *c.OldValue
				}
				if c.NewValue != nil {
					n = *c.NewValue
				}
				out[c.Field] = append(out[c.Field], o+" -> "+n)
			}
		}
		return out
	}
	m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":["b.test","a.test"],"privacy_notice_url":"https://a.test/p"}`)
	got := rows()
	if len(got["blocked_email_domains"]) != 1 || got["blocked_email_domains"][0] != "(default) -> a.test,b.test" ||
		len(got["privacy_notice_url"]) != 1 || got["privacy_notice_url"][0] != " -> https://a.test/p" {
		t.Fatalf("changelog = %v", got)
	}
	// a no-op PUT (same list in another order, same URL) writes nothing
	m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":["A.test","b.test"],"privacy_notice_url":"https://a.test/p"}`)
	if got := rows(); len(got["blocked_email_domains"]) != 1 || len(got["privacy_notice_url"]) != 1 {
		t.Errorf("no-op PUT wrote rows: %v", got)
	}
	// back to default, and NULL -> the identical built-in list is a change
	m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":null}`)
	if got := rows()["blocked_email_domains"]; len(got) != 2 || got[1] != "a.test,b.test -> (default)" {
		t.Errorf("reset log = %v", got)
	}
	builtin, _ := json.Marshal(decodeVSettings(t, m.do("manager", http.MethodGet, "/settings", "")).BuiltIn)
	m.do("manager", http.MethodPut, "/settings", `{"blocked_email_domains":`+string(builtin)+`}`)
	got = rows()
	if n := got["blocked_email_domains"]; len(n) != 3 || !strings.HasPrefix(n[2], "(default) -> ") {
		t.Errorf("NULL -> built-in list was not logged: %v", n)
	}
	if st := decodeVSettings(t, m.do("manager", http.MethodGet, "/settings", "")); st.DomainsDefault {
		t.Error("an explicit list equal to the built-in one reads as default")
	}
}

func TestTrustVisitorMgrEmailConfigured(t *testing.T) {
	m := mgrSetup(t)
	if !decodeVSettings(t, m.do("manager", http.MethodGet, "/settings", "")).EmailConfig {
		t.Error("email_configured false with a mailer")
	}
	m.s.mailer = nil
	if decodeVSettings(t, m.do("manager", http.MethodGet, "/settings", "")).EmailConfig {
		t.Error("email_configured true without a mailer")
	}
	if decodeVSettings(t, m.do("manager", http.MethodPut, "/settings", `{"title":"x"}`)).EmailConfig {
		t.Error("PUT: email_configured true without a mailer")
	}
}

type visitorRow struct {
	ID             int64   `json:"id"`
	Email          string  `json:"email"`
	Name           string  `json:"name"`
	Company        string  `json:"company"`
	VerifiedAt     *int64  `json:"verified_at"`
	FirstSeen      int64   `json:"first_seen"`
	LastSeen       int64   `json:"last_seen"`
	Blocked        bool    `json:"blocked"`
	BlockedAt      *int64  `json:"blocked_at"`
	BlockedBy      *string `json:"blocked_by"`
	ActiveSessions int     `json:"active_sessions"`
}

func (m mgrEnv) visitors(t *testing.T) []visitorRow {
	t.Helper()
	r := m.do("manager", http.MethodGet, "/visitors", "")
	if r.code != http.StatusOK {
		t.Fatalf("list = %d %s", r.code, r.body)
	}
	var rows []visitorRow
	if err := json.Unmarshal([]byte(r.body), &rows); err != nil || rows == nil {
		t.Fatalf("list body %q: %v", r.body, err)
	}
	return rows
}

func TestTrustVisitorMgrListRevokeBlock(t *testing.T) {
	m := mgrSetup(t)
	if got := m.visitors(t); len(got) != 0 {
		t.Fatalf("fresh list = %v", got)
	}
	cookie := m.signIn(t, "ada@auditco.test")
	cookie2 := m.signIn(t, "bob@auditco.test")

	rows := m.visitors(t)
	if len(rows) != 2 || rows[0].Email != "bob@auditco.test" {
		t.Fatalf("list = %+v, want bob (seen last) first", rows)
	}
	ada := rows[1]
	if ada.Email != "ada@auditco.test" || ada.Name != "Ada Auditor" || ada.Company != "Audit Co" || ada.ActiveSessions != 1 ||
		ada.Blocked || ada.VerifiedAt == nil || ada.FirstSeen == 0 || ada.LastSeen == 0 || ada.BlockedAt != nil || ada.BlockedBy != nil {
		t.Errorf("row = %+v", ada)
	}
	id := strconv.FormatInt(ada.ID, 10)

	// revoke sessions
	r := m.do("manager", http.MethodPost, "/visitors/"+id+"/revoke-sessions", "")
	if r.code != http.StatusOK || strings.TrimSpace(r.body) != `{"revoked":1}` {
		t.Fatalf("revoke = %d %s", r.code, r.body)
	}
	if v := decodeVisitor(t, vdo(m.s, http.MethodGet, m.path("/visitor"), "", withCookie(cookie))); v.Visitor != nil {
		t.Error("cookie still valid after revoke")
	}
	if r := m.do("manager", http.MethodPost, "/visitors/"+id+"/revoke-sessions", ""); strings.TrimSpace(r.body) != `{"revoked":0}` {
		t.Errorf("second revoke = %s", r.body)
	}

	// block
	bobID := strconv.FormatInt(rows[0].ID, 10)
	r = m.do("manager", http.MethodPut, "/visitors/"+bobID+"/blocked", `{"blocked":true}`)
	var bob visitorRow
	if err := json.Unmarshal([]byte(r.body), &bob); r.code != 200 || err != nil || !bob.Blocked || bob.BlockedBy == nil || *bob.BlockedBy != "manager@vmgr.test" || bob.BlockedAt == nil || bob.ActiveSessions != 0 {
		t.Fatalf("block = %d %s", r.code, r.body)
	}
	if v := decodeVisitor(t, vdo(m.s, http.MethodGet, m.path("/visitor"), "", withCookie(cookie2))); v.Visitor != nil {
		t.Error("cookie still valid after a block")
	}
	before := len(m.sink.messages())
	if r := vdo(m.s, http.MethodPost, m.path("/visitor/link"), `{"email":"bob@auditco.test"}`); r.code != http.StatusAccepted {
		t.Errorf("link for a blocked visitor = %d", r.code)
	}
	assertNoMail(t, m.sink, before)
	for _, v := range m.visitors(t) {
		if v.Email == "bob@auditco.test" && (!v.Blocked || v.BlockedBy == nil) {
			t.Errorf("list row after block = %+v", v)
		}
	}

	// unblock does not revive the old session
	r = m.do("manager", http.MethodPut, "/visitors/"+bobID+"/blocked", `{"blocked":false}`)
	if err := json.Unmarshal([]byte(r.body), &bob); r.code != 200 || err != nil || bob.Blocked || bob.BlockedBy != nil {
		t.Fatalf("unblock = %d %s", r.code, r.body)
	}
	if v := decodeVisitor(t, vdo(m.s, http.MethodGet, m.path("/visitor"), "", withCookie(cookie2))); v.Visitor != nil {
		t.Error("unblock revived the old session")
	}

	// bad bodies
	for _, body := range []string{``, `{}`, `{"blocked":"yes"}`, `nope`} {
		if r := m.do("manager", http.MethodPut, "/visitors/"+bobID+"/blocked", body); r.code != http.StatusBadRequest {
			t.Errorf("body %q = %d, want 400", body, r.code)
		}
	}
}

func TestTrustVisitorMgrIDsAndRoles(t *testing.T) {
	m := mgrSetup(t)
	other := trustNewOrg(t, m.s, "vmgro", true)
	otherEnv := visitorEnv{s: m.s, sink: m.sink, org: other}
	otherEnv.signIn(t, "ada@auditco.test")
	ov, err := m.s.db.GetTrustVisitorByEmail(context.Background(), other.id, "ada@auditco.test")
	if err != nil {
		t.Fatal(err)
	}
	m.signIn(t, "mine@auditco.test")
	foreign := strconv.FormatInt(ov.ID, 10)

	for _, id := range []string{foreign, "999999999", "abc", "-1", "1.5"} {
		for _, c := range []struct{ method, suffix, body string }{
			{http.MethodPost, "/revoke-sessions", ""}, {http.MethodPut, "/blocked", `{"blocked":true}`},
		} {
			r := m.do("manager", c.method, "/visitors/"+id+c.suffix, c.body)
			if r.code != http.StatusNotFound || codeOfBody(r) != CodeNotFound {
				t.Errorf("%s %s%s = %d %s, want 404", c.method, id, c.suffix, r.code, r.body)
			}
		}
	}
	// the foreign visitor was not touched
	if v, _ := m.s.db.GetTrustVisitorByEmail(context.Background(), other.id, "ada@auditco.test"); v.Blocked || v.ActiveSessions != 1 {
		t.Errorf("foreign visitor changed: %+v", v)
	}

	mine := strconv.FormatInt(m.visitors(t)[0].ID, 10)
	for _, role := range []string{"contributor", "reader"} {
		for _, c := range []struct{ method, path, body string }{
			{http.MethodGet, "/visitors", ""},
			{http.MethodPost, "/visitors/" + mine + "/revoke-sessions", ""},
			{http.MethodPut, "/visitors/" + mine + "/blocked", `{"blocked":true}`},
		} {
			if r := m.do(role, c.method, c.path, c.body); r.code != http.StatusForbidden {
				t.Errorf("%s %s %s = %d, want 403", role, c.method, c.path, r.code)
			}
		}
	}
	if r := m.do("admin", http.MethodGet, "/visitors", ""); r.code != 200 {
		t.Errorf("admin list = %d", r.code)
	}
	if r := vdo(m.s, http.MethodGet, tcAPI+"/visitors", ""); r.code != http.StatusUnauthorized {
		t.Errorf("anonymous list = %d, want 401", r.code)
	}
	// no changelog rows for visitor actions
	vdo(m.s, http.MethodPut, tcAPI+"/visitors/"+mine+"/blocked", `{"blocked":true}`, withBearer(m.jwt["manager"]))
	cl, _ := m.s.db.ListEntityChangelog(context.Background(), m.org.id, "trust_center_settings", int64(m.org.id))
	if len(cl) != 0 {
		t.Errorf("visitor action wrote %d changelog rows", len(cl))
	}
}

func TestTrustVisitorMgrVisitorsAreNotUsers(t *testing.T) {
	m := mgrSetup(t)
	const email = "ghost@auditco.test"
	m.signIn(t, email)
	u := vdo(m.s, http.MethodGet, "/api/v1/users", "", withBearer(m.jwt["admin"]))
	if u.code != http.StatusOK || strings.Contains(strings.ToLower(u.body), email) {
		t.Errorf("GET /users = %d, contains the visitor: %v", u.code, strings.Contains(u.body, email))
	}
}
