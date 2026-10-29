package api

import (
	"bufio"
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/mail"
)

// Requires a migrated Postgres (ISMS_TEST_DATABASE_URL); skipped otherwise.
// Every org and user is created by the test. The runs go through runDigests
// with the test's own orgs, so rows other tests left in the database cannot
// change the counts; SendDigests itself (all orgs) is covered by one test that
// looks only at its own recipients.

const digestBase = "https://isms.example"

// The clocks (wednesday, mondayEarly, mondayOpen) are in digest_period_test.go;
// 2026-W44 starts on Monday 2026-10-26.
var nextWeek = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)

type sinkMsg struct{ To, Body string }

// smtpSink accepts any number of messages. While reject is set it answers DATA
// with a permanent failure, so the sender sees a failed send.
type smtpSink struct {
	host, port string
	reject     atomic.Bool
	dropQuit   atomic.Bool // answer the end of DATA, then drop the connection
	mu         sync.Mutex
	msgs       []sinkMsg
}

func newSMTPSink(t *testing.T) *smtpSink {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	k := &smtpSink{}
	k.host, k.port, _ = net.SplitHostPort(ln.Addr().String())
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go k.serve(conn)
		}
	}()
	return k
}

func (k *smtpSink) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	write("220 fake ESMTP")
	var body strings.Builder
	var to string
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				k.mu.Lock()
				k.msgs = append(k.msgs, sinkMsg{To: to, Body: body.String()})
				k.mu.Unlock()
				body.Reset()
				write("250 OK")
				if k.dropQuit.Load() {
					return
				}
				continue
			}
			body.WriteString(line + "\n")
			continue
		}
		switch cmd := strings.ToUpper(line); {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			write("250 fake")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			to = strings.Trim(strings.TrimSpace(line[len("RCPT TO:"):]), "<>")
			write("250 OK")
		case strings.HasPrefix(cmd, "DATA"):
			if k.reject.Load() {
				write("554 rejected")
				continue
			}
			inData = true
			write("354 go ahead")
		case strings.HasPrefix(cmd, "QUIT"):
			write("221 bye")
			return
		default:
			write("250 OK")
		}
	}
}

func (k *smtpSink) mailer() *mail.Mailer {
	return mail.New(mail.Config{Host: k.host, Port: k.port, From: "noreply@isms.sh"})
}

func (k *smtpSink) messages() []sinkMsg {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]sinkMsg(nil), k.msgs...)
}

func (k *smtpSink) to() []string {
	var out []string
	for _, m := range k.messages() {
		out = append(out, m.To)
	}
	return out
}

type sendFixture struct {
	s    *Server
	ctx  context.Context
	orgs []db.Organization
	sink *smtpSink
}

func newSendFixture(t *testing.T, orgNames ...string) *sendFixture {
	t.Helper()
	s := testServer(t)
	f := &sendFixture{s: s, ctx: context.Background(), sink: newSMTPSink(t)}
	for _, n := range orgNames {
		org, err := s.db.GetOrganization(f.ctx, newTestOrg(t, s, n))
		if err != nil {
			t.Fatal(err)
		}
		f.orgs = append(f.orgs, *org)
	}
	return f
}

type userOpts struct {
	agent, inactive, unverified bool
}

// member creates a user, verified and active unless opts say otherwise, adds
// it to org i, and gives it one open task so it has something pending.
func (f *sendFixture) member(t *testing.T, i int, email string, o userOpts) *db.User {
	t.Helper()
	u := &db.User{Email: email, Name: "Name of " + email, Active: !o.inactive, IsAgent: o.agent}
	if err := f.s.db.UpsertUser(f.ctx, u); err != nil {
		t.Fatalf("UpsertUser %s: %v", email, err)
	}
	if _, err := f.s.db.Pool().Exec(f.ctx, `UPDATE users SET email_verified = $2 WHERE id = $1`, u.ID, !o.unverified); err != nil {
		t.Fatal(err)
	}
	if err := f.s.db.AddOrgMember(f.ctx, f.orgs[i].ID, u.ID, "contributor"); err != nil {
		t.Fatal(err)
	}
	f.task(t, i, email, "Task for "+email)
	return u
}

func (f *sendFixture) task(t *testing.T, i int, assignee, title string) {
	t.Helper()
	task := &db.Task{Title: title, TaskType: "general", Assignee: assignee, CreatedBy: assignee + ".creator",
		Status: "open", Priority: "medium"}
	if err := f.s.db.CreateTask(f.ctx, f.orgs[i].ID, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
}

func (f *sendFixture) run(t *testing.T, now time.Time) DigestRunResult {
	t.Helper()
	return runDigests(f.ctx, f.s.db, f.orgs, f.sink.mailer(), digestBase, false, now)
}

func (f *sendFixture) claims(t *testing.T, i int) int {
	t.Helper()
	var n int
	if err := f.s.db.Pool().QueryRow(f.ctx, `SELECT count(*) FROM digest_sends WHERE organization_id = $1`, f.orgs[i].ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantCounts(t *testing.T, got DigestRunResult, sent, empty, skipped, failed int) {
	t.Helper()
	if got.Sent != sent || got.Empty != empty || got.Skipped != skipped || got.Failed != failed {
		t.Errorf("result = sent %d empty %d skipped %d failed %d, want %d %d %d %d",
			got.Sent, got.Empty, got.Skipped, got.Failed, sent, empty, skipped, failed)
	}
}

// (a) and (c): one mail per eligible user with pending work, once per ISO week.
func TestSendDigestsOncePerWeek(t *testing.T) {
	f := newSendFixture(t, "dgs-weekly")
	f.member(t, 0, "a@dgs-weekly.test", userOpts{})
	f.member(t, 0, "b@dgs-weekly.test", userOpts{})

	r := f.run(t, wednesday)
	wantCounts(t, r, 2, 0, 0, 0)
	if got := f.sink.to(); len(got) != 2 {
		t.Fatalf("recipients = %v, want 2 mails", got)
	}
	for _, m := range f.sink.messages() {
		if !strings.Contains(m.Body, "Task for "+m.To) {
			t.Errorf("mail to %s lacks their task:\n%s", m.To, m.Body)
		}
		if !strings.Contains(m.Body, digestBase+"/"+f.orgs[0].Slug+"/inbox") {
			t.Errorf("mail to %s lacks the org inbox link", m.To)
		}
	}

	// A second run the same week sends nothing.
	r = f.run(t, wednesday.Add(3*time.Hour))
	wantCounts(t, r, 0, 0, 2, 0)
	if n := len(f.sink.messages()); n != 2 {
		t.Errorf("%d mails after the second run, want still 2", n)
	}

	// (c) The next ISO week sends again.
	r = f.run(t, nextWeek)
	wantCounts(t, r, 2, 0, 0, 0)
	if n := len(f.sink.messages()); n != 4 {
		t.Errorf("%d mails after the next week's run, want 4", n)
	}
}

// (b) Nothing before Monday 08:00 UTC, and nothing is claimed by looking.
func TestSendDigestsNotDueBeforeMondayEight(t *testing.T) {
	f := newSendFixture(t, "dgs-due")
	f.member(t, 0, "a@dgs-due.test", userOpts{})

	r := f.run(t, mondayEarly)
	wantCounts(t, r, 0, 0, 0, 0)
	if r.NotDue == false {
		t.Error("result does not say the run was not due")
	}
	if len(f.sink.messages()) != 0 || f.claims(t, 0) != 0 {
		t.Errorf("a run before Monday 08:00 UTC sent %d mails and left %d claims", len(f.sink.messages()), f.claims(t, 0))
	}
	// The same instant expressed in another zone is the same instant.
	zone := time.FixedZone("x", 9*3600)
	if r := f.run(t, mondayEarly.In(zone)); r.Sent != 0 {
		t.Error("a local-zone clock changed what is due")
	}

	r = f.run(t, mondayOpen)
	wantCounts(t, r, 1, 0, 0, 0)
}

// (d) Opted-out members, agents and inactive users get nothing. An unverified
// email still gets its digest (Cloudflare Access users are never verified).
func TestSendDigestsSkipsIneligibleMembers(t *testing.T) {
	f := newSendFixture(t, "dgs-skip")
	f.member(t, 0, "ok@dgs-skip.test", userOpts{})
	optOut := f.member(t, 0, "optout@dgs-skip.test", userOpts{})
	f.member(t, 0, "agent@dgs-skip.test", userOpts{agent: true})
	f.member(t, 0, "inactive@dgs-skip.test", userOpts{inactive: true})
	f.member(t, 0, "unverified@dgs-skip.test", userOpts{unverified: true})
	if err := f.s.db.SetDigestEnabled(f.ctx, f.orgs[0].ID, optOut.ID, false); err != nil {
		t.Fatal(err)
	}

	r := f.run(t, wednesday)
	wantCounts(t, r, 2, 0, 3, 0)
	got := f.sink.to()
	sort.Strings(got)
	if strings.Join(got, ",") != "ok@dgs-skip.test,unverified@dgs-skip.test" {
		t.Errorf("recipients = %v, want ok and unverified only", got)
	}
	if f.claims(t, 0) != 2 {
		t.Errorf("claims = %d, want 2: skipped members must not be claimed", f.claims(t, 0))
	}
}

// (e) Nothing pending: no mail, an 'empty' claim, and a later item does not
// trigger a mid-week digest.
func TestSendDigestsEmptyWeekIsEvaluatedOnce(t *testing.T) {
	f := newSendFixture(t, "dgs-empty")
	u := &db.User{Email: "idle@dgs-empty.test", Name: "Idle", Active: true}
	if err := f.s.db.UpsertUser(f.ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.db.Pool().Exec(f.ctx, `UPDATE users SET email_verified = true WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.db.AddOrgMember(f.ctx, f.orgs[0].ID, u.ID, "contributor"); err != nil {
		t.Fatal(err)
	}

	r := f.run(t, wednesday)
	wantCounts(t, r, 0, 1, 0, 0)
	if len(f.sink.messages()) != 0 {
		t.Fatal("a mail was sent with nothing pending")
	}
	var outcome string
	if err := f.s.db.Pool().QueryRow(f.ctx, `SELECT outcome FROM digest_sends WHERE organization_id = $1 AND user_id = $2`,
		f.orgs[0].ID, u.ID).Scan(&outcome); err != nil || outcome != "empty" {
		t.Fatalf("claim outcome = %q, err = %v, want empty", outcome, err)
	}

	f.task(t, 0, u.Email, "Arrived on Thursday")
	r = f.run(t, wednesday.Add(24*time.Hour))
	wantCounts(t, r, 0, 0, 1, 0)
	if len(f.sink.messages()) != 0 {
		t.Error("an item arriving later the same week triggered a digest")
	}
	// Next week it is evaluated again, and now it has an item.
	r = f.run(t, nextWeek)
	wantCounts(t, r, 1, 0, 0, 0)
}

// (f) A failed send leaves no claim behind, so the next run retries.
func TestSendDigestsFailedSendIsRetried(t *testing.T) {
	f := newSendFixture(t, "dgs-fail")
	f.member(t, 0, "a@dgs-fail.test", userOpts{})
	f.member(t, 0, "b@dgs-fail.test", userOpts{})

	f.sink.reject.Store(true)
	r := f.run(t, wednesday)
	wantCounts(t, r, 0, 0, 0, 2)
	if f.claims(t, 0) != 0 {
		t.Fatalf("%d claims left behind after failed sends", f.claims(t, 0))
	}

	f.sink.reject.Store(false)
	r = f.run(t, wednesday.Add(time.Hour))
	wantCounts(t, r, 2, 0, 0, 0)
	if len(f.sink.messages()) != 2 {
		t.Errorf("%d mails after the retry, want 2", len(f.sink.messages()))
	}
}

// A relay that takes the message and then drops the connection before QUIT has
// delivered it: the claim stays and the next run sends nothing.
func TestSendDigestsAcceptedMessageIsNotResent(t *testing.T) {
	f := newSendFixture(t, "dgs-quit")
	f.member(t, 0, "a@dgs-quit.test", userOpts{})

	f.sink.dropQuit.Store(true)
	r := f.run(t, wednesday)
	wantCounts(t, r, 1, 0, 0, 0)
	if f.claims(t, 0) != 1 {
		t.Fatalf("claims = %d, want 1", f.claims(t, 0))
	}
	r = f.run(t, wednesday.Add(time.Hour))
	wantCounts(t, r, 0, 0, 1, 0)
	if n := len(f.sink.messages()); n != 1 {
		t.Errorf("%d mails, want exactly 1", n)
	}
}

// (g) No mailer or no base URL: nothing is sent and nothing is claimed.
func TestSendDigestsDisabledWithoutMailerOrBaseURL(t *testing.T) {
	f := newSendFixture(t, "dgs-off")
	f.member(t, 0, "a@dgs-off.test", userOpts{})

	for name, run := range map[string]func() DigestRunResult{
		"no mailer": func() DigestRunResult { return runDigests(f.ctx, f.s.db, f.orgs, nil, digestBase, false, wednesday) },
		"no base url": func() DigestRunResult {
			return runDigests(f.ctx, f.s.db, f.orgs, f.sink.mailer(), "", false, wednesday)
		},
	} {
		r := run()
		if r.Disabled == "" {
			t.Errorf("%s: result carries no reason", name)
		}
		wantCounts(t, r, 0, 0, 0, 0)
	}
	if len(f.sink.messages()) != 0 || f.claims(t, 0) != 0 {
		t.Errorf("sent %d mails, left %d claims", len(f.sink.messages()), f.claims(t, 0))
	}
	// The exported entry point reports the same.
	r, err := SendDigests(f.ctx, f.s.db, nil, digestBase, false, wednesday)
	if err != nil || r.Disabled == "" {
		t.Errorf("SendDigests(nil mailer) = %+v, %v", r, err)
	}
}

// (h) Someone in two orgs gets one mail per org, each with that org's links and
// brand, and opting out of one leaves the other.
func TestSendDigestsPerOrganization(t *testing.T) {
	f := newSendFixture(t, "dgs-two-a", "dgs-two-b")
	u := f.member(t, 0, "both@dgs-two.test", userOpts{})
	if err := f.s.db.AddOrgMember(f.ctx, f.orgs[1].ID, u.ID, "contributor"); err != nil {
		t.Fatal(err)
	}
	f.task(t, 1, u.Email, "Task for both in B")

	r := f.run(t, wednesday)
	wantCounts(t, r, 2, 0, 0, 0)
	msgs := f.sink.messages()
	if len(msgs) != 2 {
		t.Fatalf("%d mails, want 2", len(msgs))
	}
	for i, org := range f.orgs {
		var found *sinkMsg
		for j := range msgs {
			if strings.Contains(msgs[j].Body, digestBase+"/"+org.Slug+"/inbox") {
				found = &msgs[j]
			}
		}
		if found == nil {
			t.Fatalf("no mail links into org %s", org.Slug)
		}
		if !strings.Contains(found.Body, "Subject: "+org.Name) {
			t.Errorf("mail for %s lacks its brand in the subject:\n%s", org.Slug, found.Body)
		}
		other := f.orgs[1-i]
		if strings.Contains(found.Body, "/"+other.Slug+"/") {
			t.Errorf("mail for %s links into %s", org.Slug, other.Slug)
		}
	}

	// Next week, opted out of A only: the mail for B still goes out.
	if err := f.s.db.SetDigestEnabled(f.ctx, f.orgs[0].ID, u.ID, false); err != nil {
		t.Fatal(err)
	}
	r = f.run(t, nextWeek)
	wantCounts(t, r, 1, 0, 1, 0)
	msgs = f.sink.messages()
	if len(msgs) != 3 || !strings.Contains(msgs[2].Body, "/"+f.orgs[1].Slug+"/inbox") {
		t.Errorf("after opting out of A, mails = %d, want the third to be org B's", len(msgs))
	}
}

// SendDigests over every org in the database reaches a seeded user.
func TestSendDigestsAllOrganizations(t *testing.T) {
	f := newSendFixture(t, "dgs-all")
	f.member(t, 0, "a@dgs-all.test", userOpts{})
	r, err := SendDigests(f.ctx, f.s.db, f.sink.mailer(), digestBase, false, wednesday)
	if err != nil {
		t.Fatalf("SendDigests: %v", err)
	}
	found := false
	for _, to := range f.sink.to() {
		if to == "a@dgs-all.test" {
			found = true
		}
	}
	if !found {
		t.Errorf("no mail to the seeded user; recipients %v, result %+v", f.sink.to(), r)
	}
}
