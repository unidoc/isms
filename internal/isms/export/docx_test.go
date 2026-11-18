package export

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"github.com/unidoc/unioffice/v2/document"
	"github.com/unidoc/unioffice/v2/schema/soo/wml"
)

// UniOffice builds a document in memory without a licence (only Save is gated),
// so the model is checked directly in CI. The same checks run on the document
// read back from a rendered file in the key-gated test.

func xmlOf(t *testing.T, v any) string {
	t.Helper()
	b, err := xml.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func paraText(p document.Paragraph) string {
	var sb strings.Builder
	for _, r := range p.Runs() {
		sb.WriteString(r.Text())
	}
	return sb.String()
}

// allParagraphs is the body's paragraphs in order, table cells excluded.
func bodyParagraphs(doc *document.Document) []document.Paragraph { return doc.Paragraphs() }

func findPara(doc *document.Document, sub string) (document.Paragraph, bool) {
	for _, p := range doc.Paragraphs() {
		if strings.Contains(paraText(p), sub) {
			return p, true
		}
	}
	return document.Paragraph{}, false
}

func headerXML(t *testing.T, doc *document.Document, typ wml.ST_HdrFtr) string {
	t.Helper()
	h, ok := doc.BodySection().GetHeader(typ)
	if !ok {
		t.Fatalf("no %v header", typ)
	}
	return xmlOf(t, h.X())
}

func footerXML(t *testing.T, doc *document.Document, typ wml.ST_HdrFtr) string {
	t.Helper()
	f, ok := doc.BodySection().GetFooter(typ)
	if !ok {
		t.Fatalf("no %v footer", typ)
	}
	return xmlOf(t, f.X())
}

// checkDOCX asserts what a rendered rich fixture must contain.
func checkDOCX(t *testing.T, doc *document.Document, draft bool) {
	t.Helper()
	paras := bodyParagraphs(doc)

	// Cover: logo paragraph aside, title, doc id and the generation time come first.
	var head []string
	for _, p := range paras {
		if s := paraText(p); s != "" {
			head = append(head, s)
		}
		if len(head) == 6 {
			break
		}
	}
	first := strings.Join(head, "\n")
	for _, want := range []string{"Acme Corp", "Context of the Organization"} {
		if !strings.Contains(first, want) {
			t.Errorf("cover lacks %q:\n%s", want, first)
		}
	}
	title, ok := findPara(doc, "Context of the Organization")
	if !ok || title.Style() != "Title" {
		t.Errorf("title paragraph style = %q (found %v)", title.Style(), ok)
	}
	if _, ok := findPara(doc, "Generated 2026-11-18 03:30 UTC"); !ok {
		t.Error("no Generated line on the cover")
	}
	if _, ok := findPara(doc, "DRAFT - not approved"); ok != draft {
		t.Errorf("cover draft banner present = %v, want %v", ok, draft)
	}

	// Tables: 0 metadata, 1 revision history, 2 the fixture table.
	tables := doc.Tables()
	if len(tables) != 3 {
		t.Fatalf("tables = %d, want 3", len(tables))
	}
	meta := map[string]string{}
	for _, row := range tables[0].Rows() {
		c := row.Cells()
		if len(c) == 2 {
			meta[cellText(c[0])] = cellText(c[1])
		}
	}
	for k, v := range map[string]string{
		"Document ID": "iso27001-4-1", "Version": "1.1", "Status": richDoc("draft").Meta.Status,
		"Classification": "Confidential", "Owner": "Ann Owner", "Author": "Bob Author", "Approved by": "Cy Approver",
		"Approved date": "2026-01-02", "Effective date": "2026-02-01", "Next review": "2027-02-01",
	} {
		if k == "Status" {
			v = map[bool]string{true: "draft", false: "approved"}[draft]
		}
		if meta[k] != v {
			t.Errorf("metadata %q = %q, want %q", k, meta[k], v)
		}
	}

	rev := tables[1].Rows()
	if len(rev) != 3 {
		t.Fatalf("revision rows = %d, want header + 2", len(rev))
	}
	if !strings.Contains(xmlOf(t, rev[0].X()), "tblHeader") {
		t.Error("revision header row is not a repeating header")
	}
	if got := cellText(rev[2].Cells()[4]); got != "Added scope" {
		t.Errorf("revision description = %q", got)
	}
	if got := cellText(rev[1].Cells()[3]); got != "Cy" {
		t.Errorf("revision approver = %q", got)
	}

	// Headings use Word's built-in styles.
	for text, style := range map[string]string{"Purpose": "Heading1", "Scope": "Heading2", "Detail": "Heading3", "Revision history": "Heading1"} {
		p, ok := findPara(doc, text)
		if !ok || p.Style() != style {
			t.Errorf("heading %q style = %q (found %v), want %s", text, p.Style(), ok, style)
		}
	}

	// The heading styles carry a size and weight (a size of 0 would be body text).
	for lv, size := range map[string]float64{"Heading1": 20, "Heading2": 16, "Heading3": 13.5} {
		st, ok := doc.Styles.SearchStyleById(lv)
		if !ok || !st.RunProperties().Bold() || st.RunProperties().SizeValue() != size {
			t.Errorf("style %s: found %v, bold %v, size %v, want bold %v pt", lv, ok, st.RunProperties().Bold(), st.RunProperties().SizeValue(), size)
		}
	}

	// Lists: real numbering, nested ordered list starts at 3, tasks carry a prefix.
	for _, text := range []string{"first bullet", "nested three", "nested four", "deep", "done task", "open task", "after the code"} {
		p, ok := findPara(doc, text)
		if !ok {
			t.Errorf("list item %q missing", text)
			continue
		}
		if p.X().PPr == nil || p.X().PPr.NumPr == nil || p.X().PPr.NumPr.NumId == nil {
			t.Errorf("list item %q has no numbering", text)
		}
	}
	p3, _ := findPara(doc, "nested three")
	if p3.X().PPr != nil && p3.X().PPr.NumPr != nil && p3.X().PPr.NumPr.NumId != nil {
		lvl := int64(0)
		if p3.X().PPr.NumPr.Ilvl != nil {
			lvl = p3.X().PPr.NumPr.Ilvl.ValAttr
		}
		l := doc.GetNumberingLevelByIds(p3.X().PPr.NumPr.NumId.ValAttr, lvl)
		if l.X() == nil || l.X().Start == nil || l.X().Start.ValAttr != 3 {
			t.Errorf("nested ordered list does not start at 3: %v", l.X())
		} else if l.X().NumFmt == nil || l.X().NumFmt.ValAttr != wml.ST_NumberFormatDecimal {
			t.Errorf("nested list is not decimal")
		}
	}
	if p, ok := findPara(doc, "done task"); ok && !strings.HasPrefix(paraText(p), "[x] ") {
		t.Errorf("checked task text = %q", paraText(p))
	}
	if p, ok := findPara(doc, "open task"); ok && !strings.HasPrefix(paraText(p), "[ ] ") {
		t.Errorf("open task text = %q", paraText(p))
	}

	// The fixture table: repeating bold header, right-aligned Score column.
	ft := tables[2].Rows()
	if len(ft) != 151 {
		t.Fatalf("fixture table rows = %d, want header + 150", len(ft))
	}
	if !strings.Contains(xmlOf(t, ft[0].X()), "tblHeader") {
		t.Error("table header row is not a repeating header")
	}
	if got := cellText(ft[0].Cells()[2]); got != "Score" {
		t.Errorf("header cell = %q", got)
	}
	for ri, row := range ft {
		cps := row.Cells()[2].Paragraphs()
		if len(cps) == 0 || cps[0].X().PPr == nil || cps[0].X().PPr.Jc == nil || cps[0].X().PPr.Jc.ValAttr != wml.ST_JcRight {
			t.Fatalf("row %d: the Score cell is not right-aligned", ri)
		}
	}
	if got := cellText(ft[150].Cells()[2]); got != "1043" {
		t.Errorf("last Score = %q, want 1043", got)
	}
	if mid := ft[1].Cells()[1].Paragraphs()[0]; mid.X().PPr == nil || mid.X().PPr.Jc == nil || mid.X().PPr.Jc.ValAttr != wml.ST_JcCenter {
		t.Error("the centre column is not centred")
	}

	// Table grid columns are twips: each table spans the text width, in the
	// proportions of its columns, with no sliver column.
	total := dxTextWidthTwips()
	fixtureCols := columnWidths(richDoc("draft").Blocks[len(richDoc("draft").Blocks)-2].(Table), 3)
	expect := [][]float64{{0.30, 0.70}, revisionFractions, fixtureCols}
	for ti, tb := range tables {
		g := tb.X().TblGrid
		if g == nil || len(g.GridCol) != len(expect[ti]) {
			t.Errorf("table %d: grid columns missing or the wrong count", ti)
			continue
		}
		sum := 0
		for ci, c := range g.GridCol {
			if c.WAttr == nil || c.WAttr.ST_UnsignedDecimalNumber == nil {
				t.Errorf("table %d column %d has no width", ti, ci)
				continue
			}
			w := int(*c.WAttr.ST_UnsignedDecimalNumber)
			sum += w
			if frac := float64(w) / float64(total); frac < expect[ti][ci]-0.01 || frac > expect[ti][ci]+0.01 {
				t.Errorf("table %d column %d is %d twips (%.1f%% of the text width), want about %.1f%%", ti, ci, w, frac*100, expect[ti][ci]*100)
			}
			if w < 400 && len(g.GridCol) <= 12 {
				t.Errorf("table %d column %d is only %d twips wide", ti, ci, w)
			}
		}
		if d := float64(sum-total) / float64(total); d > 0.02 || d < -0.02 {
			t.Errorf("table %d grid sums to %d twips, want the text width %d (within 2%%)", ti, sum, total)
		}
	}

	// Hyperlinks: exactly the three allowed targets.
	var links []string
	for _, r := range doc.DocRels().Relationships() {
		if strings.HasSuffix(r.Type(), "/hyperlink") {
			links = append(links, r.Target())
		}
	}
	want := map[string]bool{"https://example.com/a": true, "http://example.com/b": true, "mailto:a@example.com": true}
	if len(links) != len(want) {
		t.Errorf("hyperlinks = %v, want exactly %v", links, want)
	}
	for _, l := range links {
		if !want[l] {
			t.Errorf("unexpected hyperlink %q", l)
		}
	}
	for _, plain := range []string{"js link", "relative link", "fragment link", "file link", "data link"} {
		if _, ok := findPara(doc, plain); !ok {
			t.Errorf("non-clickable link text %q must still be printed", plain)
		}
	}
	if _, ok := findPara(doc, "[image: network diagram]"); !ok {
		t.Error("image placeholder missing")
	}

	// Code keeps its leading spaces.
	if p, ok := findPara(doc, "line 000 := compute(0)"); !ok || !strings.HasPrefix(paraText(p), "    line 000") {
		t.Errorf("code indentation lost: %q (found %v)", paraText(p), ok)
	}
	// Unicode text is not replaced.
	if _, ok := findPara(doc, polish); !ok {
		t.Error("Polish text did not round-trip")
	}
	if _, ok := findPara(doc, japanese); !ok {
		t.Error("Japanese text did not round-trip")
	}
	// Strike-through is a real Word property.
	if !strings.Contains(xmlOf(t, doc.X().Body), "<w:strike") {
		t.Error("struck text has no strike property")
	}

	// Header, footer and watermark. The cover has its own, empty first-page ones.
	def := headerXML(t, doc, wml.ST_HdrFtrDefault)
	firstH := headerXML(t, doc, wml.ST_HdrFtrFirst)
	if !strings.Contains(def, "CONFIDENTIAL") || strings.Contains(firstH, "CONFIDENTIAL") {
		t.Error("CONFIDENTIAL must be in the running header and not on the cover")
	}
	if !strings.Contains(def, "iso27001-4-1 v1.1") || !strings.Contains(def, "Acme Corp") {
		t.Errorf("header lacks the id/version or organisation: %s", def)
	}
	if got := strings.Contains(def, "DRAFT"); got != draft {
		t.Errorf("DRAFT in the running header = %v, want %v", got, draft)
	}
	if got := strings.Contains(firstH, "DRAFT"); got != draft {
		t.Errorf("DRAFT watermark in the cover (first-page) header = %v, want %v", got, draft)
	}
	if got := strings.Contains(def, "<w:pict"); got != draft {
		t.Errorf("watermark shape in the running header = %v, want %v", got, draft)
	}
	if !doc.BodySection().TitlePage() {
		t.Error("the first page must have its own header and footer (titlePg)")
	}
	ft0 := footerXML(t, doc, wml.ST_HdrFtrDefault)
	for _, want := range []string{"PAGE", "NUMPAGES", "Generated 2026-11-18"} {
		if !strings.Contains(ft0, want) {
			t.Errorf("footer lacks %q", want)
		}
	}
	if strings.Contains(footerXML(t, doc, wml.ST_HdrFtrFirst), "NUMPAGES") {
		t.Error("the cover footer must be empty")
	}

	// Core properties.
	cp := doc.CoreProperties
	if cp.Title() != "Context of the Organization" || cp.LastModifiedBy() != "isms" {
		t.Errorf("core properties: title %q, last modified by %q", cp.Title(), cp.LastModifiedBy())
	}
	if !strings.Contains(xmlOf(t, cp.X()), "iso27001-4-1") {
		t.Error("subject (document id) missing from core properties")
	}
}

func cellText(c document.Cell) string {
	var parts []string
	for _, p := range c.Paragraphs() {
		parts = append(parts, paraText(p))
	}
	return strings.Join(parts, "\n")
}

func TestBuildDOCXModelDraft(t *testing.T)    { checkDOCX(t, buildDOCX(richDoc("draft")), true) }
func TestBuildDOCXModelApproved(t *testing.T) { checkDOCX(t, buildDOCX(richDoc("approved")), false) }

func TestBuildDOCXEdgeCases(t *testing.T) {
	// Empty and degenerate input must still build.
	buildDOCX(&ExportDoc{})
	buildDOCX(&ExportDoc{Meta: richDoc("approved").Meta, Blocks: []Block{
		Table{}, Table{Header: []Cell{{}}, Rows: [][]Cell{{}, {{}, {}}}},
		List{}, CodeBlock{}, Quote{}, Paragraph{}, Heading{Level: 9},
		Quote{Blocks: []Block{Quote{Blocks: []Block{para(txt("nested"))}}}},
		List{Items: []ListItem{{}, {Blocks: []Block{CodeBlock{Text: "x"}}}}},
	}})
	buildDOCX(&ExportDoc{Org: OrgInfo{LogoPNG: []byte("not a png"), BrandColor: "bogus"}})
	// A real logo is embedded.
	d := buildDOCX(&ExportDoc{Org: OrgInfo{LogoPNG: tinyPNG()}})
	if !strings.Contains(xmlOf(t, d.X().Body), "<w:drawing") {
		t.Error("logo not embedded")
	}
}

func TestDOCXLicenceErrorMapping(t *testing.T) {
	if !isDOCXLicenseErr(errors.New("unioffice license required")) || isDOCXLicenseErr(errors.New("other")) || isDOCXLicenseErr(nil) {
		t.Error("licence error matching")
	}
	if !errors.Is(mapDOCXErr(errors.New("unioffice license required")), ErrUnlicensed) {
		t.Error("not wrapped as ErrUnlicensed")
	}
}

// The real library with no licence: Render must say ErrUnlicensed and the
// vendor's own Save message must still be the one we match. Skips when a
// UniOffice licence is active in the process.
func TestRenderUnlicensedDOCX(t *testing.T) {
	if docxLicensed() {
		t.Skip("UniOffice is licensed in this process")
	}
	if _, err := Render("docx", richDoc("draft")); !errors.Is(err, ErrUnlicensed) {
		t.Fatalf("Render = %v, want ErrUnlicensed", err)
	}
	doc := document.New()
	doc.AddParagraph().AddRun().AddText("x")
	var sb strings.Builder
	err := doc.Save(&sb)
	if err == nil || !isDOCXLicenseErr(err) {
		t.Fatalf("vendor Save error %v is not recognised as a licence error", err)
	}
}

// Exactly one watermark shape per header, and it is the diagonal text kind.
func TestDOCXWatermarkShapes(t *testing.T) {
	d := buildDOCX(richDoc("draft"))
	for name, typ := range map[string]wml.ST_HdrFtr{"running": wml.ST_HdrFtrDefault, "cover": wml.ST_HdrFtrFirst} {
		x := headerXML(t, d, typ)
		if n := strings.Count(x, "<v:shape "); n != 1 {
			t.Errorf("%s header has %d watermark shapes, want 1", name, n)
		}
		if !strings.Contains(x, `string="DRAFT"`) || !strings.Contains(x, "rotation:315") {
			t.Errorf("%s header watermark is not the diagonal DRAFT text", name)
		}
	}
	a := buildDOCX(richDoc("approved"))
	for _, typ := range []wml.ST_HdrFtr{wml.ST_HdrFtrDefault, wml.ST_HdrFtrFirst} {
		if strings.Contains(headerXML(t, a, typ), "<v:shape ") {
			t.Error("an approved export has a watermark")
		}
	}
}
