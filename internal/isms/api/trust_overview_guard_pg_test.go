package api

// The overview document is a public element: it is served only when it is not
// also published at a level that is not public. And the sign-in send cap shows
// as a paused-until time in the settings. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run 'TrustOverviewGuard|TrustSignInPaused'

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

type guardFx struct {
	f   *tdFx
	jwt string
	id  string // public id of the overview document's item, once published
}

func newGuardFx(t *testing.T, tag string) *guardFx {
	t.Helper()
	f := newTDFx(t, tag, "pol-ov")
	f.save("# Overview\n\nOVERVIEW BODY TEXT\n")
	f.approve()
	u := contractTestUser(t, f.s, f.o.id, "mgr-"+tag+"@guard.test", "manager")
	jwt, err := f.s.createSessionJWT(u, f.o.id, "manager", f.o.slug, "Org")
	if err != nil {
		t.Fatal(err)
	}
	return &guardFx{f: f, jwt: jwt}
}

func (g *guardFx) put(body string) trustResp {
	return vdo(g.f.s, http.MethodPut, tcAPI+"/settings", body, withBearer(g.jwt))
}

type guardSettings struct {
	OverviewID *string `json:"overview_document_id"`
	Withheld   bool    `json:"overview_withheld"`
}

func (g *guardFx) settings(t *testing.T) guardSettings {
	t.Helper()
	r := vdo(g.f.s, http.MethodGet, tcAPI+"/settings", "", withBearer(g.jwt))
	var s guardSettings
	if r.code != 200 || json.Unmarshal([]byte(r.body), &s) != nil || !strings.Contains(r.body, `"overview_withheld"`) {
		t.Fatalf("settings = %d %s", r.code, r.body)
	}
	return s
}

// overviews returns the public and the preview overview answers.
func (g *guardFx) overviews() (pub, prev trustResp) {
	return vdo(g.f.s, http.MethodGet, "/api/v1/trust/"+g.f.o.slug, ""),
		vdo(g.f.s, http.MethodGet, tcAPI+"/preview", "", withBearer(g.jwt))
}

func (g *guardFx) wantServed(t *testing.T, why string, served bool) {
	t.Helper()
	pub, prev := g.overviews()
	for name, r := range map[string]trustResp{"public": pub, "preview": prev} {
		if r.code != 200 {
			t.Fatalf("%s %s: %d %s", why, name, r.code, r.body)
		}
		has := strings.Contains(r.body, "OVERVIEW BODY TEXT")
		null := strings.Contains(r.body, `"overview":null`)
		if has != served || null == served {
			t.Errorf("%s %s: served=%v null=%v, want served=%v: %s", why, name, has, null, served, r.body)
		}
	}
}

func TestTrustOverviewGuard(t *testing.T) {
	g := newGuardFx(t, "d5g")
	if r := g.put(`{"overview_document_id":"pol-ov"}`); r.code != 200 {
		t.Fatalf("choose overview = %d %s", r.code, r.body)
	}
	// not published as an item: served
	g.wantServed(t, "unpublished", true)
	if g.settings(t).Withheld {
		t.Error("withheld while unpublished")
	}

	// published public: served
	id := g.f.publishOK("public")
	g.wantServed(t, "public", true)

	// published gated: withheld on the public page and the preview
	patch := func(level string) trustResp {
		return vdo(g.f.s, http.MethodPatch, tcAPI+"/items/"+id, `{"access_level":"`+level+`"}`, withBearer(g.jwt))
	}
	if r := patch("gated"); r.code != 200 {
		t.Fatalf("patch gated = %d %s", r.code, r.body)
	}
	g.wantServed(t, "gated", false)
	if s := g.settings(t); !s.Withheld || s.OverviewID == nil || *s.OverviewID != "pol-ov" {
		t.Errorf("settings while gated = %+v, want withheld with the setting kept", s)
	}
	// restricted too
	patch("restricted")
	g.wantServed(t, "restricted", false)
	if !g.settings(t).Withheld {
		t.Error("not withheld while restricted")
	}
	// back to public: served again
	patch("public")
	g.wantServed(t, "public again", true)
	if g.settings(t).Withheld {
		t.Error("still withheld after going public")
	}
	// a PUT response carries the flag too
	patch("gated")
	if r := g.put(`{"title":"x"}`); !strings.Contains(r.body, `"overview_withheld":true`) {
		t.Errorf("PUT response = %s", r.body)
	}
}

func TestTrustOverviewGuardRefusesAGatedChoice(t *testing.T) {
	g := newGuardFx(t, "d5c")
	g.f.publishOK("gated")
	for _, level := range []string{"gated", "restricted"} {
		if level == "restricted" {
			id := g.f.item().PublicID
			vdo(g.f.s, http.MethodPatch, tcAPI+"/items/"+id, `{"access_level":"restricted"}`, withBearer(g.jwt))
		}
		r := g.put(`{"overview_document_id":"pol-ov"}`)
		if r.code != http.StatusConflict || codeOfBody(r) != CodeTrustOverviewNotPublic {
			t.Errorf("%s: choose = %d %s, want 409 trust_overview_not_public", level, r.code, r.body)
		}
		if g.settings(t).OverviewID != nil {
			t.Errorf("%s: the refused choice was stored", level)
		}
	}
	// clearing is always fine
	if r := g.put(`{"overview_document_id":null}`); r.code != 200 {
		t.Errorf("clear = %d", r.code)
	}
}

// A retired overview document keeps behaving as it did: the setting is cleared.
func TestTrustOverviewGuardRetiredStillClearsTheSetting(t *testing.T) {
	g := newGuardFx(t, "d5r")
	if r := g.put(`{"overview_document_id":"pol-ov"}`); r.code != 200 {
		t.Fatal(r.body)
	}
	g.f.publishOK("gated")
	g.f.mustOK(g.f.docCall(g.f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", g.f.mgr), "retire")
	pub, _ := g.overviews()
	if pub.code != 200 || !strings.Contains(pub.body, `"overview":null`) {
		t.Fatalf("overview after retire = %d %s", pub.code, pub.body)
	}
	if s := g.settings(t); s.OverviewID != nil || s.Withheld {
		t.Errorf("settings after retire = %+v, want the setting cleared", s)
	}
}

// D4: the org send cap is a backstop; reaching it shows as a paused-until time.
func TestTrustSignInPausedUntilInSettings(t *testing.T) {
	m := mgrSetup(t)
	ctx := context.Background()
	t.Setenv("ISMS_TRUST_SIGNIN_ORG_HOURLY", "5")
	paused := func() *int64 {
		r := m.do("manager", http.MethodGet, "/settings", "")
		var s struct {
			P *int64 `json:"sign_in_paused_until"`
		}
		if r.code != 200 || json.Unmarshal([]byte(r.body), &s) != nil || !strings.Contains(r.body, `"sign_in_paused_until"`) {
			t.Fatalf("settings = %d %s", r.code, r.body)
		}
		return s.P
	}
	mk := func(n int) {
		for i := 0; i < n; i++ {
			_, h := newHashedToken()
			if err := m.s.db.CreateTrustVisitorLink(ctx, m.org.id, "q"+string(rune('a'+i))+"@example.com", h, time.Minute); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk(4)
	if p := paused(); p != nil {
		t.Errorf("4 of 5: paused until %v", *p)
	}
	mk(1)
	p := paused()
	if p == nil || *p <= time.Now().Unix() || *p > time.Now().Add(time.Hour+time.Minute).Unix() {
		t.Errorf("5 of 5: paused until %v, want within (now, now+1h]", p)
	}
	// the PUT response carries it too
	if r := m.do("manager", http.MethodPut, "/settings", `{"title":"x"}`); !strings.Contains(r.body, `"sign_in_paused_until":`) || strings.Contains(r.body, `"sign_in_paused_until":null`) {
		t.Errorf("PUT = %s", r.body)
	}
	// an old link does not count
	rawExec(t, `UPDATE trust_visitor_links SET created_at = now() - interval '2 hours' WHERE organization_id = $1`, m.org.id)
	if p := paused(); p != nil {
		t.Errorf("old links still pause until %v", *p)
	}
	// the backstop default
	t.Setenv("ISMS_TRUST_SIGNIN_ORG_HOURLY", "")
	if got := trustSignInOrgHourly(); got != 500 {
		t.Errorf("default cap = %d, want 500", got)
	}
}
