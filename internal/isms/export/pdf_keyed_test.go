package export

// Key-gated PDF tests. They render real documents, so they need a metered
// UniDoc key (each render costs one credit) and are skipped without one:
//
//	ISMS_TEST_UNIDOC_API_KEY=... go test ./internal/isms/export -run Keyed
//
// ISMS_TEST_EXPORT_OUT=<dir> also writes the four PDFs for a visual check.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdflic "github.com/unidoc/unipdf/v5/common/license"
	"github.com/unidoc/unipdf/v5/extractor"
	"github.com/unidoc/unipdf/v5/model"
)

var keyedReady bool

func TestMain(m *testing.M) {
	if key := os.Getenv("ISMS_TEST_UNIDOC_API_KEY"); key != "" {
		if err := pdflic.SetMeteredKey(key); err != nil {
			fmt.Fprintln(os.Stderr, "key-gated export tests: licensing failed:", err)
		} else {
			keyedReady = true
		}
	}
	os.Exit(m.Run())
}

func needKey(t *testing.T) {
	t.Helper()
	if !keyedReady {
		t.Skip("ISMS_TEST_UNIDOC_API_KEY is not set (or did not license)")
	}
}

type parsedPDF struct {
	pages []string // extracted text per page
	links []string // URIs of link annotations
	paths []int    // stroke paths per page
}

func parsePDF(t *testing.T, b []byte) *parsedPDF {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("not a PDF: %q", b[:min(len(b), 20)])
	}
	r, err := model.NewPdfReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("reading the PDF back: %v", err)
	}
	n, err := r.GetNumPages()
	if err != nil {
		t.Fatal(err)
	}
	out := &parsedPDF{}
	for i := 1; i <= n; i++ {
		pg, err := r.GetPage(i)
		if err != nil {
			t.Fatal(err)
		}
		ex, err := extractor.New(pg)
		if err != nil {
			t.Fatal(err)
		}
		txt, err := ex.ExtractText()
		if err != nil {
			t.Fatal(err)
		}
		out.pages = append(out.pages, txt)
		if sp, err := ex.ExtractStrokePaths(); err == nil {
			out.paths = append(out.paths, len(sp))
		} else {
			out.paths = append(out.paths, -1)
		}
		anns, _ := pg.GetAnnotations()
		for _, a := range anns {
			l, ok := a.GetContext().(*model.PdfAnnotationLink)
			if !ok {
				continue
			}
			act, err := l.GetAction()
			if err != nil || act == nil {
				continue
			}
			if u, ok := act.GetContext().(*model.PdfActionURI); ok {
				out.links = append(out.links, fmt.Sprint(u.URI))
			}
		}
	}
	return out
}

func (p *parsedPDF) pagesWith(s string) int {
	n := 0
	for _, pg := range p.pages {
		if strings.Contains(pg, s) {
			n++
		}
	}
	return n
}

func TestKeyedRenderPDF(t *testing.T) {
	needKey(t)
	gofonts := goFontDir(t)
	out := os.Getenv("ISMS_TEST_EXPORT_OUT")
	if out != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	type run struct {
		name, status, fonts string
		draft, std          bool
	}
	runs := []run{
		{"draft-gofont", "draft", gofonts, true, false},
		{"approved-gofont", "approved", gofonts, false, false},
		{"draft-std", "draft", "", true, true},
		{"approved-std", "approved", "", false, true},
	}
	parsed := map[string]*parsedPDF{}
	for _, rn := range runs {
		t.Run(rn.name, func(t *testing.T) {
			t.Setenv(envExportFontDir, rn.fonts)
			d := richDoc(rn.status)
			b, err := Render("pdf", d)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if out != "" {
				if err := os.WriteFile(filepath.Join(out, rn.name+".pdf"), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p := parsePDF(t, b)
			parsed[rn.name] = p

			if len(p.pages) < 3 {
				t.Fatalf("pages = %d, want at least 3", len(p.pages))
			}
			cover := p.pages[0]
			for _, want := range []string{"Context of the Organization", "iso27001-4-1", "Generated", "Acme Corp"} {
				if !strings.Contains(cover, want) {
					t.Errorf("cover lacks %q:\n%s", want, cover)
				}
			}
			// DRAFT: on every page for a draft, on none for an approved document.
			if n := p.pagesWith("DRAFT"); rn.draft && n != len(p.pages) {
				t.Errorf("DRAFT on %d of %d pages", n, len(p.pages))
			} else if !rn.draft && n != 0 {
				t.Errorf("an approved export has DRAFT on %d pages", n)
			}
			if p.pagesWith("CONFIDENTIAL") < len(p.pages)-1 {
				t.Errorf("CONFIDENTIAL in %d pages, want every page after the cover", p.pagesWith("CONFIDENTIAL"))
			}
			if p.pagesWith("Page 1 of") == 0 {
				t.Error(`no "Page 1 of" footer`)
			}
			all := strings.Join(p.pages, "\n")
			for _, want := range []string{"Revision history", "First release", "Added scope", "Cy"} {
				if !strings.Contains(all, want) {
					t.Errorf("missing %q", want)
				}
			}
			if n := p.pagesWith("Control ID"); n < 2 {
				t.Errorf("table header on %d pages, want it repeated (>= 2)", n)
			}
			if !strings.Contains(all, "[image: network diagram]") {
				t.Error("image placeholder missing")
			}
			if strings.Contains(all, "evil.example") {
				t.Error("an image src leaked into the document")
			}
			if !rn.std && !strings.Contains(all, polish) {
				t.Errorf("Polish text does not extract exactly in the embedded-font render")
			}
			if rn.std && !strings.Contains(all, "Za") {
				t.Error("standard-font render lost the Polish line entirely")
			}
			// Links: only http, https and mailto are clickable.
			want := map[string]bool{"https://example.com/a": true, "http://example.com/b": true, "mailto:a@example.com": true}
			if len(p.links) != len(want) {
				t.Errorf("link annotations = %v, want exactly %v", p.links, want)
			}
			for _, l := range p.links {
				if !want[l] {
					t.Errorf("unexpected link annotation %q", l)
				}
			}
			if !strings.Contains(all, "js link") || !strings.Contains(all, "relative link") {
				t.Error("non-clickable link text must still be printed")
			}
		})
	}
	// The watermark is vector strokes: a draft page has at least the nine of
	// "DRAFT" more than the same page of the approved render.
	for _, pair := range [][2]string{{"draft-gofont", "approved-gofont"}, {"draft-std", "approved-std"}} {
		d, a := parsed[pair[0]], parsed[pair[1]]
		if d == nil || a == nil || d.paths[0] < 0 {
			continue
		}
		if d.paths[0] < a.paths[0]+9 {
			t.Errorf("%s cover has %d stroke paths vs %d: no watermark?", pair[0], d.paths[0], a.paths[0])
		}
	}
}
