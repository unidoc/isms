package export

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// dump renders blocks as a compact string so cases stay one line each.
//
//	H2[..] P[..] UL[item..] OL3[..] CODE(lang)"text" TABLE{h:..|..;a:l,r;r:..} Q[..] HR
//	inlines: "text" with flags {b i s c}, L(href)[..], BR (hard) br (soft), IMG(alt|src)
func dump(bs []Block) string {
	var parts []string
	for _, b := range bs {
		switch b := b.(type) {
		case Heading:
			parts = append(parts, fmt.Sprintf("H%d[%s]", b.Level, dumpIn(b.Inlines)))
		case Paragraph:
			parts = append(parts, "P["+dumpIn(b.Inlines)+"]")
		case List:
			tag := "UL"
			if b.Ordered {
				tag = fmt.Sprintf("OL%d", b.Start)
			}
			var items []string
			for _, it := range b.Items {
				pre := ""
				switch it.Task {
				case TaskChecked:
					pre = "[x]"
				case TaskUnchecked:
					pre = "[ ]"
				}
				items = append(items, "<"+pre+dump(it.Blocks)+">")
			}
			parts = append(parts, tag+"["+strings.Join(items, "")+"]")
		case CodeBlock:
			parts = append(parts, fmt.Sprintf("CODE(%s)%q", b.Lang, b.Text))
		case Table:
			var al []string
			for _, a := range b.Align {
				al = append(al, map[Align]string{AlignNone: "-", AlignLeft: "l", AlignCenter: "c", AlignRight: "r"}[a])
			}
			rows := make([]string, len(b.Rows))
			for i, r := range b.Rows {
				rows[i] = dumpCells(r)
			}
			parts = append(parts, fmt.Sprintf("TABLE{h:%s;a:%s;r:%s}", dumpCells(b.Header), strings.Join(al, ","), strings.Join(rows, "/")))
		case Quote:
			parts = append(parts, "Q["+dump(b.Blocks)+"]")
		case Rule:
			parts = append(parts, "HR")
		default:
			parts = append(parts, fmt.Sprintf("?%T", b))
		}
	}
	return strings.Join(parts, " ")
}

func dumpCells(cs []Cell) string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = dumpIn(c.Inlines)
	}
	return strings.Join(out, "|")
}

func dumpIn(is []Inline) string {
	var out []string
	for _, in := range is {
		switch in := in.(type) {
		case Text:
			s := fmt.Sprintf("%q", in.Text)
			var f string
			if in.Bold {
				f += "b"
			}
			if in.Italic {
				f += "i"
			}
			if in.Strike {
				f += "s"
			}
			if in.Code {
				f += "c"
			}
			if f != "" {
				s += "{" + f + "}"
			}
			out = append(out, s)
		case Link:
			out = append(out, fmt.Sprintf("L(%s)[%s]", in.Href, dumpIn(in.Inlines)))
		case LineBreak:
			if in.Hard {
				out = append(out, "BR")
			} else {
				out = append(out, "br")
			}
		case Image:
			out = append(out, fmt.Sprintf("IMG(%s|%s)", in.Alt, in.Src))
		default:
			out = append(out, fmt.Sprintf("?%T", in))
		}
	}
	return strings.Join(out, " ")
}

func TestParse(t *testing.T) {
	cases := []struct{ name, md, want string }{
		{"empty", "", ""},
		{"blank only", "\n\n  \n", ""},
		{"heading and paragraph", "# Title\n\nHello world.", `H1["Title"] P["Hello world."]`},
		{"heading levels", "## a\n\n###### f", `H2["a"] H6["f"]`},
		{"inline styles", "a **b** *i* ~~s~~ `c`", `P["a " "b"{b} " " "i"{i} " " "s"{s} " " "c"{c}]`},
		{"nested emphasis", "***bi***", `P["bi"{bi}]`},
		{"bold in italic", "*a **b** c*", `P["a "{i} "b"{bi} " c"{i}]`},
		{"escapes and entities", `a \* b &amp; c &copy;`, `P["a * b & c ©"]`},
		{"hard and soft break", "a\nb  \nc", `P["a" br "b" BR "c"]`},
		{"rule", "a\n\n---\n\nb", `P["a"] HR P["b"]`},
		{"link", "[x](https://example.com/a?b=1)", `P[L(https://example.com/a?b=1)["x"]]`},
		{"link with formatting", "[**x** y](http://e.com)", `P[L(http://e.com)["x"{b} " y"]]`},
		{"relative link kept as is", "[r](/risks/RISK-12) [d](../a/b.md#c) [f](#frag)",
			`P[L(/risks/RISK-12)["r"] " " L(../a/b.md#c)["d"] " " L(#frag)["f"]]`},
		{"autolink and linkify", "<https://a.example> and www.b.example/x", `P[L(https://a.example)["https://a.example"] " and " L(http://www.b.example/x)["www.b.example/x"]]`},
		{"image never fetched", "![alt text](https://evil.example/x.png)", `P[IMG(alt text|https://evil.example/x.png)]`},
		{"image local path", "![l](/etc/passwd) ![f](file:///etc/hosts)", `P[IMG(l|/etc/passwd) " " IMG(f|file:///etc/hosts)]`},
		{"bullet list", "- a\n- b", `UL[<P["a"]><P["b"]>]`},
		{"ordered list start", "3. a\n4. b", `OL3[<P["a"]><P["b"]>]`},
		{"task list", "- [x] done\n- [ ] todo\n- plain", `UL[<[x]P["done"]><[ ]P["todo"]><P["plain"]>]`},
		{"nested list", "- a\n  - b\n    1. c", `UL[<P["a"] UL[<P["b"] OL1[<P["c"]>]>]>]`},
		{"loose list item paragraphs", "- a\n\n  b\n", `UL[<P["a"] P["b"]>]`},
		{"quote", "> a\n> b", `Q[P["a" br "b"]]`},
		{"nested quote", "> a\n>> b", `Q[P["a"] Q[P["b"]]]`},
		{"quote in list", "- a\n  > q", `UL[<P["a"] Q[P["q"]]>]`},
		{"list in quote", "> - a\n> - b", `Q[UL[<P["a"]><P["b"]>]]`},
		{"code fence lang", "```go\nfmt.Println(1)\n```", `CODE(go)"fmt.Println(1)"`},
		{"mermaid stays code", "```mermaid\ngraph TD\n  A-->B\n```", `CODE(mermaid)"graph TD\n  A-->B"`},
		{"unknown lang", "```zzz-unknown\nx\n```", `CODE(zzz-unknown)"x"`},
		{"no lang", "```\nx\n\ny\n```", `CODE()"x\n\ny"`},
		{"indented code", "    a\n    b", `CODE()"a\nb"`},
		{"code is not parsed", "```\n**x** <b>\n```", `CODE()"**x** <b>"`},
		{"crlf", "# T\r\n\r\na\r\nb\r\n\r\n```go\r\nx\r\n```\r\n", `H1["T"] P["a" br "b"] CODE(go)"x"`},
		{"html block is text", "<div>hi <b>x</b></div>\n\nafter", `P["<div>hi <b>x</b></div>"] P["after"]`},
		{"html comment block", "<!-- note -->\n\nx", `P["<!-- note -->"] P["x"]`},
		{"inline html is text", "a <b>bold</b> c", `P["a <b>bold</b> c"]`},
		{"inline html script", "x <script>alert(1)</script>", `P["x <script>alert(1)</script>"]`},
		{"html next to markdown", "a <i>b</i> **c**", `P["a <i>b</i> " "c"{b}]`},
		{"table", "| A | B |\n|:--|--:|\n| 1 | **2** |\n| 3 | `4` |",
			`TABLE{h:"A"|"B";a:l,r;r:"1"|"2"{b}/"3"|"4"{c}}`},
		{"table alignments", "| a | b | c | d |\n|---|:-:|:--|--:|\n| 1 | 2 | 3 | 4 |", `TABLE{h:"a"|"b"|"c"|"d";a:-,c,l,r;r:"1"|"2"|"3"|"4"}`},
		{"table ragged padded", "| a | b | c |\n|---|---|---|\n| 1 |\n| 1 | 2 | 3 |", `TABLE{h:"a"|"b"|"c";a:-,-,-;r:"1"||/"1"|"2"|"3"}`},
		{"table link cell", "| a |\n|---|\n| [x](/y) |", `TABLE{h:"a";a:-;r:L(/y)["x"]}`},
		{"table no rows", "| a | b |\n|---|---|", `TABLE{h:"a"|"b";a:-,-;r:}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bs, err := Parse([]byte(c.md))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got := dump(bs)
			if got != c.want {
				t.Errorf("md:   %q\ngot:  %s\nwant: %s", c.md, got, c.want)
			}
		})
	}
}

func TestParseEmptyBodyHasNoBlocks(t *testing.T) {
	for _, b := range [][]byte{nil, {}, []byte("")} {
		bs, err := Parse(b)
		if err != nil || len(bs) != 0 {
			t.Errorf("Parse(%q) = %v, %v", b, bs, err)
		}
	}
}

// Raw HTML is plain text: no tag is ever interpreted as formatting.
func TestParseHTMLNeverInterpreted(t *testing.T) {
	for _, md := range []string{
		"<b>x</b>", "a <b>x</b>", "<div><b>x</b></div>", "<i>x</i>", "<img src=x onerror=1>",
		"<a href=\"http://e\">x</a>",
	} {
		bs, err := Parse([]byte(md))
		if err != nil {
			t.Fatal(err)
		}
		var walk func([]Inline)
		walk = func(is []Inline) {
			for _, in := range is {
				switch in := in.(type) {
				case Text:
					if in.Bold || in.Italic || in.Code || in.Strike {
						t.Errorf("%q: HTML produced styled text %+v", md, in)
					}
				case Link:
					t.Errorf("%q: HTML produced a link %+v", md, in)
				case Image:
					t.Errorf("%q: HTML produced an image %+v", md, in)
				}
			}
		}
		for _, b := range bs {
			p, ok := b.(Paragraph)
			if !ok {
				t.Fatalf("%q: block %T", md, b)
			}
			walk(p.Inlines)
		}
		var sb strings.Builder
		for _, b := range bs {
			for _, in := range b.(Paragraph).Inlines {
				sb.WriteString(in.(Text).Text)
			}
		}
		if sb.String() != md {
			t.Errorf("%q: text = %q, want the source", md, sb.String())
		}
	}
}

func TestParseNesting(t *testing.T) {
	const depth = 50
	t.Run("quotes", func(t *testing.T) {
		md := strings.Repeat("> ", depth) + "deep"
		bs, err := Parse([]byte(md))
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		var cur []Block = bs
		for len(cur) == 1 {
			q, ok := cur[0].(Quote)
			if !ok {
				break
			}
			n++
			cur = q.Blocks
		}
		if n != depth {
			t.Errorf("quote depth = %d, want %d", n, depth)
		}
	})
	t.Run("lists", func(t *testing.T) {
		var sb strings.Builder
		for i := 0; i < depth; i++ {
			sb.WriteString(strings.Repeat("  ", i) + "- item\n")
		}
		bs, err := Parse([]byte(sb.String()))
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		cur := bs
		for len(cur) > 0 {
			l, ok := cur[len(cur)-1].(List)
			if !ok {
				break
			}
			n++
			cur = l.Items[0].Blocks
		}
		if n != depth {
			t.Errorf("list depth = %d, want %d", n, depth)
		}
	})
}

func TestParseLargeTable(t *testing.T) {
	const rows = 10000
	var sb strings.Builder
	sb.WriteString("| a | b | c |\n|---|---|---|\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&sb, "| %d | **x** | y |\n", i)
	}
	start := time.Now()
	bs, err := Parse([]byte(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("10k-row table took %s", el)
	}
	if len(bs) != 1 {
		t.Fatalf("blocks = %d", len(bs))
	}
	if tb := bs[0].(Table); len(tb.Rows) != rows || len(tb.Rows[rows-1]) != 3 {
		t.Errorf("rows = %d", len(tb.Rows))
	}
}

func TestParseNeverFetches(t *testing.T) {
	// Parse has no I/O path; this documents the contract: an image keeps only
	// alt and src, so whatever src points at is never touched.
	bs, err := Parse([]byte("![a](http://127.0.0.1:1/x.png)"))
	if err != nil {
		t.Fatal(err)
	}
	img := bs[0].(Paragraph).Inlines[0].(Image)
	if img.Alt != "a" || img.Src != "http://127.0.0.1:1/x.png" {
		t.Errorf("image = %+v", img)
	}
}
