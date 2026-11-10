package api

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Anonymous trust center visitors get a document body as HTML rendered here,
// not as markdown for the browser to render. The body is the organization's
// own text, but its links point into the application (/risks/RISK-12,
// /<slug>/documents/x, same-host URLs), which would leak internal identifiers
// and lead nowhere for a visitor. The rewrite therefore happens on the parsed
// tree, never on the source or on the HTML:
//
//   - raw HTML in the body is omitted (no html.WithUnsafe), and goldmark drops
//     dangerous URL schemes;
//   - an internal link or image is reduced to its text (a bare internal URL has
//     no text but the URL, so it is dropped), except a link to a
//     document that is published in the same trust center, which becomes
//     <a data-trust-document="<public id>">text</a> with no href so the page
//     can route it in any hosting mode;
//   - external http(s) links and images and mailto links are kept; every other
//     scheme is reduced to its text.

// trustLinkPolicy says which destinations are internal and which documents a
// link may point at.
type trustLinkPolicy struct {
	// Hosts that mean "this application": the deployment's apex host (and any
	// subdomain of it) and the organization's custom domain.
	ApexHost string
	OrgHost  string
	// Slug is the organization's slug, the optional first path segment of an
	// application URL.
	Slug string
	// Docs maps a lowercase document_id to the public id of its published,
	// servable trust center item.
	Docs map[string]string
}

const trustDocAttr = "data-trust-document"

// renderTrustMarkdown renders markdown to HTML with the link policy applied.
func renderTrustMarkdown(src []byte, p trustLinkPolicy) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList, extension.Linkify),
		goldmark.WithParserOptions(parser.WithASTTransformers(
			util.Prioritized(&trustLinkTransformer{policy: p}, 100),
		)),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			renderer.WithNodeRenderers(util.Prioritized(trustLinkRenderer{}, 100)),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type trustLinkTransformer struct{ policy trustLinkPolicy }

func (t *trustLinkTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var nodes []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n.(type) {
			case *ast.Link, *ast.Image, *ast.AutoLink:
				nodes = append(nodes, n)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, n := range nodes {
		if n.Parent() == nil {
			continue
		}
		switch n := n.(type) {
		case *ast.Link:
			switch kind, docID := t.policy.classify(string(n.Destination)); kind {
			case destKeep:
			case destDocument:
				if pub, ok := t.policy.Docs[docID]; ok {
					n.Destination = nil
					n.SetAttributeString(trustDocAttr, []byte(pub))
				} else {
					unwrapNode(n)
				}
			default:
				unwrapNode(n)
			}
		case *ast.Image:
			if kind, _ := t.policy.classify(string(n.Destination)); kind != destKeep || isMailto(string(n.Destination)) {
				unwrapNode(n)
			}
		case *ast.AutoLink:
			kind, _ := t.policy.classify(string(n.URL(src)))
			if kind == destKeep {
				continue
			}
			// The label of an autolink is the URL itself, so there is no text
			// to keep: the internal address must not appear either way.
			n.Parent().RemoveChild(n.Parent(), n)
		}
	}
}

// unwrapNode replaces n with its children, so a link keeps its text and an
// image keeps its alt text.
func unwrapNode(n ast.Node) {
	p := n.Parent()
	for c := n.FirstChild(); c != nil; {
		next := c.NextSibling()
		n.RemoveChild(n, c)
		p.InsertBefore(p, n, c)
		c = next
	}
	p.RemoveChild(p, n)
}

type destKind int

const (
	destKeep     destKind = iota // fragment, mailto, or external http(s)
	destText                     // reduce to text
	destDocument                 // internal link that may point at a published document
)

func isMailto(dest string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(dest)), "mailto:")
}

// classify decides what happens to a link destination. For destDocument the
// second result is the lowercase document id the destination names.
func (p trustLinkPolicy) classify(dest string) (destKind, string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return destText, ""
	}
	if strings.HasPrefix(dest, "#") {
		return destKeep, ""
	}
	u, err := url.Parse(dest)
	if err != nil {
		return destText, ""
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "mailto":
		return destKeep, ""
	case "http", "https":
	case "":
		// Scheme-relative //host/path names a host; anything else is a path
		// into the application.
		if !strings.HasPrefix(dest, "//") {
			return p.internalPath(u.Path)
		}
	default:
		return destText, ""
	}
	if !p.internalHost(u.Hostname()) {
		return destKeep, ""
	}
	return p.internalPath(u.Path)
}

// internalPath classifies a path that is known to be inside the application.
func (p trustLinkPolicy) internalPath(path string) (destKind, string) {
	rest := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(path, "/") && p.Slug != "" {
		if first, tail, ok := strings.Cut(rest, "/"); ok && strings.EqualFold(first, p.Slug) {
			rest = tail
		}
	}
	if !strings.HasPrefix(path, "/") {
		return destText, ""
	}
	if id, ok := strings.CutPrefix(rest, "documents/"); ok {
		id = strings.TrimSuffix(id, "/")
		if id != "" && !strings.Contains(id, "/") {
			return destDocument, strings.ToLower(id)
		}
	}
	return destText, ""
}

func (p trustLinkPolicy) internalHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return false
	}
	if a := strings.ToLower(p.ApexHost); a != "" && (host == a || strings.HasSuffix(host, "."+a)) {
		return true
	}
	return p.OrgHost != "" && host == strings.ToLower(p.OrgHost)
}

// trustLinkRenderer renders ast.Link like goldmark's own renderer, plus the
// hrefless data-trust-document form, which the stock renderer cannot emit
// because it always writes an href.
type trustLinkRenderer struct{}

func (trustLinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindLink, renderTrustLink)
}

func renderTrustLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if !entering {
		_, _ = w.WriteString("</a>")
		return ast.WalkContinue, nil
	}
	if pub, ok := n.AttributeString(trustDocAttr); ok {
		_, _ = w.WriteString(`<a ` + trustDocAttr + `="`)
		if b, isBytes := pub.([]byte); isBytes {
			_, _ = w.Write(util.EscapeHTML(b))
		}
		_, _ = w.WriteString(`">`)
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<a href="`)
	dest := util.URLEscape(n.Destination, true)
	if !html.IsDangerousURL(dest) {
		_, _ = w.Write(util.EscapeHTML(dest))
	}
	_ = w.WriteByte('"')
	if n.Title != nil {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML(n.Title))
		_ = w.WriteByte('"')
	}
	_ = w.WriteByte('>')
	return ast.WalkContinue, nil
}
