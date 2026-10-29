// Package mail provides SMTP email sending for the ISMS platform.
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"net"
	netmail "net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Config holds SMTP settings from environment variables.
type Config struct {
	Host     string // SMTP_HOST
	Port     string // SMTP_PORT (default "25")
	User     string // SMTP_USER
	Password string // SMTP_PASSWORD
	From     string // SMTP_FROM — either bare `noreply@host` or `"Display Name" <noreply@host>`
}

// ConfigFromEnv reads the SMTP_* environment variables. The server and the
// manager cron both build their mailer from it, so they cannot drift apart.
func ConfigFromEnv() Config {
	return Config{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     os.Getenv("SMTP_PORT"),
		User:     os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("SMTP_FROM"),
	}
}

// Mailer sends emails via SMTP.
type Mailer struct {
	config Config
}

// New creates a new Mailer. Returns nil if not configured.
func New(cfg Config) *Mailer {
	if cfg.Host == "" || cfg.From == "" {
		return nil
	}
	if cfg.Port == "" {
		cfg.Port = "25"
	}
	return &Mailer{config: cfg}
}

// Enabled returns true if the mailer is configured.
func (m *Mailer) Enabled() bool {
	return m != nil && m.config.Host != ""
}

// Branding holds per-org branding for email templates.
type Branding struct {
	Name  string // org display name (e.g. "Acme Corp")
	Color string // primary brand color (hex, default #2563eb)
}

func (b Branding) color() string {
	if b.Color != "" {
		return b.Color
	}
	return "#2563eb"
}

func (b Branding) name() string {
	if b.Name != "" {
		return b.Name
	}
	return "ISMS"
}

// resolveFrom builds the From header and the SMTP envelope sender.
//
// SMTP_FROM may be a bare email (`noreply@host`) or `"Display Name" <addr@host>`.
// The envelope sender (MAIL FROM) must always be the bare address — Postmark and
// most SMTP servers reject envelope senders with display-name syntax.
//
// When fromName is set it overrides the SMTP_FROM display name with the per-org
// brand while keeping the configured envelope address, so a tenant's mail shows
// the tenant's own name in the inbox — never the operator's SMTP_FROM display
// name (the multi-tenant sender leak this guards against). The envelope address
// is unchanged, so SPF/DKIM alignment is preserved.
func resolveFrom(configFrom, fromName string) (header, envelope string) {
	envelope = configFrom
	if strings.Contains(configFrom, "<") {
		if parsed, err := netmail.ParseAddress(configFrom); err == nil {
			envelope = parsed.Address
		}
	}
	if fromName == "" {
		return configFrom, envelope
	}
	// Address.String() quotes the display name and RFC 2047-encodes it when it
	// contains non-ASCII (org names like "Þórð ehf"), so the header stays valid.
	return (&netmail.Address{Name: fromName, Address: envelope}).String(), envelope
}

// PreviewFrom returns the From header and envelope sender that a branded send
// with brandName would produce, WITHOUT sending. For diagnostics (e.g.
// `isms server test-email --org`), so an operator can see the exact From a
// tenant's mail will carry. An empty brandName previews an unbranded send.
func PreviewFrom(configFrom, brandName string) (header, envelope string) {
	return resolveFrom(configFrom, brandName)
}

// Send sends an email using the configured SMTP_FROM as-is.
func (m *Mailer) Send(to, subject, body string) error {
	return m.sendAs(to, subject, body, "")
}

// SendBranded sends an email with the org brand as the From display name, so the
// recipient sees the tenant's name rather than the operator's SMTP_FROM identity.
func (m *Mailer) SendBranded(to, subject, body string, b Branding) error {
	return m.sendAs(to, subject, body, b.name())
}

// sendAs sends an email, overriding the From display name with fromName when set.
func (m *Mailer) sendAs(to, subject, body, fromName string) error {
	if m == nil {
		return fmt.Errorf("mailer not configured")
	}

	addr := net.JoinHostPort(m.config.Host, m.config.Port)
	fromHeader, envelopeFrom := resolveFrom(m.config.From, fromName)

	msg := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if m.config.User != "" {
		auth = smtp.PlainAuth("", m.config.User, m.config.Password, m.config.Host)
	}

	return deliver(m.config.Host, addr, auth, envelopeFrom, to, []byte(msg))
}

// Timeouts for one delivery. Without them a relay that accepts the connection
// and then stalls would hang the caller (the manager cron, or a request
// handler) forever. Variables so tests can shorten them.
var (
	dialTimeout = 30 * time.Second
	// sessionTimeout bounds the whole conversation, from connect to QUIT.
	sessionTimeout = 60 * time.Second
)

// deliver is net/smtp.SendMail with two differences: the connection has a dial
// timeout and an overall deadline, and once the server has accepted the message
// (the DATA terminator got its 250) nothing that happens afterwards is an
// error. SendMail returns the QUIT error, so a relay that took the message and
// then dropped the connection made a delivered mail look failed, and a caller
// that retries on failure (the weekly digest) sent it again.
//
// The rest keeps SendMail's safety behaviour: CR/LF in an address is refused,
// STARTTLS is used when offered, and AUTH is attempted only when asked for.
func deliver(host, addr string, auth smtp.Auth, from, to string, msg []byte) error {
	if strings.ContainsAny(from, "\r\n") || strings.ContainsAny(to, "\r\n") {
		return errors.New("smtp: A line must not contain CR or LF")
	}
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(sessionTimeout)); err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	if err := c.Hello("localhost"); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp: server doesn't support AUTH")
		}
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	// Accepted. A failed QUIT changes nothing about that; the deferred
	// conn.Close ends the session either way.
	_ = c.Quit()
	return nil
}

// SendVerification sends an email verification link.
func (m *Mailer) SendVerification(to, name, baseURL, token string) error {
	return m.SendVerificationBranded(to, name, baseURL, token, Branding{})
}

// SendVerificationBranded sends an email verification link with org branding.
func (m *Mailer) SendVerificationBranded(to, name, baseURL, token string, b Branding) error {
	link := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimRight(baseURL, "/"), token)

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Welcome to %s</h2>
<p>Hi %s,</p>
<p>You've been invited. Click the link below to verify your email and set your password:</p>
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">Verify Email &amp; Set Password</a></p>
<p>Or copy this link: %s</p>
<p style="color: #666; font-size: 12px;">This link expires in 72 hours.</p>
</div>`, b.name(), name, link, b.color(), link)

	return m.sendAs(to, fmt.Sprintf("%s — Verify your email", b.name()), body, b.name())
}

// SendEmailChangeVerification sends a confirmation link to the *new* address a
// user wants to switch to. Clicking it swaps the account's email over.
func (m *Mailer) SendEmailChangeVerification(to, name, baseURL, token string) error {
	return m.SendEmailChangeVerificationBranded(to, name, baseURL, token, Branding{})
}

// SendEmailChangeVerificationBranded sends the email-change confirmation link with org branding.
func (m *Mailer) SendEmailChangeVerificationBranded(to, name, baseURL, token string, b Branding) error {
	link := fmt.Sprintf("%s/verify-email-change?token=%s", strings.TrimRight(baseURL, "/"), token)

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Confirm your new %s email</h2>
<p>Hi %s,</p>
<p>We received a request to change your sign-in email to this address. Click the button below to confirm the change. Until you do, your current email stays active.</p>
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">Confirm new email</a></p>
<p>Or copy this link: %s</p>
<p style="color: #666; font-size: 12px;">This link expires in 2 hours. If you didn't request this change, you can safely ignore this email — nothing will change.</p>
</div>`, b.name(), name, link, b.color(), link)

	return m.sendAs(to, fmt.Sprintf("%s — Confirm your new email", b.name()), body, b.name())
}

// SendPasswordReset sends a password reset link. The same flow also activates
// unverified accounts — setting a new password via the reset link marks the
// email as verified.
func (m *Mailer) SendPasswordReset(to, name, baseURL, token string) error {
	return m.SendPasswordResetBranded(to, name, baseURL, token, Branding{})
}

// SendPasswordResetBranded sends a password reset link with org branding.
func (m *Mailer) SendPasswordResetBranded(to, name, baseURL, token string, b Branding) error {
	link := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimRight(baseURL, "/"), token)

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Reset your %s password</h2>
<p>Hi %s,</p>
<p>Click the button below to set a new password. The link is valid for 1 hour.</p>
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">Set new password</a></p>
<p>Or copy this link: %s</p>
<p style="color: #666; font-size: 12px;">If you didn't request a password reset, you can safely ignore this email.</p>
</div>`, b.name(), name, link, b.color(), link)

	return m.sendAs(to, fmt.Sprintf("%s — Reset your password", b.name()), body, b.name())
}

// SendReviewRequest notifies a reviewer that their review has been requested.
func (m *Mailer) SendReviewRequest(to, reviewerName, actor, docID, title, version, baseURL string, reviewID int, message string) error {
	return m.SendReviewRequestBranded(to, reviewerName, actor, docID, title, version, baseURL, reviewID, message, Branding{})
}

// SendReviewRequestBranded sends a review request with org branding.
func (m *Mailer) SendReviewRequestBranded(to, reviewerName, actor, docID, title, version, baseURL string, reviewID int, message string, b Branding) error {
	reviewerName = html.EscapeString(reviewerName)
	actor = html.EscapeString(actor)
	link := fmt.Sprintf("%s/reviews/%d", strings.TrimRight(baseURL, "/"), reviewID)

	note := ""
	if message != "" {
		note = fmt.Sprintf(`<p style="background:#f1f5f9; padding:12px; border-radius:6px; color:#334155;"><strong>Note:</strong> %s</p>`, message)
	}

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Review Requested</h2>
<p>Hi %s,</p>
<p><strong>%s</strong> has requested your review of:</p>
<p style="font-size:16px; font-weight:bold;">%s &mdash; %s (v%s)</p>
%s
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">Open Review</a></p>
<p>Or copy this link: %s</p>
</div>`, reviewerName, actor, docID, title, version, note, link, b.color(), link)

	return m.sendAs(to, fmt.Sprintf("%s — Review requested: %s v%s", b.name(), docID, version), body, b.name())
}

// SendReviewDecision notifies the document author that a review was approved or rejected.
func (m *Mailer) SendReviewDecision(to, authorName, reviewer, docID, title, version, decision, baseURL string) error {
	return m.SendReviewDecisionBranded(to, authorName, reviewer, docID, title, version, decision, baseURL, Branding{})
}

// SendReviewDecisionBranded sends a review decision with org branding.
func (m *Mailer) SendReviewDecisionBranded(to, authorName, reviewer, docID, title, version, decision, baseURL string, b Branding) error {
	authorName = html.EscapeString(authorName)
	reviewer = html.EscapeString(reviewer)
	link := fmt.Sprintf("%s/documents/%s", strings.TrimRight(baseURL, "/"), docID)

	icon := "approved"
	color := "#16a34a"
	if decision != "approved" {
		icon = "changes requested"
		color = "#dc2626"
	}

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2 style="color:%s;">Review %s</h2>
<p>Hi %s,</p>
<p><strong>%s</strong> has <strong style="color:%s;">%s</strong> your document:</p>
<p style="font-size:16px; font-weight:bold;">%s &mdash; %s (v%s)</p>
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">View Document</a></p>
</div>`, color, icon, authorName, reviewer, color, icon, docID, title, version, link, b.color())

	return m.sendAs(to, fmt.Sprintf("%s — Review %s: %s v%s", b.name(), icon, docID, version), body, b.name())
}

// SendTaskAssigned notifies an assignee about a new task.
func (m *Mailer) SendTaskAssigned(to, assigneeName, actor, taskTitle, priority, baseURL string) error {
	return m.SendTaskAssignedBranded(to, assigneeName, actor, taskTitle, priority, baseURL, Branding{})
}

// SendTaskAssignedBranded sends a task assignment with org branding.
func (m *Mailer) SendTaskAssignedBranded(to, assigneeName, actor, taskTitle, priority, baseURL string, b Branding) error {
	assigneeName = html.EscapeString(assigneeName)
	actor = html.EscapeString(actor)
	link := fmt.Sprintf("%s/tasks", strings.TrimRight(baseURL, "/"))

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Task Assigned</h2>
<p>Hi %s,</p>
<p><strong>%s</strong> has assigned you a task:</p>
<p style="font-size:16px; font-weight:bold;">%s</p>
<p>Priority: <strong>%s</strong></p>
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">View Tasks</a></p>
</div>`, assigneeName, actor, taskTitle, priority, link, b.color())

	return m.sendAs(to, fmt.Sprintf("%s — Task assigned: %s", b.name(), taskTitle), body, b.name())
}

// DigestItem is one row of a digest: something waiting on the recipient.
type DigestItem struct {
	Ref    string // identifier shown beside the title (INC-3, a document id)
	Title  string
	Detail string // one short line of context, may be empty
	Path   string // SPA route, joined to the org's app URL ("/reviews/7")
}

// DigestSection is a titled list of items of one kind.
type DigestSection struct {
	Title string
	Items []DigestItem
}

// DigestCount is one line of the org snapshot.
type DigestCount struct {
	Label string
	Count int
}

// Digest is the content of a personal digest email: the work waiting on the
// recipient, then a compact count of what is overdue across the org.
type Digest struct {
	Sections     []DigestSection
	Overdue      []DigestCount
	OverdueTotal int
}

// Empty reports whether nothing is waiting on the recipient. The org snapshot
// is deliberately ignored: a digest is only worth sending when it asks
// something of its reader.
func (d *Digest) Empty() bool {
	for _, s := range d.Sections {
		if len(s.Items) > 0 {
			return false
		}
	}
	return true
}

// digestMaxItems caps each section so one backlog cannot make the mail
// unreadable; the remainder is summarised and the Inbox link covers it.
const digestMaxItems = 10

// SendDigestBranded sends a recipient their pending-work digest. appURL is the
// org's app base (it already carries the org slug on path-based hosts), so
// every link lands in the right org. Every user-supplied string is escaped.
func (m *Mailer) SendDigestBranded(to, recipientName, appURL string, d *Digest, b Branding) error {
	base := strings.TrimRight(appURL, "/")
	color := html.EscapeString(b.color())
	esc := html.EscapeString

	total := 0
	var sections strings.Builder
	for _, s := range d.Sections {
		if len(s.Items) == 0 {
			continue
		}
		total += len(s.Items)
		fmt.Fprintf(&sections, `<h3 style="margin:24px 0 8px; font-size:15px; color:#0f172a;">%s (%d)</h3>`+"\n", esc(s.Title), len(s.Items))
		for i, it := range s.Items {
			if i == digestMaxItems {
				fmt.Fprintf(&sections, `<p style="margin:4px 0; color:#64748b;">and %d more in your <a href="%s/inbox" style="color:%s;">Inbox</a></p>`+"\n",
					len(s.Items)-digestMaxItems, esc(base), color)
				break
			}
			ref := ""
			if it.Ref != "" {
				ref = fmt.Sprintf(`<span style="color:#64748b;">%s</span> `, esc(it.Ref))
			}
			title := esc(it.Title)
			if it.Path != "" {
				title = fmt.Sprintf(`<a href="%s%s" style="color:%s; text-decoration:none; font-weight:bold;">%s</a>`, esc(base), esc(it.Path), color, title)
			} else {
				title = "<strong>" + title + "</strong>"
			}
			detail := ""
			if it.Detail != "" {
				detail = fmt.Sprintf(`<div style="color:#64748b; font-size:13px;">%s</div>`, esc(it.Detail))
			}
			fmt.Fprintf(&sections, `<div style="margin:6px 0;">%s%s%s</div>`+"\n", ref, title, detail)
		}
	}

	var snapshot strings.Builder
	if d.OverdueTotal == 0 {
		snapshot.WriteString(`<p style="margin:4px 0;">Nothing is overdue.</p>`)
	} else {
		noun := "items"
		if d.OverdueTotal == 1 {
			noun = "item"
		}
		fmt.Fprintf(&snapshot, `<p style="margin:4px 0;"><strong>%d %s overdue</strong></p>`, d.OverdueTotal, noun)
		for _, c := range d.Overdue {
			if c.Count > 0 {
				fmt.Fprintf(&snapshot, `<div style="margin:2px 0;">%s: %d</div>`, esc(c.Label), c.Count)
			}
		}
	}

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 560px; margin: 0 auto; color:#0f172a;">
<h2>Your weekly digest</h2>
<p>Hi %s, here is what is waiting on you.</p>
%s
<h3 style="margin:24px 0 8px; font-size:15px;">Overdue across the organization</h3>
%s
<p style="margin-top:24px;"><a href="%s/inbox" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">Open your Inbox</a></p>
<p style="color:#94a3b8; font-size:12px; margin-top:24px;">You get this email once a week. You can <a href="%s/settings" style="color:#94a3b8;">turn this digest off in Settings</a>.</p>
</div>`, esc(recipientName), sections.String(), snapshot.String(), esc(base), color, esc(base))

	subject := fmt.Sprintf("%s — Your weekly digest: %d waiting on you", b.name(), total)
	return m.sendAs(to, subject, body, b.name())
}

// SendOTPCode sends a one-time login code via email (magic link alternative),
// branded with the org so the From display name is the tenant's, not the
// operator's SMTP_FROM identity — consistent with every other Send* here.
func (m *Mailer) SendOTPCode(to, name, code string, b Branding) error {
	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Login Code</h2>
<p>Hi %s,</p>
<p>Your login code is:</p>
<p style="font-size: 32px; font-weight: bold; letter-spacing: 8px; padding: 16px; background: #f1f5f9; border-radius: 8px; text-align: center;">%s</p>
<p style="color: #666; font-size: 12px;">This code expires in 10 minutes.</p>
</div>`, name, code)

	return m.sendAs(to, fmt.Sprintf("%s — Login code", b.name()), body, b.name())
}
