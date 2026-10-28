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
