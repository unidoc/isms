package api

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"isms.sh/internal/isms/mail"
)

// Requires a migrated Postgres (ISMS_TEST_DATABASE_URL); skipped otherwise.

// oneMailSink is a minimal SMTP listener that returns the DATA of the first
// message it receives.
func oneMailSink(t *testing.T) (host, port string, data <-chan string) {
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

func waitMail(t *testing.T, data <-chan string) string {
	t.Helper()
	select {
	case m := <-data:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no email was sent")
		return ""
	}
}

// Creating a corrective action or an incident emails its assignee. The email
// used to be the record's raw description sent as HTML; it is now the branded,
// escaped layout with a link to the record.
func TestAssignmentEmailsForCorrectiveActionsAndIncidents(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "email-records")
	t.Setenv("ISMS_BASE_URL", "https://isms.example")
	const admin, assignee = "admin@email-records.test", "assignee@email-records.test"
	contractTestUser(t, s, orgID, admin, "admin")
	contractTestUser(t, s, orgID, assignee, "contributor")
	desc := `First line <a href="https://evil.example">click</a>` + "\n\nSecond paragraph"

	t.Run("corrective action", func(t *testing.T) {
		host, port, data := oneMailSink(t)
		s.mailer = mail.New(mail.Config{Host: host, Port: port, From: "noreply@isms.sh"})
		body := `{"title":"Fix <i>MFA</i>","description":` + jsonString(desc) + `,"source":"internal_audit","severity":"minor_nc","assignee":"` + assignee + `"}`
		c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/corrective-actions", body, "admin", admin)
		if err := s.handleCreateCorrectiveAction(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		raw := waitMail(t, data)
		for _, want := range []string{
			"Corrective action assigned", "Fix &lt;i&gt;MFA&lt;/i&gt;", "CA-", "&lt;a href=&#34;https://evil.example&#34;&gt;click&lt;/a&gt;\n\nSecond paragraph",
			"white-space:pre-wrap", "https://isms.example/", "/corrective-actions/CA-",
		} {
			if !strings.Contains(raw, want) {
				t.Errorf("email missing %q:\n%s", want, raw)
			}
		}
		// The subject is a header and carries the title as text; only the body is HTML.
		_, htmlBody, _ := strings.Cut(raw, "\n\n")
		if strings.Contains(htmlBody, `<a href="https://evil.example"`) || strings.Contains(htmlBody, "<i>MFA</i>") {
			t.Errorf("description or title reached the email as markup:\n%s", raw)
		}
	})

	t.Run("incident", func(t *testing.T) {
		host, port, data := oneMailSink(t)
		s.mailer = mail.New(mail.Config{Host: host, Port: port, From: "noreply@isms.sh"})
		body := `{"title":"Breach","description":` + jsonString(desc) + `,"severity":"high","incident_type":"incident","source":"internal","assignee":"` + assignee + `"}`
		c, rec := reviewCtxForPath(orgID, http.MethodPost, "/api/v1/incidents", body, "admin", admin)
		if err := s.handleCreateIncident(c); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		raw := waitMail(t, data)
		for _, want := range []string{
			"Subject: ", "Incident [HIGH]: Breach", "Incident assigned", "&lt;a href=&#34;https://evil.example&#34;&gt;click&lt;/a&gt;\n\nSecond paragraph",
			"INC-", "/incidents/INC-",
		} {
			if !strings.Contains(raw, want) {
				t.Errorf("email missing %q:\n%s", want, raw)
			}
		}
		if strings.Contains(raw, `<a href="https://evil.example"`) {
			t.Errorf("description reached the email as markup:\n%s", raw)
		}
	})
}
