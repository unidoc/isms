// Package mail provides SMTP email sending for the ISMS platform.
package mail

import (
	"fmt"
	"html"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/url"
	"strings"
)

// Config holds SMTP settings from environment variables.
type Config struct {
	Host     string // SMTP_HOST
	Port     string // SMTP_PORT (default "25")
	User     string // SMTP_USER
	Password string // SMTP_PASSWORD
	From     string // SMTP_FROM — either bare `noreply@host` or `"Display Name" <noreply@host>`
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

// lineBreaks turns every CR and LF into a space.
var lineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

func oneLine(s string) string { return lineBreaks.Replace(s) }

// esc escapes a value for HTML text or a quoted attribute. Every caller-supplied
// string goes through it at the point it is interpolated: the bodies are HTML.
func esc(s string) string { return html.EscapeString(s) }

// sendAs sends an email, overriding the From display name with fromName when set.
func (m *Mailer) sendAs(to, subject, body, fromName string) error {
	if m == nil {
		return fmt.Errorf("mailer not configured")
	}

	addr := net.JoinHostPort(m.config.Host, m.config.Port)
	// Subjects carry user content (titles, document ids) and the display name is
	// an org-chosen brand: a line break in either would start a new header or the
	// body, so both are flattened before they are written.
	subject = oneLine(subject)
	fromHeader, envelopeFrom := resolveFrom(m.config.From, oneLine(fromName))

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

	return smtp.SendMail(addr, auth, envelopeFrom, []string{to}, []byte(msg))
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
</div>`, esc(b.name()), esc(name), esc(link), esc(b.color()), esc(link))

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
</div>`, esc(b.name()), esc(name), esc(link), esc(b.color()), esc(link))

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
</div>`, esc(b.name()), esc(name), esc(link), esc(b.color()), esc(link))

	return m.sendAs(to, fmt.Sprintf("%s — Reset your password", b.name()), body, b.name())
}

// SendReviewRequest notifies a reviewer that their review has been requested.
func (m *Mailer) SendReviewRequest(to, reviewerName, actor, docID, title, version, baseURL string, reviewID int, message string) error {
	return m.SendReviewRequestBranded(to, reviewerName, actor, docID, title, version, baseURL, reviewID, message, Branding{})
}

// SendReviewRequestBranded sends a review request with org branding.
func (m *Mailer) SendReviewRequestBranded(to, reviewerName, actor, docID, title, version, baseURL string, reviewID int, message string, b Branding) error {
	link := fmt.Sprintf("%s/reviews/%d", strings.TrimRight(baseURL, "/"), reviewID)

	note := ""
	if message != "" {
		note = fmt.Sprintf(`<p style="background:#f1f5f9; padding:12px; border-radius:6px; color:#334155;"><strong>Note:</strong> %s</p>`, esc(message))
	}

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Review Requested</h2>
<p>Hi %s,</p>
<p><strong>%s</strong> has requested your review of:</p>
<p style="font-size:16px; font-weight:bold;">%s &mdash; %s (v%s)</p>
%s
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">Open Review</a></p>
<p>Or copy this link: %s</p>
</div>`, esc(reviewerName), esc(actor), esc(docID), esc(title), esc(version), note, esc(link), esc(b.color()), esc(link))

	return m.sendAs(to, fmt.Sprintf("%s — Review requested: %s v%s", b.name(), docID, version), body, b.name())
}

// SendReviewDecision notifies the document author that a review was approved or rejected.
func (m *Mailer) SendReviewDecision(to, authorName, reviewer, docID, title, version, decision, baseURL string) error {
	return m.SendReviewDecisionBranded(to, authorName, reviewer, docID, title, version, decision, baseURL, Branding{})
}

// SendReviewDecisionBranded sends a review decision with org branding.
func (m *Mailer) SendReviewDecisionBranded(to, authorName, reviewer, docID, title, version, decision, baseURL string, b Branding) error {
	link := fmt.Sprintf("%s/documents/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(docID))

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
</div>`, color, icon, esc(authorName), esc(reviewer), color, icon, esc(docID), esc(title), esc(version), esc(link), esc(b.color()))

	return m.sendAs(to, fmt.Sprintf("%s — Review %s: %s v%s", b.name(), icon, docID, version), body, b.name())
}

// SendTaskAssigned notifies an assignee about a new task.
func (m *Mailer) SendTaskAssigned(to, assigneeName, actor, taskTitle, priority, baseURL string) error {
	return m.SendTaskAssignedBranded(to, assigneeName, actor, taskTitle, priority, baseURL, Branding{})
}

// SendTaskAssignedBranded sends a task assignment with org branding.
func (m *Mailer) SendTaskAssignedBranded(to, assigneeName, actor, taskTitle, priority, baseURL string, b Branding) error {
	link := fmt.Sprintf("%s/tasks", strings.TrimRight(baseURL, "/"))

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Task Assigned</h2>
<p>Hi %s,</p>
<p><strong>%s</strong> has assigned you a task:</p>
<p style="font-size:16px; font-weight:bold;">%s</p>
<p>Priority: <strong>%s</strong></p>
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">View Tasks</a></p>
</div>`, esc(assigneeName), esc(actor), esc(taskTitle), esc(priority), esc(link), esc(b.color()))

	return m.sendAs(to, fmt.Sprintf("%s — Task assigned: %s", b.name(), taskTitle), body, b.name())
}

// SendCorrectiveActionAssignedBranded tells an assignee a corrective action was
// assigned to them. link is the full URL of the record. The description is free
// text, so it is escaped and shown with its line breaks kept; any markdown in it
// appears literally.
func (m *Mailer) SendCorrectiveActionAssignedBranded(to, assigneeName, actor, identifier, title, severity, description, link string, b Branding) error {
	return m.sendRecordAssigned(to, "Corrective action assigned", "a corrective action",
		fmt.Sprintf("%s — Corrective Action: %s", b.name(), title),
		assigneeName, actor, identifier, title, description, link, "View Corrective Action", b,
		[2]string{"Severity", severity})
}

// SendIncidentAssignedBranded tells an assignee an incident was assigned to
// them; see SendCorrectiveActionAssignedBranded.
func (m *Mailer) SendIncidentAssignedBranded(to, assigneeName, actor, identifier, title, severity, status, description, link string, b Branding) error {
	return m.sendRecordAssigned(to, "Incident assigned", "an incident",
		fmt.Sprintf("%s — Incident [%s]: %s", b.name(), strings.ToUpper(severity), title),
		assigneeName, actor, identifier, title, description, link, "View Incident", b,
		[2]string{"Severity", severity}, [2]string{"Status", status})
}

// sendRecordAssigned is the layout the two register assignment emails share
// (the task email's, plus the description). One helper with two thin wrappers
// keeps the wording and the escaping in one place.
func (m *Mailer) sendRecordAssigned(to, heading, what, subject, assigneeName, actor, identifier, title, description, link, button string, b Branding, facts ...[2]string) error {
	var details strings.Builder
	for _, f := range facts {
		fmt.Fprintf(&details, "<p>%s: <strong>%s</strong></p>\n", f[0], esc(f[1]))
	}
	desc := ""
	if strings.TrimSpace(description) != "" {
		desc = fmt.Sprintf(`<div style="white-space:pre-wrap; background:#f1f5f9; padding:12px; border-radius:6px; color:#334155;">%s</div>`+"\n", esc(description))
	}

	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>%s</h2>
<p>Hi %s,</p>
<p><strong>%s</strong> has assigned you %s:</p>
<p style="font-size:16px; font-weight:bold;">%s &mdash; %s</p>
%s%s<p><a href="%s" style="display: inline-block; padding: 12px 24px; background: %s; color: white; text-decoration: none; border-radius: 6px;">%s</a></p>
</div>`, heading, esc(assigneeName), esc(actor), what, esc(identifier), esc(title), details.String(), desc, esc(link), esc(b.color()), button)

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
</div>`, esc(name), esc(code))

	return m.sendAs(to, fmt.Sprintf("%s — Login code", b.name()), body, b.name())
}
