package license

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stateManager is a manager that handles the state directory, with its setenv
// recorded instead of changing the process environment.
func stateManager(dataDir string) (*manager, map[string]string) {
	m := newTestManager()
	m.noState = false
	m.dataDir = dataDir
	set := map[string]string{}
	m.setenv = func(k, v string) error { set[k] = v; return nil }
	return m, set
}

func TestStateDirDefaultsToTheDataDir(t *testing.T) {
	data := t.TempDir()
	m, set := stateManager(data)
	buf := captureLog(t)
	p, d := fakes()
	m.run(context.Background(), env(map[string]string{envAPIKey: sentinel}), []lib{p, d})
	want := filepath.Join(data, "unidoc")
	if set[envLicenseDir] != want {
		t.Fatalf("UNIDOC_LICENSE_DIR set to %q, want %q", set[envLicenseDir], want)
	}
	fi, err := os.Stat(want)
	if err != nil || !fi.IsDir() {
		t.Fatalf("state dir not created: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("state dir mode = %v, want 0700", fi.Mode().Perm())
	}
	if m.current() != (Capabilities{PDF: SourceMetered, DOCX: SourceMetered}) {
		t.Errorf("caps = %+v", m.current())
	}
	if !strings.Contains(buf.String(), want) {
		t.Errorf("the chosen dir is not logged: %s", buf.String())
	}
	if strings.Contains(buf.String(), sentinel) {
		t.Error("the key reached the log")
	}
	if ents, _ := os.ReadDir(want); len(ents) != 0 {
		t.Errorf("the probe file was left behind: %v", ents)
	}
}

func TestStateDirExplicitIsLeftAlone(t *testing.T) {
	data, explicit := t.TempDir(), t.TempDir()
	m, set := stateManager(data)
	p, d := fakes()
	m.run(context.Background(), env(map[string]string{envAPIKey: "k", envLicenseDir: explicit}), []lib{p, d})
	if _, ok := set[envLicenseDir]; ok {
		t.Errorf("an explicit %s must not be overwritten (set to %q)", envLicenseDir, set[envLicenseDir])
	}
	if _, err := os.Stat(filepath.Join(data, "unidoc")); err == nil {
		t.Error("the data dir state directory was created although an explicit one is set")
	}
	if m.current() == (Capabilities{}) {
		t.Error("an explicit writable dir must not stop licensing")
	}
}

// An unusable state directory turns export off: licensing would succeed and
// every export would then fail writing state.
func TestStateDirUnwritableTurnsExportOff(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	for name, mk := range map[string]func(t *testing.T) (m *manager, getenv map[string]string, dir string){
		"explicit": func(t *testing.T) (*manager, map[string]string, string) {
			dir := roDir(t)
			m, _ := stateManager("")
			return m, map[string]string{envAPIKey: sentinel, envLicenseDir: dir}, dir
		},
		"under the data dir": func(t *testing.T) (*manager, map[string]string, string) {
			data := roDir(t)
			m, _ := stateManager(data)
			return m, map[string]string{envAPIKey: sentinel}, filepath.Join(data, "unidoc")
		},
	} {
		t.Run(name, func(t *testing.T) {
			m, e, dir := mk(t)
			buf := captureLog(t)
			p, d := fakes()
			m.run(context.Background(), env(e), []lib{p, d})
			if got := m.current().Formats(); len(got) != 0 {
				t.Errorf("formats = %v, want none", got)
			}
			if len(p.calls)+len(d.calls) != 0 {
				t.Errorf("setters called (%d, %d) although the state dir is unusable", len(p.calls), len(d.calls))
			}
			log := buf.String()
			if n := strings.Count(log, "is not writable"); n != 1 {
				t.Errorf("%d warnings, want exactly 1:\n%s", n, log)
			}
			if !strings.Contains(log, dir) || !strings.Contains(log, envLicenseDir) {
				t.Errorf("the warning must name the directory and the fix:\n%s", log)
			}
			if strings.Contains(log, sentinel) {
				t.Error("the key reached the log")
			}
		})
	}
}

// roDir is a directory that cannot be written; the mode is restored so the temp
// directory can be removed.
func roDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return dir
}

// The libraries read UNIDOC_LICENSE_DIR on every call, so a value set in-process
// before licensing is the one they use. This pins the process environment; that
// the vendor really writes its state there is checked against the real server.
func TestStateDirIsSetInTheProcessEnvironment(t *testing.T) {
	for _, l := range realLibs() {
		if l.Licensed() {
			t.Skip("a vendor licence is active in this process")
		}
	}
	data := t.TempDir()
	t.Setenv(envLicenseDir, "") // restored afterwards; the code below sets it
	m := newManager()
	m.dataDir = data
	if err := m.prepareStateDir(os.Getenv); err != nil {
		t.Fatal(err)
	}
	if got, want := os.Getenv(envLicenseDir), filepath.Join(data, "unidoc"); got != want {
		t.Fatalf("process env %s = %q, want %q", envLicenseDir, got, want)
	}
}
