package license

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// auditedVendor is the UniPDF and UniOffice version whose licence behaviour this
// package was checked against. Two behaviours matter and neither can be tested
// without a metered key:
//
//   - the libraries read UNIDOC_LICENSE_DIR each time they load or save their
//     metered state, so Start can set it in-process after they are initialised
//     (TestKeyedLicenseDirReadPerCall in internal/isms/export proves it);
//   - SetMeteredKey refuses a second key, and SetMeteredKeyPersistentCache must
//     come after SetMeteredKey, which resets it (applyOne relies on both).
//
// When go.mod moves either library, this test fails on purpose. Re-run the keyed
// tests with a metered key:
//
//	ISMS_TEST_UNIDOC_API_KEY=... go test ./internal/isms/export -run Keyed
//
// and update the version here once they pass.
var auditedVendor = map[string]string{
	"github.com/unidoc/unipdf/v5":    "v5.2.0",
	"github.com/unidoc/unioffice/v2": "v2.15.0",
}

func TestVendorVersionsAudited(t *testing.T) {
	got := requiredVersions(t, findGoMod(t))
	for mod, want := range auditedVendor {
		v, ok := got[mod]
		if !ok {
			t.Errorf("%s is no longer in go.mod; update auditedVendor and vendor.go", mod)
			continue
		}
		if v != want {
			t.Errorf("%s is %s in go.mod but its licence behaviour was checked on %s. "+
				"Re-run the keyed export tests with a metered key "+
				"(ISMS_TEST_UNIDOC_API_KEY=... go test ./internal/isms/export -run Keyed), "+
				"then set auditedVendor to %s", mod, v, want, v)
		}
	}
}

// findGoMod walks up from the test's directory to the module's go.mod.
func findGoMod(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		p := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// requiredVersions reads the module => version pairs from go.mod's require
// lines, both the block form and the single-line form.
func requiredVersions(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	inBlock := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case line == "require (":
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case strings.HasPrefix(line, "require "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		case !inBlock:
			continue
		}
		if fs := strings.Fields(line); len(fs) == 2 {
			out[fs[0]] = fs[1]
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
