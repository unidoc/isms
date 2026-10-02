package mail

import (
	"bufio"
	"net"
	netmail "net/mail"
	"strings"
	"testing"
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
