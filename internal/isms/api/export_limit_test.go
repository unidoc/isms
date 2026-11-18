package api

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time      { return f.t }
func (f *fakeClock) add(d time.Duration) { f.t = f.t.Add(d) }
func newFakeClock() *fakeClock           { return &fakeClock{t: time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)} }
func limiterAt(c *fakeClock, u, o int) *exportLimiter {
	l := newExportLimiter(u, o)
	if l != nil {
		l.now = c.now
	}
	return l
}

func mustTake(t *testing.T, l *exportLimiter, user string, org int) func() {
	t.Helper()
	release, wait, ok := l.take(user, org)
	if !ok {
		t.Fatalf("take(%q, %d) refused, wait %s", user, org, wait)
	}
	return release
}

func TestExportLimitUserBucket(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 3, 0)
	for i := 0; i < 3; i++ {
		mustTake(t, l, "a@x", 1)
	}
	_, wait, ok := l.take("a@x", 1)
	if ok {
		t.Fatal("fourth export within the hour allowed")
	}
	// 3 an hour refills one every 20 minutes.
	if wait != 20*time.Minute {
		t.Errorf("wait = %s, want 20m", wait)
	}
	// Another member and the same member in another organization are separate
	// from the org side, but the user bucket is per member across organizations.
	mustTake(t, l, "b@x", 1)
	if _, _, ok := l.take("a@x", 2); ok {
		t.Error("the user limit must follow the member across organizations")
	}
	c.add(20 * time.Minute)
	mustTake(t, l, "a@x", 1)
	if _, _, ok := l.take("a@x", 1); ok {
		t.Error("only one export refilled after 20 minutes")
	}
}

func TestExportLimitOrgBucket(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 0, 2)
	mustTake(t, l, "a@x", 1)
	mustTake(t, l, "b@x", 1)
	_, wait, ok := l.take("c@x", 1)
	if ok {
		t.Fatal("third export for the organization allowed")
	}
	if wait != 12*time.Hour {
		t.Errorf("wait = %s, want 12h (2 a day)", wait)
	}
	mustTake(t, l, "c@x", 2) // another organization has its own allowance
}

// A refusal by one bucket takes nothing from the other, and the longer wait
// is reported.
func TestExportLimitBothOrNeither(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 1, 5)
	mustTake(t, l, "a@x", 1)
	for i := 0; i < 3; i++ {
		if _, _, ok := l.take("a@x", 1); ok {
			t.Fatal("user over the limit allowed")
		}
	}
	// The org spent 1 of 5: four more members can still export.
	for _, u := range []string{"b@x", "c@x", "d@x", "e@x"} {
		mustTake(t, l, u, 1)
	}
	_, wait, ok := l.take("f@x", 1)
	if ok {
		t.Fatal("org over the limit allowed")
	}
	if want := 24 * time.Hour / 5; wait != want {
		t.Errorf("wait = %s, want %s", wait, want)
	}

	// The org is empty now; a member with a full user bucket is refused and
	// keeps that user token.
	c2 := newFakeClock()
	l2 := limiterAt(c2, 2, 1)
	mustTake(t, l2, "a@x", 1)
	if _, wait, ok := l2.take("b@x", 1); ok || wait != 24*time.Hour {
		t.Fatalf("org-empty refusal: ok=%v wait=%s", ok, wait)
	}
	if got := l2.users["b@x"].tokens; got != 2 {
		t.Errorf("refused take spent user tokens: %v left, want 2", got)
	}
}

func TestExportLimitReleaseGivesBack(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 1, 1)
	release := mustTake(t, l, "a@x", 1)
	if _, _, ok := l.take("a@x", 1); ok {
		t.Fatal("over the limit allowed")
	}
	release()
	mustTake(t, l, "a@x", 1)
	// Giving back never overfills.
	c.add(48 * time.Hour)
	r := mustTake(t, l, "a@x", 1)
	r()
	r()
	if got := l.users["a@x"].tokens; got != 1 {
		t.Errorf("tokens after double release = %v, want 1", got)
	}
}

func TestExportLimitSweepDropsRefilledBuckets(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 4, 10)
	for i := 0; i < 4; i++ {
		mustTake(t, l, "a@x", 1)
	}
	c.add(30 * time.Minute) // two of four back: not full, kept
	mustTake(t, l, "b@x", 2)
	c.add(exportLimitSweepEvery)
	l.take("c@x", 3)
	if _, ok := l.users["a@x"]; !ok {
		t.Error("a bucket that is not full yet was dropped")
	}
	c.add(25 * time.Hour)
	l.take("c@x", 3)
	if len(l.users) != 1 || len(l.orgs) != 1 {
		t.Errorf("full buckets kept: users %d orgs %d, want only c@x / org 3", len(l.users), len(l.orgs))
	}
}

func TestExportLimitOffAndConfig(t *testing.T) {
	var l *exportLimiter
	if _, _, ok := l.take("a@x", 1); !ok {
		t.Error("a nil limiter must allow")
	}
	if newExportLimiter(0, 0) != nil {
		t.Error("both limits 0 must give no limiter")
	}

	t.Setenv("ISMS_RATE_LIMIT", "0")
	if exportLimiterFromEnv() != nil {
		t.Error("ISMS_RATE_LIMIT=0 must turn the export limit off")
	}
	t.Setenv("ISMS_RATE_LIMIT", "")
	for _, tc := range []struct {
		user, org         string
		wantUser, wantOrg int
		nilLimiter        bool
	}{
		{"", "", exportLimitUserHourlyDefault, exportLimitOrgDailyDefault, false},
		{"5", "50", 5, 50, false},
		{"0", "50", 0, 50, false},
		{"abc", "-3", exportLimitUserHourlyDefault, exportLimitOrgDailyDefault, false},
		{"0", "0", 0, 0, true},
	} {
		t.Setenv(envExportLimitUserHourly, tc.user)
		t.Setenv(envExportLimitOrgDaily, tc.org)
		got := exportLimiterFromEnv()
		if tc.nilLimiter {
			if got != nil {
				t.Errorf("%q/%q: want no limiter", tc.user, tc.org)
			}
			continue
		}
		if got == nil || got.perUser != tc.wantUser || got.perOrg != tc.wantOrg {
			t.Errorf("%q/%q: got %+v, want user %d org %d", tc.user, tc.org, got, tc.wantUser, tc.wantOrg)
		}
	}
}
