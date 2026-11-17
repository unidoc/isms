package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var exportBytes = []byte("%PDF-1.7 fake\x00bytes")

// exportServer serves a document body (for md) and an export endpoint, and
// points the CLI at itself. exportStatus 0 means 200 with the file.
func exportServer(t *testing.T, exportStatus int) *[]string {
	t.Helper()
	var reqs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case strings.HasSuffix(r.URL.Path, "/export"):
			if exportStatus != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(exportStatus)
				_, _ = w.Write([]byte(`{"message":"PDF export is not available on this server","code":"export_not_licensed"}`))
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=pol-1-v1.0.pdf")
			_, _ = w.Write(exportBytes)
		case strings.HasSuffix(r.URL.Path, "/body"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"document_id":"pol-1","title":"Access Policy","version":"1.0","status":"approved","author":"Ann","body":"Hello\n"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ISMS_API_URL", srv.URL)
	t.Setenv("ISMS_API_KEY", "test-token")
	t.Setenv("UNIDOC_API_KEY", "") // the CLI must not care about it
	return &reqs
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// runExport executes `export <sub> args...` in a fresh temp dir.
func runExport(t *testing.T, args ...string) (dir, stdout, stderr string, err error) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	cmd := exportCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	stderr = captureStderr(t, func() {
		stdout = captureStdout(t, func() { err = cmd.Execute() })
	})
	return
}

func TestExportPolicyPDFDefaultName(t *testing.T) {
	reqs := exportServer(t, 0)
	dir, out, errOut, err := runExport(t, "policy", "pol-1", "--format", "pdf", "--version", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	got, rerr := os.ReadFile(filepath.Join(dir, "pol-1-v1.0.pdf"))
	if rerr != nil || string(got) != string(exportBytes) {
		t.Fatalf("file: %v %q", rerr, got)
	}
	if !strings.Contains(errOut, "Saved pol-1-v1.0.pdf (") || !strings.Contains(errOut, "bytes)") {
		t.Errorf("stderr = %q", errOut)
	}
	if out != "" {
		t.Errorf("stdout must stay clean: %q", out)
	}
	if len(*reqs) != 1 || (*reqs)[0] != "/v1/documents/pol-1/export?format=pdf&version=1.0" {
		t.Errorf("requests = %v", *reqs)
	}
}

func TestExportPolicyRefusesToOverwriteDefaultName(t *testing.T) {
	exportServer(t, 0)
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "pol-1-v1.0.pdf")
	if err := os.WriteFile(path, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exportCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"policy", "pol-1", "--format", "pdf"})
	var err error
	captureStderr(t, func() { err = cmd.Execute() })
	if err == nil || !strings.Contains(err.Error(), "pol-1-v1.0.pdf") || !strings.Contains(err.Error(), "-o") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "mine" {
		t.Errorf("existing file was modified: %q", b)
	}
}

func TestExportPolicyOutputFileOverwrites(t *testing.T) {
	exportServer(t, 0)
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("out.pdf", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exportCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"policy", "pol-1", "--format", "pdf", "-o", "out.pdf"})
	var err error
	errOut := captureStderr(t, func() { err = cmd.Execute() })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile("out.pdf"); string(b) != string(exportBytes) {
		t.Errorf("out.pdf = %q", b)
	}
	if !strings.Contains(errOut, "Saved out.pdf (") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestExportPolicyToStdoutPipe(t *testing.T) {
	exportServer(t, 0)
	_, out, _, err := runExport(t, "policy", "pol-1", "--format", "pdf", "-o", "-")
	if err != nil {
		t.Fatal(err)
	}
	if out != string(exportBytes) {
		t.Errorf("stdout = %q", out)
	}
}

func TestExportPolicyServerRefusalWritesNothing(t *testing.T) {
	exportServer(t, http.StatusForbidden)
	dir, _, _, err := runExport(t, "policy", "pol-1", "--format", "pdf")
	if err == nil || !strings.Contains(err.Error(), "API error 403: PDF export is not available on this server") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("files created: %v", entries)
	}
}

func TestExportUnsupportedAndSingleDocumentOnly(t *testing.T) {
	exportServer(t, 0)
	for _, f := range []string{"xlsx", "odt"} {
		_, _, _, err := runExport(t, "policy", "pol-1", "--format", f)
		if err == nil || !strings.Contains(err.Error(), "unsupported format: "+f+" (supported: md, pdf, docx)") {
			t.Errorf("%s: %v", f, err)
		}
	}
	for _, sub := range []string{"documents", "risks", "assets", "suppliers", "audit-pack", "manual"} {
		for _, f := range []string{"pdf", "docx"} {
			args := []string{sub, "--format", f}
			if sub == "audit-pack" {
				args = []string{sub, "iso27001", "--format", f}
			}
			_, _, _, err := runExport(t, args...)
			if err == nil || !strings.Contains(err.Error(), "PDF/DOCX export is available for a single document: isms export policy <id> --format pdf") {
				t.Errorf("%s --format %s: %v", sub, f, err)
			}
		}
	}
}

func TestExportPolicyMarkdownUnchanged(t *testing.T) {
	exportServer(t, 0)
	_, out, _, err := runExport(t, "policy", "pol-1")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Access Policy\n\n| Field | Value |\n|-------|-------|\n| Document ID | pol-1 |\n| Version | 1.0 |\n| Status | approved |\n| Author | Ann |\n\n---\n\nHello\n"
	if out != want {
		t.Errorf("markdown output changed:\n%q\nwant\n%q", out, want)
	}
}
