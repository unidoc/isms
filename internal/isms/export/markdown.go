package export

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// The same extensions as the trust center renderer, so a document parses the
// way it renders there.
var md = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList, extension.Linkify),
)

// Parse turns a markdown body into blocks. An empty body gives no blocks and
// no error.
//
// Raw HTML (blocks and inline tags) is never interpreted: it becomes plain
// text holding its source, so "<b>x</b>" prints literally and a tag can never
// add formatting, a link or an image. Images keep only alt and src; Parse does
// no I/O.
func Parse(body []byte) ([]Block, error) {
	src := bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	src = bytes.ReplaceAll(src, []byte("\r"), []byte("\n"))
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, nil
	}
	root := md.Parser().Parse(text.NewReader(src))
	return (&walker{src: src}).blocks(root), nil
}

type walker struct{ src []byte }

type style struct{ bold, italic, strike bool }

func (w *walker) blocks(parent ast.Node) []Block {
	var out []Block
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if b := w.block(n); b != nil {
			out = append(out, b)
		}
	}
	return out
}

func (w *walker) block(n ast.Node) Block {
	switch n := n.(type) {
	case *ast.Heading:
		return Heading{Level: n.Level, Inlines: w.inlines(n, style{})}
	case *ast.Paragraph:
		return Paragraph{Inlines: w.inlines(n, style{})}
	case *ast.TextBlock: // a paragraph inside a tight list item
		return Paragraph{Inlines: w.inlines(n, style{})}
	case *ast.ThematicBreak:
		return Rule{}
	case *ast.Blockquote:
		return Quote{Blocks: w.blocks(n)}
	case *ast.FencedCodeBlock:
		return CodeBlock{Lang: string(n.Language(w.src)), Text: w.lines(n)}
	case *ast.CodeBlock:
		return CodeBlock{Text: w.lines(n)}
	case *ast.HTMLBlock:
		var sb strings.Builder
		sb.WriteString(w.lines(n))
		if n.ClosureLine.Len() > 0 {
			sb.WriteString("\n")
			sb.Write(n.ClosureLine.Value(w.src))
		}
		t := strings.TrimRight(sb.String(), "\n")
		return Paragraph{Inlines: []Inline{Text{Text: t}}}
	case *ast.List:
		l := List{Ordered: n.IsOrdered(), Start: n.Start}
		if !l.Ordered {
			l.Start = 0
		}
		for it := n.FirstChild(); it != nil; it = it.NextSibling() {
			l.Items = append(l.Items, w.item(it))
		}
		return l
	case *extast.Table:
		return w.table(n)
	}
	return nil
}

func (w *walker) item(n ast.Node) ListItem {
	it := ListItem{Blocks: w.blocks(n)}
	// The task checkbox is the first inline of the item's first paragraph.
	if fc := n.FirstChild(); fc != nil {
		if cb, ok := fc.FirstChild().(*extast.TaskCheckBox); ok {
			it.Task = TaskUnchecked
			if cb.IsChecked {
				it.Task = TaskChecked
			}
		}
	}
	return it
}

func (w *walker) lines(n ast.Node) string {
	var sb strings.Builder
	ls := n.Lines()
	for i := 0; i < ls.Len(); i++ {
		s := ls.At(i)
		sb.Write(s.Value(w.src))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (w *walker) table(t *extast.Table) Table {
	out := Table{}
	for _, a := range t.Alignments {
		switch a {
		case extast.AlignLeft:
			out.Align = append(out.Align, AlignLeft)
		case extast.AlignCenter:
			out.Align = append(out.Align, AlignCenter)
		case extast.AlignRight:
			out.Align = append(out.Align, AlignRight)
		default:
			out.Align = append(out.Align, AlignNone)
		}
	}
	cols := len(out.Align)
	pad := func(cs []Cell) []Cell {
		for len(cs) < cols {
			cs = append(cs, Cell{})
		}
		return cs
	}
	cells := func(row ast.Node) []Cell {
		var cs []Cell
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			cs = append(cs, Cell{Inlines: w.inlines(c, style{})})
		}
		return pad(cs)
	}
	for c := t.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.(type) {
		case *extast.TableHeader:
			out.Header = cells(c)
		case *extast.TableRow:
			out.Rows = append(out.Rows, cells(c))
		}
	}
	return out
}

// inlines converts the inline children of n, carrying the active style down.
func (w *walker) inlines(n ast.Node, st style) []Inline {
	var out []Inline
	emit := func(in Inline) {
		if t, ok := in.(Text); ok && len(out) > 0 {
			if prev, ok := out[len(out)-1].(Text); ok && sameStyle(prev, t) {
				prev.Text += t.Text
				out[len(out)-1] = prev
				return
			}
		}
		out = append(out, in)
	}
	text := func(s string, code bool) {
		if s != "" {
			emit(Text{Text: s, Bold: st.bold, Italic: st.italic, Strike: st.strike, Code: code})
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			text(unescape(c.Segment.Value(w.src)), false)
			if c.HardLineBreak() {
				emit(LineBreak{Hard: true})
			} else if c.SoftLineBreak() {
				emit(LineBreak{})
			}
		case *ast.String:
			text(string(c.Value), false)
		case *ast.CodeSpan:
			var sb strings.Builder
			for t := c.FirstChild(); t != nil; t = t.NextSibling() {
				if t, ok := t.(*ast.Text); ok {
					sb.Write(t.Segment.Value(w.src))
				}
			}
			text(strings.ReplaceAll(sb.String(), "\n", " "), true)
		case *ast.Emphasis:
			s := st
			if c.Level >= 2 {
				s.bold = true
			} else {
				s.italic = true
			}
			for _, in := range w.inlines(c, s) {
				emit(in)
			}
		case *extast.Strikethrough:
			s := st
			s.strike = true
			for _, in := range w.inlines(c, s) {
				emit(in)
			}
		case *ast.Link:
			emit(Link{Href: string(c.Destination), Inlines: w.inlines(c, st)})
		case *ast.AutoLink:
			label := string(c.Label(w.src))
			href := string(c.URL(w.src))
			emit(Link{Href: href, Inlines: []Inline{Text{Text: label, Bold: st.bold, Italic: st.italic, Strike: st.strike}}})
		case *ast.Image:
			emit(Image{Alt: w.plain(c), Src: string(c.Destination)})
		case *ast.RawHTML:
			// Never interpreted: the tag's source is plain text.
			var sb strings.Builder
			for i := 0; i < c.Segments.Len(); i++ {
				s := c.Segments.At(i)
				sb.Write(s.Value(w.src))
			}
			text(sb.String(), false)
		case *extast.TaskCheckBox:
			// Recorded on the list item, not printed here.
		default:
			// Unknown inline: keep its text so content is never lost.
			text(w.plain(c), false)
		}
	}
	return out
}

func sameStyle(a, b Text) bool {
	return a.Bold == b.Bold && a.Italic == b.Italic && a.Strike == b.Strike && a.Code == b.Code
}

// plain returns the text content of a node, without formatting.
func (w *walker) plain(n ast.Node) string {
	var sb strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			sb.WriteString(unescape(c.Segment.Value(w.src)))
			if c.SoftLineBreak() || c.HardLineBreak() {
				sb.WriteString(" ")
			}
		case *ast.String:
			sb.Write(c.Value)
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}

// unescape resolves backslash escapes and character references the way the
// HTML renderer does, without HTML-escaping the result.
func unescape(b []byte) string {
	b = util.UnescapePunctuations(b)
	b = util.ResolveNumericReferences(b)
	b = util.ResolveEntityNames(b)
	return string(b)
}
