package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestFontsUnsetUsesStandard(t *testing.T) {
	fd := loadFontData("")
	if !fd.std || !strings.Contains(fd.note, "ISMS_EXPORT_FONT_DIR is not set") {
		t.Fatalf("%+v", fd)
	}
	f := fd.build()
	if !f.regular.std || f.embedded {
		t.Errorf("fonts = %+v", f)
	}
}

func TestFontsDirWithoutRegularUsesStandardWithReason(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sans-bold.ttf"), goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	fd := loadFontData(dir)
	if !fd.std || !strings.Contains(fd.note, "sans-regular.ttf") || !strings.Contains(fd.note, dir) {
		t.Fatalf("note = %q std = %v", fd.note, fd.std)
	}
}

func TestFontsFullDirLoads(t *testing.T) {
	fd := loadFontData(goFontDir(t))
	if fd.std {
		t.Fatalf("fell back: %s", fd.note)
	}
	for _, n := range fontFileNames {
		if !strings.Contains(fd.note, n) {
			t.Errorf("note lacks %s: %s", n, fd.note)
		}
	}
	f := fd.build()
	if !f.embedded || f.regular.std || f.mono.font == nil {
		t.Errorf("fonts = %+v", f)
	}
	// A Polish sentence is encodable with a real TrueType font.
	if got := f.regular.clean(polish); got != polish {
		t.Errorf("clean(polish) = %q", got)
	}
}

func TestFontsOnlyRegularFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sans-regular.ttf"), goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	fd := loadFontData(dir)
	if fd.std {
		t.Fatalf("fell back: %s", fd.note)
	}
	for i := 1; i < len(fontFileNames); i++ {
		if fd.files[i] != nil {
			t.Errorf("%s should be absent", fontFileNames[i])
		}
	}
	f := fd.build()
	if f.bold.font == nil || f.mono.font == nil || f.italic.font == nil {
		t.Errorf("variants must fall back to regular: %+v", f)
	}
	if strings.Contains(fd.note, "unusable") {
		t.Errorf("missing optional files are not a problem: %s", fd.note)
	}
}

func TestFontsCorruptRegularUsesStandard(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sans-regular.ttf"), []byte("this is not a font"), 0o644); err != nil {
		t.Fatal(err)
	}
	fd := loadFontData(dir) // must not panic
	if !fd.std || !strings.Contains(fd.note, "unusable") {
		t.Fatalf("note = %q std = %v", fd.note, fd.std)
	}
	if f := fd.build(); !f.regular.std {
		t.Error("expected the standard fonts")
	}
}

func TestFontsCorruptOptionalVariantFallsBack(t *testing.T) {
	dir := goFontDir(t)
	if err := os.WriteFile(filepath.Join(dir, "sans-italic.ttf"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	fd := loadFontData(dir)
	if fd.std || fd.files[2] != nil || !strings.Contains(fd.note, "sans-italic.ttf") {
		t.Fatalf("note = %q", fd.note)
	}
}

// The standard fonts must never fail an export on text outside their encoding.
func TestStandardFontsCleanReplacesUnsupported(t *testing.T) {
	f := standardFonts()
	got := f.regular.clean(polish + " " + japanese + " café €")
	if strings.ContainsAny(got, "żłćęśąźń情報") {
		t.Errorf("unsupported runes survived: %q", got)
	}
	if !strings.Contains(got, "café") || !strings.Contains(got, "?") {
		t.Errorf("clean = %q", got)
	}
	if f.regular.clean("a\tb\x00c") != "a    bc" {
		t.Errorf("tabs and controls: %q", f.regular.clean("a\tb\x00c"))
	}
}

func TestFontsForCachesAndReportsStatus(t *testing.T) {
	dir := goFontDir(t)
	a := fontsFor(dir)
	b := fontsFor(dir)
	if a.regular.font == b.regular.font {
		t.Error("PdfFont objects must be built per render, not shared")
	}
	if !strings.Contains(FontStatus(), dir) {
		t.Errorf("FontStatus = %q", FontStatus())
	}
	fontsFor("")
	if !strings.Contains(FontStatus(), "not set") {
		t.Errorf("FontStatus = %q", FontStatus())
	}
}
