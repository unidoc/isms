package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/mail"
)

// Requires a migrated Postgres (ISMS_TEST_DATABASE_URL); skipped otherwise.
// Every org, user and row below is created by the test itself.

// digestFixture is one org with five users and one row of each kind the digest
// reads, for the contributor `me`:
//
//	needs action from me:  review R1 (to review), review R2 (changes requested,
//	  to resubmit), task T1 (assigned), comment K1 (written by other, on R1),
//	  change CH1 (approved, assigned to me), incident I1 and corrective action
//	  CA1 (assigned, open)
//	waits on someone else: review R3 (I sent it, still open), task T2 (I
//	  delegated it), comment K2 (mine, no reply), change CH2 (I requested,
//	  proposed), suggestion S1 (mine), incident I2 (closed) and CA2 (resolved)
//	  assigned to me, incident I3 assigned to someone else
//	private: task T3 (admin to other, private, overdue); T4 is public and
//	  overdue, so the overdue task count is 1 for me and 2 for an admin
type digestFixture struct {
	s   *Server
	org int

	admin, mgr, me, other string
}

func newDigestFixture(t *testing.T, name string) *digestFixture {
	t.Helper()
	s := testServer(t)
	ctx := context.Background()
	f := &digestFixture{s: s, org: newTestOrg(t, s, name)}
	mk := func(local, role string) string {
		e := fmt.Sprintf("%s@%s.digest.test", local, name)
		contractTestUser(t, s, f.org, e, role)
		return e
	}
	f.admin, f.mgr = mk("admin", "admin"), mk("manager", "manager")
	f.me, f.other = mk("me", "contributor"), mk("other", "contributor")
	if _, err := s.db.Pool().Exec(ctx, `UPDATE users SET name = $2 WHERE email = $1`, f.me, "Me Contributor"); err != nil {
		t.Fatalf("naming user: %v", err)
	}

	mustReview := func(doc, by, status string) int {
		r := &db.Review{DocumentID: doc, DocumentType: "policy", Title: "Title " + doc, Version: "1", RequestedBy: by, Status: "open"}
		if err := s.db.CreateReview(ctx, f.org, r); err != nil {
			t.Fatalf("CreateReview %s: %v", doc, err)
		}
		if status != "open" {
			if err := s.db.UpdateReviewStatus(ctx, f.org, r.ID, status); err != nil {
				t.Fatalf("review %s status: %v", doc, err)
			}
		}
		return r.ID
	}
	r1 := mustReview("dg-r1", f.other, "open")
	if err := s.db.AddReviewAssignment(ctx, f.org, &db.ReviewAssignment{ReviewID: r1, Reviewer: f.me, Status: "pending"}); err != nil {
		t.Fatalf("assigning R1: %v", err)
	}
	mustReview("dg-r2", f.me, "changes_requested")
	mustReview("dg-r3", f.me, "open")

	mustComment := func(review int, author, body string) {
		c := &db.Comment{ReviewID: &review, DocumentID: "dg-r1", Author: author, Body: body}
		if err := s.db.AddComment(ctx, f.org, c); err != nil {
			t.Fatalf("AddComment %s: %v", body, err)
		}
	}
	mustComment(r1, f.other, "K1 please clarify scope")
	mustComment(r1, f.me, "K2 my own note")

	past := db.NewEpoch(time.Now().Add(-72 * time.Hour))
	mustTask := func(title, assignee string, private bool, due *db.Epoch) {
		task := &db.Task{Title: title, TaskType: "general", Assignee: assignee, CreatedBy: f.admin,
			Status: "open", Priority: "medium", Private: private, DueDate: due}
		if title == "T2 delegated by me" {
			task.CreatedBy = f.me
		}
		if err := s.db.CreateTask(ctx, f.org, task); err != nil {
			t.Fatalf("CreateTask %s: %v", title, err)
		}
	}
	mustTask("T1 assigned to me", f.me, false, nil)
	mustTask("T2 delegated by me", f.other, false, nil)
	mustTask("T3 private of other", f.other, true, &past)
	mustTask("T4 public overdue", f.other, false, &past)

	mustChange := func(title, status, requester, assignee string) {
		ch := &db.ChangeRequest{Title: title, Description: "d", Priority: "medium", Category: "process",
			RiskLevel: "low", Status: status, RequestedBy: requester, AssignedTo: assignee}
		if err := s.db.CreateChangeRequest(ctx, f.org, ch); err != nil {
			t.Fatalf("CreateChangeRequest %s: %v", title, err)
		}
	}
	mustChange("CH1 approved for me", "approved", f.admin, f.me)
	mustChange("CH2 proposed by me", "proposed", f.me, f.other)

	mustSuggestion := func(title, by string) {
		sg := &db.Suggestion{EntityType: "risk", SuggestionType: "create", Title: title, SuggestedBy: by}
		if err := s.db.CreateSuggestion(ctx, f.org, sg); err != nil {
			t.Fatalf("CreateSuggestion %s: %v", title, err)
		}
	}
	mustSuggestion("S1 suggested by me", f.me)
	mustSuggestion("S2 suggested by other", f.other)

	mustIncident := func(title, assignee, status string) {
		inc := &db.Incident{Title: title, Severity: "high", Status: status, IncidentType: "incident", Source: "internal",
			AuthorityNotified: "not_required", SubjectsNotified: "not_required",
			Reporter: f.admin, Assignee: assignee, DetectedAt: db.NewEpoch(time.Now())}
		if err := s.db.CreateIncident(ctx, f.org, inc); err != nil {
			t.Fatalf("CreateIncident %s: %v", title, err)
		}
	}
	mustIncident("I1 open assigned to me", f.me, "investigating")
	mustIncident("I2 closed assigned to me", f.me, "closed")
	mustIncident("I3 open assigned to other", f.other, "open")

	mustCA := func(title, assignee, status string) {
		ca := &db.CorrectiveAction{Title: title, Description: "d", Source: "internal_audit", Severity: "minor_nc",
			Status: status, Assignee: assignee, CreatedBy: f.admin}
		if err := s.db.CreateCorrectiveAction(ctx, f.org, ca); err != nil {
			t.Fatalf("CreateCorrectiveAction %s: %v", title, err)
		}
	}
	mustCA("CA1 open assigned to me", f.me, "implementation")
	mustCA("CA2 resolved assigned to me", f.me, "resolved")
	return f
}

// titles returns the titles in the named section, sorted.
func sectionTitles(d *Digest, section string) []string {
	var out []string
	for _, sec := range d.Sections {
		if sec.Title != section {
			continue
		}
		for _, it := range sec.Items {
			out = append(out, it.Title)
		}
	}
	sort.Strings(out)
	return out
}

func sectionItems(d *Digest, section string) []mail.DigestItem {
	for _, sec := range d.Sections {
		if sec.Title == section {
			return sec.Items
		}
	}
	return nil
}

func firstItem(t *testing.T, d *Digest, section string) mail.DigestItem {
	t.Helper()
	items := sectionItems(d, section)
	if len(items) == 0 {
		t.Fatalf("section %q has no items", section)
	}
	return items[0]
}

func overdueCount(d *Digest, label string) int {
	for _, c := range d.Overdue {
		if c.Label == label {
			return c.Count
		}
	}
	return -1
}

func TestBuildDigestPersonalSection(t *testing.T) {
	f := newDigestFixture(t, "dg-personal")
	d, err := BuildDigest(context.Background(), f.s.db, f.org, f.me, "contributor")
	if err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}
	if d.Empty() {
		t.Fatal("digest is empty, want items")
	}
	want := map[string][]string{
		digestReviews:           {"Title dg-r1", "Title dg-r2"},
		digestComments:          {"K1 please clarify scope"},
		digestTasks:             {"T1 assigned to me"},
		digestChanges:           {"CH1 approved for me"},
		digestIncidents:         {"I1 open assigned to me"},
		digestCorrectiveActions: {"CA1 open assigned to me"},
	}
	for section, titles := range want {
		got := sectionTitles(d, section)
		if strings.Join(got, "|") != strings.Join(titles, "|") {
			t.Errorf("%s = %q, want %q", section, got, titles)
		}
	}
	// A contributor cannot act on suggestions, so the section is absent even
	// though S1 is theirs and S2 is open.
	if got := sectionTitles(d, digestSuggestions); len(got) != 0 {
		t.Errorf("contributor suggestions = %q, want none", got)
	}

	// Identifier and link per item.
	inc := firstItem(t, d, digestIncidents)
	if inc.Ref == "" || inc.Path != "/incidents/"+inc.Ref {
		t.Errorf("incident ref/path = %q %q, want path /incidents/<ref>", inc.Ref, inc.Path)
	}
	ca := firstItem(t, d, digestCorrectiveActions)
	if ca.Ref == "" || ca.Path != "/corrective-actions/"+ca.Ref {
		t.Errorf("CA ref/path = %q %q", ca.Ref, ca.Path)
	}
	ch := firstItem(t, d, digestChanges)
	if ch.Ref == "" || ch.Path != "/changes/"+ch.Ref {
		t.Errorf("change ref/path = %q %q", ch.Ref, ch.Path)
	}
	task := firstItem(t, d, digestTasks)
	if task.Ref == "" || !strings.HasPrefix(task.Path, "/tasks/") {
		t.Errorf("task ref/path = %q %q", task.Ref, task.Path)
	}
	for _, rv := range sectionItems(d, digestReviews) {
		if !strings.HasPrefix(rv.Path, "/reviews/") || rv.Ref == "" {
			t.Errorf("review ref/path = %q %q", rv.Ref, rv.Path)
		}
	}
	if k := firstItem(t, d, digestComments); !strings.HasPrefix(k.Path, "/reviews/") {
		t.Errorf("comment on a review should link to the review, got %q", k.Path)
	}
}

func TestBuildDigestManagerSeesSuggestionsAndProposedChanges(t *testing.T) {
	f := newDigestFixture(t, "dg-manager")
	d, err := BuildDigest(context.Background(), f.s.db, f.org, f.mgr, "manager")
	if err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}
	// Both open suggestions are the manager's to apply (the Inbox flags them
	// needs_action for a reviewer, including ones a contributor wrote).
	got := sectionTitles(d, digestSuggestions)
	if strings.Join(got, "|") != "S1 suggested by me|S2 suggested by other" {
		t.Errorf("manager suggestions = %q", got)
	}
	if s := firstItem(t, d, digestSuggestions); s.Path != "/inbox/suggestions" {
		t.Errorf("suggestion path = %q", s.Path)
	}
	// CH2 is proposed: a manager can approve it.
	if got := sectionTitles(d, digestChanges); strings.Join(got, "|") != "CH2 proposed by me" {
		t.Errorf("manager changes = %q", got)
	}
}

func TestBuildDigestPrivateTaskScoping(t *testing.T) {
	f := newDigestFixture(t, "dg-private")
	ctx := context.Background()

	contrib, err := BuildDigest(ctx, f.s.db, f.org, f.me, "contributor")
	if err != nil {
		t.Fatalf("BuildDigest contributor: %v", err)
	}
	for _, title := range sectionTitles(contrib, digestTasks) {
		if strings.Contains(title, "T3") {
			t.Errorf("private task of another user leaked into the personal section: %q", title)
		}
	}
	if n := overdueCount(contrib, "Tasks"); n != 1 {
		t.Errorf("contributor overdue tasks = %d, want 1 (T4 only; T3 is private)", n)
	}

	admin, err := BuildDigest(ctx, f.s.db, f.org, f.admin, "admin")
	if err != nil {
		t.Fatalf("BuildDigest admin: %v", err)
	}
	if n := overdueCount(admin, "Tasks"); n != 2 {
		t.Errorf("admin overdue tasks = %d, want 2 (T3 and T4)", n)
	}
	// The snapshot total follows the same scoping.
	if admin.OverdueTotal != contrib.OverdueTotal+1 {
		t.Errorf("overdue totals admin=%d contributor=%d, want admin = contributor+1", admin.OverdueTotal, contrib.OverdueTotal)
	}
}

func TestBuildDigestEmptyWhenNothingPending(t *testing.T) {
	f := newDigestFixture(t, "dg-empty")
	// reader has no involvement at all, but the org has overdue work: the
	// snapshot must not make the digest non-empty.
	contractTestUser(t, f.s, f.org, "reader@dg-empty.digest.test", "reader")
	d, err := BuildDigest(context.Background(), f.s.db, f.org, "reader@dg-empty.digest.test", "reader")
	if err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}
	if !d.Empty() {
		t.Errorf("digest not empty: %+v", d.Sections)
	}
	if overdueCount(d, "Tasks") != 1 || d.OverdueTotal < 1 {
		t.Errorf("snapshot should still be filled, got %+v total %d", d.Overdue, d.OverdueTotal)
	}
}

// The personal set for reviews, tasks and comments equals what GET /inbox
// returns for the same user, because both read the same scoped queries.
func TestBuildDigestMatchesInbox(t *testing.T) {
	f := newDigestFixture(t, "dg-parity")
	ctx := context.Background()
	d, err := BuildDigest(ctx, f.s.db, f.org, f.me, "contributor")
	if err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}

	c, rec := reviewCtxForPath(f.org, http.MethodGet, "/api/v1/inbox", "", "contributor", f.me)
	if err := f.s.handleInbox(c); err != nil {
		t.Fatalf("handleInbox: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("inbox status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []inboxItem `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding inbox: %v", err)
	}
	inbox := map[string][]string{}
	for _, it := range resp.Data {
		switch it.Type {
		case "review":
			inbox["review"] = append(inbox["review"], fmt.Sprintf("/reviews/%d", it.ID))
		case "task":
			inbox["task"] = append(inbox["task"], fmt.Sprintf("/tasks/%d", it.ID))
		case "comment":
			inbox["comment"] = append(inbox["comment"], it.Title)
		}
	}
	digest := map[string][]string{}
	for _, it := range sectionItems(d, digestReviews) {
		digest["review"] = append(digest["review"], it.Path)
	}
	for _, it := range sectionItems(d, digestTasks) {
		digest["task"] = append(digest["task"], it.Path)
	}
	for _, it := range sectionItems(d, digestComments) {
		digest["comment"] = append(digest["comment"], it.Title)
	}
	for _, k := range []string{"review", "task", "comment"} {
		a, b := append([]string(nil), inbox[k]...), append([]string(nil), digest[k]...)
		sort.Strings(a)
		sort.Strings(b)
		if len(a) == 0 {
			t.Errorf("fixture produced no %s in the inbox; the parity check proves nothing", k)
		}
		if strings.Join(a, "|") != strings.Join(b, "|") {
			t.Errorf("%s: inbox %q, digest %q", k, a, b)
		}
	}
}

// A failing section query must fail the whole build: a partial digest would
// say "nothing pending" about a section it never read. Every query takes the
// context, so a cancelled one fails the first section and must surface.
func TestBuildDigestSectionErrorReturnsError(t *testing.T) {
	f := newDigestFixture(t, "dg-error")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, err := BuildDigest(ctx, f.s.db, f.org, f.me, "contributor")
	if err == nil {
		t.Fatalf("BuildDigest with a cancelled context returned no error, digest %+v", d)
	}
	if d != nil {
		t.Errorf("a digest was returned alongside the error: %+v", d)
	}
}

func TestOrgMailForMatchesBrandAndLinks(t *testing.T) {
	f := newDigestFixture(t, "dg-orgmail")
	ctx := context.Background()
	m := orgMailFor(ctx, f.s.db, f.org, "https://isms.example/", false)
	org, err := f.s.db.GetOrganization(ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://isms.example/" + org.Slug; m.AppURL != want {
		t.Errorf("AppURL = %q, want %q", m.AppURL, want)
	}
	if m.PublicURL != "https://isms.example" {
		t.Errorf("PublicURL = %q", m.PublicURL)
	}
	if m.Branding.Name == "" {
		t.Error("brand name empty")
	}
	// The Server method delegates, with the same result.
	t.Setenv("ISMS_BASE_URL", "https://isms.example/")
	if got := f.s.orgMail(ctx, f.org); got != m {
		t.Errorf("Server.orgMail = %+v, orgMailFor = %+v", got, m)
	}
}

// captureSMTP is a one-message SMTP sink; the mail package has its own
// unexported equivalent for its unit tests.
func captureSMTP(t *testing.T) (host, port string, data <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake ESMTP")
		var body strings.Builder
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
					out <- body.String()
					write("250 OK")
					continue
				}
				body.WriteString(line + "\n")
				continue
			}
			switch cmd := strings.ToUpper(line); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				write("250 fake")
			case strings.HasPrefix(cmd, "DATA"):
				inData = true
				write("354 go ahead")
			case strings.HasPrefix(cmd, "QUIT"):
				write("221 bye")
				return
			default:
				write("250 OK")
			}
		}
	}()
	host, port, _ = net.SplitHostPort(ln.Addr().String())
	return host, port, out
}

// End to end: seed, build, render, deliver to a fake SMTP listener.
func TestDigestEndToEndEmail(t *testing.T) {
	f := newDigestFixture(t, "dg-e2e")
	ctx := context.Background()
	d, err := BuildDigest(ctx, f.s.db, f.org, f.me, "contributor")
	if err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}
	om := orgMailFor(ctx, f.s.db, f.org, "https://isms.example", false)
	om.Branding.Name = "Acme Security"

	host, port, data := captureSMTP(t)
	m := mail.New(mail.Config{Host: host, Port: port, From: "noreply@isms.sh"})
	if err := m.SendDigestBranded(f.me, "Me Contributor", om.AppURL, d, om.Branding); err != nil {
		t.Fatalf("SendDigestBranded: %v", err)
	}
	var body string
	select {
	case body = <-data:
	case <-time.After(5 * time.Second):
		t.Fatal("no message captured")
	}
	for _, want := range []string{
		"Subject: Acme Security", "Title dg-r1", "Title dg-r2", "K1 please clarify scope",
		"T1 assigned to me", "CH1 approved for me", "I1 open assigned to me", "CA1 open assigned to me",
		om.AppURL + "/settings", om.AppURL + "/inbox",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("email missing %q", want)
		}
	}
	for _, absent := range []string{"Title dg-r3", "K2 my own note", "T2 delegated by me", "T3 private", "CH2 proposed", "I2 closed", "I3 open", "CA2 resolved"} {
		if strings.Contains(body, absent) {
			t.Errorf("email contains %q, which waits on someone else or is not visible", absent)
		}
	}
	if out := os.Getenv("ISMS_DIGEST_SAMPLE_OUT"); out != "" {
		html := body
		if i := strings.Index(body, "\n\n"); i >= 0 {
			html = body[i+2:]
		}
		if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
			t.Fatalf("writing sample: %v", err)
		}
	}
}
