package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Live-Postgres tests for the external-links API (#316). Every user is seeded
// here, so the tests also pass when run alone:
//
//	go test ./internal/isms/api/ -run ExternalLink

type elEnv struct {
	s     *Server
	orgID int
	sup   *db.Supplier
	// lastErr is the body of the error the most recent call answered with.
	lastErr errorBody
}

func newELEnv(t *testing.T, name string) *elEnv {
	t.Helper()
	s := testServer(t)
	orgID := newTestOrg(t, s, name)
	for role, email := range map[string]string{
		"admin": "admin@" + name + ".test", "manager": "manager@" + name + ".test",
		"contributor": "contributor@" + name + ".test", "reader": "reader@" + name + ".test",
	} {
		contractTestUser(t, s, orgID, email, role)
	}
	sup := &db.Supplier{Name: "Evidence Co", SupplierType: db.SupplierTypes[0], Criticality: db.CriticalityLevels[0], Status: db.SupplierStatuses[0]}
	if err := s.db.CreateSupplier(context.Background(), orgID, sup); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	return &elEnv{s: s, orgID: orgID, sup: sup}
}

func (e *elEnv) email(role, name string) string { return role + "@" + name + ".test" }

// call runs handler as role in orgID and returns the HTTP status and body.
func (e *elEnv) call(t *testing.T, orgID int, role, email string, handler func(echo.Context) error, method, target, param, body string) (int, []byte) {
	t.Helper()
	ec := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := ec.NewContext(req, rec)
	if param != "" {
		c.SetParamNames("id")
		c.SetParamValues(param)
	}
	c.Set("org_id", orgID)
	c.Set("user_role", role)
	c.Set("user_email", email)
	if err := handler(c); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			e.lastErr, _ = he.Message.(errorBody)
			return he.Code, nil
		}
		t.Fatalf("handler error is not an HTTPError: %v", err)
	}
	return rec.Code, rec.Body.Bytes()
}

func (e *elEnv) post(t *testing.T, role, email string, body map[string]any) (int, db.ExternalLink) {
	t.Helper()
	b, _ := json.Marshal(body)
	code, out := e.call(t, e.orgID, role, email, e.s.handleCreateExternalLink, http.MethodPost, "/api/v1/external-links", "", string(b))
	var l db.ExternalLink
	if code == http.StatusCreated {
		if err := json.Unmarshal(out, &l); err != nil {
			t.Fatalf("decoding create response: %v", err)
		}
	}
	return code, l
}

func (e *elEnv) validBody(over map[string]any) map[string]any {
	b := map[string]any{"entity_type": "supplier", "entity_id": e.sup.Identifier,
		"url": "https://example.com/soc2.pdf", "label": "SOC 2 report", "link_type": "audit_report"}
	for k, v := range over {
		if v == nil {
			delete(b, k)
		} else {
			b[k] = v
		}
	}
	return b
}

func (e *elEnv) list(t *testing.T, orgID int, role, email, supplierID string) (int, []db.ExternalLink) {
	t.Helper()
	code, out := e.call(t, orgID, role, email, e.s.handleListExternalLinks, http.MethodGet,
		"/api/v1/external-links?type=supplier&id="+supplierID, "", "")
	var resp struct {
		Data []db.ExternalLink `json:"data"`
	}
	if code == http.StatusOK {
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("decoding list: %v", err)
		}
	}
	return code, resp.Data
}

func (e *elEnv) changelog(t *testing.T) []db.ChangelogEntry {
	t.Helper()
	// The same function GET /changelog/supplier/:id serves.
	entries, err := e.s.db.ListEntityChangelog(context.Background(), e.orgID, "supplier", e.sup.ID)
	if err != nil {
		t.Fatalf("ListEntityChangelog: %v", err)
	}
	var out []db.ChangelogEntry
	for _, en := range entries {
		if en.Field == "external_link" {
			out = append(out, en)
		}
	}
	// ListEntityChangelog is newest first; the tests read oldest first.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func TestExternalLinkRejectsUnsafeURLs(t *testing.T) {
	const name = "extlink-url"
	e := newELEnv(t, name)
	admin := e.email("admin", name)
	for _, bad := range []string{
		"javascript:alert(1)", "JavaScript:alert(1)", "javascript://example.com/%0Aalert(1)", "JAVASCRIPT://example.com/%0Aalert(1)", "data:text/html,<script>alert(1)</script>",
		"ftp://x.example/file", "//evil.example", "https://", "http://", "", "   ", "not a url",
		"https://" + strings.Repeat("a", 2050) + ".example",
	} {
		if code, _ := e.post(t, "admin", admin, e.validBody(map[string]any{"url": bad})); code != http.StatusBadRequest {
			t.Errorf("url %q: status %d, want 400", bad[:min(len(bad), 40)], code)
		}
	}
	// An empty URL is "required"; every other bad value is a plain invalid request.
	for _, empty := range []string{"", "   "} {
		if code, _ := e.post(t, "admin", admin, e.validBody(map[string]any{"url": empty})); code != http.StatusBadRequest ||
			e.lastErr.Code != CodeRequired || e.lastErr.Params[ParamField] != "url" {
			t.Errorf("empty url: %d %+v, want 400 required/url", code, e.lastErr)
		}
	}
	if code, _ := e.post(t, "admin", admin, e.validBody(map[string]any{"url": "javascript:alert(1)"})); code != http.StatusBadRequest || e.lastErr.Code != CodeInvalidRequest {
		t.Errorf("javascript url: %d %+v, want 400 invalid_request", code, e.lastErr)
	}
	for _, good := range []string{"http://example.com/a", "https://example.com/a?b=1#c", "HTTPS://EXAMPLE.com/x"} {
		if code, _ := e.post(t, "admin", admin, e.validBody(map[string]any{"url": good})); code != http.StatusCreated {
			t.Errorf("url %q: status %d, want 201", good, code)
		}
	}
	// PUT validates the same way.
	_, l := e.post(t, "admin", admin, e.validBody(nil))
	b, _ := json.Marshal(map[string]any{"url": "javascript:alert(1)", "label": "x", "link_type": "other"})
	if code, _ := e.call(t, e.orgID, "admin", admin, e.s.handleUpdateExternalLink, http.MethodPut, "/api/v1/external-links/1", strconv.FormatInt(l.ID, 10), string(b)); code != http.StatusBadRequest {
		t.Errorf("PUT javascript: status %d, want 400", code)
	}
}

func TestExternalLinkValidatesFields(t *testing.T) {
	const name = "extlink-fields"
	e := newELEnv(t, name)
	admin := e.email("admin", name)
	cases := map[string]map[string]any{
		"empty label":        {"label": ""},
		"blank label":        {"label": "   "},
		"201 char label":     {"label": strings.Repeat("x", 201)},
		"bad link_type":      {"link_type": "screenshot"},
		"missing link_type":  {"link_type": nil},
		"entity_type asset":  {"entity_type": "asset"},
		"entity_type system": {"entity_type": "system"},
		"entity_type legal":  {"entity_type": "legal_requirement"},
		"entity_type bogus":  {"entity_type": "nope"},
		"bad valid_until":    {"valid_until": "next tuesday"},
		"unknown supplier":   {"entity_id": "SUPPLIER-99999"},
	}
	for label, over := range cases {
		code, _ := e.post(t, "admin", admin, e.validBody(over))
		want := http.StatusBadRequest
		if label == "unknown supplier" && code == http.StatusNotFound {
			continue
		}
		if code != want {
			t.Errorf("%s: status %d, want %d", label, code, want)
		}
	}
	for _, empty := range []string{"", "   "} {
		if code, _ := e.post(t, "admin", admin, e.validBody(map[string]any{"label": empty})); code != http.StatusBadRequest ||
			e.lastErr.Code != CodeRequired || e.lastErr.Params[ParamField] != "label" {
			t.Errorf("empty label: %d %+v, want 400 required/label", code, e.lastErr)
		}
	}
	if code, _ := e.post(t, "admin", admin, e.validBody(map[string]any{"label": strings.Repeat("x", 201)})); code != http.StatusBadRequest || e.lastErr.Code != CodeInvalidRequest {
		t.Errorf("201 char label: %d %+v, want 400 invalid_request", code, e.lastErr)
	}
	if code, l := e.post(t, "admin", admin, e.validBody(map[string]any{"label": strings.Repeat("x", 200)})); code != http.StatusCreated || len(l.Label) != 200 {
		t.Errorf("200 char label: status %d", code)
	}
	// Numeric supplier id and a trimmed label are accepted.
	code, l := e.post(t, "manager", e.email("manager", name), e.validBody(map[string]any{"entity_id": strconv.FormatInt(e.sup.ID, 10), "label": "  padded  "}))
	if code != http.StatusCreated || l.Label != "padded" || l.EntityID != e.sup.ID || l.EntityType != "supplier" {
		t.Errorf("numeric id / trim: status %d link %+v", code, l)
	}
	// Nothing was written by the rejected requests.
	if _, links := e.list(t, e.orgID, "reader", e.email("reader", name), e.sup.Identifier); len(links) != 2 {
		t.Errorf("%d links stored, want only the 2 valid ones", len(links))
	}
}

func TestExternalLinkRoles(t *testing.T) {
	const name = "extlink-roles"
	e := newELEnv(t, name)
	admin := e.email("admin", name)
	_, l := e.post(t, "admin", admin, e.validBody(nil))
	id := strconv.FormatInt(l.ID, 10)
	body, _ := json.Marshal(map[string]any{"url": "https://example.com/z", "label": "z", "link_type": "other"})

	for _, role := range []string{"reader", "contributor"} {
		em := e.email(role, name)
		if code, _ := e.post(t, role, em, e.validBody(nil)); code != http.StatusForbidden {
			t.Errorf("%s POST: %d, want 403", role, code)
		}
		if code, _ := e.call(t, e.orgID, role, em, e.s.handleUpdateExternalLink, http.MethodPut, "/x", id, string(body)); code != http.StatusForbidden {
			t.Errorf("%s PUT: %d, want 403", role, code)
		}
		if code, _ := e.call(t, e.orgID, role, em, e.s.handleVerifyExternalLink, http.MethodPost, "/x", id, ""); code != http.StatusForbidden {
			t.Errorf("%s verify: %d, want 403", role, code)
		}
		if code, _ := e.call(t, e.orgID, role, em, e.s.handleDeleteExternalLink, http.MethodDelete, "/x", id, ""); code != http.StatusForbidden {
			t.Errorf("%s DELETE: %d, want 403", role, code)
		}
		if code, links := e.list(t, e.orgID, role, em, e.sup.Identifier); code != http.StatusOK || len(links) != 1 {
			t.Errorf("%s GET: %d with %d links, want 200 with 1", role, code, len(links))
		}
	}
	if got := e.changelog(t); len(got) != 1 {
		t.Errorf("forbidden writes changed the changelog: %d rows, want 1 (the admin create)", len(got))
	}
}

func TestExternalLinkOrgIsolation(t *testing.T) {
	e := newELEnv(t, "extlink-iso-a")
	b := newELEnv(t, "extlink-iso-b")
	_, l := e.post(t, "admin", e.email("admin", "extlink-iso-a"), e.validBody(nil))
	id := strconv.FormatInt(l.ID, 10)
	bAdmin := b.email("admin", "extlink-iso-b")

	// Org B asking for org A's supplier id, and for its own supplier, sees nothing.
	if _, links := b.list(t, b.orgID, "admin", bAdmin, strconv.FormatInt(e.sup.ID, 10)); len(links) != 0 {
		t.Errorf("org B lists org A's links via A's supplier id")
	}
	if _, links := b.list(t, b.orgID, "admin", bAdmin, b.sup.Identifier); len(links) != 0 {
		t.Errorf("org B's own supplier shows links: %v", links)
	}
	// Creating against org A's supplier row id from org B must not work.
	if code, _ := b.post(t, "admin", bAdmin, b.validBody(map[string]any{"entity_id": strconv.FormatInt(e.sup.ID, 10)})); code != http.StatusNotFound && code != http.StatusBadRequest {
		t.Errorf("org B create against org A's supplier: %d, want 404/400", code)
	}

	body, _ := json.Marshal(map[string]any{"url": "https://evil.example/", "label": "hijack", "link_type": "other"})
	for name, h := range map[string]func(echo.Context) error{
		"PUT": b.s.handleUpdateExternalLink, "verify": b.s.handleVerifyExternalLink, "DELETE": b.s.handleDeleteExternalLink,
	} {
		if code, _ := b.call(t, b.orgID, "admin", bAdmin, h, "X", "/x", id, string(body)); code != http.StatusNotFound {
			t.Errorf("org B %s of org A's link: %d, want 404", name, code)
		}
	}
	// Org A's link is untouched.
	_, links := e.list(t, e.orgID, "admin", e.email("admin", "extlink-iso-a"), e.sup.Identifier)
	if len(links) != 1 || links[0].Label != "SOC 2 report" || links[0].VerifiedAt != nil {
		t.Errorf("org A's link was changed: %+v", links)
	}
	// A missing, a non-numeric and a deleted id are 404 too.
	if code, _ := e.call(t, e.orgID, "admin", e.email("admin", "extlink-iso-a"), e.s.handleDeleteExternalLink, http.MethodDelete, "/x", id, ""); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := e.call(t, e.orgID, "admin", e.email("admin", "extlink-iso-a"), e.s.handleDeleteExternalLink, http.MethodDelete, "/x", id, ""); code != http.StatusNotFound {
		t.Errorf("second delete: %d, want 404", code)
	}
	if code, _ := e.call(t, e.orgID, "admin", e.email("admin", "extlink-iso-a"), e.s.handleVerifyExternalLink, http.MethodPost, "/x", "999999999", ""); code != http.StatusNotFound ||
		e.lastErr.Code != CodeNotFound || e.lastErr.Params[ParamEntity] != "external_link" {
		t.Errorf("verify missing: %d %+v, want 404 not_found/external_link", code, e.lastErr)
	}
}

func TestExternalLinkChangelogAndVerifier(t *testing.T) {
	const name = "extlink-log"
	e := newELEnv(t, name)
	admin, manager := e.email("admin", name), e.email("manager", name)

	code, l := e.post(t, "admin", admin, e.validBody(map[string]any{"valid_until": "2099-03-04"}))
	if code != http.StatusCreated || l.VerifiedBy != "" || l.VerifiedAt != nil || l.State != "current" {
		t.Fatalf("create: %d %+v", code, l)
	}
	id := strconv.FormatInt(l.ID, 10)
	put := func(role, email string, body map[string]any) db.ExternalLink {
		t.Helper()
		b, _ := json.Marshal(body)
		code, out := e.call(t, e.orgID, role, email, e.s.handleUpdateExternalLink, http.MethodPut, "/x", id, string(b))
		if code != http.StatusOK {
			t.Fatalf("PUT: %d", code)
		}
		var got db.ExternalLink
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	full := func(over map[string]any) map[string]any {
		b := map[string]any{"url": "https://example.com/soc2.pdf", "label": "SOC 2 report", "link_type": "audit_report", "valid_until": "2099-03-04"}
		for k, v := range over {
			b[k] = v
		}
		return b
	}

	// Edit: verified_at set by the admin -> admin is the verifier.
	got := put("admin", admin, full(map[string]any{"verified_at": "2030-05-06", "label": "SOC 2 Type II"}))
	if got.VerifiedBy != admin {
		t.Errorf("verified_by = %q, want %q", got.VerifiedBy, admin)
	}
	// Same verified_at saved by someone else: the verifier stays.
	got = put("manager", manager, full(map[string]any{"verified_at": "2030-05-06", "label": "SOC 2 Type II", "url": "https://example.com/v2.pdf"}))
	if got.VerifiedBy != admin {
		t.Errorf("verified_by after an unchanged verified_at = %q, want %q", got.VerifiedBy, admin)
	}
	// A different verified_at: the saver becomes the verifier.
	got = put("manager", manager, full(map[string]any{"verified_at": "2030-06-07", "label": "SOC 2 Type II", "url": "https://example.com/v2.pdf"}))
	if got.VerifiedBy != manager {
		t.Errorf("verified_by after a changed verified_at = %q, want %q", got.VerifiedBy, manager)
	}
	// valid_until omitted and verified_at null: both clear, verifier too.
	b := full(map[string]any{"verified_at": nil, "label": "SOC 2 Type II", "url": "https://example.com/v2.pdf"})
	delete(b, "valid_until")
	got = put("admin", admin, b)
	if got.ValidUntil != nil || got.VerifiedAt != nil || got.VerifiedBy != "" || got.State != "" {
		t.Errorf("omitted/null dates did not clear: %+v", got)
	}

	// Verify stamps today and the actor.
	code, out := e.call(t, e.orgID, "manager", manager, e.s.handleVerifyExternalLink, http.MethodPost, "/x", id, "")
	if code != http.StatusOK {
		t.Fatalf("verify: %d", code)
	}
	var verified db.ExternalLink
	_ = json.Unmarshal(out, &verified)
	var isToday bool
	if err := e.s.db.Pool().QueryRow(context.Background(), `SELECT verified_at = CURRENT_DATE FROM external_links WHERE id = $1`, l.ID).Scan(&isToday); err != nil || !isToday {
		t.Errorf("verified_at is not today (err %v)", err)
	}
	if verified.VerifiedBy != manager {
		t.Errorf("verify: verified_by = %q, want %q", verified.VerifiedBy, manager)
	}

	// Remove.
	if code, _ := e.call(t, e.orgID, "admin", admin, e.s.handleDeleteExternalLink, http.MethodDelete, "/x", id, ""); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}

	// Changelog: one row per write, always an update on the supplier.
	rows := e.changelog(t)
	if len(rows) != 7 { // add, 4 edits, verify, remove
		t.Fatalf("%d changelog rows, want 7: %+v", len(rows), rows)
	}
	for i, r := range rows {
		if r.Action != "update" || r.Field != "external_link" || r.EntityType != "supplier" || r.EntityID != e.sup.ID {
			t.Errorf("row %d: %+v, want update/external_link on the supplier", i, r)
		}
	}
	first, last := rows[0], rows[len(rows)-1]
	if first.OldValue != nil || first.NewValue == nil || *first.NewValue != "SOC 2 report (audit_report) https://example.com/soc2.pdf; valid until 2099-03-04; verified -" {
		t.Errorf("add row: old=%v new=%v", first.OldValue, first.NewValue)
	}
	if last.NewValue != nil || last.OldValue == nil || !strings.HasPrefix(*last.OldValue, "SOC 2 Type II (audit_report) https://example.com/v2.pdf; valid until -; verified 2") {
		t.Errorf("remove row: old=%v new=%v", last.OldValue, last.NewValue)
	}
	for _, i := range []int{1, 2, 3, 4, 5} {
		if rows[i].OldValue == nil || rows[i].NewValue == nil {
			t.Errorf("row %d (edit/verify) must carry old and new", i)
		}
	}
	if rows[0].ChangedBy != admin || rows[5].ChangedBy != manager {
		t.Errorf("changed_by: %q / %q", rows[0].ChangedBy, rows[5].ChangedBy)
	}
}

// A link whose supplier is soft-deleted cannot be edited, verified or removed,
// and nothing is written to the changelog.
func TestExternalLinkWritesRefusedForDeletedSupplier(t *testing.T) {
	const name = "extlink-delsup"
	e := newELEnv(t, name)
	admin := e.email("admin", name)
	_, l := e.post(t, "admin", admin, e.validBody(nil))
	id := strconv.FormatInt(l.ID, 10)
	before := len(e.changelog(t))

	if err := e.s.db.DeleteSupplier(context.Background(), e.orgID, e.sup.ID); err != nil {
		t.Fatalf("DeleteSupplier: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"url": "https://example.com/z", "label": "z", "link_type": "other"})
	for label, call := range map[string]func() int{
		"PUT": func() int {
			c, _ := e.call(t, e.orgID, "admin", admin, e.s.handleUpdateExternalLink, http.MethodPut, "/x", id, string(body))
			return c
		},
		"verify": func() int {
			c, _ := e.call(t, e.orgID, "admin", admin, e.s.handleVerifyExternalLink, http.MethodPost, "/x", id, "")
			return c
		},
		"DELETE": func() int {
			c, _ := e.call(t, e.orgID, "admin", admin, e.s.handleDeleteExternalLink, http.MethodDelete, "/x", id, "")
			return c
		},
	} {
		if code := call(); code != http.StatusNotFound {
			t.Errorf("%s on a deleted supplier's link: %d, want 404", label, code)
		}
	}
	if after := len(e.changelog(t)); after != before {
		t.Errorf("changelog rows %d -> %d, want no new rows", before, after)
	}
	var verified, deleted bool
	if err := e.s.db.Pool().QueryRow(context.Background(),
		`SELECT verified_at IS NOT NULL, deleted_at IS NOT NULL FROM external_links WHERE id = $1`, l.ID).Scan(&verified, &deleted); err != nil || verified || deleted {
		t.Errorf("link was written (verified=%v deleted=%v err=%v)", verified, deleted, err)
	}
}

// A supplier deleted between the handler's liveness check and its insert must
// not gain a link. The delete is held open in its own transaction, the create is
// started and (with the lock) blocks behind it, then the delete commits.
func TestExternalLinkCreateRacingSupplierDelete(t *testing.T) {
	const name = "extlink-race"
	e := newELEnv(t, name)
	ctx := context.Background()
	pool := e.s.db.Pool()

	del, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer del.Rollback(ctx)
	if _, err := del.Exec(ctx, `UPDATE suppliers SET deleted_at = now() WHERE id = $1`, e.sup.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	type result struct {
		code int
		body errorBody
	}
	done := make(chan result, 1)
	go func() {
		b, _ := json.Marshal(e.validBody(nil))
		// A private env copy: lastErr is written by call.
		local := &elEnv{s: e.s, orgID: e.orgID, sup: e.sup}
		code, _ := local.call(t, e.orgID, "admin", e.email("admin", name), e.s.handleCreateExternalLink, http.MethodPost, "/x", "", string(b))
		done <- result{code, local.lastErr}
	}()

	// Wait for the create to block on the uncommitted delete. Without the lock it
	// never blocks (it inserts and returns), so this times out and the assertions
	// below fail instead.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := del.Commit(ctx); err != nil {
		t.Fatalf("commit delete: %v", err)
	}

	r := <-done
	if r.code != http.StatusNotFound || r.body.Code != CodeNotFound || r.body.Params[ParamEntity] != "supplier" {
		t.Errorf("create racing a delete: %d %+v, want 404 not_found/supplier", r.code, r.body)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_links WHERE entity_type = 'supplier' AND entity_id = $1`, e.sup.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d link rows on the deleted supplier (err %v), want 0", n, err)
	}
	if rows := e.changelog(t); len(rows) != 0 {
		t.Errorf("%d changelog rows for the refused create, want 0", len(rows))
	}
}
