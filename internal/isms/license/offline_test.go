package license

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	envSharedFile = envSharedKey + "_FILE"
	envPDFKey     = "ISMS_UNIPDF_LICENSE_KEY"
	envDOCXKey    = "ISMS_UNIOFFICE_LICENSE_KEY"
	envDOCXFile   = envDOCXKey + "_FILE"
)

// offlineBlock looks like a real key: armour lines around a body, one line per
// row so the redaction test can check each of them.
var offlineLines = []string{
	"-----BEGIN UNIDOC LICENSE KEY-----",
	"SENTINEL-OFFKEY-LINE-ONE-aaaa",
	"SENTINEL-OFFKEY-LINE-TWO-bbbb",
	"-----END UNIDOC LICENSE KEY-----",
}
var offlineBlock = strings.Join(offlineLines, "\n")

// files is a readFile over an in-memory set of files.
func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := m[p]; ok {
			return []byte(s), nil
		}
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
}

func TestResolveOffline(t *testing.T) {
	p, d := fakes()
	libs := []lib{p, d}
	const cust = "ACME"
	pdfKey := func(k, v string) offlineKey { return offlineKey{Key: k, Customer: cust, Var: v} }
	cases := []struct {
		name    string
		env     map[string]string
		files   map[string]string
		vendor  vendorState
		wantKey string // metered key kept
		keys    map[string]offlineKey
		off     Capabilities
		warns   [][]string // each warning must contain all of these, in order
	}{
		{name: "S1 shared key + customer, both libraries",
			env:  map[string]string{envSharedKey: "K1", envSharedCustomer: cust},
			keys: map[string]offlineKey{"pdf": pdfKey("K1", envSharedKey), "docx": pdfKey("K1", envSharedKey)}},
		{name: "S3 shared key without a customer name",
			env: map[string]string{envSharedKey: "K1"},
			warns: [][]string{
				{envSharedKey, "without a customer name", "ISMS_UNIPDF_CUSTOMER_NAME", envSharedCustomer, "PDF export is off"},
				{envSharedKey, "without a customer name", "ISMS_UNIOFFICE_CUSTOMER_NAME", envSharedCustomer, "DOCX export is off"}}},
		{name: "S4 PDF override only leaves DOCX alone, no warning",
			env:  map[string]string{envPDFKey: "K2", envSharedCustomer: cust},
			keys: map[string]offlineKey{"pdf": pdfKey("K2", envPDFKey)}},
		{name: "S5 _FILE missing",
			env: map[string]string{envSharedFile: "/no/such/key", envSharedCustomer: cust},
			warns: [][]string{
				{"cannot read", envSharedFile, "/no/such/key", "does not exist", "PDF export is off"},
				{"cannot read", envSharedFile, "/no/such/key", "does not exist", "DOCX export is off"}}},
		{name: "S5b _FILE empty file",
			env:   map[string]string{envSharedFile: "/k", envSharedCustomer: cust},
			files: map[string]string{"/k": " \n"},
			warns: [][]string{{envSharedFile, "/k", "is empty", "PDF export is off"}, {envSharedFile, "is empty", "DOCX export is off"}}},
		{name: "S6 vendor variable set but library not licensed",
			env: map[string]string{"UNIPDF_LICENSE_PATH": "/x", "UNIOFFICE_LICENSE_PATH": "/y"},
			warns: [][]string{
				{"UNIPDF_LICENSE_PATH is set but UniPDF did not accept that licence", "UNIPDF_CUSTOMER_NAME", "PDF export is off"},
				{"UNIOFFICE_LICENSE_PATH is set but UniOffice did not accept that licence", "UNIOFFICE_CUSTOMER_NAME", "DOCX export is off"}}},
		{name: "S7 D6: API key + shared key refuses by presence",
			env:   map[string]string{envAPIKey: "k", envSharedKey: "garbage"},
			warns: [][]string{{envAPIKey, envSharedKey, "use one licence type", "PDF/DOCX export is off"}}},
		{name: "S8 D6: API key + vendor variable that was rejected",
			env:   map[string]string{envAPIKey: "k", "UNIPDF_LICENSE_PATH": "/x"},
			warns: [][]string{{envAPIKey, "UNIPDF_LICENSE_PATH", "use one licence type"}}},
		{name: "S9 variable and its _FILE both set",
			env: map[string]string{envSharedKey: "K1", envSharedFile: "/k", envSharedCustomer: cust},
			warns: [][]string{
				{envSharedKey, "and", envSharedFile, "both set", "PDF export is off"},
				{envSharedKey, "and", envSharedFile, "both set", "DOCX export is off"}}},
		{name: "S10 key from a file, trimmed",
			env:   map[string]string{envSharedFile: "/k", envSharedCustomer: cust},
			files: map[string]string{"/k": "\n K1 \n"},
			keys:  map[string]offlineKey{"pdf": pdfKey("K1", envSharedFile), "docx": pdfKey("K1", envSharedFile)}},
		{name: "S11 blank values are unset: the API key stays metered",
			env:     map[string]string{envAPIKey: "k", envSharedKey: "  ", envSharedFile: "\t", envPDFKey: " ", envDOCXFile: "", "UNIPDF_LICENSE_PATH": " "},
			wantKey: "k"},
		{name: "S11b blank values alone are nothing",
			env: map[string]string{envSharedKey: " ", envSharedFile: " ", envSharedCustomer: " "}},
		{name: "S12 API key only",
			env: map[string]string{envAPIKey: "k"}, wantKey: "k"},
		{name: "S13 D6: API key + UniOffice _FILE override",
			env:   map[string]string{envAPIKey: "k", envDOCXFile: "/nope"},
			warns: [][]string{{envAPIKey, envDOCXFile, "use one licence type"}}},
		{name: "D6: vendor-licensed library + API key still refuses",
			env: map[string]string{envAPIKey: "k"}, vendor: vendorState{"docx": true},
			warns: [][]string{{envAPIKey, "UNIOFFICE_LICENSE_PATH"}}},
		{name: "matrix: override replaces the shared key for its library",
			env:  map[string]string{envSharedKey: "K1", envPDFKey: "K2", envSharedCustomer: cust},
			keys: map[string]offlineKey{"pdf": pdfKey("K2", envPDFKey), "docx": pdfKey("K1", envSharedKey)}},
		{name: "matrix: both overrides, no shared key",
			env:  map[string]string{envPDFKey: "K2", envDOCXKey: "K3", envSharedCustomer: cust},
			keys: map[string]offlineKey{"pdf": pdfKey("K2", envPDFKey), "docx": pdfKey("K3", envDOCXKey)}},
		{name: "matrix: a broken override does not fall back to the shared key",
			env:   map[string]string{envSharedKey: "K1", envPDFKey: "K2", envPDFKey + "_FILE": "/k", envSharedCustomer: cust},
			keys:  map[string]offlineKey{"docx": pdfKey("K1", envSharedKey)},
			warns: [][]string{{envPDFKey, "both set", "PDF export is off"}}},
		{name: "matrix: unreadable override file does not fall back either",
			env:   map[string]string{envSharedKey: "K1", envPDFKey + "_FILE": "/gone", envSharedCustomer: cust},
			keys:  map[string]offlineKey{"docx": pdfKey("K1", envSharedKey)},
			warns: [][]string{{"cannot read", envPDFKey + "_FILE", "/gone", "PDF export is off"}}},
		{name: "customer: per-library name beats the shared one, blank falls back",
			env: map[string]string{envSharedKey: "K1", envSharedCustomer: cust, "ISMS_UNIPDF_CUSTOMER_NAME": "PDFCO", "ISMS_UNIOFFICE_CUSTOMER_NAME": "  "},
			keys: map[string]offlineKey{
				"pdf":  {Key: "K1", Customer: "PDFCO", Var: envSharedKey},
				"docx": {Key: "K1", Customer: cust, Var: envSharedKey}}},
		{name: "customer: only a per-library name, other library has none",
			env:   map[string]string{envSharedKey: "K1", "ISMS_UNIPDF_CUSTOMER_NAME": "PDFCO"},
			keys:  map[string]offlineKey{"pdf": {Key: "K1", Customer: "PDFCO", Var: envSharedKey}},
			warns: [][]string{{envSharedKey, "without a customer name", "DOCX export is off"}}},
		{name: "vendor-licensed library is offline and the shared key goes to the other",
			env: map[string]string{envSharedKey: "K1", envSharedCustomer: cust}, vendor: vendorState{"pdf": true},
			off:   Capabilities{PDF: SourceOffline},
			keys:  map[string]offlineKey{"docx": pdfKey("K1", envSharedKey)},
			warns: [][]string{{envSharedKey, "ignored for UniPDF", "UNIPDF_LICENSE_PATH"}}},
		{name: "vendor variable rejected: its library is off, ISMS key ignored with a warning",
			env: map[string]string{"UNIPDF_LICENSE_PATH": "/x", envPDFKey: "K2", envSharedCustomer: cust},
			warns: [][]string{
				{"UNIPDF_LICENSE_PATH is set but UniPDF did not accept", "PDF export is off"},
				{envPDFKey, "ignored for UniPDF", "UNIPDF_LICENSE_PATH"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pl, warns := resolve(env(c.env), files(c.files), libs, c.vendor)
			if pl.APIKey != c.wantKey {
				t.Errorf("metered key = %q, want %q", pl.APIKey, c.wantKey)
			}
			if !reflect.DeepEqual(pl.Keys, c.keys) && !(len(pl.Keys) == 0 && len(c.keys) == 0) {
				t.Errorf("offline keys = %+v, want %+v", pl.Keys, c.keys)
			}
			if pl.Offline != c.off {
				t.Errorf("offline caps = %+v, want %+v", pl.Offline, c.off)
			}
			if len(warns) != len(c.warns) {
				t.Fatalf("warnings = %q, want %d", warns, len(c.warns))
			}
			for i, w := range c.warns {
				rest := warns[i]
				for _, part := range w {
					j := strings.Index(rest, part)
					if j < 0 {
						t.Errorf("warning %d %q lacks %q (in order)", i, warns[i], part)
						break
					}
					rest = rest[j+len(part):]
				}
			}
			all := strings.Join(warns, "\n") + strings.Join(pl.Infos, "\n")
			for _, secret := range []string{"K1", "K2", "K3", "garbage"} {
				if strings.Contains(all, secret) {
					t.Errorf("a key value reached the messages: %q", all)
				}
			}
			if strings.HasPrefix(c.name, "S7") || strings.HasPrefix(c.name, "S8") || strings.HasPrefix(c.name, "S13") || strings.HasPrefix(c.name, "D6") {
				if pl.APIKey != "" || len(pl.Keys) != 0 || pl.Offline != (Capabilities{}) {
					t.Errorf("D6 refusal must turn everything off: %+v", pl)
				}
			}
		})
	}
}

func TestApplyOfflineKey(t *testing.T) {
	envShared := map[string]string{envSharedKey: offlineBlock, envSharedCustomer: "ACME"}
	t.Run("success publishes offline for each library", func(t *testing.T) {
		buf := captureLog(t)
		p, d := fakes()
		m := newTestManager()
		m.run(context.Background(), env(envShared), []lib{p, d})
		if got := m.current(); got != (Capabilities{PDF: SourceOffline, DOCX: SourceOffline}) {
			t.Fatalf("caps = %+v", got)
		}
		want := [][]string{{offlineBlock, "ACME"}}
		if !reflect.DeepEqual(p.setCalls, want) || !reflect.DeepEqual(d.setCalls, want) {
			t.Errorf("setter calls = %v / %v", p.setCalls, d.setCalls)
		}
		if len(p.calls)+len(d.calls) != 0 {
			t.Error("the metered setter must not be called")
		}
		if !strings.Contains(buf.String(), "export licence: PDF export licensed from "+envSharedKey+" (offline)") {
			t.Errorf("log = %q", buf.String())
		}
	})
	t.Run("key from a file", func(t *testing.T) {
		p, d := fakes()
		m := newTestManager()
		m.readFile = files(map[string]string{"/k": offlineBlock + "\n"})
		m.run(context.Background(), env(map[string]string{envSharedFile: "/k", envSharedCustomer: "ACME"}), []lib{p, d})
		if m.current() != (Capabilities{PDF: SourceOffline, DOCX: SourceOffline}) || p.setCalls[0][0] != offlineBlock {
			t.Errorf("caps = %+v calls = %v", m.current(), p.setCalls)
		}
	})
	t.Run("a vendor-licensed library stays offline, the other takes the key", func(t *testing.T) {
		p, d := fakes()
		p.licensed = true
		m := newTestManager()
		m.run(context.Background(), env(envShared), []lib{p, d})
		if m.current() != (Capabilities{PDF: SourceOffline, DOCX: SourceOffline}) {
			t.Errorf("caps = %+v", m.current())
		}
		if len(p.setCalls) != 0 || len(d.setCalls) != 1 {
			t.Errorf("setter calls = %d/%d, want 0/1", len(p.setCalls), len(d.setCalls))
		}
	})
	t.Run("override only configures its library", func(t *testing.T) {
		buf := captureLog(t)
		p, d := fakes()
		m := newTestManager()
		m.run(context.Background(), env(map[string]string{envPDFKey: offlineBlock, envSharedCustomer: "ACME"}), []lib{p, d})
		if m.current() != (Capabilities{PDF: SourceOffline}) || len(d.setCalls) != 0 {
			t.Errorf("caps = %+v docx calls = %v", m.current(), d.setCalls)
		}
		if strings.Contains(buf.String(), "DOCX") {
			t.Errorf("DOCX was never configured but the log mentions it: %q", buf.String())
		}
	})
	t.Run("a rejected key leaves only that library off", func(t *testing.T) {
		buf := captureLog(t)
		p, d := fakes()
		d.setErr = errors.New("header not found")
		m := newTestManager()
		m.run(context.Background(), env(envShared), []lib{p, d})
		if m.current() != (Capabilities{PDF: SourceOffline}) {
			t.Errorf("caps = %+v", m.current())
		}
		if strings.Contains(buf.String(), "UniPDF did not accept") {
			t.Errorf("PDF warning for a library that accepted the key: %q", buf.String())
		}
	})
}

func TestApplyOfflineKeyRejected(t *testing.T) {
	cases := []struct{ vendor, words string }{
		{"header not found", "not a UniDoc licence key"},
		{"invalid input, signature separator", "malformed licence key"},
		{"crypto/rsa: verification error", "signature check failed"},
		{"licence expired on 2020-01-01", "licence expired on 2020-01-01"},
	}
	for _, c := range cases {
		t.Run(c.vendor, func(t *testing.T) {
			buf := captureLog(t)
			p, d := fakes()
			p.setErr, d.setErr = errors.New(c.vendor), errors.New(c.vendor)
			m := newTestManager()
			m.run(context.Background(), env(map[string]string{envSharedKey: offlineBlock, envSharedCustomer: "ACME"}), []lib{p, d})
			if m.current() != (Capabilities{}) {
				t.Errorf("caps = %+v, want none", m.current())
			}
			out := buf.String()
			for _, want := range []string{
				"export licence: UniPDF did not accept " + envSharedKey + ": " + c.words + "; PDF export is off",
				"export licence: UniOffice did not accept " + envSharedKey + ": " + c.words + "; DOCX export is off",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("log lacks %q:\n%s", want, out)
				}
			}
			if n := strings.Count(out, "did not accept"); n != 2 {
				t.Errorf("%d warnings, want one per library:\n%s", n, out)
			}
		})
	}
	t.Run("names the variable that supplied the key", func(t *testing.T) {
		buf := captureLog(t)
		p, d := fakes()
		d.setErr = errors.New("header not found")
		m := newTestManager()
		m.readFile = files(map[string]string{"/k": offlineBlock})
		m.run(context.Background(), env(map[string]string{envSharedKey: offlineBlock, envDOCXFile: "/k", envSharedCustomer: "ACME"}), []lib{p, d})
		if !strings.Contains(buf.String(), "UniOffice did not accept "+envDOCXFile+": not a UniDoc licence key") {
			t.Errorf("log = %q", buf.String())
		}
	})
}

// TestOfflineKeyRedacted: a vendor error may carry key text (not seen yet, but
// the errors for properly signed keys are unknown); no key line may reach the log.
func TestOfflineKeyRedacted(t *testing.T) {
	buf := captureLog(t)
	p, d := fakes()
	p.setErr = fmt.Errorf("cannot parse %q: %s", offlineBlock, "bad")
	d.setErr = fmt.Errorf("line %s is wrong\n%s", offlineLines[1], offlineLines[2])
	m := newTestManager()
	m.run(context.Background(), env(map[string]string{envSharedKey: offlineBlock, envSharedCustomer: "ACME"}), []lib{p, d})
	out := buf.String()
	if !strings.Contains(out, "UniPDF did not accept") || !strings.Contains(out, "UniOffice did not accept") {
		t.Fatalf("expected both warnings:\n%s", out)
	}
	for _, line := range offlineLines {
		if len(line) >= 8 && strings.Contains(out, line) {
			t.Errorf("key line %q leaked into the log:\n%s", line, out)
		}
	}
}

func TestD6RefusalHasNoSideEffects(t *testing.T) {
	buf := captureLog(t)
	dataDir := t.TempDir()
	for _, e := range []map[string]string{
		{envAPIKey: "k", envSharedKey: offlineBlock, envSharedCustomer: "ACME"},
		{envAPIKey: "k", "UNIPDF_LICENSE_PATH": "/rejected"},
		{envAPIKey: "k", envDOCXFile: "/nope"},
	} {
		p, d := fakes()
		m := newManager()
		m.dataDir = dataDir
		m.setenv = func(k, v string) error { t.Errorf("setenv(%s) on a refusal", k); return nil }
		m.run(context.Background(), env(e), []lib{p, d})
		if len(p.calls)+len(d.calls)+len(p.setCalls)+len(d.setCalls) != 0 || m.current() != (Capabilities{}) {
			t.Errorf("%v: setters called or something advertised: %+v", e, m.current())
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, "unidoc")); err == nil {
		t.Error("state directory created on a refusal")
	}
	if strings.Contains(buf.String(), "licence state") || strings.Contains(buf.String(), "unreachable") {
		t.Errorf("state-dir or licence-server line on a refusal:\n%s", buf.String())
	}
	if strings.Count(buf.String(), "use one licence type") != 3 {
		t.Errorf("want one warning per refusal:\n%s", buf.String())
	}
}

func TestNoSettersWhenVendorVariableSet(t *testing.T) {
	// The variable alone decides, whatever the vendor init made of it.
	p, d := fakes()
	m := newTestManager()
	m.run(context.Background(), env(map[string]string{"UNIPDF_LICENSE_PATH": "/x", envAPIKey: "k"}), []lib{p, d})
	if len(p.calls)+len(d.calls) != 0 || m.current() != (Capabilities{}) {
		t.Errorf("caps = %+v, calls = %d/%d", m.current(), len(p.calls), len(d.calls))
	}
}

func TestMeteredUnchangedByBlankOfflineValues(t *testing.T) {
	p, d := fakes()
	m := newTestManager()
	m.run(context.Background(), env(map[string]string{envAPIKey: "k", envSharedKey: " ", envSharedFile: "", envPDFKey: "\t"}), []lib{p, d})
	if m.current() != (Capabilities{PDF: SourceMetered, DOCX: SourceMetered}) || len(p.setCalls)+len(d.setCalls) != 0 {
		t.Errorf("caps = %+v", m.current())
	}
}

// TestRealVendorRejectsGarbageKey needs no network and no real key: both
// libraries must refuse garbage, stay unlicensed, and not echo the key.
func TestRealVendorRejectsGarbageKey(t *testing.T) {
	for _, l := range realLibs() {
		if l.Licensed() {
			t.Skip("a vendor licence is active in this process")
		}
	}
	for _, key := range []string{"garbage", "", offlineBlock} {
		for _, l := range realLibs() {
			err := l.SetLicenseKey(key, "ACME")
			if err == nil {
				t.Errorf("%s: garbage key %q accepted", l.Name(), key)
				continue
			}
			if strings.Contains(err.Error(), "SENTINEL-OFFKEY") {
				t.Errorf("%s: vendor error echoes the key: %v", l.Name(), err)
			}
			if l.Licensed() {
				t.Errorf("%s: licensed after a rejected key", l.Name())
			}
		}
	}
	// Through the whole pipeline: both libraries off, a warning each, no key text.
	buf := captureLog(t)
	m := newTestManager()
	m.run(context.Background(), env(map[string]string{envSharedKey: offlineBlock, envSharedCustomer: "ACME"}), realLibs())
	if m.current() != (Capabilities{}) {
		t.Errorf("caps = %+v", m.current())
	}
	if n := strings.Count(buf.String(), "did not accept "+envSharedKey); n != 2 {
		t.Errorf("want one warning per library, got %d:\n%s", n, buf.String())
	}
	if strings.Contains(buf.String(), "SENTINEL-OFFKEY") {
		t.Errorf("key text in the log:\n%s", buf.String())
	}
}
