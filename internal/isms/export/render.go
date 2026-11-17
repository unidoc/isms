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

	// errRendererPending is what the placeholder renderers return until the
	// real ones are written.
	errRendererPending = errors.New("export: renderer not implemented yet")
)

// Render renders d in the given format ("pdf" or "docx").
func Render(format string, d *ExportDoc) ([]byte, error) {
	switch format {
	case "pdf":
		return renderPDF(d)
	case "docx":
		return renderDOCX(d)
	}
	return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
}

// renderPDF and renderDOCX are placeholders: the UniPDF and UniOffice renderers
// replace them. They return errRendererPending so nothing can mistake an empty
// result for a document.
func renderPDF(*ExportDoc) ([]byte, error)  { return nil, errRendererPending }
func renderDOCX(*ExportDoc) ([]byte, error) { return nil, errRendererPending }

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
