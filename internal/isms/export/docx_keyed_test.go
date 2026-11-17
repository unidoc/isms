package export

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/unidoc/unioffice/v2/document"
)

// Key-gated DOCX test: two real renders (draft and approved), each read back with
// document.Read and checked with the same assertions as the in-memory model.
// ISMS_TEST_EXPORT_OUT=<dir> also writes draft.docx and approved.docx.
func TestKeyedRenderDOCX(t *testing.T) {
	needKey(t)
	out := os.Getenv("ISMS_TEST_EXPORT_OUT")
	if out != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, rn := range []struct {
		name, status string
		draft        bool
	}{{"draft", "draft", true}, {"approved", "approved", false}} {
		t.Run(rn.name, func(t *testing.T) {
			b, err := Render("docx", richDoc(rn.status))
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !bytes.HasPrefix(b, []byte("PK")) {
				t.Fatalf("not a zip: %q", b[:min(len(b), 8)])
			}
			if out != "" {
				if err := os.WriteFile(filepath.Join(out, rn.name+".docx"), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			doc, err := document.Read(bytes.NewReader(b), int64(len(b)))
			if err != nil {
				t.Fatalf("reading the DOCX back: %v", err)
			}
			checkDOCX(t, doc, rn.draft)
		})
	}
}
