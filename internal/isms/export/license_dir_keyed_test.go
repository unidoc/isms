package export

import (
	"os"
	"testing"
)

// The server keeps the vendor libraries' metered licence state under its data
// directory by setting UNIDOC_LICENSE_DIR in-process at start-up, after both
// libraries have been imported and initialised (license.Start). That works only
// because UniPDF and UniOffice read the variable each time they load or save
// their state, instead of once at import. This test pins that: it moves the
// variable between renders and expects each render's state in the directory
// set at that moment. If a library upgrade starts caching the directory, the
// second render writes into the first directory (or the home directory) and
// this fails; license.Start then needs another way to place the state.
//
// It renders four documents (four credits). The keyless tripwire in
// internal/isms/license (TestVendorVersionsAudited) points here when either
// library's version changes.
func TestKeyedLicenseDirReadPerCall(t *testing.T) {
	needKey(t)
	for _, format := range []string{"pdf", "docx"} {
		t.Run(format, func(t *testing.T) {
			first, second := t.TempDir(), t.TempDir()

			t.Setenv("UNIDOC_LICENSE_DIR", first)
			if _, err := Render(format, richDoc("approved")); err != nil {
				t.Fatalf("first render: %v", err)
			}
			if n := countFiles(t, first); n == 0 {
				t.Fatalf("no licence state in UNIDOC_LICENSE_DIR set after start-up (%s): the library no longer reads it per call", first)
			}

			t.Setenv("UNIDOC_LICENSE_DIR", second)
			before := countFiles(t, first)
			if _, err := Render(format, richDoc("approved")); err != nil {
				t.Fatalf("second render: %v", err)
			}
			if n := countFiles(t, second); n == 0 {
				t.Fatalf("no licence state in the changed UNIDOC_LICENSE_DIR (%s): the library caches the directory", second)
			}
			if after := countFiles(t, first); after != before {
				t.Errorf("the first directory changed after UNIDOC_LICENSE_DIR moved (%d -> %d files)", before, after)
			}
		})
	}
}

// countFiles counts the regular files under dir, at any depth.
func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := walkFiles(dir, func() { n++ })
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	return n
}

func walkFiles(dir string, f func()) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		switch {
		case e.IsDir():
			if err := walkFiles(dir+string(os.PathSeparator)+e.Name(), f); err != nil {
				return err
			}
		case e.Type().IsRegular():
			f()
		}
	}
	return nil
}
