package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/unidoc/unioffice/v2/color"
	"github.com/unidoc/unioffice/v2/common"
	offlic "github.com/unidoc/unioffice/v2/common/license"
	"github.com/unidoc/unioffice/v2/document"
	"github.com/unidoc/unioffice/v2/measurement"
	"github.com/unidoc/unioffice/v2/schema/soo/wml"
)

// The DOCX renderer. People redline the file offline, so it uses real Word
// structures rather than drawn ones: built-in Title and Heading styles (the
// Navigation Pane and a table of contents work), Word numbering definitions for
// lists, real table header rows, real hyperlinks, a distinct first-page header
// and footer, and a Word watermark.
//
// Fonts are Word's defaults for text; only code names a font (Consolas, which
// Word replaces with a monospaced one where it is missing). ISMS_EXPORT_FONT_DIR
// does not apply to DOCX: nothing is embedded and the reader's Word substitutes.
//
// As in the PDF renderer, an inline image prints as "[image: alt]" and its src
// is never fetched or read.

var (
	dxGrey   = color.FromHex("#666666")
	dxBorder = color.FromHex("#BBBBBB")
	dxHead   = color.FromHex("#E6E6E6")
	dxCode   = color.FromHex("#F1F1F1")
	dxLink   = color.FromHex("#1A56C4")
	dxDraft  = color.FromHex("#B42318")
)

const (
	dxMM     measurement.Distance = measurement.Millimeter
	dxIndent measurement.Distance = 6 * measurement.Millimeter // one list or quote level
	dxPt     measurement.Distance = measurement.Point
)

func pts(v float64) measurement.Distance { return measurement.Distance(v) * dxPt }

// renderDOCX renders d to DOCX bytes. The vendor's "not licensed" failure is
// wrapped as ErrUnlicensed.
func renderDOCX(d *ExportDoc) ([]byte, error) {
	if !docxLicensed() {
		return nil, fmt.Errorf("%w: unioffice is not licensed", ErrUnlicensed)
	}
	doc := buildDOCX(d)
	var buf bytes.Buffer
	if err := doc.Save(&buf); err != nil {
		return nil, mapDOCXErr(err)
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("export: the DOCX library produced no output")
	}
	return buf.Bytes(), nil
}

func docxLicensed() bool {
	k := offlic.GetLicenseKey()
	return k != nil && k.Tier != offlic.LicenseTierUnlicensed
}

// mapDOCXErr wraps the vendor's "not licensed" failures as ErrUnlicensed. The
// vendor has no exported sentinel, so the message is matched; a CI test pins it
// against the real library.
func mapDOCXErr(err error) error {
	if isDOCXLicenseErr(err) {
		return fmt.Errorf("%w: %v", ErrUnlicensed, err)
	}
	return err
}

func isDOCXLicenseErr(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	for _, s := range []string{"unioffice license required", "license required", "unlicensed", "credits"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// docxRender holds one render's state.
type docxRender struct {
	d     *ExportDoc
	doc   *document.Document
	brand color.Color
	// pageBreak makes the next paragraph start a new page.
	pageBreak bool
}

// buildDOCX builds the document in memory. It needs no licence; Save is what the
// vendor library gates, so the result can be inspected without one.
func buildDOCX(d *ExportDoc) *document.Document {
	doc := document.New()
	r := &docxRender{d: d, doc: doc, brand: color.FromHex("#333333")}
	if validHexColor(d.Org.BrandColor) {
		r.brand = color.FromHex(d.Org.BrandColor)
	}
	r.page()
	r.styles()
	r.headerFooter()
	r.properties()
	r.cover()
	r.revisions()
	r.blocks(d.Blocks, 0)
	return doc
}

// styles gives the built-in heading styles a size, weight and spacing, so the
// outline reads as headings and not as body text; the styles keep their built-in
// names, which is what the Navigation Pane and a table of contents use.
func (r *docxRender) styles() {
	sizes := [...]float64{0, 20, 16, 13.5, 12, 11, 10.5}
	for lv := 1; lv <= 6; lv++ {
		st, ok := r.doc.Styles.SearchStyleById(fmt.Sprintf("Heading%d", lv))
		if !ok {
			continue
		}
		rp := st.RunProperties()
		rp.SetBold(true)
		rp.SetSize(pts(sizes[lv]))
		rp.SetColor(color.FromHex("#222222"))
		pp := st.ParagraphProperties()
		pp.SetKeepNext(true)
		pp.SetSpacing(pts(sizes[lv]*0.7), pts(4))
	}
}

// page is A4 with 20 mm margins.
func (r *docxRender) page() {
	s := r.doc.BodySection()
	s.SetPageSizeAndOrientation(210*dxMM, 297*dxMM, wml.ST_PageOrientationPortrait)
	m := 20 * dxMM
	s.SetPageMargins(m, m, m, m, 10*dxMM, 10*dxMM, 0)
	s.SetTitlePage(true) // the cover has its own, empty, header and footer
}

func (r *docxRender) properties() {
	cp := r.doc.CoreProperties
	cp.SetTitle(r.d.Meta.Title)
	cp.SetAuthor("isms")
	cp.SetLastModifiedBy("isms")
	if !r.d.GeneratedAt.IsZero() {
		cp.SetCreated(r.d.GeneratedAt.UTC())
		cp.SetModified(r.d.GeneratedAt.UTC())
	}
	if x := cp.X(); x.Title != nil {
		sub := *x.Title // same type, own XML name
		sub.XMLName.Local = "dc:subject"
		sub.Data = []byte(r.d.Meta.DocumentID)
		x.Subject = &sub
	}
}

// ---- runs ---------------------------------------------------------------------

type dxStyle struct {
	bold, italic, strike, code bool
	size                       float64 // points; 0 = the style's size
	color                      *color.Color
	link                       bool
}

func (r *docxRender) format(run document.Run, st dxStyle) {
	p := run.Properties()
	if st.bold {
		p.SetBold(true)
	}
	if st.italic {
		p.SetItalic(true)
	}
	if st.strike {
		p.SetStrikeThrough(true)
	}
	if st.code {
		p.SetFontFamily("Consolas")
		if st.size == 0 {
			p.SetSize(pts(9.5))
		}
		p.SetShading(wml.ST_ShdClear, color.Auto, dxCode)
	}
	if st.size != 0 {
		p.SetSize(pts(st.size))
	}
	if st.link {
		p.SetColor(dxLink)
		p.SetUnderline(wml.ST_UnderlineSingle, dxLink)
	} else if st.color != nil {
		p.SetColor(*st.color)
	}
}

func (r *docxRender) text(p document.Paragraph, s string, st dxStyle) {
	if s == "" {
		return
	}
	run := p.AddRun()
	run.AddText(s)
	r.format(run, st)
}

// inlines adds inline content to p. Hyperlinks exist only for allowed hrefs
// (http, https, mailto); any other link is plain text.
func (r *docxRender) inlines(p document.Paragraph, ins []Inline, base dxStyle) {
	for _, in := range ins {
		switch in := in.(type) {
		case Text:
			st := base
			st.bold = st.bold || in.Bold
			st.italic = st.italic || in.Italic
			st.strike = st.strike || in.Strike
			st.code = st.code || in.Code
			r.text(p, in.Text, st)
		case Link:
			if linkAllowed(in.Href) {
				hl := p.AddHyperLink()
				hl.SetTarget(strings.TrimSpace(in.Href))
				label := plainText(in.Inlines)
				if strings.TrimSpace(label) == "" {
					label = in.Href
				}
				run := hl.AddRun()
				run.AddText(label)
				st := base
				st.link = true
				r.format(run, st)
				continue
			}
			r.inlines(p, in.Inlines, base)
		case LineBreak:
			if in.Hard {
				p.AddRun().AddBreak()
			} else {
				r.text(p, " ", base)
			}
		case Image:
			label := "[image]"
			if alt := strings.TrimSpace(in.Alt); alt != "" {
				label = "[image: " + alt + "]"
			}
			g := dxGrey
			r.text(p, label, dxStyle{italic: true, color: &g, size: base.size})
		}
	}
}

// para adds a body paragraph, starting a new page when one is pending.
func (r *docxRender) para() document.Paragraph {
	p := r.doc.AddParagraph()
	if r.pageBreak {
		p.Properties().SetPageBreakBefore(true)
		r.pageBreak = false
	}
	return p
}

// ---- cover and revision history ---------------------------------------------

func (r *docxRender) cover() {
	d := r.d
	if len(d.Org.LogoPNG) > 0 {
		r.logo(d.Org.LogoPNG)
	}
	if d.Org.Name != "" {
		p := r.para()
		p.SetBeforeSpacing(pts(24))
		g := dxGrey
		r.text(p, d.Org.Name, dxStyle{bold: true, size: 12, color: &g})
	}
	t := r.para()
	t.SetStyle("Title")
	title := d.Meta.Title
	if title == "" {
		title = d.Meta.DocumentID
	}
	b := r.brand
	r.text(t, title, dxStyle{bold: true, size: 26, color: &b})
	t.Borders().SetBottom(wml.ST_BorderSingle, r.brand, 2*dxPt)
	if d.Draft() {
		p := r.para()
		p.SetBeforeSpacing(pts(8))
		dr := dxDraft
		r.text(p, "DRAFT - not approved", dxStyle{bold: true, size: 12, color: &dr})
	}
	rows := [][2]string{
		{"Document ID", d.Meta.DocumentID},
		{"Version", orDash(d.VersionLabel)},
		{"Status", d.Meta.Status},
		{"Classification", d.Meta.Classification},
		{"Owner", d.Meta.Owner},
		{"Author", d.Meta.Author},
		{"Approved by", d.Meta.ApprovedBy},
		{"Approved date", d.Meta.ApprovedDate},
		{"Effective date", d.Meta.EffectiveDate},
		{"Next review", d.Meta.NextReview},
	}
	required := map[string]bool{"Document ID": true, "Version": true, "Status": true}
	r.spacer(10)
	tb := r.table()
	for _, row := range rows {
		if row[1] == "" && !required[row[0]] {
			continue
		}
		tr := tb.AddRow()
		r.cellText(tr.AddCell(), row[0], 30, dxStyle{bold: true, size: 10}, true, wml.ST_JcLeft)
		r.cellText(tr.AddCell(), orDash(row[1]), 70, dxStyle{size: 10}, false, wml.ST_JcLeft)
	}
	g := r.para()
	g.SetBeforeSpacing(pts(14))
	gg := dxGrey
	r.text(g, "Generated "+d.GeneratedAt.UTC().Format("2006-01-02 15:04")+" UTC", dxStyle{size: 9, color: &gg})
	r.pageBreak = true
}

// spacer is an empty paragraph with the given space after it, in points.
func (r *docxRender) spacer(pt float64) {
	p := r.para()
	p.SetAfterSpacing(pts(pt))
}

// logo adds the organisation logo, at most 40 mm high. An undecodable image is
// skipped.
func (r *docxRender) logo(png []byte) {
	img, err := common.ImageFromBytes(png)
	if err != nil || img.Size.X <= 0 || img.Size.Y <= 0 {
		return
	}
	ref, err := r.doc.AddImage(img)
	if err != nil {
		return
	}
	p := r.para()
	in, err := p.AddRun().AddDrawingInline(ref)
	if err != nil {
		return
	}
	w, h := float64(img.Size.X), float64(img.Size.Y)
	if max := 40 * float64(measurement.Millimeter); h > max {
		w, h = w*max/h, max
	}
	in.SetSize(pts(w), pts(h))
}

func (r *docxRender) revisions() {
	rows := r.d.RevisionRows()
	if len(rows) == 0 {
		return
	}
	h := r.para()
	h.SetStyle("Heading1")
	r.text(h, "Revision history", dxStyle{})
	tb := r.table()
	widths := []float64{10, 14, 17, 17, 42}
	head := tb.AddRow()
	head.Properties().SetTblHeader(true)
	for i, s := range []string{"Version", "Date", "Author", "Approved by", "Description"} {
		r.cellText(head.AddCell(), s, widths[i], dxStyle{bold: true, size: 9}, true, wml.ST_JcLeft)
	}
	for _, ch := range rows {
		tr := tb.AddRow()
		tr.Properties().SetCantSplit(true)
		for i, s := range []string{ch.Version, ch.Date, ch.Author, ch.ApprovedBy, ch.Description} {
			r.cellText(tr.AddCell(), orDash(s), widths[i], dxStyle{size: 9}, false, wml.ST_JcLeft)
		}
	}
	r.pageBreak = true
}

// ---- tables -------------------------------------------------------------------

// table adds a full-width bordered table.
func (r *docxRender) table() document.Table {
	t := r.doc.AddTable()
	p := t.Properties()
	p.SetWidthPercent(100)
	p.SetLayout(wml.ST_TblLayoutTypeFixed)
	p.Borders().SetAll(wml.ST_BorderSingle, dxBorder, dxPt/2)
	return t
}

// cellText fills a cell with one plain paragraph.
func (r *docxRender) cellText(c document.Cell, s string, widthPct float64, st dxStyle, shade bool, jc wml.ST_Jc) {
	p := r.cell(c, widthPct, shade)
	p.SetAlignment(jc)
	r.text(p, s, st)
}

// cell sets a cell's width and shading and returns its paragraph.
func (r *docxRender) cell(c document.Cell, widthPct float64, shade bool) document.Paragraph {
	cp := c.Properties()
	cp.SetWidthPercent(widthPct)
	if shade {
		cp.SetShading(wml.ST_ShdClear, color.Auto, dxHead)
	}
	p := c.AddParagraph()
	p.SetAfterSpacing(pts(2))
	p.SetBeforeSpacing(pts(2))
	return p
}

func (r *docxRender) tableBlock(tb Table) {
	if r.pageBreak {
		r.spacer(0) // a table cannot carry a page break itself
	}
	cols := len(tb.Header)
	if cols == 0 {
		cols = 1
	}
	widths := columnWidths(tb, cols)
	t := r.table()
	jc := func(i int) wml.ST_Jc {
		if i < len(tb.Align) {
			switch tb.Align[i] {
			case AlignCenter:
				return wml.ST_JcCenter
			case AlignRight:
				return wml.ST_JcRight
			}
		}
		return wml.ST_JcLeft
	}
	row := func(cells []Cell, head bool) {
		tr := t.AddRow()
		tr.Properties().SetCantSplit(true)
		if head {
			tr.Properties().SetTblHeader(true) // repeats on every page the table spans
		}
		for i := 0; i < cols; i++ {
			var c Cell
			if i < len(cells) {
				c = cells[i]
			}
			p := r.cell(tr.AddCell(), widths[i]*100, head)
			p.SetAlignment(jc(i))
			r.inlines(p, c.Inlines, dxStyle{size: 9.5, bold: head})
		}
	}
	if len(tb.Header) > 0 {
		row(tb.Header, true)
	}
	for _, rw := range tb.Rows {
		row(rw, false)
	}
	r.spacer(2)
}

// ---- header, footer, watermark ----------------------------------------------

// headerFooter sets the running header and footer of every page after the cover,
// and empty ones for the cover (first page). The draft watermark is on both.
//
// Footer page numbers use Word's PAGE and NUMPAGES fields, so the cover counts:
// the first body page shows "Page 2 of N", N being the total page count.
func (r *docxRender) headerFooter() {
	s := r.doc.BodySection()
	d := r.d
	const contentMM = 170.0 // 210 mm page less 2 x 20 mm margins

	hdr := r.doc.AddHeader()
	left, centre, right, draft := headerParts(d)
	hp := hdr.AddParagraph()
	hp.Properties().AddTabStop(contentMM/2*dxMM, wml.ST_TabJcCenter, wml.ST_TabTlcNone)
	hp.Properties().AddTabStop(contentMM*dxMM, wml.ST_TabJcRight, wml.ST_TabTlcNone)
	g := dxGrey
	small := dxStyle{size: 8, color: &g}
	add := func(s string, st dxStyle) {
		run := hp.AddRun()
		run.AddText(s)
		r.format(run, st)
	}
	add(left, small)
	hp.AddRun().AddTab()
	add(centre, dxStyle{size: 8, color: &g, bold: true})
	hp.AddRun().AddTab()
	add(right, small)
	if draft != "" {
		dp := hdr.AddParagraph()
		dp.SetAlignment(wml.ST_JcCenter)
		red := dxDraft
		run := dp.AddRun()
		run.AddText(draft)
		r.format(run, dxStyle{size: 8, bold: true, color: &red})
	}
	s.SetHeader(hdr, wml.ST_HdrFtrDefault)

	first := r.doc.AddHeader()
	first.AddParagraph()
	s.SetHeader(first, wml.ST_HdrFtrFirst)

	ftr := r.doc.AddFooter()
	fl, _ := footerParts(d, 0, 0)
	fp := ftr.AddParagraph()
	fp.Properties().AddTabStop(contentMM*dxMM, wml.ST_TabJcRight, wml.ST_TabTlcNone)
	addf := func(s string) {
		run := fp.AddRun()
		run.AddText(s)
		r.format(run, small)
	}
	addf(fl)
	fp.AddRun().AddTab()
	addf("Page ")
	pg := fp.AddRun()
	pg.AddField(document.FieldCurrentPage)
	r.format(pg, small)
	addf(" of ")
	np := fp.AddRun()
	np.AddField(document.FieldNumberOfPages)
	r.format(np, small)
	s.SetFooter(ftr, wml.ST_HdrFtrDefault)

	firstF := r.doc.AddFooter()
	firstF.AddParagraph()
	s.SetFooter(firstF, wml.ST_HdrFtrFirst)

	if d.Draft() {
		// The watermark is added by hand to both headers: the library's
		// AddWatermarkText repeats its shape in every run of the header's first
		// paragraph and only reaches the default header, so the cover would have no
		// DRAFT mark. A floating shape in its own run takes no space.
		r.watermark(hdr.Paragraphs()[0])
		r.watermark(first.Paragraphs()[0])
	}
}

// watermark adds a diagonal, half-transparent "DRAFT" text shape to a header
// paragraph.
func (r *docxRender) watermark(p document.Paragraph) {
	wm := document.NewWatermarkText()
	wm.SetText("DRAFT")
	wm.EnableDiagonalLayout(true)
	wm.SetOpacity(0.5)
	wm.SetColor("silver")
	wm.SetFontFamily("Calibri")
	run := p.AddRun()
	run.X().EG_RunInnerContent = append(run.X().EG_RunInnerContent, &wml.EG_RunInnerContent{
		RunInnerContentChoice: &wml.EG_RunInnerContentChoice{Pict: wm.Pict()},
	})
}

// ---- body ---------------------------------------------------------------------

// blocks adds blocks at a left indent of ind.
func (r *docxRender) blocks(bs []Block, ind measurement.Distance) {
	for _, b := range bs {
		switch b := b.(type) {
		case Heading:
			r.heading(b, ind)
		case Paragraph:
			p := r.para()
			r.indent(p, ind)
			p.SetAfterSpacing(pts(6))
			r.inlines(p, b.Inlines, dxStyle{})
		case List:
			r.list(b, 0, ind)
		case CodeBlock:
			r.codeBlock(b, ind)
		case Table:
			r.tableBlock(b)
		case Quote:
			r.quote(b, ind)
		case Rule:
			p := r.para()
			r.indent(p, ind)
			p.Borders().SetBottom(wml.ST_BorderSingle, dxBorder, dxPt*3/4)
			p.SetAfterSpacing(pts(6))
		}
	}
}

func (r *docxRender) indent(p document.Paragraph, ind measurement.Distance) {
	if ind > 0 {
		p.SetLeftIndent(ind)
	}
}

func (r *docxRender) heading(h Heading, ind measurement.Distance) {
	lv := h.Level
	if lv < 1 || lv > 6 {
		lv = 6
	}
	p := r.para()
	p.SetStyle(fmt.Sprintf("Heading%d", lv))
	r.indent(p, ind)
	r.inlines(p, h.Inlines, dxStyle{})
}

// codeBlock is one shaded paragraph per line, in a monospaced font, with the
// line's leading blanks kept.
func (r *docxRender) codeBlock(cb CodeBlock, ind measurement.Distance) {
	line := func(s string, st dxStyle) {
		p := r.para()
		r.indent(p, ind)
		p.SetBeforeSpacing(0)
		p.SetAfterSpacing(0)
		x := p.Properties().X()
		x.Shd = wml.NewCT_Shd()
		x.Shd.ValAttr = wml.ST_ShdClear
		fill := *dxCode.AsRGBString()
		x.Shd.FillAttr = &wml.ST_HexColor{ST_HexColorRGB: &fill}
		run := p.AddRun()
		run.AddText(s)
		r.format(run, st)
		run.Properties().SetFontFamily("Consolas")
	}
	if lang := strings.TrimSpace(cb.Lang); lang != "" {
		g := dxGrey
		line(lang, dxStyle{size: 7, color: &g})
	}
	for _, l := range strings.Split(cb.Text, "\n") {
		line(strings.ReplaceAll(l, "\t", "    "), dxStyle{size: 9})
	}
	r.spacer(4)
}

// quote is a left border and an indent, in grey text.
func (r *docxRender) quote(q Quote, ind measurement.Distance) {
	n := len(r.doc.Paragraphs())
	r.blocks(q.Blocks, ind+dxIndent)
	g := dxGrey
	for _, p := range r.doc.Paragraphs()[n:] {
		p.Borders().SetLeft(wml.ST_BorderSingle, dxBorder, 3*dxPt)
		for _, run := range p.Runs() {
			if run.X().RPr == nil || run.X().RPr.Color == nil {
				run.Properties().SetColor(g)
			}
		}
	}
}

// ---- lists --------------------------------------------------------------------

// list adds a list as paragraphs of a Word numbering definition, so the list
// continues in Word. Each list has its own definition (its own numbering
// instance, so a nested ordered list can start at its own number); a block that
// follows an item, such as code, does not end the list: the next item carries on.
func (r *docxRender) list(l List, depth int, ind measurement.Distance) {
	nd := r.numbering(l, depth)
	for _, it := range l.Items {
		rest := it.Blocks
		numbered := false
		if len(rest) > 0 {
			if pb, ok := rest[0].(Paragraph); ok {
				p := r.para()
				p.SetNumberingDefinition(nd)
				p.SetNumberingLevel(depth)
				p.SetAfterSpacing(pts(2))
				switch it.Task {
				case TaskChecked:
					r.text(p, "[x] ", dxStyle{})
				case TaskUnchecked:
					r.text(p, "[ ] ", dxStyle{})
				}
				r.inlines(p, pb.Inlines, dxStyle{})
				rest = rest[1:]
				numbered = true
			}
		}
		if !numbered { // an item with no leading paragraph still gets its marker
			p := r.para()
			p.SetNumberingDefinition(nd)
			p.SetNumberingLevel(depth)
		}
		inner := ind + dxIndent*measurement.Distance(depth+1)
		for _, b := range rest {
			if sub, ok := b.(List); ok {
				r.list(sub, depth+1, ind)
				continue
			}
			r.blocks([]Block{b}, inner)
		}
	}
}

// numbering defines the list's Word numbering: a bullet or decimal level at
// depth (and the levels above it, so the level index exists), starting at the
// list's start number.
func (r *docxRender) numbering(l List, depth int) document.NumberingDefinition {
	nd := r.doc.Numbering.AddDefinition()
	start := l.Start
	if start == 0 {
		start = 1
	}
	for i := 0; i <= depth; i++ {
		lv := nd.AddLevel()
		if l.Ordered {
			lv.SetFormat(wml.ST_NumberFormatDecimal)
			lv.SetText(fmt.Sprintf("%%%d.", i+1))
		} else {
			lv.SetFormat(wml.ST_NumberFormatBullet)
			lv.SetText(string(rune(0x2022)))
		}
		lv.X().Start = wml.NewCT_DecimalNumber()
		lv.X().Start.ValAttr = int64(start)
		lv.Properties().SetLeftIndent(dxIndent * measurement.Distance(i+1))
		lv.Properties().SetHangingIndent(dxIndent)
	}
	return nd
}
