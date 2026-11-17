package license

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const sentinel = "SENTINEL-KEY-DO-NOT-LOG-7f3a"

type fakeLib struct {
	name, format, vvar string
	licensed           bool

	mu     sync.Mutex
	errs   []error // returned by successive SetMeteredKey calls; then nil
	calls  []string
	events []string
}

func (f *fakeLib) Name() string      { return f.name }
func (f *fakeLib) Format() string    { return f.format }
func (f *fakeLib) VendorVar() string { return f.vvar }
func (f *fakeLib) Licensed() bool    { return f.licensed }
func (f *fakeLib) SetMeteredKey(k string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, k)
	f.events = append(f.events, "set")
	if len(f.errs) == 0 {
		return nil
	}
	e := f.errs[0]
	f.errs = f.errs[1:]
	return e
}
func (f *fakeLib) SetPersistentCache(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "cache")
}

func (f *fakeLib) eventList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func fakes() (*fakeLib, *fakeLib) {
	return &fakeLib{name: "PDF", format: "pdf", vvar: "UNIPDF_LICENSE_PATH"},
		&fakeLib{name: "DOCX", format: "docx", vvar: "UNIOFFICE_LICENSE_PATH"}
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

var netErr = &url.Error{Op: "Post", URL: "http://127.0.0.1:9", Err: errors.New("connection refused")}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestResolve(t *testing.T) {
	p, d := fakes()
	libs := []lib{p, d}
	cases := []struct {
		name     string
		env      map[string]string
		vendor   vendorState
		wantKey  string
		wantOff  Capabilities
		warnings int
		infoHas  string
	}{
		{name: "nothing"},
		{name: "isms key", env: map[string]string{envAPIKey: "k1"}, wantKey: "k1"},
		{name: "legacy only", env: map[string]string{envLegacyAPIKey: "k2"}, wantKey: "k2"},
		{name: "both keys isms wins", env: map[string]string{envAPIKey: "k1", envLegacyAPIKey: "k2"},
			wantKey: "k1", infoHas: envLegacyAPIKey + " is ignored"},
		{name: "whitespace key is unset", env: map[string]string{envAPIKey: "  \t", envLegacyAPIKey: " "}},
		{name: "key trimmed", env: map[string]string{envAPIKey: "  k1 "}, wantKey: "k1"},
		{name: "key + vendor pdf", env: map[string]string{envAPIKey: "k1"},
			vendor: vendorState{"pdf": true}, warnings: 1},
		{name: "key + vendor docx", env: map[string]string{envAPIKey: "k1"},
			vendor: vendorState{"docx": true}, warnings: 1},
		{name: "vendor pdf only", vendor: vendorState{"pdf": true},
			wantOff: Capabilities{PDF: SourceOffline}, infoHas: "UNIPDF_LICENSE_PATH"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, warns := resolve(env(c.env), libs, c.vendor)
			if plan.APIKey != c.wantKey {
				t.Errorf("key = %q, want %q", plan.APIKey, c.wantKey)
			}
			if plan.Offline != c.wantOff {
				t.Errorf("offline = %+v, want %+v", plan.Offline, c.wantOff)
			}
			if len(warns) != c.warnings {
				t.Errorf("warnings = %v, want %d", warns, c.warnings)
			}
			if c.infoHas != "" && !strings.Contains(strings.Join(plan.Infos, "\n"), c.infoHas) {
				t.Errorf("infos %v lack %q", plan.Infos, c.infoHas)
			}
			if c.warnings == 1 {
				if !strings.Contains(warns[0], envAPIKey) || plan.APIKey != "" || plan.Offline != (Capabilities{}) {
					t.Errorf("D6 case must name the variable and turn everything off: %v %+v", warns, plan)
				}
			}
		})
	}
	// The conflict warning names the vendor variable.
	_, warns := resolve(env(map[string]string{envAPIKey: "k"}), libs, vendorState{"pdf": true})
	if !strings.Contains(warns[0], "UNIPDF_LICENSE_PATH") {
		t.Errorf("warning does not name the vendor variable: %v", warns)
	}
}

func newTestManager() *manager {
	m := newManager()
	m.schedule = func(n int) time.Duration { return time.Duration(n+1) * time.Minute }
	m.sleep = func(ctx context.Context, d time.Duration) bool { return ctx.Err() == nil }
	return m
}

func TestApplyBothSucceed(t *testing.T) {
	p, d := fakes()
	m := newTestManager()
	m.run(context.Background(), env(map[string]string{envAPIKey: "k"}), []lib{p, d})
	if got := m.current(); got != (Capabilities{PDF: SourceMetered, DOCX: SourceMetered}) {
		t.Fatalf("caps = %+v", got)
	}
	if !reflect.DeepEqual(p.eventList(), []string{"set"}) {
		t.Errorf("cache setter called without the opt-out: %v", p.eventList())
	}
}

func TestApplyPartialThenRetry(t *testing.T) {
	p, d := fakes()
	d.errs = []error{netErr, netErr}
	m := newTestManager()
	var slept []time.Duration
	var smu sync.Mutex
	sawPDFWhileWaiting := false
	m.sleep = func(ctx context.Context, dur time.Duration) bool {
		smu.Lock()
		defer smu.Unlock()
		slept = append(slept, dur)
		// PDF is published before DOCX gets its next attempt.
		if c := m.current(); c.PDF == SourceMetered && c.DOCX == SourceOff {
			sawPDFWhileWaiting = true
		}
		time.Sleep(20 * time.Millisecond) // let the PDF goroutine finish
		return true
	}
	m.run(context.Background(), env(map[string]string{envAPIKey: "k"}), []lib{p, d})
	if !sawPDFWhileWaiting {
		t.Error("PDF was not published while DOCX was still retrying")
	}
	if !reflect.DeepEqual(slept, []time.Duration{time.Minute, 2 * time.Minute}) {
		t.Errorf("schedule = %v", slept)
	}
	if got := m.current(); got != (Capabilities{PDF: SourceMetered, DOCX: SourceMetered}) {
		t.Fatalf("caps = %+v", got)
	}
	if len(d.calls) != 3 {
		t.Errorf("docx calls = %d, want 3", len(d.calls))
	}
}

func TestApplyNotRetried(t *testing.T) {
	for _, e := range []error{errors.New("key not valid"), errors.New("license key already set")} {
		p, d := fakes()
		p.errs = []error{e}
		d.errs = []error{e}
		m := newTestManager()
		m.sleep = func(context.Context, time.Duration) bool { t.Error("must not sleep"); return false }
		m.run(context.Background(), env(map[string]string{envAPIKey: "k"}), []lib{p, d})
		if len(p.calls) != 1 || len(d.calls) != 1 {
			t.Errorf("%v: calls = %d/%d, want 1/1", e, len(p.calls), len(d.calls))
		}
		if got := m.current(); got != (Capabilities{}) {
			t.Errorf("caps = %+v", got)
		}
	}
}

func TestCacheFlagAfterSet(t *testing.T) {
	for _, v := range []string{"0", "false", "off", "OFF"} {
		p, d := fakes()
		m := newTestManager()
		m.run(context.Background(), env(map[string]string{envAPIKey: "k", envCache: v}), []lib{p, d})
		want := []string{"set", "cache"}
		if !reflect.DeepEqual(p.eventList(), want) || !reflect.DeepEqual(d.eventList(), want) {
			t.Errorf("%s: events = %v / %v, want %v", v, p.eventList(), d.eventList(), want)
		}
	}
	// A failed attempt must not touch the cache flag; the retry's success does.
	p, d := fakes()
	p.errs = []error{netErr}
	m := newTestManager()
	m.run(context.Background(), env(map[string]string{envAPIKey: "k", envCache: "0"}), []lib{p, d})
	if want := []string{"set", "set", "cache"}; !reflect.DeepEqual(p.eventList(), want) {
		t.Errorf("events = %v, want %v", p.eventList(), want)
	}
	// Any other value leaves the vendor default.
	p, d = fakes()
	m = newTestManager()
	m.run(context.Background(), env(map[string]string{envAPIKey: "k", envCache: "1"}), []lib{p, d})
	if want := []string{"set"}; !reflect.DeepEqual(p.eventList(), want) {
		t.Errorf("events = %v, want %v", p.eventList(), want)
	}
}

func TestNoSettersOnConflictOrOffline(t *testing.T) {
	p, d := fakes()
	p.licensed = true
	m := newTestManager()
	m.run(context.Background(), env(map[string]string{envAPIKey: "k"}), []lib{p, d})
	if len(p.calls)+len(d.calls) != 0 || m.current() != (Capabilities{}) {
		t.Errorf("D6 conflict must not call setters or advertise: %+v", m.current())
	}
	m = newTestManager()
	m.run(context.Background(), env(nil), []lib{p, d})
	if len(p.calls)+len(d.calls) != 0 || m.current() != (Capabilities{PDF: SourceOffline}) {
		t.Errorf("offline: %+v", m.current())
	}
}

func TestCancelStopsRetries(t *testing.T) {
	p, d := fakes()
	p.errs = []error{netErr, netErr, netErr, netErr, netErr}
	d.errs = p.errs
	ctx, cancel := context.WithCancel(context.Background())
	m := newTestManager()
	m.sleep = func(ctx context.Context, _ time.Duration) bool { cancel(); return false }
	done := make(chan struct{})
	go func() {
		m.run(ctx, env(map[string]string{envAPIKey: "k"}), []lib{p, d})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	if len(p.calls) != 1 || len(d.calls) != 1 {
		t.Errorf("calls after cancel = %d/%d", len(p.calls), len(d.calls))
	}
}

func TestDefaultSchedule(t *testing.T) {
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour}
	for i, w := range want {
		if got := defaultSchedule(i); got != w {
			t.Errorf("schedule(%d) = %s, want %s", i, got, w)
		}
	}
}

func TestStartTwiceRunsOnce(t *testing.T) {
	p, d := fakes()
	m := newTestManager()
	var once sync.Once
	e := env(map[string]string{envAPIKey: "k"})
	startOn(&once, context.Background(), m, e, []lib{p, d})
	startOn(&once, context.Background(), m, e, []lib{p, d})
	deadline := time.Now().Add(5 * time.Second)
	for m.current() != (Capabilities{PDF: SourceMetered, DOCX: SourceMetered}) {
		if time.Now().After(deadline) {
			t.Fatal("never licensed")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if len(p.calls) != 1 || len(d.calls) != 1 {
		t.Errorf("setter calls = %d/%d, want 1/1", len(p.calls), len(d.calls))
	}
}

func TestStartReturnsImmediately(t *testing.T) {
	p, d := fakes()
	block := make(chan struct{})
	defer close(block)
	m := newTestManager()
	p.errs = []error{netErr}
	m.sleep = func(ctx context.Context, _ time.Duration) bool { <-block; return false }
	var once sync.Once
	start := time.Now()
	startOn(&once, context.Background(), m, env(map[string]string{envAPIKey: "k"}), []lib{p, d})
	if time.Since(start) > time.Second {
		t.Error("Start blocked")
	}
}

func TestRealVendorErrorIsRetryable(t *testing.T) {
	for _, l := range realLibs() {
		if l.Licensed() {
			t.Skip("a vendor licence is active in this process")
		}
	}
	t.Setenv("UNIDOC_LICENSE_SERVER_URL", "http://127.0.0.1:9")
	t.Setenv("UNIDOC_LICENSE_DIR", t.TempDir())
	buf := captureLog(t)
	libs := realLibs()
	for _, l := range libs {
		err := l.SetMeteredKey(sentinel)
		if err == nil {
			t.Fatalf("%s: expected an error from a closed port", l.Name())
		}
		if !retryable(err) {
			t.Errorf("%s: %T %v not classified retryable", l.Name(), err, err)
		}
		if strings.Contains(err.Error(), sentinel) {
			t.Errorf("%s: vendor error contains the key", l.Name())
		}
		if l.Licensed() {
			t.Errorf("%s: licensed after a failed call", l.Name())
		}
	}
	// And the whole pipeline with real libs, scheduling instantly.
	m := newTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	m.sleep = func(context.Context, time.Duration) bool { cancel(); return false }
	m.run(ctx, env(map[string]string{envAPIKey: sentinel}), libs)
	if strings.Contains(buf.String(), sentinel) {
		t.Error("log contains the key")
	}
	if !strings.Contains(buf.String(), "retrying in") {
		t.Errorf("expected a retry line, got %q", buf.String())
	}
}

func TestSecretNeverLogged(t *testing.T) {
	buf := captureLog(t)
	keyEnv := map[string]string{envAPIKey: sentinel, envCache: "0"}
	// success
	p, d := fakes()
	newTestManager().run(context.Background(), env(keyEnv), []lib{p, d})
	// network failure then success
	p, d = fakes()
	p.errs = []error{netErr}
	newTestManager().run(context.Background(), env(keyEnv), []lib{p, d})
	// invalid key
	p, d = fakes()
	p.errs = []error{errors.New("key not valid")}
	d.errs = []error{errors.New("license key already set")}
	newTestManager().run(context.Background(), env(keyEnv), []lib{p, d})
	// conflict
	p, d = fakes()
	d.licensed = true
	newTestManager().run(context.Background(), env(keyEnv), []lib{p, d})
	// legacy name, both names
	p, d = fakes()
	newTestManager().run(context.Background(), env(map[string]string{envLegacyAPIKey: sentinel}), []lib{p, d})
	newTestManager().run(context.Background(), env(map[string]string{envAPIKey: sentinel, envLegacyAPIKey: sentinel + "x"}), []lib{p, d})
	_, warns := resolve(env(keyEnv), []lib{p, d}, vendorState{"pdf": true})

	out := buf.String() + strings.Join(warns, "\n")
	if out == "" {
		t.Fatal("expected some log output")
	}
	if strings.Contains(out, "SENTINEL-KEY") {
		t.Errorf("key leaked into the log:\n%s", out)
	}
}

func TestFormatsNeverNil(t *testing.T) {
	f := Capabilities{}.Formats()
	if f == nil || len(f) != 0 {
		t.Fatalf("Formats() = %#v", f)
	}
	b, _ := json.Marshal(f)
	if string(b) != "[]" {
		t.Errorf("json = %s", b)
	}
	if got := (Capabilities{PDF: SourceMetered, DOCX: SourceOffline}).Formats(); !reflect.DeepEqual(got, []string{"pdf", "docx"}) {
		t.Errorf("got %v", got)
	}
	if got := (Capabilities{DOCX: SourceMetered}).Formats(); !reflect.DeepEqual(got, []string{"docx"}) {
		t.Errorf("got %v", got)
	}
}
