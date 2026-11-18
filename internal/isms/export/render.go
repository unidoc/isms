package export

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrUnlicensed means the vendor library refused to render because it is not
	// licensed (no key, or the licence was lost at runtime, for example when
	// metered credits ran out).
	ErrUnlicensed = errors.New("export: format not licensed")
	// ErrUnsupportedFormat means the format is neither "pdf" nor "docx".
	ErrUnsupportedFormat = errors.New("export: unsupported format")
)

// Report says what a render could not reproduce faithfully. The zero value is
// a faithful render.
type Report struct {
	// Unprintable is the number of distinct characters the PDF fonts cannot
	// print; every occurrence of one is printed as "?". The standard PDF fonts
	// (no ISMS_EXPORT_FONT_DIR) cover Western European text only. Always 0 for
	// DOCX, whose text is drawn by the reader's own fonts.
	Unprintable int
}

// Render renders d in the given format ("pdf" or "docx").
func Render(format string, d *ExportDoc) ([]byte, error) {
	b, _, err := RenderReport(format, d)
	return b, err
}

// RenderReport is Render that also reports what the render could not
// reproduce faithfully.
func RenderReport(format string, d *ExportDoc) ([]byte, Report, error) {
	switch format {
	case "pdf":
		return renderPDF(d)
	case "docx":
		b, err := renderDOCX(d)
		return b, Report{}, err
	}
	return nil, Report{}, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
}

// MIMEType is the Content-Type of an export, or "" for an unknown format.
func MIMEType(format string) string {
	switch strings.ToLower(format) {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return ""
}

// Ext is the file extension of an export without the dot, or "" for an unknown
// format.
func Ext(format string) string {
	switch strings.ToLower(format) {
	case "pdf":
		return "pdf"
	case "docx":
		return "docx"
	}
	return ""
}
