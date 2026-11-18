package export

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/unidoc/unipdf/v5/model"
)

// Fonts for the PDF renderer come from disk, never from the binary.
//
// ISMS_EXPORT_FONT_DIR names a directory of TrueType files:
//
//	sans-regular.ttf   required to use the directory at all
//	sans-bold.ttf, sans-italic.ttf, sans-bolditalic.ttf, mono-regular.ttf
//	                   optional; a missing one falls back to sans-regular
//
// Without a usable directory the PDF standard 14 fonts are used (Helvetica
// family, Courier for code). Those cover Western European text only; any other
// character prints as "?". Fonts never fail an export.
const envExportFontDir = "ISMS_EXPORT_FONT_DIR"

// face is one font and what it can print.
type face struct {
	font *model.PdfFont
	std  bool         // a standard 14 font (one-byte encoding)
	miss *unprintable // this render's record of characters printed as "?"
}

// unprintable collects, for one render, the distinct characters its fonts
// could not print. Distinct, because the running header repeats the title on
// every page: a count of occurrences would grow with the page count.
type unprintable struct{ runes map[rune]struct{} }

func (u *unprintable) add(r rune) {
	if u == nil {
		return
	}
	if u.runes == nil {
		u.runes = map[rune]struct{}{}
	}
	u.runes[r] = struct{}{}
}

func (u *unprintable) count() int {
	if u == nil {
		return 0
	}
	return len(u.runes)
}

// has reports whether the font can encode r.
func (f face) has(r rune) bool {
	if f.font == nil {
		return false
	}
	_, ok := f.font.Encoder().RuneToCharcode(r)
	return ok
}

// clean makes s printable with this font: tabs become spaces, other control
// characters are dropped, and any rune the font cannot encode becomes "?".
// This is what keeps a document with, say, Japanese text exportable with the
// standard fonts instead of failing.
func (f face) clean(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			sb.WriteString("    ")
		case r == '\n':
			sb.WriteRune(r)
		case r == '\u00a0':
			// The standard fonts' encoder accepts it but the layout rejects it.
			if !f.std && f.has(r) {
				sb.WriteRune(r)
			} else {
				sb.WriteRune(' ')
			}
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			// control characters are dropped
		case f.has(r):
			sb.WriteRune(r)
		default:
			f.miss.add(r)
			sb.WriteRune('?')
		}
	}
	return sb.String()
}

// pdfFonts is the set of faces one render uses.
type pdfFonts struct {
	regular, bold, italic, boldItalic, mono face
	embedded                                bool // TrueType fonts (subsetting applies)
	miss                                    *unprintable
}

var fontFileNames = [...]string{"sans-regular.ttf", "sans-bold.ttf", "sans-italic.ttf", "sans-bolditalic.ttf", "mono-regular.ttf"}

// fontData is a validated set of font files: read once, parsed once to check
// they load, and kept as bytes. A PdfFont is built from the bytes per render
// because subsetting records the glyphs a document used in the font object, so
// one object shared by concurrent renders would mix their glyph sets.
type fontData struct {
	dir   string
	files [len(fontFileNames)][]byte // nil entries fall back to sans-regular
	std   bool                       // use the standard fonts
	note  string                     // what was chosen and why
}

func loadFontFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if _, err := model.NewCompositePdfFont(bytes.NewReader(b)); err != nil {
		return nil, fmt.Errorf("not a usable TrueType font: %w", err)
	}
	return b, nil
}

// loadFontData reads dir. An empty dir, a missing or unusable sans-regular.ttf
// gives the standard fonts, with the reason in note.
func loadFontData(dir string) *fontData {
	fd := &fontData{dir: dir}
	if dir == "" {
		fd.std = true
		fd.note = envExportFontDir + " is not set: using the standard PDF fonts (Western European characters only)"
		return fd
	}
	reg, err := loadFontFile(filepath.Join(dir, fontFileNames[0]))
	if err != nil {
		fd.std = true
		fd.note = fmt.Sprintf("%s=%s: %s unusable (%v): using the standard PDF fonts", envExportFontDir, dir, fontFileNames[0], err)
		return fd
	}
	fd.files[0] = reg
	used := []string{fontFileNames[0]}
	var skipped []string
	for i := 1; i < len(fontFileNames); i++ {
		b, err := loadFontFile(filepath.Join(dir, fontFileNames[i]))
		switch {
		case err == nil:
			fd.files[i] = b
			used = append(used, fontFileNames[i])
		case os.IsNotExist(unwrapPathErr(err)):
			// optional: falls back to the regular face
		default:
			skipped = append(skipped, fmt.Sprintf("%s (%v)", fontFileNames[i], err))
		}
	}
	fd.note = fmt.Sprintf("%s=%s: using %s", envExportFontDir, dir, strings.Join(used, ", "))
	if len(skipped) > 0 {
		fd.note += "; unusable, regular face used instead: " + strings.Join(skipped, "; ")
	}
	return fd
}

func unwrapPathErr(err error) error {
	if pe, ok := err.(*os.PathError); ok {
		return pe
	}
	return err
}

// build creates this render's PdfFont objects. A failure falls back to the
// standard fonts rather than failing the export.
func (fd *fontData) build() *pdfFonts {
	if fd.std {
		return standardFonts()
	}
	load := func(i int) (*model.PdfFont, error) {
		b := fd.files[i]
		if b == nil {
			b = fd.files[0]
		}
		return model.NewCompositePdfFont(bytes.NewReader(b))
	}
	var fs [len(fontFileNames)]*model.PdfFont
	for i := range fs {
		f, err := load(i)
		if err != nil {
			log.Printf("export: font %s failed to load, using the standard PDF fonts: %v", fontFileNames[i], err)
			return standardFonts()
		}
		fs[i] = f
	}
	miss := &unprintable{}
	return &pdfFonts{
		regular: face{font: fs[0], miss: miss}, bold: face{font: fs[1], miss: miss}, italic: face{font: fs[2], miss: miss},
		boldItalic: face{font: fs[3], miss: miss}, mono: face{font: fs[4], miss: miss}, embedded: true, miss: miss,
	}
}

func standardFonts() *pdfFonts {
	miss := &unprintable{}
	mk := func(n model.StdFontName) face {
		return face{font: model.NewStandard14FontMustCompile(n), std: true, miss: miss}
	}
	return &pdfFonts{
		regular: mk(model.HelveticaName), bold: mk(model.HelveticaBoldName),
		italic: mk(model.HelveticaObliqueName), boldItalic: mk(model.HelveticaBoldObliqueName),
		mono: mk(model.CourierName), miss: miss,
	}
}

var (
	fontCache  sync.Map // dir -> *fontData
	fontStatus atomic.Value
)

// fontsForRender returns this render's fonts, from ISMS_EXPORT_FONT_DIR. The
// directory is read and validated once per path, and the choice is logged once.
func fontsForRender() *pdfFonts { return fontsFor(os.Getenv(envExportFontDir)) }

func fontsFor(dir string) *pdfFonts {
	v, loaded := fontCache.Load(dir)
	if !loaded {
		fd := loadFontData(dir)
		v, loaded = fontCache.LoadOrStore(dir, fd)
		if !loaded {
			log.Printf("export: %s", fd.note)
		}
	}
	fd := v.(*fontData)
	fontStatus.Store(fd.note)
	return fd.build()
}

// FontStatus describes which fonts the PDF export used last, and why, for logs
// and tests. It is empty before the first PDF export.
func FontStatus() string {
	s, _ := fontStatus.Load().(string)
	return s
}
