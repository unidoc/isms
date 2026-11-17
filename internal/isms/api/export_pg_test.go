package api

// Postgres-gated tests for GET /documents/:docId/export. The licence state and
// the renderer are injected, so no key and no vendor library are involved. Run:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TestExport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"isms.sh/internal/isms/blob"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/export"
	"isms.sh/internal/isms/license"
)

var fakeExportBytes = []byte("%FAKE-EXPORT-BYTES%")

type exFx struct {
	*tdFx
	caps   license.Capabilities
	docs   []export.ExportDoc
	render func(string, *export.ExportDoc) ([]byte, error)
}

func newExFx(t *testing.T, tag string) *exFx {
	t.Helper()
	f := &exFx{tdFx: newTDFx(t, tag, "exp-"+tag), caps: license.Capabilities{PDF: license.SourceMetered, DOCX: license.SourceMetered}}
	f.s.exportCaps = func() license.Capabilities { return f.caps }
	f.s.exportRender = func(format string, d *export.ExportDoc) ([]byte, error) {
		f.docs = append(f.docs, *d)
		if f.render != nil {
			return f.render(format, d)
		}
		return fakeExportBytes, nil
	}
	return f
}

// get runs the handler and then Echo's error handler, so an error answer has
// the exact wire shape a client sees.
func (f *exFx) get(docID, query, role string) *httptest.ResponseRecorder {
	f.t.Helper()
	c, rec := reviewCtxForPath(f.o.id, http.MethodGet, "/api/v1/documents/"+docID+"/export"+query, "", role, f.admin)
	c.SetParamNames("docId")
	c.SetParamValues(docID)
	if err := f.s.handleExportDocument(c); err != nil {
		f.s.echo.HTTPErrorHandler(err, c)
	}
	return rec
}

func (f *exFx) activity() []db.Activity {
	f.t.Helper()
	rows, err := f.s.db.ActivityForDocument(context.Background(), f.o.id, f.docID, 100)
	if err != nil {
		f.t.Fatal(err)
	}
	var out []db.Activity
	for _, a := range rows {
		if a.Action == "document_exported" {
			out = append(out, a)
		}
	}
	return out
}

func exCode(rec *httptest.ResponseRecorder) string {
	var b struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	return b.Code
}

func (f *exFx) last() export.ExportDoc {
	f.t.Helper()
	if len(f.docs) == 0 {
		f.t.Fatal("the renderer was not called")
	}
	return f.docs[len(f.docs)-1]
}

func TestExportFormatValidation(t *testing.T) {
	f := newExFx(t, "exfmt")
	for _, q := range []string{"", "?format=", "?format=xls", "?format=PDF", "?format=pdf,docx"} {
		rec := f.get(f.docID, q, "admin")
		if rec.Code != http.StatusBadRequest || exCode(rec) != CodeExportFormatInvalid {
			t.Errorf("%q: %d %s", q, rec.Code, rec.Body)
		}
	}
	if len(f.docs) != 0 {
		t.Error("renderer called for a bad format")
	}
}

func TestExportLicenceGate(t *testing.T) {
	f := newExFx(t, "exlic")
	f.caps = license.Capabilities{}
	for _, fm := range []string{"pdf", "docx"} {
		rec := f.get(f.docID, "?format="+fm, "admin")
		if rec.Code != http.StatusForbidden || exCode(rec) != CodeExportNotLicensed {
			t.Errorf("%s unlicensed: %d %s", fm, rec.Code, rec.Body)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q", ct)
		}
	}
	f.caps = license.Capabilities{PDF: license.SourceMetered}
	if rec := f.get(f.docID, "?format=pdf", "admin"); rec.Code != http.StatusOK {
		t.Errorf("pdf licensed: %d %s", rec.Code, rec.Body)
	}
	if rec := f.get(f.docID, "?format=docx", "admin"); rec.Code != http.StatusForbidden || exCode(rec) != CodeExportNotLicensed {
		t.Errorf("docx not licensed: %d %s", rec.Code, rec.Body)
	}
	if rec := f.get(f.docID, "?format=docx", "admin"); !strings.Contains(rec.Body.String(), "DOCX export is not available on this server") {
		t.Errorf("message: %s", rec.Body)
	}
}

func TestExportUnknownDocAndVersion(t *testing.T) {
	f := newExFx(t, "exnf")
	if rec := f.get("no-such-doc", "?format=pdf", "admin"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown doc: %d %s", rec.Code, rec.Body)
	}
	rec := f.get(f.docID, "?format=pdf&version=9.9", "admin")
	if rec.Code != http.StatusNotFound || exCode(rec) != CodeExportVersionNotFound {
		t.Errorf("unknown version: %d %s", rec.Code, rec.Body)
	}
	if len(f.docs) != 0 {
		t.Error("renderer called")
	}
}

func TestExportCurrentResponse(t *testing.T) {
	f := newExFx(t, "excur")
	f.save("# Heading\n\nHello **world**")
	for _, q := range []string{"?format=pdf", "?format=pdf&version=current"} {
		rec := f.get(f.docID, q, "admin")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", q, rec.Code, rec.Body)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
			t.Errorf("Content-Type = %q", ct)
		}
		if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename=exp-excur-v1.0.pdf` {
			t.Errorf("Content-Disposition = %q", cd)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("headers: %v", rec.Header())
		}
		if rec.Body.String() != string(fakeExportBytes) {
			t.Errorf("body = %q", rec.Body)
		}
		d := f.last()
		if d.Meta.Title != "Test Doc" || d.Meta.Status != "draft" || !d.Draft() {
			t.Errorf("meta = %+v", d.Meta)
		}
		if len(d.Blocks) != 2 {
			t.Errorf("blocks = %+v", d.Blocks)
		}
		if d.GeneratedAt.IsZero() || d.GeneratedAt.Location().String() != "UTC" {
			t.Errorf("GeneratedAt = %v", d.GeneratedAt)
		}
		if d.Org.Name == "" {
			t.Error("org name missing")
		}
	}
	rec := f.get(f.docID, "?format=docx", "admin")
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Errorf("docx Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasSuffix(cd, ".docx") {
		t.Errorf("docx Content-Disposition = %q", cd)
	}
}

func TestExportOldVersionUsesThatVersion(t *testing.T) {
	f := newExFx(t, "exold")
	f.save("# Old\n\nold text")
	f.approve()
	versions, err := f.s.db.ListVersions(context.Background(), f.o.id, f.docID)
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	oldV := versions[0].Version
	f.save("# New\n\nnew text")

	rec := f.get(f.docID, "?format=pdf&version="+oldV, "admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("old: %d %s", rec.Code, rec.Body)
	}
	d := f.last()
	if d.Meta.Status != "approved" || d.Draft() {
		t.Errorf("old version status = %q, Draft = %v", d.Meta.Status, d.Draft())
	}
	if h, ok := d.Blocks[0].(export.Heading); !ok || h.Inlines[0].(export.Text).Text != "Old" {
		t.Errorf("old blocks = %+v", d.Blocks)
	}
	if d.VersionLabel != oldV {
		t.Errorf("VersionLabel = %q, want %q", d.VersionLabel, oldV)
	}

	rec = f.get(f.docID, "?format=pdf", "admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("current: %d %s", rec.Code, rec.Body)
	}
	d = f.last()
	if !d.Draft() {
		t.Errorf("current status = %q, want a draft", d.Meta.Status)
	}
	if h, ok := d.Blocks[0].(export.Heading); !ok || h.Inlines[0].(export.Text).Text != "New" {
		t.Errorf("current blocks = %+v", d.Blocks)
	}
}

func TestExportRendererErrors(t *testing.T) {
	f := newExFx(t, "exerr")
	f.render = func(string, *export.ExportDoc) ([]byte, error) { return nil, export.ErrUnlicensed }
	rec := f.get(f.docID, "?format=pdf", "admin")
	if rec.Code != http.StatusForbidden || exCode(rec) != CodeExportNotLicensed {
		t.Errorf("unlicensed at render: %d %s", rec.Code, rec.Body)
	}
	f.render = func(string, *export.ExportDoc) ([]byte, error) {
		return nil, errors.New("boom-internal-detail")
	}
	rec = f.get(f.docID, "?format=pdf", "admin")
	if rec.Code != http.StatusInternalServerError || exCode(rec) != CodeExportFailed {
		t.Errorf("render error: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "boom-internal-detail") {
		t.Errorf("internal error text leaked: %s", rec.Body)
	}
}

func TestExportAccess(t *testing.T) {
	f := newExFx(t, "exacc")
	if rec := f.get(f.docID, "?format=pdf", "reader"); rec.Code != http.StatusOK {
		t.Errorf("reader: %d %s", rec.Code, rec.Body)
	}
	// Through the real router with no session: the same 401 as every route.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+f.docID+"/export?format=pdf", nil)
	req.Host = f.o.domain
	rec := httptest.NewRecorder()
	f.s.echo.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no auth: %d %s", rec.Code, rec.Body)
	}
	if len(f.docs) != 1 {
		t.Errorf("renderer calls = %d, want 1 (reader only)", len(f.docs))
	}
}

func TestExportOrgLogo(t *testing.T) {
	f := newExFx(t, "exlogo")
	f.s.blobs = blob.NewLocal(t.TempDir())
	org, err := f.s.db.GetOrganization(context.Background(), f.o.id)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Only an SVG: no logo.
	if err := f.s.blobs.Put(ctx, org.UUID, "branding/logo.svg", []byte("<svg/>")); err != nil {
		t.Fatal(err)
	}
	f.get(f.docID, "?format=pdf", "admin")
	if f.last().Org.LogoPNG != nil {
		t.Error("the SVG logo must never be read")
	}

	png := []byte("\x89PNG\r\n\x1a\nfake")
	if err := f.s.blobs.Put(ctx, org.UUID, "branding/logo.png", png); err != nil {
		t.Fatal(err)
	}
	f.get(f.docID, "?format=pdf", "admin")
	if string(f.last().Org.LogoPNG) != string(png) {
		t.Errorf("LogoPNG = %q", f.last().Org.LogoPNG)
	}

	// Over the cap: skipped, export still works.
	if err := f.s.blobs.Put(ctx, org.UUID, "branding/logo.png", make([]byte, maxExportLogo+1)); err != nil {
		t.Fatal(err)
	}
	if rec := f.get(f.docID, "?format=pdf", "admin"); rec.Code != http.StatusOK || f.last().Org.LogoPNG != nil {
		t.Errorf("oversize logo: %d, LogoPNG len %d", rec.Code, len(f.last().Org.LogoPNG))
	}
}

func TestExportFilename(t *testing.T) {
	cases := []struct{ id, v, ext, want string }{
		{"iso27001-4-1", "1.0", "pdf", "iso27001-4-1-v1.0.pdf"},
		{"iso27001-4-1", "", "docx", "iso27001-4-1.docx"},
		{"Weird ID/../x", "v 2\"", "pdf", "weird-id-..-x-vv-2-.pdf"},
	}
	for _, c := range cases {
		if got := exportFilename(c.id, c.v, c.ext); got != c.want {
			t.Errorf("exportFilename(%q,%q) = %q, want %q", c.id, c.v, got, c.want)
		}
	}
}

func TestExportRecordsActivity(t *testing.T) {
	f := newExFx(t, "exact")
	f.save("# Old\n\nold")
	f.approve()
	versions, _ := f.s.db.ListVersions(context.Background(), f.o.id, f.docID)
	oldV := versions[0].Version
	f.save("# New\n\nnew")

	if rec := f.get(f.docID, "?format=pdf", "admin"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	if got := f.activity(); len(got) != 1 {
		t.Fatalf("after one export: %d rows", len(got))
	}
	if rec := f.get(f.docID, "?format=docx&version="+oldV, "admin"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	if rec := f.get(f.docID, "?format=pdf", "reader"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	rows := f.activity() // newest first
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3: %+v", len(rows), rows)
	}
	for _, a := range rows {
		if a.DocumentID != f.docID || a.Actor != f.admin {
			t.Errorf("row = %+v", a)
		}
	}
	details := map[string]bool{}
	for _, a := range rows {
		details[a.Detail] = true
	}
	for _, want := range []string{
		"Exported " + f.docID + " version " + oldV + " as DOCX",
		"Exported " + f.docID + " version 1.1 as PDF",
	} {
		if !details[want] {
			t.Errorf("no row with detail %q in %v", want, details)
		}
	}
	if len(details) != 2 {
		t.Errorf("details = %v", details)
	}
}

func TestExportActivityReaderActorAndNoVersionLabel(t *testing.T) {
	f := newExFx(t, "exact2")
	// A reader's own email is recorded, not the admin's.
	c, rec := reviewCtxForPath(f.o.id, http.MethodGet, "/x?format=pdf", "", "reader", f.read)
	c.SetParamNames("docId")
	c.SetParamValues(f.docID)
	if err := f.s.handleExportDocument(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	rows := f.activity()
	if len(rows) != 1 || rows[0].Actor != f.read {
		t.Fatalf("rows = %+v", rows)
	}
	// The label is read after rendering, so the stub empties it: a document
	// with no version label reads "(current)".
	orig := f.s.exportRender
	f.s.exportRender = func(format string, d *export.ExportDoc) ([]byte, error) {
		d.VersionLabel = ""
		return orig(format, d)
	}
	f.get(f.docID, "?format=docx&version=current", "admin")
	found := false
	for _, a := range f.activity() {
		if a.Detail == "Exported "+f.docID+" (current) as DOCX" {
			found = true
		}
	}
	if !found {
		t.Errorf("no (current) row: %+v", f.activity())
	}
}

func TestExportFailuresWriteNoActivity(t *testing.T) {
	f := newExFx(t, "exnone")
	f.get(f.docID, "", "admin")
	f.get(f.docID, "?format=xls", "admin")
	f.get("no-such-doc", "?format=pdf", "admin")
	f.get(f.docID, "?format=pdf&version=9.9", "admin")
	f.render = func(string, *export.ExportDoc) ([]byte, error) { return nil, export.ErrUnlicensed }
	f.get(f.docID, "?format=pdf", "admin")
	f.render = func(string, *export.ExportDoc) ([]byte, error) { return nil, errors.New("boom") }
	f.get(f.docID, "?format=pdf", "admin")
	f.caps = license.Capabilities{}
	f.get(f.docID, "?format=pdf", "admin")
	if got := f.activity(); len(got) != 0 {
		t.Errorf("failed exports wrote %d rows: %+v", len(got), got)
	}
}

// The activity insert uses the request context; cancelling it while rendering
// makes the insert fail without any production seam.
func TestExportFailsClosedWhenActivityCannotBeWritten(t *testing.T) {
	f := newExFx(t, "exclosed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.render = func(string, *export.ExportDoc) ([]byte, error) {
		cancel()
		return fakeExportBytes, nil
	}
	c, rec := reviewCtxForPath(f.o.id, http.MethodGet, "/x?format=pdf", "", "admin", f.admin)
	c.SetRequest(c.Request().WithContext(ctx))
	c.SetParamNames("docId")
	c.SetParamValues(f.docID)
	if err := f.s.handleExportDocument(c); err != nil {
		f.s.echo.HTTPErrorHandler(err, c)
	}
	if rec.Code != http.StatusInternalServerError || exCode(rec) != CodeExportFailed {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), string(fakeExportBytes)) {
		t.Error("file bytes sent although the export was not recorded")
	}
	if rec.Header().Get("Content-Disposition") != "" {
		t.Error("download headers set on a failed export")
	}
	if got := f.activity(); len(got) != 0 {
		t.Errorf("rows = %d", len(got))
	}
}
