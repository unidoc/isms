package export

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"isms.sh/internal/isms/model"
)

const (
	polish   = "Zażółć gęślą jaźń"
	japanese = "情報セキュリティ"
)

// goFontDir writes the Go fonts (BSD, test-only: nothing ships in the binary)
// under the file names the renderer expects, and returns the directory.
func goFontDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, b := range map[string][]byte{
		"sans-regular.ttf": goregular.TTF, "sans-bold.ttf": gobold.TTF,
		"sans-italic.ttf": goitalic.TTF, "sans-bolditalic.ttf": gobolditalic.TTF,
		"mono-regular.ttf": gomono.TTF,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func tinyPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 200, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func txt(s string) Inline     { return Text{Text: s} }
func para(in ...Inline) Block { return Paragraph{Inlines: in} }

// richDoc covers every block and inline kind.
func richDoc(status string) *ExportDoc {
	rows := make([][]Cell, 150)
	for i := range rows {
		rows[i] = []Cell{
			{Inlines: []Inline{txt(fmt.Sprintf("ROW-%03d", i))}},
			{Inlines: []Inline{txt("value "), Text{Text: "bold", Bold: true}}},
			{Inlines: []Inline{txt(fmt.Sprintf("%d", i*7))}},
		}
	}
	var code string
	for i := 0; i < 120; i++ {
		code += fmt.Sprintf("    line %03d := compute(%d) // a long line that should wrap around the right edge of the page when it is longer than the page is wide\n", i, i)
	}
	return &ExportDoc{
		Meta: model.DocumentFrontmatter{
			DocumentID: "iso27001-4-1", Title: "Context of the Organization", Status: status,
			Classification: "Confidential", Owner: "Ann Owner", Author: "Bob Author", ApprovedBy: "Cy Approver",
			ApprovedDate: "2026-01-02", EffectiveDate: "2026-02-01", NextReview: "2027-02-01",
			Changelog: []model.DocumentChange{
				{Version: "1.0", Date: "2026-01-02", Author: "Bob", ApprovedBy: "Cy", Description: "First release"},
				{Version: "1.1", Date: "2026-03-04", Author: "Bob", ApprovedBy: "Cy", Description: "Added scope"},
			},
		},
		Org:          OrgInfo{Name: "Acme Corp", BrandColor: "#0a5fb4", LogoPNG: tinyPNG()},
		VersionLabel: "1.1",
		GeneratedAt:  time.Date(2026, 11, 18, 3, 30, 0, 0, time.UTC),
		Blocks: []Block{
			Heading{Level: 1, Inlines: []Inline{txt("Purpose")}},
			Heading{Level: 2, Inlines: []Inline{txt("Scope")}},
			Heading{Level: 3, Inlines: []Inline{txt("Detail")}},
			para(txt("Plain "), Text{Text: "bold", Bold: true}, txt(" "), Text{Text: "italic", Italic: true}, txt(" "),
				Text{Text: "both", Bold: true, Italic: true}, txt(" "), Text{Text: "code", Code: true}, txt(" "),
				Text{Text: "struck", Strike: true}, LineBreak{Hard: true}, txt("after hard break"), LineBreak{}, txt("after soft break.")),
			para(txt(polish + " / " + japanese)),
			para(
				Link{Href: "https://example.com/a", Inlines: []Inline{txt("web link")}}, txt(" "),
				Link{Href: "http://example.com/b", Inlines: []Inline{txt("http link")}}, txt(" "),
				Link{Href: "mailto:a@example.com", Inlines: []Inline{txt("mail link")}}, txt(" "),
				Link{Href: "javascript:alert(1)", Inlines: []Inline{txt("js link")}}, txt(" "),
				Link{Href: "/risks/RISK-12", Inlines: []Inline{txt("relative link")}}, txt(" "),
				Link{Href: "#frag", Inlines: []Inline{txt("fragment link")}}, txt(" "),
				Link{Href: "file:///etc/passwd", Inlines: []Inline{txt("file link")}}, txt(" "),
				Link{Href: "data:text/html,x", Inlines: []Inline{txt("data link")}},
			),
			para(Image{Alt: "network diagram", Src: "https://evil.example/x.png"}, txt(" "), Image{Src: "/local.png"}),
			List{Ordered: false, Items: []ListItem{
				{Blocks: []Block{para(txt("first bullet")), List{Ordered: true, Start: 3, Items: []ListItem{
					{Blocks: []Block{para(txt("nested three"))}},
					{Blocks: []Block{para(txt("nested four")), List{Items: []ListItem{{Blocks: []Block{para(txt("deep"))}}}}}},
				}}}},
				{Task: TaskChecked, Blocks: []Block{para(txt("done task"))}},
				{Task: TaskUnchecked, Blocks: []Block{para(txt("open task")), CodeBlock{Lang: "sh", Text: "echo in list"}}},
				{Blocks: []Block{para(txt("after the code"))}},
			}},
			Quote{Blocks: []Block{para(txt("quoted text")), List{Items: []ListItem{{Blocks: []Block{para(txt("quoted bullet"))}}}}}},
			Rule{},
			CodeBlock{Lang: "go", Text: code},
			CodeBlock{Lang: "mermaid", Text: "graph TD\n  A-->B"},
			Table{
				Header: []Cell{{Inlines: []Inline{txt("Control ID")}}, {Inlines: []Inline{txt("Description")}}, {Inlines: []Inline{txt("Score")}}},
				Align:  []Align{AlignLeft, AlignCenter, AlignRight},
				Rows:   rows,
			},
			para(txt("The end.")),
		},
	}
}
