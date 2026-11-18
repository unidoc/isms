package api

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Export rate limit. Every PDF/DOCX export spends a metered UniDoc credit, and
// any member who can read a document may export it, a read-only API key
// included. Two token buckets cap that spend:
//
//   - per user (the member's email; an API key counts as its owner), across
//     organizations: ISMS_EXPORT_LIMIT_USER_HOURLY, default 30 an hour;
//   - per organization: ISMS_EXPORT_LIMIT_ORG_DAILY, default 500 a day.
//
// A bucket starts full and refills evenly, so a member can export up to the
// hourly number at once and then about one every two minutes. 0 turns that one
// limit off; an invalid value logs a warning and keeps the default.
// ISMS_RATE_LIMIT=0 turns both off, the same dev/test switch as the other
// limits. Read once, when the server is built.
//
// The buckets live in this process: a restart refills them, and each server
// instance counts on its own. A token is taken only when a render is about to
// run, after the format, licence, document and version checks, so a refused
// request costs nothing; a render that fails gives its token back.
const (
	envExportLimitUserHourly = "ISMS_EXPORT_LIMIT_USER_HOURLY"
	envExportLimitOrgDaily   = "ISMS_EXPORT_LIMIT_ORG_DAILY"

	exportLimitUserHourlyDefault = 30
	exportLimitOrgDailyDefault   = 500

	// exportLimitSweepEvery is how often idle buckets are dropped. A bucket idle
	// for its whole refill period is full again, the same as a new one.
	exportLimitSweepEvery = 10 * time.Minute
)

// exportBucket is one token bucket: size tokens, refilled one per every.
type exportBucket struct {
	size   float64
	every  time.Duration
	tokens float64
	last   time.Time
}

func newExportBucket(perPeriod int, period time.Duration, now time.Time) *exportBucket {
	return &exportBucket{size: float64(perPeriod), every: period / time.Duration(perPeriod), tokens: float64(perPeriod), last: now}
}

func (b *exportBucket) refill(now time.Time) {
	if now.After(b.last) {
		b.tokens = min(b.size, b.tokens+float64(now.Sub(b.last))/float64(b.every))
		b.last = now
	}
}

// wait is how long until one token is there (0 when it already is).
func (b *exportBucket) wait() time.Duration {
	if b.tokens >= 1 {
		return 0
	}
	return time.Duration((1 - b.tokens) * float64(b.every))
}

// full reports whether the bucket would be full by now, so dropping it changes nothing.
func (b *exportBucket) full(now time.Time) bool {
	return float64(now.Sub(b.last))/float64(b.every)+b.tokens >= b.size
}

// exportLimiter holds the per-user and per-organization buckets. A nil
// *exportLimiter allows everything. A zero perUser or perOrg turns that limit
// off.
type exportLimiter struct {
	mu        sync.Mutex
	perUser   int // per hour
	perOrg    int // per day
	users     map[string]*exportBucket
	orgs      map[int]*exportBucket
	now       func() time.Time
	lastSweep time.Time
}

func newExportLimiter(perUserHourly, perOrgDaily int) *exportLimiter {
	if perUserHourly <= 0 && perOrgDaily <= 0 {
		return nil
	}
	return &exportLimiter{
		perUser: max(perUserHourly, 0),
		perOrg:  max(perOrgDaily, 0),
		users:   map[string]*exportBucket{},
		orgs:    map[int]*exportBucket{},
		now:     time.Now,
	}
}

// exportLimiterFromEnv builds the limiter from the environment, or nil when
// ISMS_RATE_LIMIT=0 or both limits are 0.
func exportLimiterFromEnv() *exportLimiter {
	if rateLimitDisabled() {
		return nil
	}
	user := exportLimitEnv(envExportLimitUserHourly, exportLimitUserHourlyDefault)
	org := exportLimitEnv(envExportLimitOrgDaily, exportLimitOrgDailyDefault)
	return newExportLimiter(user, org)
}

func exportLimitEnv(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		log.Printf("export: %s must be a whole number (0 turns the limit off); using %d", name, def)
		return def
	}
	return n
}

// take takes one export for user in org from both buckets, or from neither.
// It returns ok and a release func that gives the export back (for a render
// that failed), or !ok and how long until an export would be allowed.
func (l *exportLimiter) take(user string, org int) (release func(), retryAfter time.Duration, ok bool) {
	if l == nil {
		return func() {}, 0, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	var bs []*exportBucket
	if l.perUser > 0 {
		b := l.users[user]
		if b == nil {
			b = newExportBucket(l.perUser, time.Hour, now)
			l.users[user] = b
		}
		bs = append(bs, b)
	}
	if l.perOrg > 0 {
		b := l.orgs[org]
		if b == nil {
			b = newExportBucket(l.perOrg, 24*time.Hour, now)
			l.orgs[org] = b
		}
		bs = append(bs, b)
	}
	for _, b := range bs {
		b.refill(now)
		retryAfter = max(retryAfter, b.wait())
	}
	if retryAfter > 0 {
		return nil, retryAfter, false
	}
	for _, b := range bs {
		b.tokens--
	}
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		now := l.now()
		for _, b := range bs {
			b.refill(now)
			b.tokens = min(b.size, b.tokens+1)
		}
	}, 0, true
}

// sweep drops buckets that are full again, so the maps hold only members and
// organizations that exported recently. Called with l.mu held.
func (l *exportLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < exportLimitSweepEvery {
		return
	}
	l.lastSweep = now
	for k, b := range l.users {
		if b.full(now) {
			delete(l.users, k)
		}
	}
	for k, b := range l.orgs {
		if b.full(now) {
			delete(l.orgs, k)
		}
	}
}
