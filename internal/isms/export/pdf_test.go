package export

import (
	"errors"
	"strings"
	"testing"

	"github.com/unidoc/unipdf/v5/creator"
)

func TestLinkAllowed(t *testing.T) {
	cases := []struct {
		href string
		want bool
	}{
		{"https://example.com/a?b=1", true},
		{"http://example.com", true},
		{"HTTPS://EXAMPLE.COM", true},
		{"mailto:a@example.com", true},
		{"  https://example.com ", true},
		{"javascript:alert(1)", false},
		{"JavaScript:alert(1)", false},
		{"file:///etc/passwd", false},
		{"data:text/html,x", false},
		{"/risks/RISK-12", false},
		{"../a/b.md#c", false},
		{"#frag", false},
		{"", false},
		{"https://", false},
		{"https:///path", false},
		{"ftp://example.com", false},
		{"//example.com/x", false},
		{"https://exa mple.com", false},
		{"https://example.com/\nx", false},
		{"mailto:", false},
	}
	for _, c := range cases {
		if got := linkAllowed(c.href); got != c.want {
			t.Errorf("linkAllowed(%q) = %v, want %v", c.href, got, c.want)
		}
	}
}

func TestHeaderAndFooterParts(t *testing.T) {
	d := richDoc("draft")
	l, c, r, draft := headerParts(d)
	if l != "Acme Corp" || c != "CONFIDENTIAL" || r != "iso27001-4-1 v1.1" || draft != "DRAFT - not approved" {
		t.Errorf("draft header = %q %q %q %q", l, c, r, draft)
	}
	d = richDoc("approved")
	d.VersionLabel = ""
	d.Meta.Classification = ""
	l, c, r, draft = headerParts(d)
	if c != "" || r != "iso27001-4-1" || draft != "" {
		t.Errorf("approved header = %q %q %q %q", l, c, r, draft)
	}
	fl, fr := footerParts(d, 1, 7)
	if fl != "Generated 2026-11-18" || fr != "Page 1 of 7" {
		t.Errorf("footer = %q %q", fl, fr)
	}
}

func TestBrandColorValidation(t *testing.T) {
	for s, want := range map[string]bool{"#0a5fb4": true, "#FFFFFF": true, "0a5fb4": false, "#0a5fb": false, "#gggggg": false, "": false, "red": false} {
		if validHexColor(s) != want {
			t.Errorf("validHexColor(%q) != %v", s, want)
		}
	}
}

func TestColumnWidthsSumToOne(t *testing.T) {
	tb := richDoc("draft").Blocks[len(richDoc("draft").Blocks)-2].(Table)
	var sum float64
	for _, w := range columnWidths(tb, 3) {
		sum += w
	}
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("sum = %v", sum)
	}
}

// ---- layout without a licence: Finalize needs none, only Write does ---------

func buildWith(t *testing.T, fontDir string, d *ExportDoc) *pdfRender {
	t.Helper()
	t.Setenv(envExportFontDir, fontDir)
	r, err := buildPDF(d)
	if err != nil {
		t.Fatalf("buildPDF: %v", err)
	}
	return r
}

func TestLayoutRichDocStandardFontsHasNoEncodingErrors(t *testing.T) {
	for _, status := range []string{"draft", "approved"} {
		r := buildWith(t, "", richDoc(status)) // Polish and Japanese included
		if len(r.c.Errors) > 0 {
			t.Errorf("%s: %d layout errors, first: %v", status, len(r.c.Errors), r.c.Errors[0])
		}
		if !r.f.regular.std {
			t.Error("expected the standard fonts")
		}
	}
}

func TestLayoutRichDocEmbeddedFonts(t *testing.T) {
	r := buildWith(t, goFontDir(t), richDoc("draft"))
	if len(r.c.Errors) > 0 {
		t.Errorf("%d layout errors, first: %v", len(r.c.Errors), r.c.Errors[0])
	}
	if !r.f.embedded {
		t.Error("expected embedded fonts")
	}
}

func TestLayoutEdgeCases(t *testing.T) {
	d := &ExportDoc{} // empty everything
	if r := buildWith(t, "", d); len(r.c.Errors) > 0 {
		t.Errorf("empty doc: %v", r.c.Errors[0])
	}
	d = &ExportDoc{Meta: richDoc("approved").Meta, Blocks: []Block{
		Table{}, // no header, no rows
		Table{Header: []Cell{{}}, Rows: [][]Cell{{}, {{}, {}}}}, // ragged
		List{}, CodeBlock{}, Quote{}, Paragraph{}, Heading{Level: 9},
		Quote{Blocks: []Block{Quote{Blocks: []Block{para(txt("nested"))}}}},
	}}
	if _, err := buildPDF(d); err != nil {
		t.Errorf("edge cases: %v", err)
	}
	if _, err := buildPDF(&ExportDoc{Org: OrgInfo{LogoPNG: []byte("not a png"), BrandColor: "bogus"}}); err != nil {
		t.Errorf("bad logo and colour must not fail the export: %v", err)
	}
}

// ---- licence mapping --------------------------------------------------------

func TestIsPDFLicenseErr(t *testing.T) {
	if !isPDFLicenseErr(errors.New("unipdf license code required")) {
		t.Error("the pinned vendor message is not recognised")
	}
	if isPDFLicenseErr(errors.New("some other failure")) || isPDFLicenseErr(nil) {
		t.Error("false positive")
	}
	if !errors.Is(mapPDFErr(errors.New("unipdf license code required")), ErrUnlicensed) {
		t.Error("not wrapped as ErrUnlicensed")
	}
}

// The real library, with no licence: Render must say ErrUnlicensed, and the
// message the vendor really sends must still be the one we match. Licence state
// is process-global, so this skips when a key is active.
func TestRenderUnlicensedPDF(t *testing.T) {
	if pdfLicensed() {
		t.Skip("UniPDF is licensed in this process")
	}
	if _, err := Render("pdf", richDoc("draft")); !errors.Is(err, ErrUnlicensed) {
		t.Fatalf("Render = %v, want ErrUnlicensed", err)
	}
	// Pin the vendor's own message against the real Write.
	c := creator.New()
	p := c.NewStyledParagraph()
	p.Append("x")
	_ = c.Draw(p)
	var sb strings.Builder
	err := c.Write(&sb)
	if err == nil || !isPDFLicenseErr(err) {
		t.Fatalf("vendor Write error %v is not recognised as a licence error", err)
	}
	if sb.Len() != 0 {
		t.Errorf("unlicensed Write produced %d bytes", sb.Len())
	}
}

// The draft header line is on every page after the cover for a draft and on none
// for an approved document. Header callbacks run in Finalize, so this needs no
// licence. (The watermark is added to the writer, see pdf_watermark_test.go.)
func TestDraftHeaderLineOnEveryPage(t *testing.T) {
	dr := buildWith(t, "", richDoc("draft"))
	if dr.pages < 3 {
		t.Fatalf("pages = %d, want a multi-page document", dr.pages)
	}
	if dr.draftLines != dr.pages-1 {
		t.Errorf("draft header line on %d pages, want %d (every page after the cover)", dr.draftLines, dr.pages-1)
	}
	ap := buildWith(t, "", richDoc("approved"))
	if ap.draftLines != 0 {
		t.Errorf("an approved export has %d draft lines", ap.draftLines)
	}
	for _, status := range []string{"retired", "in_review", ""} {
		if r := buildWith(t, "", richDoc(status)); r.draftLines == 0 {
			t.Errorf("status %q is not approved and must be marked draft", status)
		}
	}
}
