package api

import (
	"strings"
	"testing"
)

func trustTestPolicy() trustLinkPolicy {
	return trustLinkPolicy{
		ApexHost: "isms.test",
		OrgHost:  "trust.acme.example",
		Slug:     "acme",
		Docs:     map[string]string{"pub-doc": "11111111-2222-3333-4444-555555555555"},
	}
}

func TestRenderTrustMarkdownLinks(t *testing.T) {
	const pubID = "11111111-2222-3333-4444-555555555555"
	cases := []struct {
		name    string
		md      string
		want    []string // substrings the HTML must contain
		notWant []string // substrings it must not contain
	}{
		{"absolute app path", "see [r](/risks/RISK-12) now", []string{"see r now"}, []string{"RISK-12", "<a"}},
		{"slug-prefixed app path", "[r](/acme/risks/RISK-12)", []string{">r<"}, []string{"RISK-12", "<a"}},
		{"relative path", "[r](risks/RISK-1)", []string{">r<"}, []string{"RISK-1", "<a"}},
		{"dot relative", "[x](./x) [y](../y)", []string{"x y"}, []string{"<a", "./x", "../y"}},
		{"apex url", "[r](https://isms.test/acme/risks/RISK-1)", []string{">r<"}, []string{"RISK-1", "<a"}},
		{"subdomain url", "[r](https://acme.isms.test/risks/1)", []string{">r<"}, []string{"/risks/1", "<a"}},
		{"org custom domain", "[d](https://trust.acme.example/documents/x)", []string{">d<"}, []string{"/documents/", "<a"}},
		{"linkified bare apex", "go to https://isms.test/acme/risks/RISK-1 please", []string{"go to", "please"}, []string{"<a", "href", "isms.test", "RISK-1"}},
		{"linkified www apex-like external", "see https://example.com/x here", []string{`<a href="https://example.com/x">`}, nil},
		{"reference style", "[a][r]\n\n[r]: /risks/RISK-1\n", []string{">a<"}, []string{"RISK-1", "<a"}},
		{"autolink external", "<https://example.com>", []string{`<a href="https://example.com">https://example.com</a>`}, nil},
		{"autolink internal", "<https://isms.test/risks/RISK-9>", nil, []string{"<a", "href", "RISK-9", "isms.test"}},
		{"external link", "[e](https://example.com/x)", []string{`<a href="https://example.com/x">e</a>`}, nil},
		{"external look-alike host", "[e](https://evilisms.test/x) [f](https://isms.test.evil.com/x)", []string{`href="https://evilisms.test/x"`, `href="https://isms.test.evil.com/x"`}, nil},
		{"scheme relative external", "[e](//example.com/x)", []string{`href="//example.com/x"`}, nil},
		{"scheme relative internal", "[e](//isms.test/risks/1)", []string{">e<"}, []string{"<a", "/risks/1"}},
		{"javascript", "[j](javascript:alert(1))", []string{">j<"}, []string{"javascript", "<a"}},
		{"data scheme", "[j](data:text/html;base64,AAAA)", nil, []string{"data:", "<a"}},
		{"mailto", "[m](mailto:sec@acme.test)", []string{`<a href="mailto:sec@acme.test">m</a>`}, nil},
		{"raw html anchor", `<a href="/risks/1">x</a>`, nil, []string{"/risks/1", "<a "}},
		{"script", "hi\n\n<script>alert(1)</script>\n\nbye", []string{"hi", "bye"}, []string{"<script", "alert(1)"}},
		{"internal image", "![alt text](/x.png)", []string{"alt text"}, []string{"<img", "/x.png"}},
		{"relative image", "![alt](x.png)", []string{"alt"}, []string{"<img", "x.png"}},
		{"external image", "![a](https://example.com/x.png)", []string{`<img src="https://example.com/x.png" alt="a">`}, nil},
		{"image inside internal link", "[![pic](/p.png)](/risks/1)", []string{"pic"}, []string{"<a", "<img", "/p.png", "/risks/1"}},
		{"fragment", "[s](#section)", []string{`<a href="#section">s</a>`}, nil},
		{"published document", "[p](/documents/pub-doc)", []string{`<a data-trust-document="` + pubID + `">p</a>`}, []string{"href", "/documents/", "pub-doc"}},
		{"published document with slug and case", "[p](/acme/documents/PUB-DOC)", []string{`<a data-trust-document="` + pubID + `">p</a>`}, []string{"href", "pub-doc", "PUB-DOC"}},
		{"published document by host", "[p](https://acme.isms.test/documents/pub-doc)", []string{`data-trust-document="` + pubID + `"`}, []string{"href", "isms.test"}},
		{"unpublished document", "[u](/documents/secret-doc)", []string{">u<"}, []string{"<a", "secret-doc", "/documents/"}},
		{"formatted text in link survives", "[**bold** text](/risks/1)", []string{"<strong>bold</strong> text"}, []string{"<a", "/risks/1"}},
		{"hard wrap", "a\nb", []string{"<br"}, nil},
		{"table", "| a | b |\n|---|---|\n| 1 | 2 |\n", []string{"<table>", "<td>1</td>"}, nil},
		{"strikethrough", "~~x~~", []string{"<del>x</del>"}, nil},
		{"task list", "- [x] done\n", []string{`type="checkbox"`}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderTrustMarkdown([]byte(tc.md), trustTestPolicy())
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("output contains %q\n%s", w, got)
				}
			}
		})
	}
}

// A link target must never reach the output as an attribute value or text
// except for the kept external ones, whatever the markup around it.
func TestRenderTrustMarkdownNoInternalTargets(t *testing.T) {
	md := strings.Join([]string{
		"[a](/risks/RISK-12)", "[b](/acme/assets/A-1)", "<https://isms.test/suppliers/3>",
		"![i](https://acme.isms.test/branding/logo.svg)", "[c][ref]", "https://trust.acme.example/tasks/9",
		"", "[ref]: /incidents/INC-4",
	}, "\n")
	got, err := renderTrustMarkdown([]byte(md), trustTestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"RISK-12", "A-1", "suppliers", "branding", "INC-4", "tasks/9", "href", "<img"} {
		if strings.Contains(got, bad) {
			t.Errorf("output contains internal target %q:\n%s", bad, got)
		}
	}
}
