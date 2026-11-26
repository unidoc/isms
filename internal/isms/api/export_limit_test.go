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

// A bucket holds half the configured number, rounded up, and refills at the
// full rate.
func TestExportLimitBucketHoldsHalf(t *testing.T) {
	for _, tc := range []struct{ limit, atOnce int }{{1, 1}, {2, 1}, {3, 2}, {30, 15}, {500, 250}} {
		c := newFakeClock()
		l := limiterAt(c, tc.limit, 0)
		n := 0
		for {
			if _, _, ok := l.take("a@x", 1); !ok {
				break
			}
			n++
		}
		if n != tc.atOnce {
			t.Errorf("limit %d: %d exports at once, want %d", tc.limit, n, tc.atOnce)
		}
	}
}

// The worst case in an hour is half the number plus an hour's refill: 45 for
// the default 30 counting both ends of the hour, 44 in [0, 60) as measured
// here, against 60 for a full-size bucket.
func TestExportLimitWorstHour(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 30, 0)
	n := 0
	for m := 0; m < 60; m++ {
		for {
			if _, _, ok := l.take("a@x", 1); !ok {
				break
			}
			n++
		}
		c.add(time.Minute)
	}
	if n != 15+29 { // the refill at minute 60 falls outside the hour
		t.Errorf("exports in the first hour = %d, want 44", n)
	}
}

func TestExportLimitUserBucket(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 6, 0) // 3 at once, one back every 10 minutes
	for i := 0; i < 3; i++ {
		mustTake(t, l, "a@x", 1)
	}
	_, wait, ok := l.take("a@x", 1)
	if ok {
		t.Fatal("fourth export at once allowed")
	}
	if wait != 10*time.Minute {
		t.Errorf("wait = %s, want 10m", wait)
	}
	// Another member is separate, but the user bucket follows the member
	// across organizations.
	mustTake(t, l, "b@x", 1)
	if _, _, ok := l.take("a@x", 2); ok {
		t.Error("the user limit must follow the member across organizations")
	}
	c.add(10 * time.Minute)
	mustTake(t, l, "a@x", 1)
	if _, _, ok := l.take("a@x", 1); ok {
		t.Error("only one export refilled after 10 minutes")
	}
}

func TestExportLimitOrgBucket(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 0, 4) // 2 at once, one back every 6 hours
	mustTake(t, l, "a@x", 1)
	mustTake(t, l, "b@x", 1)
	_, wait, ok := l.take("c@x", 1)
	if ok {
		t.Fatal("third export for the organization allowed")
	}
	if wait != 6*time.Hour {
		t.Errorf("wait = %s, want 6h (4 a day)", wait)
	}
	mustTake(t, l, "c@x", 2) // another organization has its own allowance
}

// A refusal by one bucket takes nothing from the other, and the longer wait
// is reported.
func TestExportLimitBothOrNeither(t *testing.T) {
	c := newFakeClock()
	l := limiterAt(c, 2, 10) // user: 1 at once; org: 5 at once
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
	if want := 24 * time.Hour / 10; wait != want {
		t.Errorf("wait = %s, want %s", wait, want)
	}

	// The org is empty now; a member with a full user bucket is refused and
	// keeps those user tokens.
	c2 := newFakeClock()
	l2 := limiterAt(c2, 4, 2) // user: 2 at once; org: 1 at once
	mustTake(t, l2, "a@x", 1)
	if _, wait, ok := l2.take("b@x", 1); ok || wait != 12*time.Hour {
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
	l := limiterAt(c, 8, 10) // user: 4 at once, one back every 7.5 minutes
	for i := 0; i < 4; i++ {
		mustTake(t, l, "a@x", 1)
	}
	c.add(15 * time.Minute) // two of four back: not full, kept
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
