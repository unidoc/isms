package mail

import (
	"bufio"
	"net"
	netmail "net/mail"
	"strings"
	"testing"
	"time"
)

// resolveFrom is the guard against the multi-tenant sender leak (#16): a tenant's
// email must show the tenant's brand in the From display name, never the
// operator's SMTP_FROM display name — while the envelope address (SPF/DKIM)
// stays the configured sender.
func TestResolveFrom(t *testing.T) {
	const addr = "noreply@isms.sh"

	t.Run("no brand keeps SMTP_FROM verbatim", func(t *testing.T) {
		configFrom := `"Operator Inc" <` + addr + `>`
		header, envelope := resolveFrom(configFrom, "")
		if header != configFrom {
			t.Errorf("header = %q, want unchanged %q", header, configFrom)
		}
		if envelope != addr {
			t.Errorf("envelope = %q, want bare %q", envelope, addr)
		}
	})

	t.Run("brand overrides the operator display name", func(t *testing.T) {
		// The operator's SMTP_FROM carries their own brand; a tenant must not see it.
		configFrom := `"Operator Inc" <` + addr + `>`
		header, envelope := resolveFrom(configFrom, "Acme Audit ehf")

		if strings.Contains(header, "Operator Inc") {
			t.Fatalf("LEAK: operator display name present in tenant From header: %q", header)
		}
		if envelope != addr {
			t.Errorf("envelope = %q, want bare %q (SPF/DKIM alignment)", envelope, addr)
		}
		// Round-trip: the header must parse back to the tenant brand + the
		// configured envelope address, regardless of quoting details.
		parsed, err := netmail.ParseAddress(header)
		if err != nil {
			t.Fatalf("From header does not parse: %q: %v", header, err)
		}
		if parsed.Name != "Acme Audit ehf" {
			t.Errorf("display name = %q, want %q", parsed.Name, "Acme Audit ehf")
		}
		if parsed.Address != addr {
			t.Errorf("address = %q, want %q", parsed.Address, addr)
		}
	})

	t.Run("bare SMTP_FROM gains the brand display name", func(t *testing.T) {
		header, envelope := resolveFrom(addr, "Acme Corp")
		if envelope != addr {
			t.Errorf("envelope = %q, want %q", envelope, addr)
		}
		parsed, err := netmail.ParseAddress(header)
		if err != nil {
			t.Fatalf("From header does not parse: %q: %v", header, err)
		}
		if parsed.Name != "Acme Corp" || parsed.Address != addr {
			t.Errorf("parsed = %q <%q>, want %q <%q>", parsed.Name, parsed.Address, "Acme Corp", addr)
		}
	})

	t.Run("non-ASCII brand stays a valid encoded header", func(t *testing.T) {
		header, _ := resolveFrom(addr, "Þórð slf")
		if strings.Contains(header, "Þórð") {
			t.Errorf("non-ASCII name must be RFC 2047 encoded, got raw bytes: %q", header)
		}
		parsed, err := netmail.ParseAddress(header)
		if err != nil {
			t.Fatalf("encoded From header does not parse: %q: %v", header, err)
		}
		if parsed.Name != "Þórð slf" {
			t.Errorf("decoded display name = %q, want %q", parsed.Name, "Þórð slf")
		}
	})
}

// SendBranded with an empty Branding must fall back to the neutral platform name
// ("ISMS"), never the operator's SMTP_FROM display name.
func TestBrandingNameFallback(t *testing.T) {
	if got := (Branding{}).name(); got != "ISMS" {
		t.Errorf("empty Branding.name() = %q, want %q", got, "ISMS")
	}
	if got := (Branding{Name: "Acme"}).name(); got != "Acme" {
		t.Errorf("Branding.name() = %q, want %q", got, "Acme")
	}
}

// fakeSMTP accepts one message on a loopback port and returns the DATA section
// the client sent, so a test can inspect the rendered body without a hook in
// the Mailer.
func fakeSMTP(t *testing.T) (host, port string, data <-chan string) {
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

// A user's display name is untrusted input, and the review request greets and
// attributes by name, so a markup-bearing name must arrive escaped.
func TestSendReviewRequestBrandedEscapesNames(t *testing.T) {
	host, port, data := fakeSMTP(t)
	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	err := m.SendReviewRequestBranded("eve@x.io", "Eve <b>X</b>", "Mallory <i>Y</i> (m@x.io)",
		"doc-1", "Title", "1", "https://isms.example", 7, "", Branding{})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	body := <-data
	if !strings.Contains(body, "Eve &lt;b&gt;X&lt;/b&gt;") {
		t.Errorf("reviewer name not escaped in body:\n%s", body)
	}
	if !strings.Contains(body, "Mallory &lt;i&gt;Y&lt;/i&gt; (m@x.io)") {
		t.Errorf("actor not escaped in body:\n%s", body)
	}
	if strings.Contains(body, "<b>X</b>") || strings.Contains(body, "<i>Y</i>") {
		t.Errorf("raw markup leaked into body:\n%s", body)
	}
}

// message splits a captured message into its header block and body.
func message(t *testing.T, raw string) (headers []string, body string) {
	t.Helper()
	head, body, ok := strings.Cut(raw, "\n\n")
	if !ok {
		t.Fatalf("no blank line between headers and body:\n%s", raw)
	}
	return strings.Split(head, "\n"), body
}

// capture sends through fn and returns the captured message.
func capture(t *testing.T, fn func(m *Mailer) error) (headers []string, body string) {
	t.Helper()
	host, port, data := fakeSMTP(t)
	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	if err := fn(m); err != nil {
		t.Fatalf("send: %v", err)
	}
	return message(t, <-data)
}

// markup returns a tag-shaped value that names the parameter it was passed in,
// so a failure says which one arrived raw. It has no quotes: html.EscapeString
// turns it into &lt;b id=NAME&gt;.
func markup(name string) string { return "<b id=" + name + ">" }

// Every caller-supplied value in every email is user content (a signup name, a
// document title, a review message) and the mail is HTML, so each must arrive
// escaped, wherever it lands in the body.
func TestEveryEmailEscapesUserContent(t *testing.T) {
	brand := Branding{Name: markup("brand"), Color: markup("color")}
	const base = "https://isms.example"
	cases := []struct {
		name   string
		params []string // parameters that show up as text in the body
		send   func(m *Mailer) error
	}{
		{"verification", []string{"name", "brand"}, func(m *Mailer) error {
			return m.SendVerificationBranded("a@x.io", markup("name"), base, "tok", brand)
		}},
		{"email change", []string{"name", "brand"}, func(m *Mailer) error {
			return m.SendEmailChangeVerificationBranded("a@x.io", markup("name"), base, "tok", brand)
		}},
		{"password reset", []string{"name", "brand"}, func(m *Mailer) error {
			return m.SendPasswordResetBranded("a@x.io", markup("name"), base, "tok", brand)
		}},
		{"review request", []string{"reviewer", "actor", "doc", "title", "version", "message"}, func(m *Mailer) error {
			return m.SendReviewRequestBranded("a@x.io", markup("reviewer"), markup("actor"), markup("doc"),
				markup("title"), markup("version"), base, 7, markup("message"), brand)
		}},
		{"review decision", []string{"author", "reviewer", "doc", "title", "version"}, func(m *Mailer) error {
			return m.SendReviewDecisionBranded("a@x.io", markup("author"), markup("reviewer"), markup("doc"),
				markup("title"), markup("version"), "approved", base, brand)
		}},
		{"task assigned", []string{"assignee", "actor", "title", "priority"}, func(m *Mailer) error {
			return m.SendTaskAssignedBranded("a@x.io", markup("assignee"), markup("actor"), markup("title"), markup("priority"), base, brand)
		}},
		{"otp", []string{"name", "code"}, func(m *Mailer) error {
			return m.SendOTPCode("a@x.io", markup("name"), markup("code"), brand)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, body := capture(t, tc.send)
			if strings.Contains(body, "<b id=") {
				t.Errorf("raw markup reached the body:\n%s", body)
			}
			for _, p := range tc.params {
				if want := "&lt;b id=" + p + "&gt;"; !strings.Contains(body, want) {
					t.Errorf("parameter %q not escaped in the body (want %q):\n%s", p, want, body)
				}
			}
			if strings.Contains(body, "style=\"") && strings.Contains(body, "background: <") {
				t.Errorf("raw markup inside a style attribute:\n%s", body)
			}
		})
	}
}

// A newline in a subject must not start a header or the body. Titles and
// document ids go into subjects.
func TestSubjectLineBreaksCannotInjectHeaders(t *testing.T) {
	headers, body := capture(t, func(m *Mailer) error {
		return m.SendTaskAssignedBranded("a@x.io", "Eve", "Bob", "Fix it\r\nBcc: evil@x.io\r\n\r\ninjected body", "high",
			"https://isms.example", Branding{Name: "Acme\r\nX-Evil: 1"})
	})
	if len(headers) != 5 {
		t.Errorf("header block has %d lines, want 5 (From, To, Subject, MIME-Version, Content-Type): %q", len(headers), headers)
	}
	for _, h := range headers {
		if strings.HasPrefix(h, "Bcc:") || strings.HasPrefix(h, "X-Evil:") {
			t.Errorf("injected header line %q in %q", h, headers)
		}
	}
	if !strings.HasPrefix(body, "<div") {
		t.Errorf("the body does not start with the template (the subject ended the headers early): %q", body)
	}
	if subject := strings.Join(headers, "\n"); !strings.Contains(subject, "Subject: Acme") || !strings.Contains(subject, "Fix it") {
		t.Errorf("the subject lost its text: %q", headers)
	}
}

// Escaping must not break what the mail is for: links still go to the right
// place, and a normal name reads normally, with an ampersand escaped once.
func TestEscapingKeepsLinksAndNormalText(t *testing.T) {
	const base = "https://isms.example/acme"
	_, body := capture(t, func(m *Mailer) error {
		return m.SendVerificationBranded("a@x.io", "Tom & Jerry", base+"/", "tok123", Branding{Name: "Smith & Co", Color: "#ff0000"})
	})
	for _, want := range []string{
		`href="` + base + `/verify-email?token=tok123"`,
		"Or copy this link: " + base + "/verify-email?token=tok123",
		"Hi Tom &amp; Jerry,",
		"Welcome to Smith &amp; Co",
		"background: #ff0000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "&amp;amp;") {
		t.Errorf("an ampersand was escaped twice:\n%s", body)
	}

	_, body = capture(t, func(m *Mailer) error {
		return m.SendReviewRequestBranded("a@x.io", "Eve", "Bob", "policy-1", "Access & control", "2", base, 7, "Please look", Branding{})
	})
	for _, want := range []string{`href="` + base + `/reviews/7"`, "policy-1 &mdash; Access &amp; control (v2)", "Please look"} {
		if !strings.Contains(body, want) {
			t.Errorf("review body missing %q:\n%s", want, body)
		}
	}

	_, body = capture(t, func(m *Mailer) error {
		return m.SendReviewDecisionBranded("a@x.io", "Ann", "Bob", "policy-1", "Policy", "2", "approved", base, Branding{})
	})
	if !strings.Contains(body, `href="`+base+`/documents/policy-1"`) {
		t.Errorf("decision link wrong:\n%s", body)
	}
	_, body = capture(t, func(m *Mailer) error {
		return m.SendTaskAssignedBranded("a@x.io", "Eve", "Bob", "Task", "high", base, Branding{})
	})
	if !strings.Contains(body, `href="`+base+`/tasks"`) {
		t.Errorf("task link wrong:\n%s", body)
	}
}

// Corrective-action and incident assignment emails used to send the record's
// raw description as the whole HTML body. They now use the branded layout, with
// every value escaped and the description's line breaks kept.
func TestRecordAssignedEmailsEscapeAndKeepLayout(t *testing.T) {
	const link = "https://isms.example/acme/corrective-actions/CA-4"
	brand := Branding{Name: markup("brand"), Color: "#112233"}
	desc := "Line one with <script>alert(1)</script>\n\n**line three** & more"

	for _, tc := range []struct {
		name    string
		send    func(m *Mailer) error
		subject string
		params  []string
		heading string
	}{
		{"corrective action", func(m *Mailer) error {
			return m.SendCorrectiveActionAssignedBranded("a@x.io", markup("assignee"), markup("actor"), markup("ident"),
				markup("title"), markup("severity"), desc, link, brand)
		}, "Subject: <b id=brand> — Corrective Action: <b id=title>", []string{"assignee", "actor", "ident", "title", "severity"}, "Corrective action assigned"},
		{"incident", func(m *Mailer) error {
			return m.SendIncidentAssignedBranded("a@x.io", markup("assignee"), markup("actor"), markup("ident"),
				markup("title"), markup("severity"), markup("status"), desc, link, brand)
		}, "", []string{"assignee", "actor", "ident", "title", "severity", "status"}, "Incident assigned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers, body := capture(t, tc.send)
			if strings.Contains(body, "<b id=") || strings.Contains(body, "<script>") {
				t.Errorf("raw markup reached the body:\n%s", body)
			}
			for _, p := range tc.params {
				if want := "&lt;b id=" + p + "&gt;"; !strings.Contains(body, want) {
					t.Errorf("parameter %q not escaped (want %q):\n%s", p, want, body)
				}
			}
			for _, want := range []string{
				tc.heading,
				"&lt;script&gt;alert(1)&lt;/script&gt;",
				"**line three** &amp; more", // markdown shown literally, ampersand escaped once
				`white-space:pre-wrap`,
				`href="` + link + `"`,
				"background: #112233",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q:\n%s", want, body)
				}
			}
			// The description keeps its line breaks, inside the pre-wrap block.
			if !strings.Contains(body, "alert(1)&lt;/script&gt;\n\n**line three**") {
				t.Errorf("line breaks of the description were lost:\n%s", body)
			}
			if !strings.HasPrefix(body, "<div") {
				t.Errorf("body is not the branded layout:\n%s", body)
			}
			if tc.subject != "" && !strings.Contains(strings.Join(headers, "\n"), tc.subject) {
				t.Errorf("subject = %q, want it to contain %q", headers, tc.subject)
			}
		})
	}
}

func TestRecordAssignedEmailsReadNormally(t *testing.T) {
	const base = "https://isms.example/acme"
	headers, body := capture(t, func(m *Mailer) error {
		return m.SendIncidentAssignedBranded("a@x.io", "Eve", "Bob (bob@x.io)", "INC-12", "Phishing & spoofing", "high", "open",
			"Users got a mail.\nSecond line.", base+"/incidents/INC-12", Branding{Name: "Acme"})
	})
	h := strings.Join(headers, "\n")
	if !strings.Contains(h, "Subject: Acme — Incident [HIGH]: Phishing & spoofing") {
		t.Errorf("subject wrong: %q", headers)
	}
	for _, want := range []string{
		"Hi Eve,", "Bob (bob@x.io)", "INC-12", "Phishing &amp; spoofing", "Severity: <strong>high</strong>",
		"Status: <strong>open</strong>", "Users got a mail.\nSecond line.", `href="` + base + `/incidents/INC-12"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "&amp;amp;") {
		t.Errorf("double escaping:\n%s", body)
	}

	headers, body = capture(t, func(m *Mailer) error {
		return m.SendCorrectiveActionAssignedBranded("a@x.io", "Eve", "Bob", "CA-4", "Fix MFA", "minor_nc", "Do it",
			base+"/corrective-actions/CA-4", Branding{Name: "Acme"})
	})
	if !strings.Contains(strings.Join(headers, "\n"), "Subject: Acme — Corrective Action: Fix MFA") {
		t.Errorf("subject wrong: %q", headers)
	}
	for _, want := range []string{"CA-4", "Fix MFA", "Severity: <strong>minor_nc</strong>", `href="` + base + `/corrective-actions/CA-4"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func sampleDigest() *Digest {
	return &Digest{
		Sections: []DigestSection{
			{Title: "Reviews", Items: []DigestItem{
				{Ref: "doc-1", Title: "Access control policy", Detail: "Review requested by ann@x.io", Path: "/reviews/7"},
			}},
			{Title: "Tasks", Items: []DigestItem{
				{Ref: "TASK-3", Title: "Rotate <script>alert(1)</script> keys", Detail: "Due soon", Path: "/tasks/3"},
				{Ref: "TASK-4", Title: "Fix <b>bold</b> thing", Path: "/tasks/4"},
			}},
		},
		Overdue:      []DigestCount{{Label: "Risks", Count: 2}, {Label: "Suppliers", Count: 0}, {Label: "Tasks", Count: 1}},
		OverdueTotal: 3,
	}
}

func TestSendDigestBrandedRenders(t *testing.T) {
	host, port, data := fakeSMTP(t)
	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	err := m.SendDigestBranded("eve@x.io", "Eve", "https://isms.example/acme", sampleDigest(),
		Branding{Name: "Acme Corp", Color: "#ff0000"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	body := <-data
	for _, want := range []string{
		"Subject: Acme Corp",
		"Hi Eve",
		"Access control policy",
		"https://isms.example/acme/reviews/7",
		"https://isms.example/acme/tasks/3",
		"Reviews (1)",
		"Tasks (2)",
		"Risks: 2",
		"3 items overdue",
		"https://isms.example/acme/inbox",
		`href="https://isms.example/acme/settings"`,
		"#ff0000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Suppliers: 0") {
		t.Errorf("zero-count row should be omitted:\n%s", body)
	}
}

// Titles are user content: markup in them must arrive escaped.
func TestSendDigestBrandedEscapesUserContent(t *testing.T) {
	host, port, data := fakeSMTP(t)
	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	d := sampleDigest()
	d.Sections[0].Items[0].Ref = "<i>ref</i>"
	d.Sections[0].Items[0].Detail = "by <u>x</u>"
	if err := m.SendDigestBranded("eve@x.io", "Eve <b>X</b>", "https://isms.example", d, Branding{Name: "A&B"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	body := <-data
	for _, raw := range []string{"<script>", "<b>bold</b>", "<i>ref</i>", "<u>x</u>", "Eve <b>X</b>"} {
		if strings.Contains(body, raw) {
			t.Errorf("raw markup %q leaked into body:\n%s", raw, body)
		}
	}
	if !strings.Contains(body, "Subject: A&B") {
		t.Errorf("the subject is a header, not HTML, so the brand name stays literal:\n%s", body)
	}
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "Fix &lt;b&gt;bold&lt;/b&gt; thing"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing escaped %q:\n%s", want, body)
		}
	}
}

func TestDigestEmpty(t *testing.T) {
	d := &Digest{Overdue: []DigestCount{{Label: "Risks", Count: 4}}, OverdueTotal: 4}
	if !d.Empty() {
		t.Error("a digest with only a snapshot must be empty: the snapshot never triggers a send")
	}
	d.Sections = []DigestSection{{Title: "Tasks"}}
	if !d.Empty() {
		t.Error("a section with no items must still count as empty")
	}
	d.Sections[0].Items = []DigestItem{{Ref: "T-1", Title: "x"}}
	if d.Empty() {
		t.Error("a digest with an item is not empty")
	}
}

// serveOnce runs handle against the first connection on a loopback port.
func serveOnce(t *testing.T, handle func(conn net.Conn, r *bufio.Reader)) (host, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn, bufio.NewReader(conn))
	}()
	host, port, _ = net.SplitHostPort(ln.Addr().String())
	return host, port
}

// Once the server has answered 250 to the end of DATA the message is accepted,
// so a connection that drops before QUIT is answered must not turn it into a
// failure: a caller that retries on error would send the mail again.
func TestSendAcceptedMessageSurvivesDroppedQuit(t *testing.T) {
	host, port := serveOnce(t, func(conn net.Conn, r *bufio.Reader) {
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake ESMTP")
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case inData && line == ".":
				write("250 queued")
				return // drop the connection without answering QUIT
			case inData:
			case strings.HasPrefix(strings.ToUpper(line), "EHLO"), strings.HasPrefix(strings.ToUpper(line), "HELO"):
				write("250 fake")
			case strings.HasPrefix(strings.ToUpper(line), "DATA"):
				inData = true
				write("354 go ahead")
			default:
				write("250 OK")
			}
		}
	})
	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	if err := m.Send("eve@x.io", "subject", "<p>body</p>"); err != nil {
		t.Fatalf("an accepted message reported as failed: %v", err)
	}
}

// A message the server refuses at DATA is still an error.
func TestSendRefusedAtDataIsAnError(t *testing.T) {
	host, port := serveOnce(t, func(conn net.Conn, r *bufio.Reader) {
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch up := strings.ToUpper(line); {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				write("250 fake")
			case strings.HasPrefix(up, "DATA"):
				write("554 no")
			default:
				write("250 OK")
			}
		}
	})
	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	if err := m.Send("eve@x.io", "subject", "<p>body</p>"); err == nil {
		t.Fatal("a refused message reported as sent")
	}
}

// A relay that accepts the connection and never answers must not hang the
// sender: the overall deadline turns it into an error.
func TestSendStalledRelayTimesOut(t *testing.T) {
	old := sessionTimeout
	sessionTimeout = 300 * time.Millisecond
	t.Cleanup(func() { sessionTimeout = old })

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	host, port := serveOnce(t, func(conn net.Conn, r *bufio.Reader) { <-release })

	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	done := make(chan error, 1)
	go func() { done <- m.Send("eve@x.io", "subject", "<p>body</p>") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a stalled relay reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send did not return: no deadline on the SMTP session")
	}
}

// An address carrying a line break must never reach the wire.
func TestSendRefusesLineBreaksInAddresses(t *testing.T) {
	host, port, _ := fakeSMTP(t)
	m := New(Config{Host: host, Port: port, From: "noreply@isms.sh"})
	if err := m.Send("eve@x.io\r\nRCPT TO:<evil@x.io>", "s", "b"); err == nil {
		t.Fatal("a recipient with CRLF was accepted")
	}
}
