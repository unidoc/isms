package export

import (
	"errors"
	"testing"
)

func TestRenderDispatch(t *testing.T) {
	if _, err := Render("docx", &ExportDoc{}); !errors.Is(err, errRendererPending) {
		t.Errorf("Render(docx) = %v, want the pending placeholder", err)
	}
	// pdf is dispatched to the real renderer: with no licence that is
	// ErrUnlicensed, with one it succeeds. Never "unsupported".
	if _, err := Render("pdf", &ExportDoc{}); errors.Is(err, ErrUnsupportedFormat) || errors.Is(err, errRendererPending) {
		t.Errorf("Render(pdf) = %v", err)
	}
	for _, f := range []string{"", "xls", "PDF", "pdf ", "odt"} {
		if _, err := Render(f, &ExportDoc{}); !errors.Is(err, ErrUnsupportedFormat) {
			t.Errorf("Render(%q) = %v, want ErrUnsupportedFormat", f, err)
		}
	}
}

func TestMIMEAndExt(t *testing.T) {
	cases := []struct{ f, mime, ext string }{
		{"pdf", "application/pdf", "pdf"},
		{"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"},
		{"xls", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := MIMEType(c.f); got != c.mime {
			t.Errorf("MIMEType(%q) = %q", c.f, got)
		}
		if got := Ext(c.f); got != c.ext {
			t.Errorf("Ext(%q) = %q", c.f, got)
		}
	}
}
