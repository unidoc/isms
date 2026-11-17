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
	"github.com/unidoc/unipdf/v5/core"
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

	texts []string                   // same as pages
	marks []*extractor.TextMarkArray // text runs with positions, per page
}

// bbox is the bounding box of the first occurrence of s on any page.
func (p *parsedPDF) bbox(s string) (model.PdfRectangle, bool) {
	for i, txt := range p.texts {
		if k := strings.Index(txt, s); k >= 0 {
			sub, err := p.marks[i].RangeOffset(len(txt[:k]), len(txt[:k])+len(s))
			if err != nil {
				continue
			}
			if b, ok := sub.BBox(); ok {
				return b, true
			}
		}
	}
	return model.PdfRectangle{}, false
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
		if pt, _, _, err := ex.ExtractPageText(); err == nil {
			out.texts = append(out.texts, pt.Text())
			out.marks = append(out.marks, pt.Marks())
		} else {
			out.texts = append(out.texts, "")
			out.marks = append(out.marks, &extractor.TextMarkArray{})
		}
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
			// The /A dictionary is read directly: GetAction returns nil for the
			// reader's lazily loaded objects.
			if d, ok := core.TraceToDirectObject(l.A).(*core.PdfObjectDictionary); ok {
				if u, ok := core.GetString(d.Get("URI")); ok {
					out.links = append(out.links, u.Str())
				}
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
			// Lists: each marker is on its text's baseline, with a gap of about
			// 2 mm (4 to 14 points here), and the numbering continues from Start.
			for _, pair := range [][2]string{{"3.", "nested three"}, {"4.", "nested four"}, {"[x]", "done task"}, {"[ ]", "open task"}} {
				m, ok1 := p.bbox(pair[0])
				x, ok2 := p.bbox(pair[1])
				if !ok1 || !ok2 {
					t.Errorf("list item %q %q not found (marker %v, text %v)", pair[0], pair[1], ok1, ok2)
					continue
				}
				if d := m.Lly - x.Lly; d > 1.5 || d < -1.5 {
					t.Errorf("marker %q is %.1f pt off the baseline of %q", pair[0], d, pair[1])
				}
				if gap := x.Llx - m.Urx; gap < 2 || gap > 14 {
					t.Errorf("gap between marker %q and %q is %.1f pt", pair[0], pair[1], gap)
				}
			}
			// The last column is right-aligned: it must still print, header and
			// last row included, and every header cell must be on each table page.
			if !strings.Contains(all, "Score") || !strings.Contains(all, "1043") {
				t.Errorf("right-aligned column missing: Score=%v 1043=%v", strings.Contains(all, "Score"), strings.Contains(all, "1043"))
			}
			for i, pg := range p.pages {
				if strings.Contains(pg, "Control ID") {
					for _, h := range []string{"Description", "Score"} {
						if !strings.Contains(pg, h) {
							t.Errorf("table page %d lacks the header cell %q", i+1, h)
						}
					}
				}
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
