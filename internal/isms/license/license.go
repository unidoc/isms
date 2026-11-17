// Package license licenses the UniPDF and UniOffice libraries used for document
// export and reports which formats are available.
//
// Licensing runs in the background from Start so a slow or unreachable licence
// server never delays the HTTP listener. Current returns the formats that are
// licensed right now; it is safe to call from any goroutine.
//
// The API key is never logged, wrapped into an error or printed.
package license

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	envAPIKey       = "ISMS_UNIDOC_API_KEY"
	envLegacyAPIKey = "UNIDOC_API_KEY"
	envCache        = "ISMS_UNIDOC_METERED_CACHE"
	// envLicenseDir is where the vendor libraries keep their metered state. They
	// read it on every call, so setting it in-process before licensing works.
	envLicenseDir = "UNIDOC_LICENSE_DIR"
)

// Source says how a format was licensed.
type Source string

const (
	SourceOff     Source = ""
	SourceMetered Source = "metered"
	SourceOffline Source = "offline"
)

// Capabilities is the licensed state of each export format.
type Capabilities struct {
	PDF  Source
	DOCX Source
}

// Formats lists the licensed formats. It is never nil.
func (c Capabilities) Formats() []string {
	out := []string{}
	if c.PDF != SourceOff {
		out = append(out, "pdf")
	}
	if c.DOCX != SourceOff {
		out = append(out, "docx")
	}
	return out
}

func (c Capabilities) with(format string, s Source) Capabilities {
	switch format {
	case "pdf":
		c.PDF = s
	case "docx":
		c.DOCX = s
	}
	return c
}

// lib is one licensable vendor library.
type lib interface {
	Name() string // "PDF" or "DOCX"
	Format() string
	VendorVar() string // offline licence variable the library reads at import
	Licensed() bool    // already licensed (by the library's own init)
	SetMeteredKey(apiKey string) error
	SetPersistentCache(on bool)
}

// vendorState is which libraries were already licensed before we ran.
type vendorState map[string]bool // keyed by lib Format()

// plan is what resolve decided.
type plan struct {
	APIKey  string       // non-empty: call SetMeteredKey with it
	Offline Capabilities // formats licensed by the vendor variables
	Infos   []string
}

// resolve decides what to do from the environment and the vendor state. It is
// pure: it reads neither the process environment nor the libraries. Warnings and
// info lines name variables only, never values.
func resolve(getenv func(string) string, libs []lib, vendor vendorState) (plan, []string) {
	var p plan
	var warnings []string

	key := strings.TrimSpace(getenv(envAPIKey))
	legacy := strings.TrimSpace(getenv(envLegacyAPIKey))
	switch {
	case key != "" && legacy != "":
		p.Infos = append(p.Infos, envLegacyAPIKey+" is ignored because "+envAPIKey+" is set")
	case key == "" && legacy != "":
		key = legacy
	}
	keyVar := envAPIKey
	if strings.TrimSpace(getenv(envAPIKey)) == "" {
		keyVar = envLegacyAPIKey
	}

	var vendorVars []string
	var vendorLicensed []string
	for _, l := range libs {
		if vendor[l.Format()] {
			vendorVars = append(vendorVars, l.VendorVar())
			vendorLicensed = append(vendorLicensed, l.Name())
			p.Offline = p.Offline.with(l.Format(), SourceOffline)
		}
	}

	switch {
	case key != "" && len(vendorVars) > 0:
		warnings = append(warnings, keyVar+" and "+strings.Join(vendorVars, " and ")+
			" are both set; use one licence type. PDF/DOCX export is off.")
		return plan{}, warnings
	case key != "":
		p.APIKey = key
	case len(vendorVars) > 0:
		p.Infos = append(p.Infos, strings.Join(vendorLicensed, " and ")+
			" export licensed from "+strings.Join(vendorVars, " and "))
	}
	return p, warnings
}

// retryable reports whether err means the licence server could not be reached.
func retryable(err error) bool {
	if err == nil {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// defaultSchedule is the wait before retry n (0-based); the last step repeats.
func defaultSchedule(n int) time.Duration {
	steps := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	if n >= len(steps) {
		n = len(steps) - 1
	}
	return steps[n]
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// manager holds the published capabilities and the injectable pieces.
type manager struct {
	mu       sync.Mutex
	caps     atomic.Pointer[Capabilities]
	schedule func(n int) time.Duration
	sleep    func(ctx context.Context, d time.Duration) bool

	// dataDir is the server's data directory: the licence state lives under it
	// unless UNIDOC_LICENSE_DIR says otherwise. setenv is os.Setenv.
	dataDir string
	setenv  func(key, value string) error
	// noState skips the state directory handling (tests of the licensing flow).
	noState bool
}

func newManager() *manager {
	m := &manager{schedule: defaultSchedule, sleep: sleepCtx, setenv: os.Setenv}
	m.caps.Store(&Capabilities{})
	return m
}

func (m *manager) current() Capabilities { return *m.caps.Load() }

// publish sets one format (copy on write under a lock so two libraries
// finishing together cannot lose each other's update).
func (m *manager) publish(format string, s Source) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.caps.Load().with(format, s)
	m.caps.Store(&next)
}

func (m *manager) setAll(c Capabilities) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.caps.Store(&c)
}

// run resolves the configuration and applies it. It blocks until every library
// is licensed, failed for good, or ctx is cancelled.
func (m *manager) run(ctx context.Context, getenv func(string) string, libs []lib) {
	vendor := vendorState{}
	for _, l := range libs {
		vendor[l.Format()] = l.Licensed()
	}
	p, warnings := resolve(getenv, libs, vendor)
	for _, w := range warnings {
		log.Printf("export licence: %s", w)
	}
	for _, i := range p.Infos {
		log.Printf("export licence: %s", i)
	}
	m.setAll(p.Offline)
	if p.APIKey == "" {
		return
	}
	if !m.noState {
		if err := m.prepareStateDir(getenv); err != nil {
			// Licensing would succeed but every export would then fail writing its
			// state, so export is not offered at all.
			log.Printf("export licence: %v; PDF/DOCX export is off", err)
			return
		}
	}
	cacheOff := false
	switch strings.ToLower(strings.TrimSpace(getenv(envCache))) {
	case "0", "false", "off":
		cacheOff = true
	}
	var wg sync.WaitGroup
	for _, l := range libs {
		wg.Add(1)
		go func(l lib) {
			defer wg.Done()
			m.applyOne(ctx, l, p.APIKey, cacheOff)
		}(l)
	}
	wg.Wait()
}

// prepareStateDir makes sure the vendor libraries have a writable place for their
// metered state. An explicit UNIDOC_LICENSE_DIR is used as it is; otherwise the
// state goes under the data directory ("<data>/unidoc"), which is the operator's
// persistent, writable volume, instead of the vendor default under the home
// directory that a container often cannot write. Either way the directory is
// created (0700) and a temporary file is written and removed, so an unusable
// directory is found now and not on the first export.
func (m *manager) prepareStateDir(getenv func(string) string) error {
	dir := strings.TrimSpace(getenv(envLicenseDir))
	switch {
	case dir != "":
	case m.dataDir != "":
		dir = filepath.Join(m.dataDir, "unidoc")
		if err := m.setenv(envLicenseDir, dir); err != nil {
			return fmt.Errorf("setting %s: %v", envLicenseDir, err)
		}
		log.Printf("export licence: licence state is kept in %s (set %s to change it)", dir, envLicenseDir)
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("no home directory for the licence state and %s is unset: set %s to a writable directory", envLicenseDir, envLicenseDir)
		}
		dir = filepath.Join(home, ".unidoc")
	}
	if err := checkWritable(dir); err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err // the path is named once, in the message
		}
		return fmt.Errorf("the licence state directory %s is not writable (%v): set %s to a writable directory, or make this one writable", dir, err, envLicenseDir)
	}
	return nil
}

// checkWritable creates dir if needed and proves it can be written to.
func checkWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func (m *manager) applyOne(ctx context.Context, l lib, key string, cacheOff bool) {
	for n := 0; ; n++ {
		err := l.SetMeteredKey(key)
		if err == nil {
			// SetMeteredKey builds a fresh key object with the cache on, so
			// turning it off has to come after.
			if cacheOff {
				l.SetPersistentCache(false)
			}
			m.publish(l.Format(), SourceMetered)
			log.Printf("export licence: %s export licensed (metered)", l.Name())
			return
		}
		if !retryable(err) {
			log.Printf("export licence: %s licensing failed, not retrying: %v", l.Name(), err)
			return
		}
		d := m.schedule(n)
		log.Printf("export licence: %s licence server unreachable (%v); retrying in %s", l.Name(), err, d)
		if !m.sleep(ctx, d) {
			return
		}
	}
}

var (
	std      = newManager()
	startOne sync.Once
)

// Current returns the formats licensed right now.
func Current() Capabilities { return std.current() }

// Start licenses the libraries in the background. It returns immediately and
// is idempotent.
//
// dataDir is the server's data directory; the licence state is kept under it
// unless UNIDOC_LICENSE_DIR is set.
func Start(ctx context.Context, dataDir string) {
	std.dataDir = dataDir
	startOn(&startOne, ctx, std, os.Getenv, realLibs())
}

func startOn(once *sync.Once, ctx context.Context, m *manager, getenv func(string) string, libs []lib) {
	once.Do(func() {
		go m.run(ctx, getenv, libs)
	})
}
