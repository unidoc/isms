// Package export turns a document into a renderer-neutral model (ExportDoc)
// that the PDF and DOCX renderers walk. This file and markdown.go import no
// vendor library: the model and the markdown parsing are tested without a
// licence.
package export

import (
	"strings"
	"time"

	"isms.sh/internal/isms/model"
)

// OrgInfo is the organisation branding printed on an export.
type OrgInfo struct {
	Name       string
	BrandColor string // "#rrggbb" or ""
	LogoPNG    []byte // nil when the organisation has no raster logo
}

// ExportDoc is everything a renderer needs; renderers read no other source.
type ExportDoc struct {
	Meta         model.DocumentFrontmatter
	Org          OrgInfo
	VersionLabel string
	GeneratedAt  time.Time // UTC
	Blocks       []Block
}

// Draft reports whether the export must be stamped as a draft: anything that is
// not approved (draft, in_review, retired, unknown), compared case-insensitively.
func (d ExportDoc) Draft() bool {
	return !strings.EqualFold(d.Meta.Status, "approved")
}

// RevisionRows returns the changelog in file order, as a copy.
func (d ExportDoc) RevisionRows() []model.DocumentChange {
	out := make([]model.DocumentChange, len(d.Meta.Changelog))
	copy(out, d.Meta.Changelog)
	return out
}

// Block is a block-level element: Heading, Paragraph, List, CodeBlock, Table,
// Quote or Rule.
type Block interface{ isBlock() }

// Inline is an inline element: Text, Link, LineBreak or Image.
type Inline interface{ isInline() }

type (
	Heading struct {
		Level   int // 1-6
		Inlines []Inline
	}
	Paragraph struct{ Inlines []Inline }
	List      struct {
		Ordered bool
		Start   int // first number of an ordered list
		Items   []ListItem
	}
	// CodeBlock is preformatted text. Mermaid and any other fence language stay
	// text: there are no diagrams or highlighting.
	CodeBlock struct {
		Lang string
		Text string // no trailing newline
	}
	Table struct {
		Header []Cell
		Rows   [][]Cell // every row has len(Header) cells
		Align  []Align  // one per column
	}
	Quote struct{ Blocks []Block }
	Rule  struct{}
)

func (Heading) isBlock()   {}
func (Paragraph) isBlock() {}
func (List) isBlock()      {}
func (CodeBlock) isBlock() {}
func (Table) isBlock()     {}
func (Quote) isBlock()     {}
func (Rule) isBlock()      {}

// TaskState is the checkbox of a task list item.
type TaskState int

const (
	TaskNone TaskState = iota
	TaskChecked
	TaskUnchecked
)

// ListItem is one list entry; Blocks may contain nested lists and quotes.
type ListItem struct {
	Task   TaskState
	Blocks []Block
}

// Cell is one table cell.
type Cell struct{ Inlines []Inline }

// Align is a table column alignment.
type Align int

const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

type (
	// Text is a run of text with its styles. Adjacent runs with equal styles
	// are merged.
	Text struct {
		Text                       string
		Bold, Italic, Strike, Code bool
	}
	// Link keeps its destination exactly as written, relative or internal
	// hrefs included; rewriting is not the parser's job.
	Link struct {
		Inlines []Inline
		Href    string
	}
	// LineBreak is a hard (two spaces or backslash) or soft (newline) break.
	LineBreak struct{ Hard bool }
	// Image carries only alt and src. Nothing in this package or the renderers
	// ever fetches src (no network, no file read): a server that fetched
	// author-controlled URLs would be an SSRF hole. Renderers print the alt.
	Image struct{ Alt, Src string }
)

func (Text) isInline()      {}
func (Link) isInline()      {}
func (LineBreak) isInline() {}
func (Image) isInline()     {}
