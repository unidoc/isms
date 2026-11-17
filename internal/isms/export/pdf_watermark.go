package export

import (
	"fmt"
	"math"

	"github.com/unidoc/unipdf/v5/core"
	"github.com/unidoc/unipdf/v5/model"
)

// The DRAFT watermark is real text in Helvetica-Bold, a standard 14 font that is
// never embedded and that every viewer has (so it does not depend on
// ISMS_EXPORT_FONT_DIR). It is set at 45 degrees, centred, large and light grey
// with a fill opacity from an ExtGState, and it goes BEHIND the page content so
// body text stays crisp.
//
// The creator library draws headers after the body and has no text transparency,
// so the watermark is added at the last moment instead: SetPdfWriterAccessFunc
// hands over the writer just before the document is written, in the same pass,
// and each page's content streams get the watermark stream prepended.
const (
	wmFontName  = "FWM"  // font resource name
	wmStateName = "GSWM" // ExtGState resource name
	wmOpacity   = 0.13   // fill opacity, 0.12 to 0.15
	wmGray      = 0.55   // fill grey level
	wmSpan      = 0.64   // fraction of the page diagonal the word spans
	wmWordEm    = 3.388  // Helvetica-Bold "DRAFT" width at size 1: D 722, R 722, A 722, F 611, T 611
	wmCapHeight = 0.718  // Helvetica-Bold cap height at size 1
	wmWord      = "DRAFT"
)

// watermarkContent is the content stream of the watermark for a page of the
// given size in points: saved graphics state, opacity, grey, rotated text.
func watermarkContent(pageW, pageH float64) []byte {
	const c, s = 0.7071067811865476, 0.7071067811865476 // cos and sin of 45 degrees
	diag := pageDiagonal(pageW, pageH)
	size := wmSpan * diag / wmWordEm
	w, h := wmWordEm*size, wmCapHeight*size
	// Start of the baseline so the word's centre is the page centre.
	x := pageW/2 - (w/2)*c + (h/2)*s
	y := pageH/2 - (w/2)*s - (h/2)*c
	return []byte(fmt.Sprintf("q\n/%s gs\n%.3f g\nBT\n/%s %.2f Tf\n%.5f %.5f %.5f %.5f %.2f %.2f Tm\n(%s) Tj\nET\nQ\n",
		wmStateName, wmGray, wmFontName, size, c, s, -s, c, x, y, wmWord))
}

func pageDiagonal(w, h float64) float64 { return math.Hypot(w, h) }

// watermarkFont is the font resource: Helvetica-Bold, not embedded.
func watermarkFont() *core.PdfObjectDictionary {
	d := core.MakeDict()
	d.Set("Type", core.MakeName("Font"))
	d.Set("Subtype", core.MakeName("Type1"))
	d.Set("BaseFont", core.MakeName("Helvetica-Bold"))
	d.Set("Encoding", core.MakeName("WinAnsiEncoding"))
	return d
}

// watermarkState is the ExtGState resource: fill and stroke opacity.
func watermarkState() *core.PdfObjectDictionary {
	d := core.MakeDict()
	d.Set("Type", core.MakeName("ExtGState"))
	d.Set("ca", core.MakeFloat(wmOpacity))
	d.Set("CA", core.MakeFloat(wmOpacity))
	return d
}

// subDict returns the named dictionary of d, creating it when missing.
func subDict(d *core.PdfObjectDictionary, key core.PdfObjectName) *core.PdfObjectDictionary {
	if sub, ok := core.GetDict(d.Get(key)); ok && sub != nil {
		return sub
	}
	sub := core.MakeDict()
	d.Set(key, sub)
	return sub
}

// watermarkPage prepends the watermark to one page dictionary, adds the font and
// opacity resources it uses, and returns the new content stream object.
func watermarkPage(page *core.PdfObjectDictionary, pageW, pageH float64) (*core.PdfObjectStream, error) {
	res := subDict(page, "Resources")
	subDict(res, "Font").Set(wmFontName, watermarkFont())
	subDict(res, "ExtGState").Set(wmStateName, watermarkState())

	stream, err := core.MakeStream(watermarkContent(pageW, pageH), core.NewFlateEncoder())
	if err != nil {
		return nil, err
	}
	items := []core.PdfObject{stream}
	switch old := core.TraceToDirectObject(page.Get("Contents")).(type) {
	case *core.PdfObjectArray:
		items = append(items, old.Elements()...)
	case nil:
	default:
		items = append(items, page.Get("Contents"))
	}
	page.Set("Contents", core.MakeArray(items...))
	return stream, nil
}

// watermarkOptimizer is the hook that reaches the pages. The creator's writer
// hook (SetPdfWriterAccessFunc) runs before the pages are added to the writer,
// but the optimizer runs on the writer's complete object list just before the
// objects are numbered and written, in the same pass: it finds every page
// dictionary, prepends the watermark, and adds the new streams to the list.
type watermarkOptimizer struct {
	pageW, pageH float64
	next         model.Optimizer // any optimizer already set, run afterwards
}

// Optimize implements model.Optimizer.
func (o *watermarkOptimizer) Optimize(objects []core.PdfObject) ([]core.PdfObject, error) {
	// New objects are numbered after the highest number in use: the writer has
	// numbered its objects before the optimizer runs.
	var last int64
	for _, obj := range objects {
		switch o := obj.(type) {
		case *core.PdfIndirectObject:
			last = max(last, o.ObjectNumber)
		case *core.PdfObjectStream:
			last = max(last, o.ObjectNumber)
		}
	}
	var added []core.PdfObject
	for _, obj := range objects {
		ind, ok := obj.(*core.PdfIndirectObject)
		if !ok {
			continue
		}
		page, ok := core.GetDict(ind.PdfObject)
		if !ok || page == nil {
			continue
		}
		if t, ok := core.GetName(page.Get("Type")); !ok || t.String() != "Page" {
			continue
		}
		wm, err := watermarkPage(page, o.pageW, o.pageH)
		if err != nil {
			return nil, fmt.Errorf("export: watermark: %w", err)
		}
		last++
		wm.ObjectNumber, wm.GenerationNumber = last, 0
		added = append(added, wm)
	}
	objects = append(objects, added...)
	if o.next != nil {
		return o.next.Optimize(objects)
	}
	return objects, nil
}
