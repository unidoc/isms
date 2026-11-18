package export

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/unidoc/unipdf/v5/core"
)

// The watermark is plain PDF objects built here, so what goes into the file is
// checked without a licence; the key-gated test checks it in a rendered file.

func TestWatermarkContentIsRotatedHelveticaDRAFT(t *testing.T) {
	const w, h = 595.28, 841.89
	c := string(watermarkContent(w, h))
	for _, want := range []string{"(DRAFT) Tj", "/FWM ", "/GSWM gs", "0.70711 0.70711 -0.70711 0.70711"} {
		if !strings.Contains(c, want) {
			t.Errorf("content lacks %q:\n%s", want, c)
		}
	}
	if !strings.HasPrefix(c, "q\n") || !strings.HasSuffix(c, "Q\n") {
		t.Errorf("the watermark must save and restore the graphics state:\n%s", c)
	}
	// The word spans about 64% of the page diagonal.
	var size float64
	for _, line := range strings.Split(c, "\n") {
		if strings.HasSuffix(line, " Tf") {
			if _, err := fmt.Sscanf(line, "/FWM %f Tf", &size); err != nil {
				t.Fatal(err)
			}
		}
	}
	span := wmWordEm * size / math.Hypot(w, h)
	if span < 0.6 || span > 0.8 {
		t.Errorf("the word spans %.0f%% of the diagonal, want about 64%%", span*100)
	}
}

func TestWatermarkResources(t *testing.T) {
	f := watermarkFont()
	if got := f.Get("BaseFont").(*core.PdfObjectName).String(); got != "Helvetica-Bold" {
		t.Errorf("BaseFont = %q", got)
	}
	if f.Get("FontDescriptor") != nil || f.Get("FontFile2") != nil {
		t.Error("the watermark font must be a standard font, never embedded")
	}
	g := watermarkState()
	ca := float64(*g.Get("ca").(*core.PdfObjectFloat))
	if ca <= 0 || ca > 0.2 {
		t.Errorf("fill opacity = %v, want a light value of at most 0.2", ca)
	}
}

// A page's content gets the watermark FIRST, so everything else is drawn over it.
func TestWatermarkOptimizerPutsItBehindTheContent(t *testing.T) {
	var objs []core.PdfObject
	const pages = 3
	for i := 0; i < pages; i++ {
		body, err := core.MakeStream([]byte("BT /F1 12 Tf (BODY) Tj ET"), nil)
		if err != nil {
			t.Fatal(err)
		}
		page := core.MakeDict()
		page.Set("Type", core.MakeName("Page"))
		page.Set("Contents", core.MakeIndirectObject(body))
		page.Set("Resources", core.MakeDict())
		objs = append(objs, core.MakeIndirectObject(page), core.MakeIndirectObject(core.MakeDict())) // a non-page object too
	}
	for i, o := range objs {
		o.(*core.PdfIndirectObject).ObjectNumber = int64(i + 1) // as the writer numbers them
	}
	out, err := (&watermarkOptimizer{pageW: 595.28, pageH: 841.89}).Optimize(objs)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(objs)+pages {
		t.Fatalf("objects = %d, want %d (one new stream per page)", len(out), len(objs)+pages)
	}
	seen := map[int64]bool{}
	for _, o := range out[len(objs):] {
		n := o.(*core.PdfObjectStream).ObjectNumber
		if n <= int64(len(objs)) || seen[n] {
			t.Errorf("new stream has object number %d: must be unique and above the %d in use", n, len(objs))
		}
		seen[n] = true
	}
	for i := 0; i < pages; i++ {
		page, _ := core.GetDict(objs[2*i].(*core.PdfIndirectObject).PdfObject)
		arr, ok := core.TraceToDirectObject(page.Get("Contents")).(*core.PdfObjectArray)
		if !ok || arr.Len() != 2 {
			t.Fatalf("page %d: Contents is %T", i+1, page.Get("Contents"))
		}
		first, ok1 := core.GetStream(core.TraceToDirectObject(arr.Get(0)))
		second, ok2 := core.GetStream(core.TraceToDirectObject(arr.Get(1)))
		if !ok1 || !ok2 {
			t.Fatalf("page %d: content objects are not streams", i+1)
		}
		wm, _ := core.DecodeStream(first)
		body, _ := core.DecodeStream(second)
		if !strings.Contains(string(wm), "(DRAFT) Tj") || strings.Contains(string(wm), "BODY") {
			t.Errorf("page %d: the first content stream is not the watermark: %q", i+1, wm)
		}
		if string(body) != "BT /F1 12 Tf (BODY) Tj ET" {
			t.Errorf("page %d: the body content changed or moved: %q", i+1, body)
		}
		res, _ := core.GetDict(page.Get("Resources"))
		if fonts, ok := core.GetDict(res.Get("Font")); !ok || fonts.Get(wmFontName) == nil {
			t.Errorf("page %d: no watermark font resource", i+1)
		}
		if gs, ok := core.GetDict(res.Get("ExtGState")); !ok || gs.Get(wmStateName) == nil {
			t.Errorf("page %d: no watermark opacity resource", i+1)
		}
	}
}
