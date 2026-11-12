package api

// Postgres-gated tests for what an anonymous visitor's page needs beyond the
// item list (#519): the overview text, the organization default locale, the
// footer setting, and the served version on document items. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustOverview

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"isms.sh/internal/isms/db"
)

type tdOverviewResp struct {
	Title           string `json:"title"`
	DefaultLocale   string `json:"default_locale"`
	ShowPoweredBy   *bool  `json:"show_powered_by"`
	SecurityContact string `json:"security_contact"`
	Overview        *struct {
		Version     string `json:"version"`
		ApprovedAt  int64  `json:"approved_at"`
		ContentHash string `json:"content_hash"`
		HTML        string `json:"html"`
	} `json:"overview"`
}

func (f *tdFx) setOverview(id string) {
	f.t.Helper()
	st, err := f.s.db.GetTrustCenterSettings(context.Background(), f.o.id)
	if err != nil {
		f.t.Fatal(err)
	}
	if id == "" {
		st.OverviewDocumentID = nil
	} else {
		st.OverviewDocumentID = &id
	}
	if err := f.s.db.UpsertTrustCenterSettings(context.Background(), f.o.id, st, f.mgr); err != nil {
		f.t.Fatal(err)
	}
}

func (f *tdFx) overviewSetting() *string {
	f.t.Helper()
	st, err := f.s.db.GetTrustCenterSettings(context.Background(), f.o.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return st.OverviewDocumentID
}

func (f *tdFx) overviewResp() (tdOverviewResp, string) {
	f.t.Helper()
	r := trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/"+f.o.slug, "")
	if r.code != http.StatusOK {
		f.t.Fatalf("overview = %d %s", r.code, r.body)
	}
	var o tdOverviewResp
	if err := json.Unmarshal([]byte(r.body), &o); err != nil {
		f.t.Fatalf("decode %q: %v", r.body, err)
	}
	return o, r.body
}

func (f *tdFx) setOrgSetting(key, value string) {
	f.t.Helper()
	if err := f.s.db.SetOrgSetting(context.Background(), f.o.id, key, value); err != nil {
		f.t.Fatal(err)
	}
}

func (f *tdFx) settingsChangelog() []db.ChangelogEntry {
	f.t.Helper()
	cl, err := f.s.db.ListEntityChangelog(context.Background(), f.o.id, "trust_center_settings", int64(f.o.id))
	if err != nil {
		f.t.Fatal(err)
	}
	return cl
}

func TestTrustOverviewCarriesLocaleFooterAndServedText(t *testing.T) {
	f := newTDFx(t, "tov1", "pol-ov")
	f.save("# Overview\n\nApproved overview text.\n")
	f.approve()
	f.setOverview("POL-OV") // stored as typed; the lookup is case-insensitive
	servedVersion := f.state().ServedVersion.Version
	f.save("# Overview\n\nSECRET OVERVIEW DRAFT.\n") // HEAD is now a newer draft

	o, raw := f.overviewResp()
	if o.DefaultLocale != "en" || o.ShowPoweredBy == nil || !*o.ShowPoweredBy {
		t.Errorf("defaults: default_locale %q show_powered_by %v in %s", o.DefaultLocale, o.ShowPoweredBy, raw)
	}
	if o.Overview == nil {
		t.Fatalf("overview is null: %s", raw)
	}
	if o.Overview.Version != servedVersion || o.Overview.ContentHash == "" || o.Overview.ApprovedAt == 0 {
		t.Errorf("overview = %+v, want the served version %s", o.Overview, servedVersion)
	}
	if !strings.Contains(o.Overview.HTML, "Approved overview text.") || strings.Contains(raw, "SECRET OVERVIEW DRAFT") {
		t.Errorf("overview html must be the approved text, never HEAD: %s", raw)
	}
	// It is not a published item, so it is not in the list.
	if ids := f.listedIDs(); len(ids) != 0 {
		t.Errorf("overview document leaked into items: %s", f.items().body)
	}

	f.setOrgSetting("default_locale", "de-DE")
	f.setOrgSetting("show_powered_by", "false")
	o, raw = f.overviewResp()
	if o.DefaultLocale != "de-DE" || o.ShowPoweredBy == nil || *o.ShowPoweredBy {
		t.Errorf("org settings: default_locale %q show_powered_by %v in %s", o.DefaultLocale, o.ShowPoweredBy, raw)
	}
}

func TestTrustOverviewNullWhenUnsetOrUnreadable(t *testing.T) {
	f := newTDFx(t, "tov2", "pol-ov2")
	f.save("text\n")
	f.approve()
	o, raw := f.overviewResp()
	if o.Overview != nil || !strings.Contains(raw, `"overview":null`) {
		t.Errorf("unset overview must be JSON null: %s", raw)
	}

	// A document whose recorded version cannot be read: 200, overview null,
	// the rest of the response intact, and the setting is not cleared (an
	// error is never a deletion).
	body := `{"folder":"test","filename":"pol-bad.md","document_id":"pol-bad","title":"Bad"}`
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", body, "manager", f.mgr, "", ""), "create bad")
	v := &db.DocumentVersion{DocumentID: "pol-bad", Version: "9.9", FilePath: "documents/test/pol-bad.md",
		CommitHash: "0000000000000000000000000000000000000000", CreatedBy: f.admin}
	if err := f.s.db.RecordVersion(context.Background(), f.o.id, v); err != nil {
		t.Fatal(err)
	}
	f.setOverview("pol-bad")
	o, raw = f.overviewResp()
	if o.Overview != nil || o.Title != "Trust tov2" || o.SecurityContact != "tov2@sec.test" || o.DefaultLocale != "en" || o.ShowPoweredBy == nil {
		t.Errorf("unreadable overview: %s", raw)
	}
	if got := f.overviewSetting(); got == nil || *got != "pol-bad" {
		t.Errorf("an error cleared the setting: %v", got)
	}
}

func TestTrustOverviewRetireAndDeleteClearTheSetting(t *testing.T) {
	f := newTDFx(t, "tov3", "pol-ov3")
	f.save("text\n")
	f.approve()

	// Retiring a different document leaves the setting alone.
	f.setOverview(f.docID)
	other := `{"folder":"test","filename":"pol-other.md","document_id":"pol-other","title":"Other"}`
	f.mustOK(f.call(f.s.handleCreateDocument, http.MethodPost, "/api/v1/documents", other, "manager", f.mgr, "", ""), "create other")
	f.mustOK(f.call(f.s.handleUpdateDocumentMetadata, http.MethodPut, "/api/v1/documents/pol-other",
		`{"fields":{"status":"retired"}}`, "manager", f.mgr, "docId", "pol-other"), "retire other")
	if got := f.overviewSetting(); got == nil || *got != f.docID {
		t.Fatalf("retiring another document changed the setting: %v", got)
	}
	if n := len(f.settingsChangelog()); n != 0 {
		t.Fatalf("changelog rows = %d, want 0", n)
	}

	// Retiring the overview document clears it, once.
	retire := func() {
		f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", f.mgr), "retire")
	}
	retire()
	if got := f.overviewSetting(); got != nil {
		t.Errorf("setting after retire = %q, want cleared", *got)
	}
	cl := f.settingsChangelog()
	if len(cl) != 1 || cl[0].Field != "overview_document_id" || cl[0].OldValue == nil || *cl[0].OldValue != f.docID ||
		cl[0].Reason != "retired" || cl[0].ChangedBy != f.mgr {
		t.Fatalf("changelog = %+v", cl)
	}
	retire()
	if n := len(f.settingsChangelog()); n != 1 {
		t.Errorf("a second retire wrote a changelog row: %d", n)
	}

	// Deleting the overview document clears it too.
	f.setOverview(f.docID)
	f.mustOK(f.docCall(f.s.handleDeleteDocument, http.MethodDelete, "", "manager", f.mgr), "delete")
	if got := f.overviewSetting(); got != nil {
		t.Errorf("setting after delete = %q, want cleared", *got)
	}
	if cl := f.settingsChangelog(); len(cl) != 2 || cl[0].Reason != "deleted" {
		t.Errorf("changelog after delete = %+v", cl)
	}
}

// The read path clears the setting when HEAD says retired or (clean not-found)
// deleted, without the handlers being involved, and answers null.
func TestTrustOverviewReadPathClearsDeleted(t *testing.T) {
	f := newTDFx(t, "tov4", "pol-ov4")
	f.save("text\n")
	f.approve()
	f.setOverview(f.docID)
	if o, _ := f.overviewResp(); o.Overview == nil {
		t.Fatal("precondition: overview served")
	}
	abs := f.st.FindDocumentByID(f.docID)
	if _, err := f.st.DeleteFile(abs, "x", "x@example.com", "docs: delete"); err != nil {
		t.Fatal(err)
	}
	o, raw := f.overviewResp()
	if o.Overview != nil {
		t.Errorf("deleted overview still served: %s", raw)
	}
	if got := f.overviewSetting(); got != nil {
		t.Errorf("setting after read-path delete = %q, want cleared", *got)
	}
	if cl := f.settingsChangelog(); len(cl) != 1 || cl[0].Reason != "deleted" || cl[0].ChangedBy != "system" {
		t.Errorf("changelog = %+v", cl)
	}
}

// Gated items still show which version they are, never the text.
func TestTrustItemsCarryServedVersionForEveryLevel(t *testing.T) {
	f := newTDFx(t, "tov5", "pol-ov5")
	f.save("gated body text\n")
	f.approve()
	id := f.publishOK("gated")
	want := f.state().ServedVersion
	f.save("newer draft\n")

	r := f.items()
	var items []struct {
		ID          string `json:"id"`
		Version     string `json:"version"`
		ApprovedAt  int64  `json:"approved_at"`
		ContentHash string `json:"content_hash"`
	}
	if err := json.Unmarshal([]byte(r.body), &items); err != nil || len(items) != 1 {
		t.Fatalf("items %s: %v", r.body, err)
	}
	it := items[0]
	if it.ID != id || it.Version != want.Version || it.ContentHash != want.ContentHash || it.ApprovedAt != want.ApprovedAt || it.ApprovedAt == 0 {
		t.Errorf("item = %+v, want served version %+v", it, want)
	}
	if strings.Contains(r.body, "gated body text") || strings.Contains(r.body, "newer draft") {
		t.Errorf("items leak text: %s", r.body)
	}
	f.assertNotFound(f.public(id), "gated document body")
}
