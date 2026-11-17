package export

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"unicode"

	pdflic "github.com/unidoc/unipdf/v5/common/license"
	"github.com/unidoc/unipdf/v5/contentstream/draw"
	"github.com/unidoc/unipdf/v5/core"
	"github.com/unidoc/unipdf/v5/creator"
	"github.com/unidoc/unipdf/v5/model"
)

// The PDF renderer: a walk over the ExportDoc blocks, one function per block
// kind. Layout is in points; creator.PPMM converts millimetres.
//
// The renderer never fetches or reads an image's src: an inline image prints as
// "[image: alt]". Only the organisation logo (already bytes in the ExportDoc) is
// embedded.

const (
	pageMarginMM = 20.0
	baseSize     = 10.5
	lineSpacing  = 1.35
	indentMM     = 6.0
	paraGap      = 6.0 // points after a paragraph
)

var (
	colText   = creator.ColorRGBFromHex("#222222")
	colGrey   = creator.ColorRGBFromHex("#666666")
	colLight  = creator.ColorRGBFromHex("#f1f1f1")
	colBorder = creator.ColorRGBFromHex("#bbbbbb")
	colHead   = creator.ColorRGBFromHex("#e6e6e6")
	colLink   = creator.ColorRGBFromHex("#1a56c4")
	colDraft  = creator.ColorRGBFromHex("#b42318")
	colMark   = creator.ColorRGBFromHex("#9a9a9a") // draft watermark
)

// renderPDF renders d to PDF bytes. Errors from the vendor library that mean
// "not licensed" are wrapped as ErrUnlicensed.
func renderPDF(d *ExportDoc) ([]byte, error) {
	if !pdfLicensed() {
		return nil, fmt.Errorf("%w: unipdf is not licensed", ErrUnlicensed)
	}
	r, err := buildPDF(d)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := r.c.Write(&buf); err != nil {
		return nil, mapPDFErr(err)
	}
	if buf.Len() == 0 {
		return nil, errors.New("export: the PDF library produced no output")
	}
	return buf.Bytes(), nil
}

func pdfLicensed() bool {
	k := pdflic.GetLicenseKey()
	return k != nil && k.Tier != pdflic.LicenseTierUnlicensed
}

// mapPDFErr wraps the vendor's "not licensed" failures as ErrUnlicensed. The
// vendor has no exported sentinel, so the messages are matched; a CI test pins
// the unlicensed one against the real library.
func mapPDFErr(err error) error {
	if isPDFLicenseErr(err) {
		return fmt.Errorf("%w: %v", ErrUnlicensed, err)
	}
	return err
}

func isPDFLicenseErr(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	for _, s := range []string{"unipdf license code required", "license code required", "unlicensed", "credits"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// pdfRender holds one render's state.
type pdfRender struct {
	d     *ExportDoc
	c     *creator.Creator
	f     *pdfFonts
	brand creator.Color
	w     float64 // content width in points

	// Counters for tests: pages given a running header, and pages given the draft
	// watermark and the draft header line.
	pages, watermarks, draftLines int
}

// buildPDF lays the document out and finalises it. It needs no licence; Write
// is what the vendor library gates.
func buildPDF(d *ExportDoc) (*pdfRender, error) {
	f := fontsForRender()
	c := creator.New()
	c.SetPageSize(creator.PageSizeA4)
	m := pageMarginMM * creator.PPMM
	c.SetPageMargins(m, m, m, m)
	c.SetDefaultFont(f.regular.font, f.bold.font)
	if f.embedded {
		for _, fc := range []face{f.regular, f.bold, f.italic, f.boldItalic, f.mono} {
			c.EnableFontSubsetting(fc.font)
		}
	}
	c.SetLanguage("en")
	c.SetPdfWriterAccessFunc(func(w *model.PdfWriter) error { return setDocInfo(w, d) })

	r := &pdfRender{d: d, c: c, f: f, brand: brandColor(d.Org.BrandColor), w: c.Width() - 2*m}
	r.hooks()
	r.cover()
	r.revisions()
	for _, v := range r.blocks(d.Blocks, ctx{}) {
		if err := c.Draw(v); err != nil {
			return nil, fmt.Errorf("export: drawing: %w", err)
		}
	}
	if err := c.Finalize(); err != nil {
		return nil, fmt.Errorf("export: layout: %w", err)
	}
	if len(c.Errors) > 0 {
		// Text is cleaned for the font before drawing, so this is unexpected;
		// log it but still produce the document.
		log.Printf("export: %d layout warnings, first: %v", len(c.Errors), c.Errors[0])
	}
	return r, nil
}

func setDocInfo(w *model.PdfWriter, d *ExportDoc) error {
	info := &model.PdfInfo{
		Title:   core.MakeString(d.Meta.Title),
		Subject: core.MakeString(d.Meta.DocumentID),
		Creator: core.MakeString("isms"),
	}
	if !d.GeneratedAt.IsZero() {
		if pd, err := model.NewPdfDateFromTime(d.GeneratedAt.UTC()); err == nil {
			info.CreationDate = &pd
		}
	}
	w.SetDocInfo(info)
	return nil
}

// brandColor returns the accent colour: a valid #rrggbb, else dark grey.
func brandColor(s string) creator.Color {
	if validHexColor(s) {
		return creator.ColorRGBFromHex(s)
	}
	return creator.ColorRGBFromHex("#333333")
}

func validHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return false
		}
	}
	return true
}

// linkAllowed says which hrefs become clickable annotations: absolute http and
// https URLs with a host, and mailto. Everything else (relative or internal
// paths, fragments, javascript:, file:, data:, ...) is printed as plain text.
func linkAllowed(href string) bool {
	href = strings.TrimSpace(href)
	if href == "" || strings.ContainsAny(href, "\x00\r\n\t ") {
		return false
	}
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.Host != ""
	case "mailto":
		return u.Opaque != "" || u.Path != ""
	}
	return false
}

// headerParts is the text of the running header of every page after the cover.
func headerParts(d *ExportDoc) (left, centre, right, draft string) {
	left = d.Org.Name
	right = d.Meta.DocumentID
	if d.VersionLabel != "" {
		right += " v" + d.VersionLabel
	}
	centre = strings.ToUpper(d.Meta.Classification)
	if d.Draft() {
		draft = "DRAFT - not approved"
	}
	return
}

// footerParts is the text of the running footer; page is 1-based after the cover.
func footerParts(d *ExportDoc, page, pages int) (left, right string) {
	return "Generated " + d.GeneratedAt.UTC().Format("2006-01-02"), fmt.Sprintf("Page %d of %d", page, pages)
}

// ---- header, footer, watermark -------------------------------------------

func (r *pdfRender) hooks() {
	m := pageMarginMM * creator.PPMM
	r.c.DrawHeader(func(b *creator.Block, a creator.HeaderFunctionArgs) {
		r.pages++
		if r.d.Draft() {
			r.watermark(b)
			r.watermarks++
		}
		if a.PageNum <= 1 {
			return // the cover has no running header
		}
		left, centre, right, draft := headerParts(r.d)
		y := m/2 - 6
		r.hfText(b, left, m, y, r.w, creator.TextAlignmentLeft, colGrey, false)
		r.hfText(b, centre, m, y, r.w, creator.TextAlignmentCenter, colGrey, true)
		r.hfText(b, right, m, y, r.w, creator.TextAlignmentRight, colGrey, false)
		if draft != "" {
			r.draftLines++
			r.hfText(b, draft, m, y+10, r.w, creator.TextAlignmentCenter, colDraft, true)
		}
	})
	r.c.DrawFooter(func(b *creator.Block, a creator.FooterFunctionArgs) {
		if a.PageNum <= 1 {
			return
		}
		left, right := footerParts(r.d, a.PageNum-1, a.TotalPages-1)
		y := m / 2
		r.hfText(b, left, m, y, r.w, creator.TextAlignmentLeft, colGrey, false)
		r.hfText(b, right, m, y, r.w, creator.TextAlignmentRight, colGrey, false)
	})
}

func (r *pdfRender) hfText(b *creator.Block, text string, x, y, width float64, align creator.TextAlignment, col creator.Color, bold bool) {
	if text == "" {
		return
	}
	p := r.c.NewStyledParagraph()
	fc := r.f.regular
	if bold {
		fc = r.f.bold
	}
	ch := p.Append(fc.clean(text))
	ch.Style.Font, ch.Style.FontSize, ch.Style.Color = fc.font, 8, col
	p.SetWidth(width)
	p.SetTextAlignment(align)
	p.SetPos(x, y)
	if err := b.Draw(p); err != nil {
		log.Printf("export: header/footer: %v", err)
	}
}

// watermarkLetters is a stroke font for "DRAFT": each letter is polylines on a
// 6x10 grid. It is drawn as translucent vector strokes because the library has
// no text transparency and no font ships in the binary.
var watermarkLetters = [][][][2]float64{
	{{{0, 0}, {0, 10}, {3, 10}, {5.5, 8}, {6, 5}, {5.5, 2}, {3, 0}, {0, 0}}},           // D
	{{{0, 10}, {0, 0}, {4, 0}, {6, 1.5}, {6, 3.5}, {4, 5}, {0, 5}}, {{3, 5}, {6, 10}}}, // R
	{{{0, 10}, {3, 0}, {6, 10}}, {{1.2, 6.5}, {4.8, 6.5}}},                             // A
	{{{6, 0}, {0, 0}, {0, 10}}, {{0, 5}, {4.5, 5}}},                                    // F
	{{{0, 0}, {6, 0}}, {{3, 0}, {3, 10}}},                                              // T
}

// watermarkStrokes returns the polylines of "DRAFT", rotated 45 degrees about
// (cx, cy), with the given scale (points per grid unit).
func watermarkStrokes(cx, cy, scale float64) [][]draw.Point {
	const gap, letterW, h = 2.0, 6.0, 10.0
	total := float64(len(watermarkLetters))*letterW + float64(len(watermarkLetters)-1)*gap
	const cos, sin = 0.7071067811865476, -0.7071067811865476 // 45 degrees counter-clockwise on the page
	var out [][]draw.Point
	for i, strokes := range watermarkLetters {
		ox := float64(i) * (letterW + gap)
		for _, s := range strokes {
			pts := make([]draw.Point, len(s))
			for j, p := range s {
				x := (ox + p[0] - total/2) * scale
				y := (p[1] - h/2) * scale
				pts[j] = draw.Point{X: cx + x*cos - y*sin, Y: cy + x*sin + y*cos}
			}
			out = append(out, pts)
		}
	}
	return out
}

func (r *pdfRender) watermark(b *creator.Block) {
	pageW, pageH := r.c.Width(), r.c.Height()
	for _, pts := range watermarkStrokes(pageW/2, pageH/2, 0.7*r.w/38) {
		pl := r.c.NewPolyline(pts)
		pl.SetLineColor(colMark)
		pl.SetLineWidth(0.9 * r.w / 38)
		pl.SetLineOpacity(0.14)
		if err := b.Draw(pl); err != nil {
			log.Printf("export: watermark: %v", err)
			return
		}
	}
}

// ---- cover and revision history -------------------------------------------

func (r *pdfRender) para() *creator.StyledParagraph {
	p := r.c.NewStyledParagraph()
	p.SetLineHeight(lineSpacing)
	return p
}

// text appends one styled run to p.
func (r *pdfRender) text(p *creator.StyledParagraph, fc face, s string, size float64, col creator.Color) *creator.TextChunk {
	ch := p.Append(fc.clean(s))
	ch.Style.Font, ch.Style.FontSize, ch.Style.Color = fc.font, size, col
	return ch
}

func (r *pdfRender) draw(v creator.Drawable) {
	if err := r.c.Draw(v); err != nil {
		log.Printf("export: cover: %v", err)
	}
}

func (r *pdfRender) cover() {
	d := r.d
	if len(d.Org.LogoPNG) > 0 {
		if img, err := r.c.NewImageFromData(d.Org.LogoPNG); err == nil && img.Height() > 0 {
			maxH := 40 * creator.PPMM
			if img.Height() > maxH {
				img.ScaleToHeight(maxH)
			}
			img.SetMargins(0, 0, 0, 14)
			r.draw(img)
		}
	}
	if d.Org.Name != "" {
		p := r.para()
		r.text(p, r.f.bold, d.Org.Name, 12, colGrey)
		p.SetMargins(0, 0, 30, 6)
		r.draw(p)
	}
	t := r.para()
	title := d.Meta.Title
	if title == "" {
		title = d.Meta.DocumentID
	}
	r.text(t, r.f.bold, title, 26, r.brand)
	t.SetLineHeight(1.2)
	t.SetMargins(0, 0, 6, 8)
	r.draw(t)
	r.draw(r.rule(r.brand, 2, 0, 12))
	if d.Draft() {
		p := r.para()
		r.text(p, r.f.bold, "DRAFT - not approved", 12, colDraft)
		p.SetMargins(0, 0, 0, 12)
		r.draw(p)
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
	tbl := r.c.NewTable(2)
	_ = tbl.SetColumnWidths(0.3, 0.7)
	for _, row := range rows {
		if row[1] == "" && !required[row[0]] {
			continue
		}
		r.cell(tbl, r.textPara(row[0], r.f.bold, 10, colText), creator.CellHorizontalAlignmentLeft, colHead, true)
		r.cell(tbl, r.textPara(orDash(row[1]), r.f.regular, 10, colText), creator.CellHorizontalAlignmentLeft, nil, true)
	}
	r.draw(tbl)

	g := r.para()
	r.text(g, r.f.regular, "Generated "+d.GeneratedAt.UTC().Format("2006-01-02 15:04")+" UTC", 9, colGrey)
	g.SetMargins(0, 0, 14, 0)
	r.draw(g)
	r.draw(r.c.NewPageBreak())
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func (r *pdfRender) textPara(s string, fc face, size float64, col creator.Color) *creator.StyledParagraph {
	p := r.para()
	r.text(p, fc, s, size, col)
	return p
}

// rule is a horizontal line: a one-cell table with a bottom border.
func (r *pdfRender) rule(col creator.Color, width, top, bottom float64) *creator.Table {
	t := r.c.NewTable(1)
	cell := t.NewCell()
	p := r.para()
	r.text(p, r.f.regular, " ", 1, colText)
	_ = cell.SetContent(p)
	cell.SetBorder(creator.CellBorderSideBottom, creator.CellBorderStyleSingle, width)
	cell.SetSideBorderColor(creator.CellBorderSideBottom, col)
	t.SetMargins(0, 0, top, bottom)
	return t
}

// cell adds a bordered table cell holding p.
func (r *pdfRender) cell(t *creator.Table, p *creator.StyledParagraph, h creator.CellHorizontalAlignment, bg creator.Color, border bool) {
	c := t.NewCell()
	p.SetMargins(3, 3, 2, 2)
	_ = c.SetContent(p)
	c.SetHorizontalAlignment(h)
	if bg != nil {
		c.SetBackgroundColor(bg)
	}
	if border {
		c.SetBorder(creator.CellBorderSideAll, creator.CellBorderStyleSingle, 0.5)
		c.SetBorderColor(colBorder)
	}
}

func (r *pdfRender) revisions() {
	rows := r.d.RevisionRows()
	if len(rows) == 0 {
		return
	}
	h := r.textPara("Revision history", r.f.bold, 16, colText)
	h.SetMargins(0, 0, 0, 8)
	r.draw(h)
	heads := []string{"Version", "Date", "Author", "Approved by", "Description"}
	t := r.c.NewTable(len(heads))
	_ = t.SetColumnWidths(0.1, 0.14, 0.17, 0.17, 0.42)
	for _, s := range heads {
		r.cell(t, r.textPara(s, r.f.bold, 9, colText), creator.CellHorizontalAlignmentLeft, colHead, true)
	}
	for _, ch := range rows {
		for _, s := range []string{ch.Version, ch.Date, ch.Author, ch.ApprovedBy, ch.Description} {
			r.cell(t, r.textPara(orDash(s), r.f.regular, 9, colText), creator.CellHorizontalAlignmentLeft, nil, true)
		}
	}
	_ = t.SetHeaderRows(1, 1)
	r.draw(t)
	r.draw(r.c.NewPageBreak())
}

// ---- body ------------------------------------------------------------------

// ctx is the position of a block in the tree.
type ctx struct {
	indent float64 // left indent in points
	inList bool    // inside a list item: only what a list can hold is drawn natively
}

// margined is what every drawable of this renderer can do.
type margined interface {
	SetMargins(left, right, top, bottom float64)
}

// blocks converts blocks to drawables, in order.
func (r *pdfRender) blocks(bs []Block, cx ctx) []creator.VectorDrawable {
	var out []creator.VectorDrawable
	for _, b := range bs {
		switch b := b.(type) {
		case Heading:
			out = append(out, r.heading(b, cx))
		case Paragraph:
			out = append(out, r.paragraph(b, cx))
		case List:
			out = append(out, r.list(b, cx)...)
		case CodeBlock:
			out = append(out, r.codeBlock(b, cx))
		case Table:
			out = append(out, r.table(b, cx))
		case Quote:
			out = append(out, r.quote(b, cx)...)
		case Rule:
			out = append(out, r.rule(colBorder, 0.7, 6, 8))
		}
	}
	return out
}

var headingSizes = [...]float64{0, 20, 16, 13.5, 12, 11, 10.5}

func (r *pdfRender) heading(h Heading, cx ctx) *creator.StyledParagraph {
	lv := h.Level
	if lv < 1 || lv > 6 {
		lv = 6
	}
	p := r.para()
	p.SetLineHeight(1.2)
	r.inlines(p, h.Inlines, inl{face: r.f.bold, size: headingSizes[lv], col: colText, bold: true})
	p.SetMargins(cx.indent, 0, headingSizes[lv]*0.7, 4)
	return p
}

func (r *pdfRender) paragraph(pb Paragraph, cx ctx) *creator.StyledParagraph {
	p := r.para()
	r.inlines(p, pb.Inlines, inl{face: r.f.regular, size: baseSize, col: colText})
	p.SetMargins(cx.indent, 0, 0, paraGap)
	return p
}

// inl is the running inline style.
type inl struct {
	face         face
	size         float64
	col          creator.Color
	bold, italic bool
	underline    bool
}

func (r *pdfRender) variant(s inl, code bool) face {
	switch {
	case code:
		return r.f.mono
	case s.bold && s.italic:
		return r.f.boldItalic
	case s.bold:
		return r.f.bold
	case s.italic:
		return r.f.italic
	}
	return r.f.regular
}

// inlines appends inline content to p. Strike-through has no vector API in the
// library, so struck text prints plain.
func (r *pdfRender) inlines(p *creator.StyledParagraph, ins []Inline, s inl) {
	for _, in := range ins {
		switch in := in.(type) {
		case Text:
			st := s
			st.bold = st.bold || in.Bold
			st.italic = st.italic || in.Italic
			fc := r.variant(st, in.Code)
			size := st.size
			if in.Code {
				size *= 0.92
			}
			ch := r.text(p, fc, in.Text, size, st.col)
			ch.Style.Underline = st.underline
		case Link:
			if linkAllowed(in.Href) {
				r.link(p, in, s)
				continue
			}
			r.inlines(p, in.Inlines, s)
		case LineBreak:
			if in.Hard {
				r.text(p, r.f.regular, "\n", s.size, s.col)
			} else {
				r.text(p, r.f.regular, " ", s.size, s.col)
			}
		case Image:
			alt := strings.TrimSpace(in.Alt)
			label := "[image]"
			if alt != "" {
				label = "[image: " + alt + "]"
			}
			r.text(p, r.f.italic, label, s.size, colGrey)
		}
	}
}

// link adds a clickable external link, blue and underlined.
func (r *pdfRender) link(p *creator.StyledParagraph, l Link, s inl) {
	label := plainText(l.Inlines)
	if strings.TrimSpace(label) == "" {
		label = l.Href
	}
	fc := r.variant(s, false)
	ch := p.AddExternalLink(fc.clean(label), strings.TrimSpace(l.Href))
	ch.Style.Font, ch.Style.FontSize, ch.Style.Color, ch.Style.Underline = fc.font, s.size, colLink, true
}

// plainText flattens inlines to their text.
func plainText(ins []Inline) string {
	var sb strings.Builder
	for _, in := range ins {
		switch in := in.(type) {
		case Text:
			sb.WriteString(in.Text)
		case Link:
			sb.WriteString(plainText(in.Inlines))
		case LineBreak:
			sb.WriteString(" ")
		case Image:
			sb.WriteString(in.Alt)
		}
	}
	return sb.String()
}

// list draws a list. Blocks inside an item that a native list cannot hold
// (code, tables, quotes, rules) end the list, are drawn at the item's indent, and
// the list continues after them with its numbering intact.
func (r *pdfRender) list(l List, cx ctx) []creator.VectorDrawable {
	var out []creator.VectorDrawable
	cur := r.newList(cx)
	flush := func() {
		if cur != nil {
			out = append(out, cur)
			cur = nil
		}
	}
	n := l.Start
	if n == 0 && l.Ordered {
		n = 1
	}
	for _, it := range l.Items {
		marker := "\u2022"
		if l.Ordered {
			marker = fmt.Sprintf("%d.", n)
			n++
		}
		switch it.Task {
		case TaskChecked:
			marker = "[x]"
		case TaskUnchecked:
			marker = "[ ]"
		}
		if cur == nil {
			cur = r.newList(cx)
		}
		extras := r.listItem(cur, marker, it, cx)
		if len(extras) > 0 {
			flush()
			out = append(out, extras...)
		}
	}
	flush()
	return out
}

func (r *pdfRender) newList(cx ctx) *creator.List {
	ls := r.c.NewList()
	ls.SetIndent(indentMM * creator.PPMM)
	ls.SetMargins(cx.indent, 0, 0, 2)
	return ls
}

// listItem adds one item to ls and returns blocks to draw after the list.
func (r *pdfRender) listItem(ls *creator.List, marker string, it ListItem, cx ctx) []creator.VectorDrawable {
	var first *creator.StyledParagraph
	var nested []*creator.List
	var after []creator.VectorDrawable
	inner := ctx{indent: cx.indent + indentMM*creator.PPMM, inList: true}
	for _, b := range it.Blocks {
		switch b := b.(type) {
		case Paragraph:
			if first == nil {
				first = r.para()
				r.inlines(first, b.Inlines, inl{face: r.f.regular, size: baseSize, col: colText})
			} else {
				r.text(first, r.f.regular, "\n", baseSize, colText)
				r.inlines(first, b.Inlines, inl{face: r.f.regular, size: baseSize, col: colText})
			}
		case List:
			sub := r.nestedList(b)
			nested = append(nested, sub)
		default:
			after = append(after, r.blocks([]Block{b}, inner)...)
		}
	}
	if first == nil {
		first = r.para()
		r.text(first, r.f.regular, " ", baseSize, colText)
	}
	mk, err := ls.Add(first)
	if err != nil {
		log.Printf("export: list item: %v", err)
		return after
	}
	mk.Text = r.f.regular.clean(marker)
	mk.Style.Font, mk.Style.FontSize, mk.Style.Color = r.f.regular.font, baseSize, colText
	for _, sub := range nested {
		if _, err := ls.Add(sub); err != nil {
			log.Printf("export: nested list: %v", err)
		}
	}
	return after
}

// nestedList builds a list for use inside a list item. A native list can only
// hold paragraphs and lists, so anything else inside it is drawn as text.
func (r *pdfRender) nestedList(l List) *creator.List {
	ls := r.c.NewList()
	ls.SetIndent(indentMM * creator.PPMM)
	n := l.Start
	if n == 0 && l.Ordered {
		n = 1
	}
	for _, it := range l.Items {
		marker := "\u2022"
		if l.Ordered {
			marker = fmt.Sprintf("%d.", n)
			n++
		}
		switch it.Task {
		case TaskChecked:
			marker = "[x]"
		case TaskUnchecked:
			marker = "[ ]"
		}
		p := r.para()
		var subs []*creator.List
		for _, b := range it.Blocks {
			switch b := b.(type) {
			case List:
				subs = append(subs, r.nestedList(b))
			default:
				if t := blockText(b); t != "" {
					if len(p.GetChunks()) > 0 {
						r.text(p, r.f.regular, "\n", baseSize, colText)
					}
					r.inlines(p, []Inline{Text{Text: t}}, inl{face: r.f.regular, size: baseSize, col: colText})
				}
			}
		}
		if len(p.GetChunks()) == 0 {
			r.text(p, r.f.regular, " ", baseSize, colText)
		}
		mk, err := ls.Add(p)
		if err != nil {
			continue
		}
		mk.Text = r.f.regular.clean(marker)
		mk.Style.Font, mk.Style.FontSize, mk.Style.Color = r.f.regular.font, baseSize, colText
		for _, s := range subs {
			_, _ = ls.Add(s)
		}
	}
	return ls
}

// blockText is a block's text, for places that can only hold a paragraph.
func blockText(b Block) string {
	switch b := b.(type) {
	case Paragraph:
		return plainText(b.Inlines)
	case Heading:
		return plainText(b.Inlines)
	case CodeBlock:
		return b.Text
	case Quote:
		var parts []string
		for _, c := range b.Blocks {
			if t := blockText(c); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// codeBlock is mono text on a light grey background, one paragraph per line so
// whitespace is kept and the block can break across pages.
func (r *pdfRender) codeBlock(cb CodeBlock, cx ctx) *creator.Division {
	div := r.c.NewDivision()
	div.SetMargins(cx.indent, 0, 2, paraGap+2)
	div.SetPadding(5, 5, 4, 4)
	div.SetBackground(&creator.Background{FillColor: colLight, BorderColor: colBorder, BorderSize: 0.5})
	if lang := strings.TrimSpace(cb.Lang); lang != "" {
		p := r.para()
		r.text(p, r.f.mono, lang, 6.5, colGrey)
		_ = div.Add(p)
	}
	for _, line := range strings.Split(cb.Text, "\n") {
		p := r.c.NewStyledParagraph()
		p.SetLineHeight(1.25)
		line = r.f.mono.clean(line)
		// Leading blanks become non-breaking spaces (only where the font really
		// has one: the standard fonts do not) so indentation survives.
		nb := " "
		if !r.f.mono.std && r.f.mono.has('\u00a0') {
			nb = "\u00a0"
		}
		trimmed := strings.TrimLeft(line, " ")
		line = strings.Repeat(nb, len(line)-len(trimmed)) + trimmed
		if line == "" {
			line = nb
		}
		ch := p.Append(line)
		ch.Style.Font, ch.Style.FontSize, ch.Style.Color = r.f.mono.font, 8.5, colText
		_ = div.Add(p)
	}
	return div
}

// table is a bordered table with a shaded bold header row that repeats on every
// page the table spans.
func (r *pdfRender) table(tb Table, cx ctx) *creator.Table {
	cols := len(tb.Header)
	if cols == 0 {
		cols = 1
	}
	t := r.c.NewTable(cols)
	_ = t.SetColumnWidths(columnWidths(tb, cols)...)
	t.SetMargins(cx.indent, 0, 2, paraGap+2)
	al := func(i int) creator.CellHorizontalAlignment {
		if i < len(tb.Align) {
			switch tb.Align[i] {
			case AlignCenter:
				return creator.CellHorizontalAlignmentCenter
			case AlignRight:
				return creator.CellHorizontalAlignmentRight
			}
		}
		return creator.CellHorizontalAlignmentLeft
	}
	row := func(cells []Cell, head bool) {
		for i := 0; i < cols; i++ {
			var c Cell
			if i < len(cells) {
				c = cells[i]
			}
			p := r.para()
			st := inl{face: r.f.regular, size: 9.5, col: colText}
			var bg creator.Color
			if head {
				st.bold = true
				st.face = r.f.bold
				bg = colHead
			}
			r.inlines(p, c.Inlines, st)
			if len(p.GetChunks()) == 0 {
				r.text(p, r.f.regular, " ", 9.5, colText)
			}
			p.SetTextAlignment(textAlign(al(i)))
			r.cell(t, p, al(i), bg, true)
		}
	}
	if len(tb.Header) > 0 {
		row(tb.Header, true)
		_ = t.SetHeaderRows(1, 1)
	}
	for _, rw := range tb.Rows {
		row(rw, false)
	}
	return t
}

func textAlign(a creator.CellHorizontalAlignment) creator.TextAlignment {
	switch a {
	case creator.CellHorizontalAlignmentCenter:
		return creator.TextAlignmentCenter
	case creator.CellHorizontalAlignmentRight:
		return creator.TextAlignmentRight
	}
	return creator.TextAlignmentLeft
}

// columnWidths weights columns by their longest text (bounded), so a narrow ID
// column is not as wide as a description column. The result sums to 1.
func columnWidths(tb Table, cols int) []float64 {
	w := make([]float64, cols)
	for i := range w {
		w[i] = 8
	}
	measure := func(cells []Cell) {
		for i := 0; i < cols && i < len(cells); i++ {
			n := float64(len([]rune(plainText(cells[i].Inlines))))
			if n > 48 {
				n = 48
			}
			if n > w[i] {
				w[i] = n
			}
		}
	}
	measure(tb.Header)
	for i, rw := range tb.Rows {
		if i >= 200 {
			break
		}
		measure(rw)
	}
	var sum float64
	for _, v := range w {
		sum += v
	}
	for i := range w {
		w[i] /= sum
	}
	return w
}

// quote draws each child with a grey bar on its left and grey text.
func (r *pdfRender) quote(q Quote, cx ctx) []creator.VectorDrawable {
	var out []creator.VectorDrawable
	for _, v := range r.blocks(q.Blocks, ctx{indent: 0, inList: cx.inList}) {
		t := r.c.NewTable(2)
		barW := 3.0 / (r.w - cx.indent)
		_ = t.SetColumnWidths(barW, 1-barW)
		t.SetMargins(cx.indent, 0, 0, 0)
		bar := t.NewCell()
		bp := r.para()
		r.text(bp, r.f.regular, " ", 2, colText)
		_ = bar.SetContent(bp)
		bar.SetBackgroundColor(colBorder)
		body := t.NewCell()
		if sp, ok := v.(*creator.StyledParagraph); ok {
			for _, ch := range sp.GetChunks() {
				ch.Style.Color = colGrey
			}
			sp.SetMargins(6, 0, 0, paraGap)
		} else if m, ok := v.(margined); ok {
			m.SetMargins(6, 0, 0, paraGap)
		}
		if err := body.SetContent(v); err != nil {
			log.Printf("export: quote: %v", err)
			continue
		}
		out = append(out, t)
	}
	return out
}
