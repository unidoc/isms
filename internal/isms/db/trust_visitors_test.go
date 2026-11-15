package db

// Round-trip tests for trust center visitors: one-time links, sessions and
// blocking. They run only when ISMS_TEST_DATABASE_URL points at a migrated
// database:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/db/ -run TrustVisitor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

// visitorHash returns a fresh sha256-hex token hash, the only form the DB
// layer ever sees.
func visitorHash(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

const (
	linkTTL    = 15 * time.Minute
	sessionTTL = 7 * 24 * time.Hour
)

// signIn creates a link and redeems it, returning the visitor and session hash.
func signIn(t *testing.T, d *DB, orgID int, email string) (*TrustVisitor, string) {
	t.Helper()
	ctx := context.Background()
	link, sess := visitorHash(t), visitorHash(t)
	if err := d.CreateTrustVisitorLink(ctx, orgID, email, link, linkTTL); err != nil {
		t.Fatalf("create link: %v", err)
	}
	v, err := d.RedeemTrustVisitorLink(ctx, orgID, link, "Ada Auditor", "Audit Co", sess, sessionTTL)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	return v, sess
}

func TestTrustVisitorLinkRedeemOnce(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vlonce")

	link := visitorHash(t)
	if err := d.CreateTrustVisitorLink(ctx, orgID, "  Ada@Example.COM ", link, linkTTL); err != nil {
		t.Fatalf("create: %v", err)
	}
	v, err := d.RedeemTrustVisitorLink(ctx, orgID, link, " Ada ", " Audit Co ", visitorHash(t), sessionTTL)
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if v.Email != "ada@example.com" || v.Name != "Ada" || v.Company != "Audit Co" || v.VerifiedAt == nil || v.Blocked {
		t.Errorf("visitor = %+v", v)
	}
	if _, err := d.RedeemTrustVisitorLink(ctx, orgID, link, "Ada", "Audit Co", visitorHash(t), sessionTTL); !errors.Is(err, ErrTrustLinkInvalid) {
		t.Errorf("second redeem err = %v, want ErrTrustLinkInvalid", err)
	}
	var sessions int
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM trust_visitor_sessions WHERE organization_id = $1`, orgID).Scan(&sessions); err != nil || sessions != 1 {
		t.Errorf("sessions = %d, %v, want 1", sessions, err)
	}
}

func TestTrustVisitorLinkUnknownExpiredAndOtherOrg(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA := trustTestOrg(t, d, "vlA")
	orgB := trustTestOrg(t, d, "vlB")

	if _, err := d.RedeemTrustVisitorLink(ctx, orgA, visitorHash(t), "A", "B", visitorHash(t), sessionTTL); !errors.Is(err, ErrTrustLinkInvalid) {
		t.Errorf("unknown link err = %v", err)
	}

	expired := visitorHash(t)
	if err := d.CreateTrustVisitorLink(ctx, orgA, "e@example.com", expired, -time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RedeemTrustVisitorLink(ctx, orgA, expired, "A", "B", visitorHash(t), sessionTTL); !errors.Is(err, ErrTrustLinkInvalid) {
		t.Errorf("expired redeem err = %v", err)
	}
	if _, _, err := d.PeekTrustVisitorLink(ctx, orgA, expired); !errors.Is(err, ErrTrustLinkInvalid) {
		t.Errorf("expired peek err = %v", err)
	}

	link := visitorHash(t)
	if err := d.CreateTrustVisitorLink(ctx, orgA, "x@example.com", link, linkTTL); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RedeemTrustVisitorLink(ctx, orgB, link, "A", "B", visitorHash(t), sessionTTL); !errors.Is(err, ErrTrustLinkInvalid) {
		t.Errorf("other org redeem err = %v", err)
	}
	if _, _, err := d.PeekTrustVisitorLink(ctx, orgB, link); !errors.Is(err, ErrTrustLinkInvalid) {
		t.Errorf("other org peek err = %v", err)
	}
	// The failed attempt in org B must not have consumed it.
	if _, err := d.RedeemTrustVisitorLink(ctx, orgA, link, "A", "B", visitorHash(t), sessionTTL); err != nil {
		t.Errorf("redeem in own org after foreign attempt: %v", err)
	}
}

func TestTrustVisitorProfileRequiredLeavesLinkUsable(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vlprof")

	link := visitorHash(t)
	if err := d.CreateTrustVisitorLink(ctx, orgID, "new@example.com", link, linkTTL); err != nil {
		t.Fatal(err)
	}
	email, v, err := d.PeekTrustVisitorLink(ctx, orgID, link)
	if err != nil || email != "new@example.com" || v != nil {
		t.Fatalf("peek = %q, %v, %v; want email, nil visitor", email, v, err)
	}
	for _, c := range [][2]string{{"", ""}, {"Ada", ""}, {"", "Co"}, {"  ", "Co"}} {
		if _, err := d.RedeemTrustVisitorLink(ctx, orgID, link, c[0], c[1], visitorHash(t), sessionTTL); !errors.Is(err, ErrTrustVisitorProfileRequired) {
			t.Errorf("redeem(%q,%q) err = %v, want ErrTrustVisitorProfileRequired", c[0], c[1], err)
		}
	}
	var n int
	_ = d.pool.QueryRow(ctx, `SELECT count(*) FROM trust_visitors WHERE organization_id = $1`, orgID).Scan(&n)
	if n != 0 {
		t.Errorf("profile-required left %d visitor rows, want 0", n)
	}
	if _, err := d.RedeemTrustVisitorLink(ctx, orgID, link, "Ada", "Co", visitorHash(t), sessionTTL); err != nil {
		t.Errorf("redeem with profile after refusal: %v", err)
	}
	// Over-long profile fields are refused and leave the link usable.
	link2 := visitorHash(t)
	_ = d.CreateTrustVisitorLink(ctx, orgID, "long@example.com", link2, linkTTL)
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := d.RedeemTrustVisitorLink(ctx, orgID, link2, string(long), "Co", visitorHash(t), sessionTTL); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("long name err = %v, want ErrTrustCenterInvalid", err)
	}
	if _, err := d.RedeemTrustVisitorLink(ctx, orgID, link2, "Ok", "Co", visitorHash(t), sessionTTL); err != nil {
		t.Errorf("redeem after long-name refusal: %v", err)
	}
}

func TestTrustVisitorConcurrentDoubleRedeem(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vlrace")

	for round := 0; round < 5; round++ {
		link := visitorHash(t)
		email := "race" + string(rune('a'+round)) + "@example.com"
		if err := d.CreateTrustVisitorLink(ctx, orgID, email, link, linkTTL); err != nil {
			t.Fatal(err)
		}
		const workers = 8
		errs := make([]error, workers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, errs[i] = d.RedeemTrustVisitorLink(ctx, orgID, link, "Ada", "Co", visitorHash(t), sessionTTL)
			}(i)
		}
		close(start)
		wg.Wait()
		ok, invalid := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrTrustLinkInvalid):
				invalid++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}
		if ok != 1 || invalid != workers-1 {
			t.Errorf("round %d: %d succeeded, %d invalid; want 1 and %d", round, ok, invalid, workers-1)
		}
		var sessions int
		_ = d.pool.QueryRow(ctx, `
			SELECT count(*) FROM trust_visitor_sessions s JOIN trust_visitors v ON v.id = s.visitor_id
			WHERE s.organization_id = $1 AND v.email = $2`, orgID, email).Scan(&sessions)
		if sessions != 1 {
			t.Errorf("round %d: %d sessions for the link, want 1", round, sessions)
		}
	}
}

func TestTrustVisitorBlockedRedeemKeepsLink(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vlblock")

	v, _ := signIn(t, d, orgID, "b@example.com")
	if _, err := d.SetTrustVisitorBlocked(ctx, orgID, v.ID, true, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	link := visitorHash(t)
	_ = d.CreateTrustVisitorLink(ctx, orgID, "b@example.com", link, linkTTL)
	if _, err := d.RedeemTrustVisitorLink(ctx, orgID, link, "", "", visitorHash(t), sessionTTL); !errors.Is(err, ErrTrustVisitorBlocked) {
		t.Fatalf("blocked redeem err = %v, want ErrTrustVisitorBlocked", err)
	}
	var used *time.Time
	if err := d.pool.QueryRow(ctx, `SELECT used_at FROM trust_visitor_links WHERE token_hash = $1`, link).Scan(&used); err != nil || used != nil {
		t.Errorf("link used_at = %v, %v; want nil", used, err)
	}
	// Unblocked, the same link works.
	if _, err := d.SetTrustVisitorBlocked(ctx, orgID, v.ID, false, "mgr@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RedeemTrustVisitorLink(ctx, orgID, link, "", "", visitorHash(t), sessionTTL); err != nil {
		t.Errorf("redeem after unblock: %v", err)
	}
}

func TestTrustVisitorSessionLookup(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA := trustTestOrg(t, d, "vsA")
	orgB := trustTestOrg(t, d, "vsB")

	v, sess := signIn(t, d, orgA, "s@example.com")
	got, err := d.GetTrustVisitorSession(ctx, orgA, sess)
	if err != nil || got.ID != v.ID || got.Email != "s@example.com" {
		t.Fatalf("lookup = %+v, %v", got, err)
	}
	if _, err := d.GetTrustVisitorSession(ctx, orgB, sess); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("other org lookup err = %v, want ErrTrustVisitorNotFound", err)
	}
	if _, err := d.GetTrustVisitorSession(ctx, orgA, visitorHash(t)); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("unknown session err = %v", err)
	}

	// expired
	_, _ = d.pool.Exec(ctx, `UPDATE trust_visitor_sessions SET expires_at = now() - interval '1 second' WHERE token_hash = $1`, sess)
	if _, err := d.GetTrustVisitorSession(ctx, orgA, sess); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("expired session err = %v", err)
	}

	// revoked (sign-out)
	_, sess2 := signIn(t, d, orgA, "s2@example.com")
	if err := d.RevokeTrustVisitorSession(ctx, orgA, sess2); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetTrustVisitorSession(ctx, orgA, sess2); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("revoked session err = %v", err)
	}
	// sign-out of an unknown or foreign token is not an error and touches nothing
	if err := d.RevokeTrustVisitorSession(ctx, orgA, visitorHash(t)); err != nil {
		t.Errorf("sign-out unknown: %v", err)
	}

	// blocked visitor: session row is still active but must not resolve
	v3, sess3 := signIn(t, d, orgA, "s3@example.com")
	_, _ = d.pool.Exec(ctx, `UPDATE trust_visitors SET blocked = true WHERE id = $1`, v3.ID)
	if _, err := d.GetTrustVisitorSession(ctx, orgA, sess3); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("blocked visitor session err = %v", err)
	}
}

func TestTrustVisitorSessionLastSeenThrottled(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vsseen")
	v, sess := signIn(t, d, orgID, "seen@example.com")

	old := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	_, _ = d.pool.Exec(ctx, `UPDATE trust_visitor_sessions SET last_seen_at = $2 WHERE token_hash = $1`, sess, old)
	_, _ = d.pool.Exec(ctx, `UPDATE trust_visitors SET last_seen = $2 WHERE id = $1`, v.ID, old)
	if _, err := d.GetTrustVisitorSession(ctx, orgID, sess); err != nil {
		t.Fatal(err)
	}
	var ls, vs time.Time
	_ = d.pool.QueryRow(ctx, `SELECT last_seen_at FROM trust_visitor_sessions WHERE token_hash = $1`, sess).Scan(&ls)
	_ = d.pool.QueryRow(ctx, `SELECT last_seen FROM trust_visitors WHERE id = $1`, v.ID).Scan(&vs)
	if time.Since(ls) > time.Minute || time.Since(vs) > time.Minute {
		t.Errorf("last seen not refreshed after an hour: session %v visitor %v", ls, vs)
	}
	// Within five minutes a second lookup leaves the stamp alone.
	recent := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	_, _ = d.pool.Exec(ctx, `UPDATE trust_visitor_sessions SET last_seen_at = $2 WHERE token_hash = $1`, sess, recent)
	_, _ = d.GetTrustVisitorSession(ctx, orgID, sess)
	var again time.Time
	_ = d.pool.QueryRow(ctx, `SELECT last_seen_at FROM trust_visitor_sessions WHERE token_hash = $1`, sess).Scan(&again)
	if !again.Equal(recent) {
		t.Errorf("last_seen_at rewritten within 5 minutes: %v -> %v", recent, again)
	}
}

func TestTrustVisitorBlockRevokesAndUnblockDoesNotResurrect(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA := trustTestOrg(t, d, "vbA")
	orgB := trustTestOrg(t, d, "vbB")

	v, s1 := signIn(t, d, orgA, "blk@example.com")
	// a second session for the same visitor
	link, s2 := visitorHash(t), visitorHash(t)
	_ = d.CreateTrustVisitorLink(ctx, orgA, "blk@example.com", link, linkTTL)
	if _, err := d.RedeemTrustVisitorLink(ctx, orgA, link, "", "", s2, sessionTTL); err != nil {
		t.Fatal(err)
	}

	if _, err := d.SetTrustVisitorBlocked(ctx, orgB, v.ID, true, "mgr"); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("block via other org err = %v, want ErrTrustVisitorNotFound", err)
	}
	if _, err := d.SetTrustVisitorBlocked(ctx, orgA, v.ID+999999, true, "mgr"); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("block unknown err = %v", err)
	}

	got, err := d.SetTrustVisitorBlocked(ctx, orgA, v.ID, true, "mgr@example.com")
	if err != nil || !got.Blocked || got.BlockedAt == nil || got.BlockedBy == nil || *got.BlockedBy != "mgr@example.com" {
		t.Fatalf("block = %+v, %v", got, err)
	}
	for _, s := range []string{s1, s2} {
		if _, err := d.GetTrustVisitorSession(ctx, orgA, s); !errors.Is(err, ErrTrustVisitorNotFound) {
			t.Errorf("session valid after block: %v", err)
		}
	}
	var revoked int
	_ = d.pool.QueryRow(ctx, `SELECT count(*) FROM trust_visitor_sessions WHERE visitor_id = $1 AND revoked_at IS NOT NULL`, v.ID).Scan(&revoked)
	if revoked != 2 {
		t.Errorf("revoked rows = %d, want 2", revoked)
	}

	got, err = d.SetTrustVisitorBlocked(ctx, orgA, v.ID, false, "mgr@example.com")
	if err != nil || got.Blocked || got.BlockedAt != nil || got.BlockedBy != nil {
		t.Fatalf("unblock = %+v, %v", got, err)
	}
	for _, s := range []string{s1, s2} {
		if _, err := d.GetTrustVisitorSession(ctx, orgA, s); !errors.Is(err, ErrTrustVisitorNotFound) {
			t.Errorf("unblock resurrected a session: %v", err)
		}
	}
}

func TestTrustVisitorRevokeSessionsAndList(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA := trustTestOrg(t, d, "vrA")
	orgB := trustTestOrg(t, d, "vrB")

	v1, s1 := signIn(t, d, orgA, "one@example.com")
	v2, s2 := signIn(t, d, orgA, "two@example.com")
	signIn(t, d, orgB, "one@example.com") // same email, other org: a separate visitor
	_, _ = d.pool.Exec(ctx, `UPDATE trust_visitors SET last_seen = now() - interval '1 hour' WHERE id = $1`, v1.ID)

	list, err := d.ListTrustVisitors(ctx, orgA)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d, %v", len(list), err)
	}
	if list[0].ID != v2.ID || list[1].ID != v1.ID {
		t.Errorf("order = %d,%d; want newest last_seen first", list[0].ID, list[1].ID)
	}
	if list[0].ActiveSessions != 1 || list[1].ActiveSessions != 1 {
		t.Errorf("active sessions = %d,%d", list[0].ActiveSessions, list[1].ActiveSessions)
	}
	if orgBList, _ := d.ListTrustVisitors(ctx, orgB); len(orgBList) != 1 || orgBList[0].ID == v1.ID {
		t.Errorf("org B list leaks or is wrong: %+v", orgBList)
	}

	if n, err := d.RevokeTrustVisitorSessions(ctx, orgB, v1.ID); err != nil || n != 0 {
		t.Errorf("revoke via other org = %d, %v; want 0", n, err)
	}
	if n, err := d.RevokeTrustVisitorSessions(ctx, orgA, v1.ID); err != nil || n != 1 {
		t.Errorf("revoke = %d, %v; want 1", n, err)
	}
	if n, _ := d.RevokeTrustVisitorSessions(ctx, orgA, v1.ID); n != 0 {
		t.Errorf("second revoke = %d, want 0", n)
	}
	if _, err := d.GetTrustVisitorSession(ctx, orgA, s1); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("revoked session still valid: %v", err)
	}
	if _, err := d.GetTrustVisitorSession(ctx, orgA, s2); err != nil {
		t.Errorf("other visitor's session was touched: %v", err)
	}
	list, _ = d.ListTrustVisitors(ctx, orgA)
	for _, v := range list {
		if v.ID == v1.ID && v.ActiveSessions != 0 {
			t.Errorf("revoked visitor active sessions = %d", v.ActiveSessions)
		}
	}
}

func TestTrustVisitorProfileAskedOnce(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vpone")

	v, _ := signIn(t, d, orgID, "once@example.com")
	link := visitorHash(t)
	_ = d.CreateTrustVisitorLink(ctx, orgID, "once@example.com", link, linkTTL)
	email, peek, err := d.PeekTrustVisitorLink(ctx, orgID, link)
	if err != nil || email != "once@example.com" || peek == nil || peek.ID != v.ID {
		t.Fatalf("peek = %q, %+v, %v", email, peek, err)
	}
	// a later sign-in with a different name/company, or none, changes nothing
	got, err := d.RedeemTrustVisitorLink(ctx, orgID, link, "Mallory", "Evil Inc", visitorHash(t), sessionTTL)
	if err != nil || got.Name != "Ada Auditor" || got.Company != "Audit Co" || got.ID != v.ID {
		t.Fatalf("second sign-in = %+v, %v", got, err)
	}
	link2 := visitorHash(t)
	_ = d.CreateTrustVisitorLink(ctx, orgID, "once@example.com", link2, linkTTL)
	if got, err := d.RedeemTrustVisitorLink(ctx, orgID, link2, "", "", visitorHash(t), sessionTTL); err != nil || got.Name != "Ada Auditor" {
		t.Errorf("sign-in without profile = %+v, %v", got, err)
	}

	byEmail, err := d.GetTrustVisitorByEmail(ctx, orgID, " ONCE@example.com ")
	if err != nil || byEmail.ID != v.ID {
		t.Errorf("by email = %+v, %v", byEmail, err)
	}
	if _, err := d.GetTrustVisitorByEmail(ctx, orgID, "nobody@example.com"); !errors.Is(err, ErrTrustVisitorNotFound) {
		t.Errorf("by email unknown err = %v", err)
	}
}

func TestTrustVisitorLinkCounts(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA := trustTestOrg(t, d, "vcA")
	orgB := trustTestOrg(t, d, "vcB")

	for i := 0; i < 3; i++ {
		_ = d.CreateTrustVisitorLink(ctx, orgA, "c1@example.com", visitorHash(t), linkTTL)
	}
	_ = d.CreateTrustVisitorLink(ctx, orgA, "C2@example.com", visitorHash(t), linkTTL)
	_ = d.CreateTrustVisitorLink(ctx, orgB, "c1@example.com", visitorHash(t), linkTTL)

	since := time.Now().Add(-time.Hour)
	if n, err := d.CountTrustVisitorLinks(ctx, orgA, "c1@example.com", since); err != nil || n != 3 {
		t.Errorf("per email = %d, %v; want 3", n, err)
	}
	if n, _ := d.CountTrustVisitorLinks(ctx, orgA, " C2@example.com", since); n != 1 {
		t.Errorf("per email, normalised = %d, want 1", n)
	}
	if n, _ := d.CountTrustVisitorLinks(ctx, orgA, "", since); n != 4 {
		t.Errorf("org-wide = %d, want 4", n)
	}
	if n, _ := d.CountTrustVisitorLinks(ctx, orgA, "", time.Now().Add(time.Minute)); n != 0 {
		t.Errorf("future since = %d, want 0", n)
	}
}

func TestTrustVisitorsCreateNoUsers(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vnouser")

	count := func() (int, int) {
		var u, m int
		_ = d.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE lower(email) = 'ghost@example.com'`).Scan(&u)
		_ = d.pool.QueryRow(ctx, `SELECT count(*) FROM organization_members WHERE organization_id = $1`, orgID).Scan(&m)
		return u, m
	}
	signIn(t, d, orgID, "ghost@example.com")
	if u, m := count(); u != 0 || m != 0 {
		t.Errorf("sign-in created users=%d members=%d, want 0 and 0", u, m)
	}
}

func TestTrustCenterDomainBlocked(t *testing.T) {
	def := &TrustCenterSettings{}
	if !def.DomainBlocked("x@gmail.com") || !def.DomainBlocked(" X@GMAIL.COM ") || !def.DomainBlocked("a@outlook.com") {
		t.Error("default list does not block gmail/outlook")
	}
	if def.DomainBlocked("x@acme.test") || def.DomainBlocked("x@mail.gmail.com") {
		t.Error("default list blocks a company domain or matches a subdomain")
	}
	if def.DomainBlocked("no-at-sign") || def.DomainBlocked("") {
		t.Error("unexpected result for a malformed address")
	}
	if len(def.EffectiveBlockedEmailDomains()) != len(TrustBuiltinBlockedEmailDomains()) {
		t.Error("nil list should be the built-in list")
	}

	custom := &TrustCenterSettings{BlockedEmailDomains: []string{"Rival.test", "gmail.com"}}
	if !custom.DomainBlocked("a@rival.test") || !custom.DomainBlocked("a@gmail.com") || custom.DomainBlocked("a@outlook.com") {
		t.Error("explicit list not applied exactly")
	}
	empty := &TrustCenterSettings{BlockedEmailDomains: []string{}}
	if empty.DomainBlocked("a@gmail.com") || len(empty.EffectiveBlockedEmailDomains()) != 0 {
		t.Error("empty list should allow every domain")
	}
	// the built-in list must not be mutable through the returned slice
	b := TrustBuiltinBlockedEmailDomains()
	b[0] = "mutated.test"
	if TrustBuiltinBlockedEmailDomains()[0] == "mutated.test" {
		t.Error("built-in list is shared and mutable")
	}
}

func TestTrustCenterSettingsVisitorFields(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "vset")

	s, err := d.GetTrustCenterSettings(ctx, orgID)
	if err != nil || s.BlockedEmailDomains != nil || s.PrivacyNoticeURL != "" {
		t.Fatalf("missing row = %+v, %v; want nil domains, empty URL", s, err)
	}
	if err := d.UpsertTrustCenterSettings(ctx, orgID, &TrustCenterSettings{DefaultAccessLevel: TrustAccessGated}, "mgr"); err != nil {
		t.Fatal(err)
	}
	s, err = d.GetTrustCenterSettings(ctx, orgID)
	if err != nil || s.BlockedEmailDomains != nil || s.PrivacyNoticeURL != "" {
		t.Fatalf("NULL row = %+v, %v; want nil domains", s, err)
	}
	// An explicit array, and an explicit empty array, read back distinctly.
	_, _ = d.pool.Exec(ctx, `UPDATE trust_center_settings SET blocked_email_domains = ARRAY['a.test'], privacy_notice_url = 'https://acme.test/privacy' WHERE organization_id = $1`, orgID)
	s, _ = d.GetTrustCenterSettings(ctx, orgID)
	if len(s.BlockedEmailDomains) != 1 || s.BlockedEmailDomains[0] != "a.test" || s.PrivacyNoticeURL != "https://acme.test/privacy" {
		t.Errorf("explicit = %+v", s)
	}
	_, _ = d.pool.Exec(ctx, `UPDATE trust_center_settings SET blocked_email_domains = '{}' WHERE organization_id = $1`, orgID)
	s, _ = d.GetTrustCenterSettings(ctx, orgID)
	if s.BlockedEmailDomains == nil || len(s.BlockedEmailDomains) != 0 {
		t.Errorf("empty array read back as %#v, want non-nil empty", s.BlockedEmailDomains)
	}
	// Upsert (whole-row replace) must not wipe the new columns.
	if err := d.UpsertTrustCenterSettings(ctx, orgID, &TrustCenterSettings{Enabled: true, DefaultAccessLevel: TrustAccessGated}, "mgr"); err != nil {
		t.Fatal(err)
	}
	s, _ = d.GetTrustCenterSettings(ctx, orgID)
	if s.BlockedEmailDomains == nil || s.PrivacyNoticeURL != "https://acme.test/privacy" {
		t.Errorf("upsert wiped visitor settings: %+v", s)
	}
}

// capsFor builds caps where only the named one is tight.
func capsFor(perEmail, perOrg, perAddress int) TrustLinkCaps {
	return TrustLinkCaps{
		PerEmail: perEmail, EmailWindow: 15 * time.Minute,
		PerOrg: perOrg, OrgWindow: time.Hour,
		PerAddress: perAddress, AddressWindow: time.Hour,
	}
}

// runCapped creates one link per address in parallel and tallies the outcomes.
func runCapped(t *testing.T, d *DB, orgID int, emails []string, caps TrustLinkCaps) (created int, reasons map[string]int) {
	t.Helper()
	var mu sync.Mutex
	reasons = map[string]int{}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, em := range emails {
		wg.Add(1)
		go func(em string) {
			defer wg.Done()
			<-start
			ok, reason, err := d.CreateTrustVisitorLinkCapped(context.Background(), orgID, em, visitorHash(t), linkTTL, caps)
			if err != nil {
				t.Errorf("capped create: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if ok {
				created++
			} else {
				reasons[reason]++
			}
		}(em)
	}
	close(start)
	wg.Wait()
	return created, reasons
}

// The caps are decided inside one transaction under an advisory lock, so
// parallel requests cannot all pass the count before any of them inserts.
func TestTrustVisitorLinkCapPerEmailIsAtomic(t *testing.T) {
	d := testDB(t)
	orgID := trustTestOrg(t, d, "capemail")
	emails := make([]string, 20)
	for i := range emails {
		emails[i] = "same@example.com"
	}
	created, reasons := runCapped(t, d, orgID, emails, capsFor(3, 1000, 1000))
	if created != 3 || reasons[TrustLinkCapEmail] != 17 {
		t.Errorf("created %d, reasons %v; want 3 and 17 per-email refusals", created, reasons)
	}
	if n, _ := d.CountTrustVisitorLinks(context.Background(), orgID, "same@example.com", time.Now().Add(-time.Hour)); n != 3 {
		t.Errorf("rows = %d, want 3", n)
	}
}

func TestTrustVisitorLinkCapPerOrgIsAtomic(t *testing.T) {
	d := testDB(t)
	orgID := trustTestOrg(t, d, "caporg")
	emails := make([]string, 20)
	for i := range emails {
		emails[i] = "u" + strconv.Itoa(i) + "@example.com"
	}
	created, reasons := runCapped(t, d, orgID, emails, capsFor(1000, 5, 1000))
	if created != 5 || reasons[TrustLinkCapOrg] != 15 {
		t.Errorf("created %d, reasons %v; want 5 and 15 per-org refusals", created, reasons)
	}
}

// One inbox cannot be flooded through many organizations.
func TestTrustVisitorLinkCapPerAddressAcrossOrgs(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	var orgs []int
	for i := 0; i < 4; i++ {
		orgs = append(orgs, trustTestOrg(t, d, "capaddr"+strconv.Itoa(i)))
	}
	email := "flood" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com"
	created, refused := 0, 0
	for i := 0; i < 12; i++ {
		ok, reason, err := d.CreateTrustVisitorLinkCapped(ctx, orgs[i%4], email, visitorHash(t), linkTTL, capsFor(1000, 1000, 10))
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			created++
		} else if reason == TrustLinkCapAddress {
			refused++
		}
	}
	if created != 10 || refused != 2 {
		t.Errorf("created %d, refused %d; want 10 and 2 over 4 organizations", created, refused)
	}
	// concurrently as well
	email2 := "flood2" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com"
	var wg sync.WaitGroup
	var mu sync.Mutex
	n := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, _, err := d.CreateTrustVisitorLinkCapped(ctx, orgs[i%4], email2, visitorHash(t), linkTTL, capsFor(1000, 1000, 10))
			if err != nil {
				t.Errorf("%v", err)
			}
			if ok {
				mu.Lock()
				n++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if n != 10 {
		t.Errorf("concurrent across orgs created %d, want 10", n)
	}
}

// A session can never belong to another organization's visitor.
func TestTrustVisitorSessionOrgMustMatchVisitorOrg(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgA := trustTestOrg(t, d, "fkA")
	orgB := trustTestOrg(t, d, "fkB")
	v, _ := signIn(t, d, orgA, "fk@example.com")
	_, err := d.pool.Exec(ctx, `
		INSERT INTO trust_visitor_sessions (organization_id, visitor_id, token_hash, expires_at)
		VALUES ($1, $2, $3, now() + interval '1 hour')`, orgB, v.ID, visitorHash(t))
	if err == nil {
		t.Error("a session of org B for org A's visitor was accepted")
	}
	if _, err := d.pool.Exec(ctx, `
		INSERT INTO trust_visitor_links (organization_id, email, token_hash, expires_at)
		VALUES ($1, 'Upper@Example.com', $2, now() + interval '1 hour')`, orgA, visitorHash(t)); err == nil {
		t.Error("a link with an upper-case email was accepted")
	}
}

// The pause time is derived from the links themselves: when the cap is
// reached, the moment the slot that frees up first turns one window old.
func TestTrustVisitorLinkPausedUntil(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "paused")
	mk := func(n int) {
		for i := 0; i < n; i++ {
			if err := d.CreateTrustVisitorLink(ctx, orgID, "p"+strconv.Itoa(i)+"@example.com", visitorHash(t), linkTTL); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk(4)
	if got, err := d.TrustVisitorLinkPausedUntil(ctx, orgID, 5, time.Hour); err != nil || got != nil {
		t.Fatalf("4 of 5 = %v, %v; want nil", got, err)
	}
	mk(1)
	got, err := d.TrustVisitorLinkPausedUntil(ctx, orgID, 5, time.Hour)
	if err != nil || got == nil || !got.After(time.Now()) || got.After(time.Now().Add(time.Hour+time.Minute)) {
		t.Fatalf("5 of 5 = %v, %v; want a time in (now, now+1h]", got, err)
	}
	// Two links older than the window no longer count.
	if _, err := d.pool.Exec(ctx, `UPDATE trust_visitor_links SET created_at = now() - interval '2 hours'
		WHERE id IN (SELECT id FROM trust_visitor_links WHERE organization_id = $1 ORDER BY id LIMIT 2)`, orgID); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.TrustVisitorLinkPausedUntil(ctx, orgID, 5, time.Hour); got != nil {
		t.Errorf("3 in the window of 5 = %v, want nil", got)
	}
	// Past the cap, the first slot to free is the one that brings the count back under it.
	mk(4) // 7 in the window
	got, _ = d.TrustVisitorLinkPausedUntil(ctx, orgID, 5, time.Hour)
	if got == nil {
		t.Fatal("7 of 5 = nil")
	}
	var third time.Time
	_ = d.pool.QueryRow(ctx, `SELECT created_at FROM trust_visitor_links WHERE organization_id = $1 AND created_at > now() - interval '1 hour'
		ORDER BY created_at, id OFFSET 2 LIMIT 1`, orgID).Scan(&third)
	if diff := got.Sub(third.Add(time.Hour)); diff < -time.Second || diff > time.Second {
		t.Errorf("paused until %v, want the 3rd oldest link + 1h (%v)", got, third.Add(time.Hour))
	}
	if got, _ := d.TrustVisitorLinkPausedUntil(ctx, orgID, 0, time.Hour); got != nil {
		t.Error("a zero cap pauses")
	}
}
