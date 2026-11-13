package api

// Postgres-gated tests for trust center certifications (#521): the manager
// endpoints, uploads, the file download helper, the public items and file
// route, the preview, the body limits and the overdue summary. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustCert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/blob"
	"isms.sh/internal/isms/db"
)

var (
	certPDF  = append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("pdf body "), 50)...)
	certPNG  = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0, 1, 2, 3}, 40)...)
	certJPEG = append([]byte("\xff\xd8\xff\xe0"), bytes.Repeat([]byte{9, 8, 7}, 40)...)
)

type certResp struct {
	code int
	body []byte
	hdr  http.Header
}

func (r certResp) text() string { return string(r.body) }

func (r certResp) code400(t *testing.T, want string) {
	t.Helper()
	if r.code != http.StatusBadRequest || codeOf(tdResp{r.code, r.text()}) != want {
		t.Errorf("got %d %s, want 400 %s", r.code, r.text(), want)
	}
}

// certFx is a trust-center fixture with a local blob store.
type certFx struct {
	*tdFx
	blobDir string
}

func newCertFx(t *testing.T, tag string) *certFx {
	t.Helper()
	f := newTDFx(t, tag, "pol-"+tag)
	dir := t.TempDir()
	f.s.blobs = blob.NewLocal(dir)
	return &certFx{tdFx: f, blobDir: dir}
}

// req runs a handler directly with the given body and content type.
func (f *certFx) req(h echo.HandlerFunc, method, path string, body io.Reader, ctype, role, email string, params ...string) certResp {
	f.t.Helper()
	e := echo.New()
	r := httptest.NewRequest(method, path, body)
	if ctype != "" {
		r.Header.Set(echo.HeaderContentType, ctype)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.Set("org_id", f.o.id)
	c.Set("user_role", role)
	c.Set("user_email", email)
	if len(params) > 0 {
		var names, vals []string
		for i := 0; i+1 < len(params); i += 2 {
			names, vals = append(names, params[i]), append(vals, params[i+1])
		}
		c.SetParamNames(names...)
		c.SetParamValues(vals...)
	}
	if err := h(c); err != nil {
		var he *echo.HTTPError
		if !errors.As(err, &he) {
			f.t.Fatalf("%s %s: handler error %v", method, path, err)
		}
		b, _ := json.Marshal(he.Message)
		return certResp{code: he.Code, body: b, hdr: rec.Header()}
	}
	return certResp{code: rec.Code, body: rec.Body.Bytes(), hdr: rec.Header()}
}

func (f *certFx) json(h echo.HandlerFunc, method, path, body, role, email string, params ...string) certResp {
	return f.req(h, method, path, strings.NewReader(body), echo.MIMEApplicationJSON, role, email, params...)
}

const certBase = "/api/v1/trust-center/certifications"

func (f *certFx) mgrJSON(h echo.HandlerFunc, method, path, body string, params ...string) certResp {
	return f.json(h, method, path, body, "manager", f.mgr, params...)
}

type certPart struct {
	name, filename, ctype string
	data                  []byte
}

func multipartBody(t *testing.T, fields map[string]string, file *certPart) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if file != nil {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, file.filename))
		if file.ctype != "" {
			h.Set("Content-Type", file.ctype)
		}
		p, err := w.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = p.Write(file.data)
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func (f *certFx) upload(fields map[string]string, file *certPart) certResp {
	f.t.Helper()
	body, ctype := multipartBody(f.t, fields, file)
	return f.req(f.s.handleCreateTrustCertification, http.MethodPost, certBase, body, ctype, "manager", f.mgr)
}

func decodeCert(t *testing.T, r certResp) trustCertificationJSON {
	t.Helper()
	if r.code < 200 || r.code > 299 {
		t.Fatalf("status %d: %s", r.code, r.text())
	}
	var c trustCertificationJSON
	if err := json.Unmarshal(r.body, &c); err != nil {
		t.Fatalf("decode %s: %v", r.text(), err)
	}
	return c
}

// mkFile creates a file certification and returns it.
func (f *certFx) mkFile(label string, data []byte, extra map[string]string) trustCertificationJSON {
	f.t.Helper()
	fields := map[string]string{"label": label, "link_type": "certificate", "issuer": "BSI"}
	for k, v := range extra {
		fields[k] = v
	}
	return decodeCert(f.t, f.upload(fields, &certPart{"file", label + ".pdf", "application/pdf", data}))
}

func (f *certFx) mkLink(label string, extra string) trustCertificationJSON {
	f.t.Helper()
	body := fmt.Sprintf(`{"label":%q,"link_type":"audit_report","issuer":"Acme","url":"https://example.com/r"%s}`, label, extra)
	return decodeCert(f.t, f.mgrJSON(f.s.handleCreateTrustCertification, http.MethodPost, certBase, body))
}

func (f *certFx) publish(id int64, level string) certResp {
	body := ""
	if level != "" {
		body = fmt.Sprintf(`{"access_level":%q}`, level)
	}
	return f.mgrJSON(f.s.handlePublishTrustCertification, http.MethodPost, fmt.Sprintf("%s/%d/publish", certBase, id), body, "id", fmt.Sprint(id))
}

func (f *certFx) dl(h echo.HandlerFunc, path, role, email string, id int64) certResp {
	return f.req(h, http.MethodGet, path, nil, "", role, email, "id", fmt.Sprint(id))
}

func (f *certFx) mgrDL(id int64) certResp {
	return f.dl(f.s.handleDownloadTrustCertificationFile, fmt.Sprintf("%s/%d/file", certBase, id), "manager", f.mgr, id)
}

func (f *certFx) blobFiles() []string {
	var out []string
	_ = filepath.Walk(f.blobDir, func(p string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// pg runs SQL on a separate connection, for setting dates by Postgres's clock.
func pgExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("ISMS_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

// setUntil sets valid_until to CURRENT_DATE + n (Postgres's date); nil clears it.
func setUntil(t *testing.T, id int64, n *int) {
	t.Helper()
	if n == nil {
		pgExec(t, `UPDATE trust_center_certifications SET valid_until = NULL WHERE id = $1`, id)
		return
	}
	pgExec(t, `UPDATE trust_center_certifications SET valid_until = CURRENT_DATE + $2::int WHERE id = $1`, id, *n)
}

func publicItems(t *testing.T, f *certFx) []map[string]any {
	t.Helper()
	r := trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/"+f.o.slug+"/items", "")
	if r.code != http.StatusOK {
		t.Fatalf("public items %d %s", r.code, r.body)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(r.body), &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func previewItems(t *testing.T, f *certFx) []map[string]any {
	t.Helper()
	r := f.req(f.s.handleTrustPreviewItems, http.MethodGet, "/api/v1/trust-center/preview/items", nil, "", "manager", f.mgr)
	if r.code != http.StatusOK {
		t.Fatalf("preview items %d %s", r.code, r.text())
	}
	var items []map[string]any
	if err := json.Unmarshal(r.body, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func itemByPublicID(items []map[string]any, id string) map[string]any {
	for _, it := range items {
		if it["id"] == id {
			return it
		}
	}
	return nil
}

func (f *certFx) publicFile(host, itemID, bearer string) trustResp {
	return trustDo(f.s, host, http.MethodGet, "/api/v1/trust/"+f.o.slug+"/certifications/"+itemID+"/file", bearer)
}

func (f *certFx) previewFile(itemID string) certResp {
	return f.req(f.s.handleTrustPreviewCertificationFile, http.MethodGet, "/api/v1/trust-center/preview/certifications/"+itemID+"/file", nil, "", "manager", f.mgr, "id", itemID)
}

// ---- roles ------------------------------------------------------------------

func TestTrustCertEndpointRolesAndAuth(t *testing.T) {
	f := newCertFx(t, "tcrole")
	type ep struct {
		name, method, path, body string
		h                        echo.HandlerFunc
	}
	id := "1"
	eps := []ep{
		{"GET list", "GET", certBase, "", f.s.handleListTrustCertifications},
		{"POST create", "POST", certBase, `{"label":"x"}`, f.s.handleCreateTrustCertification},
		{"PUT update", "PUT", certBase + "/1", `{}`, f.s.handleUpdateTrustCertification},
		{"POST file", "POST", certBase + "/1/file", ``, f.s.handleReplaceTrustCertificationFile},
		{"POST verify", "POST", certBase + "/1/verify", ``, f.s.handleVerifyTrustCertification},
		{"DELETE", "DELETE", certBase + "/1", ``, f.s.handleDeleteTrustCertification},
		{"POST publish", "POST", certBase + "/1/publish", `{}`, f.s.handlePublishTrustCertification},
		{"DELETE publish", "DELETE", certBase + "/1/publish", ``, f.s.handleUnpublishTrustCertification},
		{"GET file", "GET", certBase + "/1/file", ``, f.s.handleDownloadTrustCertificationFile},
		{"GET preview file", "GET", "/api/v1/trust-center/preview/certifications/1/file", ``, f.s.handleTrustPreviewCertificationFile},
	}
	for _, e := range eps {
		for _, who := range []struct{ role, email string }{{"reader", f.read}, {"contributor", f.contr}} {
			if r := f.json(e.h, e.method, e.path, e.body, who.role, who.email, "id", id); r.code != http.StatusForbidden {
				t.Errorf("%s as %s = %d %s, want 403", e.name, who.role, r.code, r.text())
			}
		}
		if code := serveTM(f.s, newTMReq(e.method, e.path, e.body)); code != http.StatusUnauthorized {
			t.Errorf("%s with no token = %d, want 401", e.name, code)
		}
	}
	// The public file route is anonymous by design; a made-up item is a 404.
	if r := f.publicFile(trustTestBase, "00000000-0000-0000-0000-000000000000", ""); r.code != http.StatusNotFound {
		t.Errorf("public file of unknown item = %d", r.code)
	}
}

// ---- upload -----------------------------------------------------------------

func TestTrustCertUploadSniffsTheBytes(t *testing.T) {
	f := newCertFx(t, "tcsniff")
	accept := []struct {
		name, filename, claimed string
		data                    []byte
		wantType, wantExt       string
	}{
		{"pdf", "a.pdf", "application/pdf", certPDF, "application/pdf", ".pdf"},
		{"png", "a.png", "image/png", certPNG, "image/png", ".png"},
		{"jpeg", "a.jpg", "image/jpeg", certJPEG, "image/jpeg", ".jpg"},
		{"png named pdf, claimed pdf", "report.pdf", "application/pdf", certPNG, "image/png", ".png"},
		{"jpeg named html, claimed html", "x.html", "text/html", certJPEG, "image/jpeg", ".jpg"},
		{"pdf with no claimed type", "noext", "", certPDF, "application/pdf", ".pdf"},
	}
	for _, a := range accept {
		r := f.upload(map[string]string{"label": a.name, "link_type": "other"}, &certPart{"file", a.filename, a.claimed, a.data})
		c := decodeCert(t, r)
		if c.FileContentType != a.wantType || c.Source != "file" || c.FileSize != int64(len(a.data)) {
			t.Errorf("%s: %+v", a.name, c)
		}
		row, err := f.s.db.GetTrustCertification(context.Background(), f.o.id, c.ID)
		if err != nil || !strings.HasSuffix(row.FileKey, a.wantExt) || !strings.HasPrefix(row.FileKey, "trust/certifications/") {
			t.Errorf("%s: stored key %q (%v), want trust/certifications/<uuid>%s", a.name, row.FileKey, err, a.wantExt)
		}
		if got := f.mgrDL(c.ID); got.code != 200 || got.hdr.Get("Content-Type") != a.wantType || !bytes.Equal(got.body, a.data) {
			t.Errorf("%s: download %d type %q", a.name, got.code, got.hdr.Get("Content-Type"))
		}
	}
	before := len(f.blobFiles())
	reject := map[string]certPart{
		"html":               {"file", "a.html", "text/html", []byte("<html><script>alert(1)</script></html>")},
		"svg":                {"file", "a.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`)},
		"text":               {"file", "a.txt", "text/plain", []byte("just text")},
		"html named pdf":     {"file", "x.pdf", "application/pdf", []byte("<!DOCTYPE html><html><body>hi</body></html>")},
		"empty":              {"file", "empty.pdf", "application/pdf", nil},
		"gif":                {"file", "a.gif", "image/gif", []byte("GIF89a....")},
		"zip":                {"file", "a.pdf", "application/pdf", []byte("PK\x03\x04rest")},
		"pdf magic too late": {"file", "a.pdf", "application/pdf", append([]byte(" \n<html>"), certPDF...)},
	}
	for name, p := range reject {
		p := p
		f.upload(map[string]string{"label": "x", "link_type": "other"}, &p).code400(t, "trust_file_type")
		_ = name
	}
	if after := len(f.blobFiles()); after != before {
		t.Errorf("rejected uploads left %d blob(s)", after-before)
	}
	if certs, _ := f.s.db.ListTrustCertifications(context.Background(), f.o.id); len(certs) != len(accept) {
		t.Errorf("certifications = %d, want %d (rejected uploads create nothing)", len(certs), len(accept))
	}
}

func TestTrustCertCreateValidation(t *testing.T) {
	f := newCertFx(t, "tccreate")
	post := func(body string) certResp {
		return f.mgrJSON(f.s.handleCreateTrustCertification, http.MethodPost, certBase, body)
	}
	good := `"label":"L","link_type":"certificate","url":"https://e.com/c"`
	for name, body := range map[string]string{
		"number date":  `{` + good + `,"valid_until":1794000000}`,
		"rfc3339":      `{` + good + `,"valid_from":"2026-11-13T00:00:00Z"}`,
		"empty string": `{` + good + `,"verified_at":""}`,
		"bad month":    `{` + good + `,"valid_until":"2026-13-01"}`,
		"object":       `{` + good + `,"valid_until":{}}`,
		"short form":   `{` + good + `,"valid_until":"2026-1-5"}`,
	} {
		if r := post(body); r.code != 400 || codeOf(tdResp{r.code, r.text()}) != "trust_date_invalid" {
			t.Errorf("%s: %d %s, want 400 trust_date_invalid", name, r.code, r.text())
		}
	}
	for name, body := range map[string]string{
		"contract type":    `{"label":"L","link_type":"contract","url":"https://e.com"}`,
		"unknown type":     `{"label":"L","link_type":"nope","url":"https://e.com"}`,
		"ftp url":          `{"label":"L","link_type":"other","url":"ftp://e.com/x"}`,
		"javascript url":   `{"label":"L","link_type":"other","url":"javascript:alert(1)"}`,
		"from after until": `{` + good + `,"valid_from":"2026-12-01","valid_until":"2026-11-01"}`,
		"label too long":   `{"label":"` + strings.Repeat("a", 201) + `","link_type":"other","url":"https://e.com"}`,
		"issuer too long":  `{` + good + `,"issuer":"` + strings.Repeat("a", 201) + `"}`,
		"bad access":       `{` + good + `,"publish":true,"access_level":"secret"}`,
		"not json":         `{`,
	} {
		if r := post(body); r.code != 400 {
			t.Errorf("%s: %d %s, want 400", name, r.code, r.text())
		}
	}
	if r := post(`{"label":" ","link_type":"other","url":"https://e.com"}`); r.code != 400 || codeOf(tdResp{r.code, r.text()}) != "required" {
		t.Errorf("blank label: %d %s", r.code, r.text())
	}
	post(`{"label":"L","link_type":"other"}`).code400(t, "trust_certification_source")
	if certs, _ := f.s.db.ListTrustCertifications(context.Background(), f.o.id); len(certs) != 0 {
		t.Errorf("failed creates left %d certifications", len(certs))
	}

	// Multipart: both and neither; an empty date field is no date, a bad one is 400.
	f.upload(map[string]string{"label": "L", "link_type": "other", "url": "https://e.com"}, &certPart{"file", "a.pdf", "", certPDF}).code400(t, "trust_certification_source")
	f.upload(map[string]string{"label": "L", "link_type": "other"}, nil).code400(t, "trust_certification_source")
	f.upload(map[string]string{"label": "L", "link_type": "other"}, &certPart{"file", "a.pdf", "", certPDF}) // sanity: ok below
	f.upload(map[string]string{"label": "L", "link_type": "other", "valid_until": "13/11/2026"}, &certPart{"file", "a.pdf", "", certPDF}).code400(t, "trust_date_invalid")
	f.upload(map[string]string{"label": "L", "link_type": "other", "valid_from": "2026-11-13T00:00:00Z"}, &certPart{"file", "a.pdf", "", certPDF}).code400(t, "trust_date_invalid")
	ok := f.upload(map[string]string{"label": "L", "link_type": "other", "valid_from": "", "valid_until": ""}, &certPart{"file", "a.pdf", "", certPDF})
	if c := decodeCert(t, ok); c.ValidFrom != nil || c.ValidUntil != nil || c.State != "" {
		t.Errorf("empty date fields = %+v", c)
	}
	// An expired date is allowed: a certificate can be recorded as already expired.
	past := f.mkLink("Old", `,"valid_until":"2020-01-31"`)
	if past.State != "expired" || past.ValidUntil == nil || *past.ValidUntil != "2020-01-31" {
		t.Errorf("past certification = %+v", past)
	}
	// A file part with no content-type header and a quoted filename round-trips.
	if c := f.mkLink("JSON", ``); c.Source != "link" || c.URL != "https://example.com/r" || c.Item != nil {
		t.Errorf("link = %+v", c)
	}
}

func TestTrustCertCreateAndPublish(t *testing.T) {
	f := newCertFx(t, "tccpub")
	if _, err := f.s.db.GetTrustCenterSettings(context.Background(), f.o.id); err != nil {
		t.Fatal(err)
	}
	// Default level comes from the settings (gated in the fixture).
	c := f.mkLink("Pub", `,"publish":true`)
	if c.Item == nil || c.Item.AccessLevel != "gated" || c.Item.SortOrder != 1 {
		t.Errorf("published at default = %+v", c.Item)
	}
	c2 := f.mkLink("Pub2", `,"publish":true,"access_level":"public"`)
	if c2.Item == nil || c2.Item.AccessLevel != "public" || c2.Item.SortOrder != 2 {
		t.Errorf("published public = %+v", c2.Item)
	}
	// Multipart publish.
	c3 := f.mkFile("Pub3", certPDF, map[string]string{"publish": "true", "access_level": "restricted"})
	if c3.Item == nil || c3.Item.AccessLevel != "restricted" {
		t.Errorf("multipart publish = %+v", c3.Item)
	}
	// Publish endpoint: 201, then 409 already published, 404 for another org.
	u := f.mkLink("Later", ``)
	if r := f.publish(u.ID, "public"); r.code != 201 || decodeCert(t, r).Item == nil {
		t.Errorf("publish = %d %s", r.code, r.text())
	}
	if r := f.publish(u.ID, "public"); r.code != 409 || codeOf(tdResp{r.code, r.text()}) != "trust_certification_already_published" {
		t.Errorf("republish = %d %s", r.code, r.text())
	}
	if r := f.publish(u.ID+99999, ""); r.code != 404 {
		t.Errorf("publish unknown = %d", r.code)
	}
	if r := f.mgrJSON(f.s.handlePublishTrustCertification, http.MethodPost, certBase+"/x/publish", `{}`, "id", "x"); r.code != 400 {
		t.Errorf("publish non-numeric id = %d", r.code)
	}
	// Unpublish keeps the certification; a second one is a 404.
	del := func() certResp {
		return f.mgrJSON(f.s.handleUnpublishTrustCertification, http.MethodDelete, fmt.Sprintf("%s/%d/publish", certBase, u.ID), "", "id", fmt.Sprint(u.ID))
	}
	if r := del(); r.code != 204 {
		t.Errorf("unpublish = %d %s", r.code, r.text())
	}
	if r := del(); r.code != 404 {
		t.Errorf("unpublish again = %d", r.code)
	}
	if got, err := f.s.db.GetTrustCertification(context.Background(), f.o.id, u.ID); err != nil || got.Item != nil {
		t.Errorf("after unpublish: %+v %v", got, err)
	}
	// An invalid level on create+publish creates nothing; a non-published create has no item.
	if r := f.mgrJSON(f.s.handleCreateTrustCertification, http.MethodPost, certBase,
		`{"label":"Z","link_type":"other","url":"https://e.com","publish":true,"access_level":"x"}`); r.code != 400 {
		t.Errorf("bad level = %d", r.code)
	}
	if list, _ := f.s.db.ListTrustCertifications(context.Background(), f.o.id); len(list) != 4 {
		t.Errorf("certifications = %d, want 4", len(list))
	}
}

func TestTrustCertUpdateIsPartial(t *testing.T) {
	f := newCertFx(t, "tcupd")
	c := f.mkLink("Orig", `,"valid_from":"2026-01-01","valid_until":"2027-01-01","verified_at":"2026-02-02"`)
	put := func(c trustCertificationJSON, body string) certResp {
		return f.mgrJSON(f.s.handleUpdateTrustCertification, http.MethodPut, fmt.Sprintf("%s/%d", certBase, c.ID), body, "id", fmt.Sprint(c.ID))
	}
	// Absent keys stay; null clears a date.
	got := decodeCert(t, put(c, `{"label":"Renamed","valid_until":null}`))
	if got.Label != "Renamed" || got.ValidUntil != nil || got.ValidFrom == nil || *got.ValidFrom != "2026-01-01" ||
		got.Issuer != "Acme" || got.LinkType != "audit_report" || got.VerifiedAt == nil || got.URL != "https://example.com/r" {
		t.Errorf("partial update = %+v", got)
	}
	got = decodeCert(t, put(c, `{"url":"https://example.com/new","verified_at":null,"issuer":""}`))
	if got.URL != "https://example.com/new" || got.VerifiedAt != nil || got.Issuer != "" || got.Label != "Renamed" {
		t.Errorf("second update = %+v", got)
	}
	for name, body := range map[string]string{
		"number":  `{"valid_until":1794000000}`,
		"rfc3339": `{"valid_from":"2026-11-13T10:00:00Z"}`,
		"empty":   `{"verified_at":""}`,
		"bad day": `{"valid_until":"2026-02-30"}`,
	} {
		if r := put(c, body); r.code != 400 || codeOf(tdResp{r.code, r.text()}) != "trust_date_invalid" {
			t.Errorf("%s: %d %s", name, r.code, r.text())
		}
	}
	for name, body := range map[string]string{
		"contract": `{"link_type":"contract"}`, "empty label": `{"label":""}`, "ftp": `{"url":"ftp://x/y"}`,
		"order": `{"valid_from":"2030-01-01","valid_until":"2029-01-01"}`, "not json": `[`,
	} {
		if r := put(c, body); r.code != 400 {
			t.Errorf("%s: %d %s, want 400", name, r.code, r.text())
		}
	}
	// A file certification takes no url; a missing or other-org id is 404.
	fc := f.mkFile("File", certPDF, nil)
	if r := put(fc, `{"url":"https://e.com"}`); r.code != 400 || codeOf(tdResp{r.code, r.text()}) != "trust_certification_source" {
		t.Errorf("url on file cert = %d %s", r.code, r.text())
	}
	if r := put(trustCertificationJSON{ID: c.ID + 99999}, `{}`); r.code != 404 {
		t.Errorf("missing = %d", r.code)
	}
	other := newCertFx(t, "tcupd2")
	oc := other.mkLink("Theirs", ``)
	if r := put(trustCertificationJSON{ID: oc.ID}, `{"label":"hijack"}`); r.code != 404 {
		t.Errorf("other org update = %d %s, want 404", r.code, r.text())
	}
	if r := other.mgrDL(oc.ID); r.code != 404 { // a link certification has no file
		t.Errorf("download of link cert = %d", r.code)
	}
	if r := f.mgrDL(oc.ID); r.code != 404 {
		t.Errorf("other org download = %d", r.code)
	}
	if r := f.mgrJSON(f.s.handleDeleteTrustCertification, http.MethodDelete, certBase+"/"+fmt.Sprint(oc.ID), "", "id", fmt.Sprint(oc.ID)); r.code != 404 {
		t.Errorf("other org delete = %d", r.code)
	}
	if r := f.mgrJSON(f.s.handleVerifyTrustCertification, http.MethodPost, certBase+"/"+fmt.Sprint(oc.ID)+"/verify", "", "id", fmt.Sprint(oc.ID)); r.code != 404 {
		t.Errorf("other org verify = %d", r.code)
	}
	if got, err := other.s.db.GetTrustCertification(context.Background(), other.o.id, oc.ID); err != nil || got.Label != "Theirs" {
		t.Errorf("other org's certification changed: %+v %v", got, err)
	}
	// Verify stamps today and the verifier.
	v := decodeCert(t, f.mgrJSON(f.s.handleVerifyTrustCertification, http.MethodPost, certBase+"/"+fmt.Sprint(c.ID)+"/verify", "", "id", fmt.Sprint(c.ID)))
	if v.VerifiedAt == nil || v.VerifiedBy != f.mgr {
		t.Errorf("verify = %+v", v)
	}
}

func TestTrustCertReplaceFileAndDeleteRemoveBlobs(t *testing.T) {
	f := newCertFx(t, "tcblob")
	c := f.mkFile("Blob", certPDF, nil)
	if n := len(f.blobFiles()); n != 1 {
		t.Fatalf("blobs after create = %d", n)
	}
	oldRow, _ := f.s.db.GetTrustCertification(context.Background(), f.o.id, c.ID)

	repl := func(id int64, p *certPart) certResp {
		body, ctype := multipartBody(t, nil, p)
		return f.req(f.s.handleReplaceTrustCertificationFile, http.MethodPost, fmt.Sprintf("%s/%d/file", certBase, id), body, ctype, "manager", f.mgr, "id", fmt.Sprint(id))
	}
	// A rejected replacement changes nothing and writes nothing.
	repl(c.ID, &certPart{"file", "x.pdf", "application/pdf", []byte("<html>")}).code400(t, "trust_file_type")
	repl(c.ID, nil).code400(t, "trust_file_required")
	if n := len(f.blobFiles()); n != 1 {
		t.Errorf("blobs after rejected replace = %d, want 1", n)
	}
	got := decodeCert(t, repl(c.ID, &certPart{"file", "new.png", "image/png", certPNG}))
	if got.FileContentType != "image/png" || got.FileName != "new.png" {
		t.Errorf("replaced = %+v", got)
	}
	files := f.blobFiles()
	if len(files) != 1 || strings.HasSuffix(files[0], filepath.Base(oldRow.FileKey)) {
		t.Errorf("blobs after replace = %v, want only the new one", files)
	}
	if r := f.mgrDL(c.ID); r.code != 200 || !bytes.Equal(r.body, certPNG) {
		t.Errorf("download after replace = %d", r.code)
	}
	// A link certification has no file to replace.
	lc := f.mkLink("Link", ``)
	if r := repl(lc.ID, &certPart{"file", "x.pdf", "", certPDF}); r.code != 400 {
		t.Errorf("replace on link = %d", r.code)
	}
	if n := len(f.blobFiles()); n != 1 {
		t.Errorf("blobs = %d", n)
	}
	// Delete removes the row, the item and, after the commit, the blob.
	f.publish(c.ID, "public")
	if r := f.mgrJSON(f.s.handleDeleteTrustCertification, http.MethodDelete, fmt.Sprintf("%s/%d", certBase, c.ID), "", "id", fmt.Sprint(c.ID)); r.code != 204 {
		t.Fatalf("delete = %d %s", r.code, r.text())
	}
	if n := len(f.blobFiles()); n != 0 {
		t.Errorf("blobs after delete = %d", n)
	}
	if items, _ := f.s.db.ListPublishedTrustCenterItems(context.Background(), f.o.id); len(items) != 0 {
		t.Errorf("items after delete = %d", len(items))
	}
}

func TestTrustCertFailedInsertRemovesTheBlob(t *testing.T) {
	f := newCertFx(t, "tcfail")
	org, _ := f.s.db.GetOrganization(context.Background(), f.o.id)
	body, ctype := multipartBody(t, nil, &certPart{"file", "a.pdf", "", certPDF})
	r := httptest.NewRequest(http.MethodPost, certBase, body)
	r.Header.Set(echo.HeaderContentType, ctype)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	fh := r.MultipartForm.File["file"][0]
	// Past the handler's own checks the insert still fails (the dates are out
	// of order): the blob written for it must go.
	from, until := "2030-01-01", "2020-01-01"
	_, err := f.s.createTrustCertification(context.Background(), org,
		db.TrustCertificationInput{Label: "x", LinkType: "other", ValidFrom: &from, ValidUntil: &until}, fh, f.mgr)
	if !errors.Is(err, db.ErrTrustCenterInvalid) {
		t.Fatalf("err = %v, want ErrTrustCenterInvalid", err)
	}
	if n := len(f.blobFiles()); n != 0 {
		t.Errorf("blobs after failed insert = %d, want 0", n)
	}
}

// ---- download helper --------------------------------------------------------

func TestTrustDownloadNameAndDisposition(t *testing.T) {
	cases := []struct{ name, ctype, want string }{
		{"report.pdf", "application/pdf", "report.pdf"},
		{"REPORT.PDF", "application/pdf", "REPORT.PDF"},
		{"report.png", "application/pdf", "report.png.pdf"},
		{"photo.jpeg", "image/jpeg", "photo.jpeg"},
		{"photo", "image/jpeg", "photo.jpg"},
		{"evil.html", "image/png", "evil.html.png"},
		{"a\r\nSet-Cookie: x=1.pdf", "application/pdf", "aSet-Cookie: x=1.pdf"},
		{"../../etc/passwd", "application/pdf", "passwd.pdf"},
		{`C:\dir\x.pdf`, "application/pdf", "x.pdf"},
		{"", "application/pdf", "certificate.pdf"},
		{"a\u202Eexe.pdf", "application/pdf", "aexe.pdf"}, // U+202E, a bidi override, is a format character
		{"a\u200Bb\ufeff.pdf", "application/pdf", "ab.pdf"},
	}
	for _, c := range cases {
		if got := trustDownloadName(c.name, c.ctype); got != c.want {
			t.Errorf("trustDownloadName(%q, %s) = %q, want %q", c.name, c.ctype, got, c.want)
		}
	}
	for _, name := range []string{`a"b.pdf`, "naïve é 日本.pdf", "semi;colon.pdf", `back\slash.pdf`, "a\r\nb.pdf"} {
		n := trustDownloadName(name, "application/pdf")
		disp := mime.FormatMediaType("attachment", map[string]string{"filename": n})
		if strings.ContainsAny(disp, "\r\n") || !strings.HasPrefix(disp, "attachment") {
			t.Errorf("disposition for %q = %q", name, disp)
		}
		typ, params, err := mime.ParseMediaType(disp)
		if err != nil || typ != "attachment" || params["filename"] != n {
			t.Errorf("disposition %q does not round-trip %q: %v %v", disp, n, params, err)
		}
	}
}

func TestTrustCertDownloadHeadersOnEveryRoute(t *testing.T) {
	f := newCertFx(t, "tchdr")
	c := decodeCert(t, f.upload(map[string]string{"label": "H", "link_type": "certificate", "publish": "true", "access_level": "public"},
		&certPart{"file", `na"ïve é.pdf`, "text/html", certPDF}))
	check := func(name string, r certResp) {
		t.Helper()
		if r.code != 200 || !bytes.Equal(r.body, certPDF) {
			t.Errorf("%s: %d len %d", name, r.code, len(r.body))
			return
		}
		h := r.hdr
		if h.Get("Content-Type") != "application/pdf" || h.Get("X-Content-Type-Options") != "nosniff" ||
			h.Get("Cache-Control") != "no-store" || h.Get("Content-Length") != fmt.Sprint(len(certPDF)) {
			t.Errorf("%s: headers %v", name, h)
		}
		typ, params, err := mime.ParseMediaType(h.Get("Content-Disposition"))
		if err != nil || typ != "attachment" || params["filename"] != `na"ïve é.pdf` {
			t.Errorf("%s: disposition %q -> %v %v", name, h.Get("Content-Disposition"), params, err)
		}
		if r.hdr.Get("Location") != "" {
			t.Errorf("%s: redirects to %s", name, r.hdr.Get("Location"))
		}
	}
	check("manager", f.mgrDL(c.ID))
	check("preview", f.previewFile(c.Item.ID))
	pr := f.publicFile(trustTestBase, c.Item.ID, "")
	check("public", certResp{pr.code, []byte(pr.body), pr.hdr})
}

// ---- public API and preview -------------------------------------------------

func TestTrustCertPublicItemShape(t *testing.T) {
	f := newCertFx(t, "tcshape")
	until := "2099-12-31"
	pubFile := f.mkFile("Public file", certPDF, map[string]string{"valid_until": until, "valid_from": "2026-01-01", "publish": "true", "access_level": "public"})
	pubLink := f.mkLink("Public link", `,"publish":true,"access_level":"public"`)
	gatedFile := f.mkFile("Gated file", certPDF, map[string]string{"publish": "true", "access_level": "gated"})
	f.mkLink("Restricted link", `,"publish":true,"access_level":"restricted"`)
	f.mkLink("Unpublished", ``)

	items := publicItems(t, f)
	byTitle := map[string]map[string]any{}
	for _, it := range items {
		if it["kind"] == "certification" {
			byTitle[it["title"].(string)] = it
		}
	}
	if len(byTitle) != 4 {
		t.Fatalf("public certification items = %d, want 4 (unpublished is absent): %v", len(byTitle), items)
	}
	pf := byTitle["Public file"]
	if pf["id"] != pubFile.Item.ID || pf["access_level"] != "public" || pf["source"] != "file" || pf["file_name"] != "Public file.pdf" ||
		pf["link_type"] != "certificate" || pf["issuer"] != "BSI" || pf["valid_until"] != until || pf["valid_from"] != "2026-01-01" ||
		pf["state"] != "current" || pf["file_size"] != float64(len(certPDF)) {
		t.Errorf("public file item = %v", pf)
	}
	if _, ok := pf["url"]; ok {
		t.Errorf("file item has a url: %v", pf)
	}
	pl := byTitle["Public link"]
	if pl["url"] != "https://example.com/r" || pl["source"] != "link" || pl["id"] != pubLink.Item.ID {
		t.Errorf("public link item = %v", pl)
	}
	for _, title := range []string{"Gated file", "Restricted link"} {
		g := byTitle[title]
		for _, banned := range []string{"url", "file_name", "file_size", "file_key", "file_sha256", "created_by", "verified_by", "reference"} {
			if _, ok := g[banned]; ok {
				t.Errorf("%s exposes %s: %v", title, banned, g)
			}
		}
		if g["title"] != title || g["source"] == nil || g["link_type"] == nil {
			t.Errorf("%s lost its summary: %v", title, g)
		}
	}
	for _, it := range items {
		raw, _ := json.Marshal(it)
		for _, banned := range []string{"file_key", "file_sha256", "created_by", "verified_by", "trust/certifications", gatedFile.FileSHA256} {
			if strings.Contains(string(raw), banned) {
				t.Errorf("public item leaks %q: %s", banned, raw)
			}
		}
	}
	if _, ok := byTitle["Public file"]["id"].(string); !ok || byTitle["Public file"]["id"] == fmt.Sprint(pubFile.ID) {
		t.Errorf("item id must be the public uuid, got %v", byTitle["Public file"]["id"])
	}
	// Documents are unchanged in shape: no certification keys leak onto them.
	f.s.db.PublishTrustCenterItem(context.Background(), f.o.id, &db.TrustCenterItem{Kind: db.TrustKindFAQ, Reference: "q", AccessLevel: "public"}, "m@x.test")
	for _, it := range publicItems(t, f) {
		if it["kind"] == "faq" {
			if len(it) != 5 {
				t.Errorf("non-certification item gained fields: %v", it)
			}
		}
	}
	// The preview matches the public list.
	pi := previewItems(t, f)
	if a, b := mustJSON(publicItems(t, f)), mustJSON(pi); a != b {
		t.Errorf("preview differs from public:\n%s\n%s", a, b)
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestTrustCertExpiryBoundary(t *testing.T) {
	f := newCertFx(t, "tcexp")
	c := f.mkFile("Exp", certPDF, map[string]string{"publish": "true", "access_level": "public"})
	id := c.Item.ID
	served := func() (items, preview bool, pub, prev int) {
		items = itemByPublicID(publicItems(t, f), id) != nil
		preview = itemByPublicID(previewItems(t, f), id) != nil
		return items, preview, f.publicFile(trustTestBase, id, "").code, f.previewFile(id).code
	}
	for _, tc := range []struct {
		name string
		days *int
		want bool
	}{{"yesterday", intp(-1), false}, {"today", intp(0), true}, {"tomorrow", intp(1), true}, {"no date", nil, true}, {"yesterday again", intp(-1), false}} {
		setUntil(t, c.ID, tc.days)
		i, p, pub, prev := served()
		wantCode := map[bool]int{true: 200, false: 404}[tc.want]
		if i != tc.want || p != tc.want || pub != wantCode || prev != wantCode {
			t.Errorf("%s: items %v preview %v file %d preview file %d, want served=%v", tc.name, i, p, pub, prev, tc.want)
		}
	}
	// The manager still sees and can download an expired certification, flagged expired.
	setUntil(t, c.ID, intp(-1))
	if r := f.mgrDL(c.ID); r.code != 200 {
		t.Errorf("manager download of expired = %d", r.code)
	}
	list := f.mgrJSON(f.s.handleListTrustCertifications, http.MethodGet, certBase, "")
	var certs []trustCertificationJSON
	_ = json.Unmarshal(list.body, &certs)
	if len(certs) != 1 || certs[0].State != "expired" || certs[0].Item == nil {
		t.Errorf("manager list = %s", list.text())
	}
	// No job ran: renewing the date brings it back, nothing else written.
	setUntil(t, c.ID, intp(40))
	if i, _, pub, _ := served(); !i || pub != 200 {
		t.Errorf("renewed certification not served")
	}
}

func intp(n int) *int { return &n }

func TestTrustCertFileGate(t *testing.T) {
	f := newCertFx(t, "tcgate")
	c := f.mkFile("Gate", certPDF, map[string]string{"publish": "true", "access_level": "gated"})
	id := c.Item.ID
	// Gated: 404 for visitors (with or without a bearer) and in the preview; managers still get it.
	for _, bearer := range []string{"", "garbage", "Bearer-of-someone"} {
		if r := f.publicFile(trustTestBase, id, bearer); r.code != 404 {
			t.Errorf("gated public file (bearer %q) = %d", bearer, r.code)
		}
	}
	if r := f.previewFile(id); r.code != 404 {
		t.Errorf("gated preview file = %d", r.code)
	}
	if r := f.mgrDL(c.ID); r.code != 200 {
		t.Errorf("gated manager file = %d", r.code)
	}
	// Public: served, and a bearer changes nothing (the route is session-blind).
	patch := f.patch(id, `{"access_level":"public"}`, "manager", f.mgr)
	if patch.code != 200 {
		t.Fatalf("patch = %d %s", patch.code, patch.body)
	}
	a := f.publicFile(trustTestBase, id, "")
	b := f.publicFile(trustTestBase, id, "garbage")
	if a.code != 200 || b.code != 200 || a.body != b.body {
		t.Errorf("public file with/without bearer: %d %d", a.code, b.code)
	}
	// Restricted again: gone from the public route.
	f.patch(id, `{"access_level":"restricted"}`, "manager", f.mgr)
	if r := f.publicFile(trustTestBase, id, ""); r.code != 404 {
		t.Errorf("restricted public file = %d", r.code)
	}
}

func TestTrustCertPublicRouteGuards(t *testing.T) {
	f := newCertFx(t, "tcguard")
	other := trustNewOrg(t, f.s, "tcguard-o", true)
	c := f.mkFile("G", certPDF, map[string]string{"publish": "true", "access_level": "public"})
	id := c.Item.ID
	if r := f.publicFile(trustTestBase, id, ""); r.code != 200 {
		t.Fatalf("baseline = %d", r.code)
	}
	// Uppercase ids resolve like documents' do.
	if r := f.publicFile(trustTestBase, strings.ToUpper(id), ""); r.code != 200 {
		t.Errorf("uppercase id = %d", r.code)
	}
	for name, path := range map[string]string{
		"other org slug": "/api/v1/trust/" + other.slug + "/certifications/" + id + "/file",
		"unknown slug":   "/api/v1/trust/nope-" + f.o.slug + "/certifications/" + id + "/file",
		"not a uuid":     "/api/v1/trust/" + f.o.slug + "/certifications/" + c.Item.ID[:8] + "/file",
		"serial id":      "/api/v1/trust/" + f.o.slug + "/certifications/" + fmt.Sprint(c.ID) + "/file",
		"wrong tail":     "/api/v1/trust/" + f.o.slug + "/certifications/" + id + "/download",
		"no tail":        "/api/v1/trust/" + f.o.slug + "/certifications/" + id,
		"extra segment":  "/api/v1/trust/" + f.o.slug + "/certifications/" + id + "/file/x",
	} {
		if r := trustDo(f.s, trustTestBase, http.MethodGet, path, ""); r.code != 404 {
			t.Errorf("%s = %d, want 404", name, r.code)
		}
	}
	// A document item's id is not a certification.
	doc := trustPublishItem(t, f.s, f.o, db.TrustKindDocument, "pol-x", db.TrustAccessPublic)
	if r := f.publicFile(trustTestBase, doc.PublicID, ""); r.code != 404 {
		t.Errorf("document id on the certification route = %d", r.code)
	}
	// The host's organization must match the slug.
	if r := f.publicFile(other.domain, id, ""); r.code != 404 {
		t.Errorf("file through another org's host = %d", r.code)
	}
	if r := f.publicFile(f.o.domain, id, ""); r.code != 200 {
		t.Errorf("file through its own host = %d", r.code)
	}
	// Methods other than GET.
	if r := trustDo(f.s, trustTestBase, http.MethodPost, "/api/v1/trust/"+f.o.slug+"/certifications/"+id+"/file", ""); r.code != 404 {
		t.Errorf("POST = %d", r.code)
	}
	// Hidden: gone from the file route and the items.
	itm, _ := f.s.db.GetTrustCenterItem(context.Background(), f.o.id, id)
	if err := f.s.db.HideTrustCenterItem(context.Background(), f.o.id, itm.ID, db.TrustHiddenRetired, "system"); err != nil {
		t.Fatal(err)
	}
	if r := f.publicFile(trustTestBase, id, ""); r.code != 404 || itemByPublicID(publicItems(t, f), id) != nil {
		t.Errorf("hidden item still served: %d", r.code)
	}
	// Unpublished.
	f.mgrJSON(f.s.handleUnpublishTrustCertification, http.MethodDelete, fmt.Sprintf("%s/%d/publish", certBase, c.ID), "", "id", fmt.Sprint(c.ID))
	if r := f.publicFile(trustTestBase, id, ""); r.code != 404 {
		t.Errorf("unpublished file = %d", r.code)
	}
	// Trust center off: public 404, preview still works.
	d := f.mkFile("D", certPDF, map[string]string{"publish": "true", "access_level": "public"})
	st, _ := f.s.db.GetTrustCenterSettings(context.Background(), f.o.id)
	st.Enabled = false
	if err := f.s.db.UpsertTrustCenterSettings(context.Background(), f.o.id, st, "m@x.test"); err != nil {
		t.Fatal(err)
	}
	if r := f.publicFile(trustTestBase, d.Item.ID, ""); r.code != 404 {
		t.Errorf("file while off = %d", r.code)
	}
	if r := f.previewFile(d.Item.ID); r.code != 200 || !bytes.Equal(r.body, certPDF) {
		t.Errorf("preview file while off = %d", r.code)
	}
	// A missing blob is the standard 404 for visitors.
	for _, p := range f.blobFiles() {
		_ = os.Remove(p)
	}
	st.Enabled = true
	_ = f.s.db.UpsertTrustCenterSettings(context.Background(), f.o.id, st, "m@x.test")
	if r := f.publicFile(trustTestBase, d.Item.ID, ""); r.code != 404 {
		t.Errorf("file with missing blob = %d", r.code)
	}
}

func TestTrustCertPublicItemsSkipItemsWithoutACertification(t *testing.T) {
	f := newCertFx(t, "tcorphan")
	// An item of kind certification whose row never existed (or was removed
	// behind our back) must not be emitted bare, publicly or in the preview, and
	// the manager list shows it with certification null instead of failing.
	it := trustPublishItem(t, f.s, f.o, db.TrustKindCertification, "999999", db.TrustAccessPublic)
	nonNumeric := trustPublishItem(t, f.s, f.o, db.TrustKindCertification, "not-a-number", db.TrustAccessPublic)
	if itemByPublicID(publicItems(t, f), it.PublicID) != nil || itemByPublicID(publicItems(t, f), nonNumeric.PublicID) != nil {
		t.Error("orphan certification item emitted publicly")
	}
	if itemByPublicID(previewItems(t, f), it.PublicID) != nil {
		t.Error("orphan certification item emitted in the preview")
	}
	r := f.mgrJSON(f.s.handleListTrustItems, http.MethodGet, tmItems, "")
	if r.code != 200 {
		t.Fatalf("manager items with an orphan = %d %s", r.code, r.text())
	}
	var rows []trustManagerItem
	_ = json.Unmarshal(r.body, &rows)
	if len(rows) != 2 || rows[0].Certification != nil || rows[1].Certification != nil {
		t.Errorf("rows = %s", r.text())
	}
	// And one that is real carries its summary.
	c := f.mkFile("Real", certPDF, map[string]string{"publish": "true", "valid_until": "2099-01-01"})
	r = f.mgrJSON(f.s.handleListTrustItems, http.MethodGet, tmItems, "")
	_ = json.Unmarshal(r.body, &rows)
	var got *trustManagerItem
	for i := range rows {
		if rows[i].ID == c.Item.ID {
			got = &rows[i]
		}
	}
	if got == nil || got.Certification == nil || got.Certification.Label != "Real" || got.Certification.Source != "file" ||
		got.Certification.State != "current" || got.Certification.ID != c.ID || got.Document != nil {
		t.Errorf("certification row = %+v", got)
	}
}

func TestTrustCertPatchAndReorder(t *testing.T) {
	f := newCertFx(t, "tcorder")
	a := f.mkLink("A", `,"publish":true`)
	b := f.mkLink("B", `,"publish":true`)
	c := f.mkLink("C", `,"publish":true`)
	// PATCH changes the level, with no classification guard for certifications.
	if r := f.patch(a.Item.ID, `{"access_level":"public"}`, "manager", f.mgr); r.code != 200 || !strings.Contains(r.body, `"access_level":"public"`) || !strings.Contains(r.body, `"certification":{`) {
		t.Errorf("patch = %d %s", r.code, r.body)
	}
	if got, _ := f.s.db.GetTrustCertification(context.Background(), f.o.id, a.ID); got.Item.AccessLevel != "public" {
		t.Errorf("level = %s", got.Item.AccessLevel)
	}
	if r := f.patch(a.Item.ID, `{"access_level":"bogus"}`, "manager", f.mgr); r.code != 400 {
		t.Errorf("bad level = %d", r.code)
	}
	order := func(ids ...string) tdResp {
		b, _ := json.Marshal(map[string]any{"kind": "certification", "ids": ids})
		return f.mgrCall(f.s.handleReorderTrustItems, http.MethodPut, tmOrder, string(b))
	}
	if r := order(c.Item.ID, a.Item.ID, b.Item.ID); r.code != 200 {
		t.Fatalf("reorder = %d %s", r.code, r.body)
	}
	certs, _ := f.s.db.ListTrustCertifications(context.Background(), f.o.id)
	var labels []string
	for _, x := range certs {
		labels = append(labels, x.Label)
	}
	if strings.Join(labels, ",") != "C,A,B" {
		t.Errorf("order = %v, want C,A,B", labels)
	}
	if got := publicItems(t, f); got[0]["title"] != "C" || got[1]["title"] != "A" {
		t.Errorf("public order = %v", got)
	}
	// A stale list (missing one id) is the 409.
	if r := order(c.Item.ID, a.Item.ID); r.code != 409 {
		t.Errorf("stale reorder = %d %s", r.code, r.body)
	}
}

func TestTrustCertOverdueJSONKey(t *testing.T) {
	f := newCertFx(t, "tcover")
	c := f.mkLink("Soon", `,"valid_until":"2020-01-01"`)
	setUntil(t, c.ID, intp(5))
	r := f.mgrJSON(f.s.handleOverdueSummary, http.MethodGet, "/api/v1/overdue", "")
	if r.code != 200 {
		t.Fatalf("overdue = %d %s", r.code, r.text())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(r.body, &raw); err != nil {
		t.Fatal(err)
	}
	var items []db.OverdueItem
	if err := json.Unmarshal(raw["trust_certifications"], &items); err != nil {
		t.Fatalf("trust_certifications: %s", r.text())
	}
	if len(items) != 1 || items[0].Title != "Soon" || items[0].State != "expiring" || items[0].EntityType != "trust_certification" ||
		items[0].LinkID != c.ID || items[0].ParentTitle != "Acme" {
		t.Errorf("items = %+v", items)
	}
	// Every role sees it (GET /overdue is open to all roles).
	if r := f.json(f.s.handleOverdueSummary, http.MethodGet, "/api/v1/overdue", "", "reader", f.read); r.code != 200 || !strings.Contains(r.text(), `"trust_certifications":[{`) {
		t.Errorf("reader overdue = %d", r.code)
	}
}

// ---- body limits ------------------------------------------------------------

// limitServer is the production body-limit middleware in front of the real
// routes, with a fixed session standing in for authentication.
func limitServer(t *testing.T, f *certFx) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Use(globalBodyLimit())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("org_id", f.o.id)
			c.Set("user_role", "manager")
			c.Set("user_email", f.mgr)
			return next(c)
		}
	})
	f.s.echo = e
	f.s.routes()
	return e
}

func bigPDF(n int) []byte {
	return append([]byte("%PDF-1.4\n"), make([]byte, n-9)...)
}

func serve(e *echo.Echo, method, path string, body io.Reader, ctype string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	if ctype != "" {
		r.Header.Set(echo.HeaderContentType, ctype)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	return rec
}

func TestTrustCertBodyLimits(t *testing.T) {
	f := newCertFx(t, "tclimit")
	e := limitServer(t, f)
	const MB = 1 << 20

	upload := func(path string, size int) *httptest.ResponseRecorder {
		body, ctype := multipartBody(t, map[string]string{"label": "Big", "link_type": "audit_report"}, &certPart{"file", "big.pdf", "application/pdf", bigPDF(size)})
		return serve(e, http.MethodPost, path, body, ctype)
	}
	if rec := upload(certBase, 40*MB); rec.Code != http.StatusCreated {
		t.Fatalf("40 MB certification upload = %d %.200s", rec.Code, rec.Body.String())
	}
	if rec := upload(certBase, 60*MB); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("60 MB certification upload = %d, want 413", rec.Code)
	}
	c, _ := f.s.db.ListTrustCertifications(context.Background(), f.o.id)
	if len(c) != 1 || len(f.blobFiles()) != 1 {
		t.Fatalf("after 40+60 MB: %d certifications, %d blobs", len(c), len(f.blobFiles()))
	}
	if rec := upload(fmt.Sprintf("%s/%d/file", certBase, c[0].ID), 40*MB); rec.Code != http.StatusOK {
		t.Errorf("40 MB replace = %d %.200s", rec.Code, rec.Body.String())
	}
	if rec := upload(fmt.Sprintf("%s/%d/file", certBase, c[0].ID), 60*MB); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("60 MB replace = %d, want 413", rec.Code)
	}

	// Evidence uploads were capped at 10 MB by the global limit before.
	p := &db.Program{Key: fmt.Sprintf("EV%d", f.o.id), Title: "p"}
	if err := f.s.db.CreateProgram(context.Background(), f.o.id, p); err != nil {
		t.Fatal(err)
	}
	obj := &db.Objective{ProgramID: p.ID, Title: "o"}
	if err := f.s.db.CreateObjective(context.Background(), f.o.id, obj); err != nil {
		t.Fatal(err)
	}
	ck := &db.Checkin{ObjectiveID: obj.ID, CreatedBy: f.mgr}
	if err := f.s.db.CreateCheckin(context.Background(), f.o.id, ck); err != nil {
		t.Fatal(err)
	}
	evPath := fmt.Sprintf("/api/v1/checkins/%d/evidence", ck.ID)
	if rec := upload(evPath, 40*MB); rec.Code != http.StatusCreated {
		t.Errorf("40 MB evidence upload = %d %.200s", rec.Code, rec.Body.String())
	} else {
		// Streamed, not read into memory: the recorded size and hash are still right.
		var ev struct {
			Size   int64  `json:"size_bytes"`
			SHA256 string `json:"sha256"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &ev)
		sum := sha256.Sum256(bigPDF(40 * MB))
		if ev.Size != 40*MB || ev.SHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("evidence size %d sha %.12s do not match the upload", ev.Size, ev.SHA256)
		}
	}
	if rec := upload(evPath, 60*MB); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("60 MB evidence upload = %d, want 413", rec.Code)
	}

	// Every other route keeps the 10 MB cap: a JSON route, another trust-center
	// POST, and the same evidence path with a different method or a lookalike path.
	pad := `{"title":"` + strings.Repeat("x", 11*MB) + `"}`
	if rec := serve(e, http.MethodPut, "/api/v1/trust-center/settings", strings.NewReader(pad), echo.MIMEApplicationJSON); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("11 MB JSON = %d, want 413", rec.Code)
	}
	if rec := serve(e, http.MethodPost, fmt.Sprintf("%s/%d/verify", certBase, c[0].ID), bytes.NewReader(make([]byte, 11*MB)), "application/octet-stream"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("11 MB to verify = %d, want 413", rec.Code)
	}
	if rec := serve(e, http.MethodPut, certBase+"/1", bytes.NewReader(make([]byte, 11*MB)), echo.MIMEApplicationJSON); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("11 MB to PUT certification = %d, want 413", rec.Code)
	}
	for _, path := range []string{"/api/v1/checkins/1/evidence/x", "/api/v1/checkins/1/evidencex", "/api/v1/trust-center/certifications/1/file/x"} {
		if rec := serve(e, http.MethodPost, path, bytes.NewReader(make([]byte, 11*MB)), "application/octet-stream"); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("11 MB to %s = %d, want 413", path, rec.Code)
		}
	}
	if rec := serve(e, http.MethodPut, "/api/v1/checkins/1/evidence", bytes.NewReader(make([]byte, 11*MB)), "application/octet-stream"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("11 MB PUT to the evidence path = %d, want 413", rec.Code)
	}
}

func TestBodyLimitSkipsOnlyTheRoutePattern(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/api/v1/checkins/:id/evidence", true},
		{"POST", "/api/v1/trust-center/certifications", true},
		{"POST", "/api/v1/trust-center/certifications/:id/file", true},
		{"PUT", "/api/v1/checkins/:id/evidence", false},
		{"POST", "/api/v1/checkins/7/evidence", false}, // a raw URL is not a pattern
		{"POST", "/api/v1/trust-center/certifications/:id/verify", false},
		{"POST", "/api/v1/risks", false},
		{"POST", "", false},
	} {
		c := echo.New().NewContext(httptest.NewRequest(tc.method, "/", nil), httptest.NewRecorder())
		c.SetPath(tc.path)
		if got := bodyLimitSkipper(c); got != tc.want {
			t.Errorf("%s %q skip = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

// ---- review round: cleanup, memory, limits, validation -----------------------

// ctxStore fails Delete when its context is cancelled, like S3 does, and can
// fail PutStream after writing part of the file.
type ctxStore struct {
	blob.Store
	failPut bool
}

func (c ctxStore) Delete(ctx context.Context, org, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Store.Delete(ctx, org, path)
}

func (c ctxStore) PutStream(ctx context.Context, org, path, ct string, r io.Reader) error {
	if c.failPut {
		_ = c.Store.PutStream(ctx, org, path, ct, io.LimitReader(r, 10))
		return errors.New("connection reset")
	}
	return c.Store.PutStream(ctx, org, path, ct, r)
}

func TestTrustCertBlobCleanupSurvivesACancelledRequest(t *testing.T) {
	f := newCertFx(t, "tcctx")
	f.s.blobs = ctxStore{Store: f.s.blobs}
	c := f.mkFile("Cancel", certPDF, nil)
	org, _ := f.s.db.GetOrganization(context.Background(), f.o.id)
	row, _ := f.s.db.GetTrustCertification(context.Background(), f.o.id, c.ID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.s.deleteTrustBlob(ctx, org, row.FileKey)
	if n := len(f.blobFiles()); n != 0 {
		t.Errorf("blob left behind after the request context was cancelled: %d", n)
	}
}

func TestTrustCertReleaseKeepsABlobTheRowUses(t *testing.T) {
	f := newCertFx(t, "tcrel")
	c := f.mkFile("Rel", certPDF, nil)
	org, _ := f.s.db.GetOrganization(context.Background(), f.o.id)
	row, _ := f.s.db.GetTrustCertification(context.Background(), f.o.id, c.ID)
	// A commit error that was really a commit: the row points at the new blob.
	f.s.releaseTrustBlob(context.Background(), org, row.FileKey)
	if n := len(f.blobFiles()); n != 1 {
		t.Fatalf("blob in use was deleted (%d left)", n)
	}
	// A blob no row references is removed.
	if err := f.s.blobs.Put(context.Background(), org.UUID, "trust/certifications/stray.pdf", certPDF); err != nil {
		t.Fatal(err)
	}
	f.s.releaseTrustBlob(context.Background(), org, "trust/certifications/stray.pdf")
	if n := len(f.blobFiles()); n != 1 {
		t.Errorf("unreferenced blob kept (%d files)", n)
	}
}

func TestTrustCertFailedPartialWriteRemovesTheKey(t *testing.T) {
	f := newCertFx(t, "tcpart")
	f.s.blobs = ctxStore{Store: f.s.blobs, failPut: true}
	r := f.upload(map[string]string{"label": "P", "link_type": "other"}, &certPart{"file", "a.pdf", "", certPDF})
	if r.code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", r.code)
	}
	if n := len(f.blobFiles()); n != 0 {
		t.Errorf("partial write left %d blob(s)", n)
	}
}

func TestTrustCertUploadsSpillToDisk(t *testing.T) {
	body, ctype := multipartBody(t, map[string]string{"label": "x"}, &certPart{"file", "big.pdf", "", bigPDF(3 << 20)})
	r := httptest.NewRequest(http.MethodPost, certBase, body)
	r.Header.Set(echo.HeaderContentType, ctype)
	c := echo.New().NewContext(r, httptest.NewRecorder())
	if err := parseUploadForm(c); err != nil {
		t.Fatal(err)
	}
	fh, err := c.FormFile("file")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := fh.Open()
	defer src.Close()
	if _, onDisk := src.(*os.File); !onDisk {
		t.Errorf("a 3 MB file part is held in memory (%T), want a temporary file", src)
	}
	if c.FormValue("label") != "x" {
		t.Error("form values lost")
	}
}

func TestTrustCertChunkedOversizeBodyIs413(t *testing.T) {
	f := newCertFx(t, "tcchunk")
	e := limitServer(t, f)
	body, ctype := multipartBody(t, map[string]string{"label": "Big", "link_type": "other"}, &certPart{"file", "big.pdf", "application/pdf", bigPDF(60 << 20)})
	// Hide the length: the body is chunked, so the limit is hit while reading.
	rec := serve(e, http.MethodPost, certBase, struct{ io.Reader }{body}, ctype)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("chunked 60 MB create = %d %.120s, want 413", rec.Code, rec.Body.String())
	}
	if len(f.blobFiles()) != 0 {
		t.Error("blob written for an oversize body")
	}
	c := f.mkFile("C", certPDF, nil)
	body, ctype = multipartBody(t, nil, &certPart{"file", "big.pdf", "application/pdf", bigPDF(60 << 20)})
	rec = serve(e, http.MethodPost, fmt.Sprintf("%s/%d/file", certBase, c.ID), struct{ io.Reader }{body}, ctype)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("chunked 60 MB replace = %d, want 413", rec.Code)
	}
}

func TestTrustCertRoundTwoValidation(t *testing.T) {
	f := newCertFx(t, "tcr2")
	post := func(body string) certResp {
		return f.mgrJSON(f.s.handleCreateTrustCertification, http.MethodPost, certBase, body)
	}
	// Year 0 passes a naive parser and used to reach Postgres, which refuses it.
	for _, d := range []string{"0000-01-01", "0000-12-31"} {
		r := post(`{"label":"L","link_type":"other","url":"https://e.com","valid_until":"` + d + `"}`)
		r.code400(t, "trust_date_invalid")
	}
	// Multipart: access_level is checked without publish; publish takes true/false only.
	f.upload(map[string]string{"label": "L", "link_type": "other", "url": "https://e.com", "access_level": "secret"}, nil).code400(t, "invalid_request")
	for _, v := range []string{"on", "yes", "1", "TRUE"} {
		f.upload(map[string]string{"label": "L", "link_type": "other", "url": "https://e.com", "publish": v}, nil).code400(t, "invalid_request")
	}
	if r := post(`{"label":"L","link_type":"other","url":"https://e.com","access_level":"secret"}`); r.code != 400 {
		t.Errorf("json bad level without publish = %d", r.code)
	}
	ok := f.upload(map[string]string{"label": "L", "link_type": "other", "url": "https://e.com", "publish": "false", "access_level": "public"}, nil)
	if c := decodeCert(t, ok); c.Item != nil {
		t.Errorf("publish=false published: %+v", c.Item)
	}
	if list, _ := f.s.db.ListTrustCertifications(context.Background(), f.o.id); len(list) != 1 {
		t.Errorf("rejected requests created %d certifications", len(list)-1)
	}
	// PUT trims the url like create does.
	c := f.mkLink("Trim", ``)
	put := f.mgrJSON(f.s.handleUpdateTrustCertification, http.MethodPut, fmt.Sprintf("%s/%d", certBase, c.ID), `{"url":"  https://example.com/trimmed  "}`, "id", fmt.Sprint(c.ID))
	if got := decodeCert(t, put); got.URL != "https://example.com/trimmed" {
		t.Errorf("url = %q", got.URL)
	}
}
