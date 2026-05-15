// Package mail provides SMTP email sending for the ISMS platform.
package mail

import (
	"fmt"
	"net"
	netmail "net/mail"
	"net/smtp"
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

// Send sends an email.
func (m *Mailer) Send(to, subject, body string) error {
	if m == nil {
		return fmt.Errorf("mailer not configured")
	}

	addr := net.JoinHostPort(m.config.Host, m.config.Port)

	// SMTP_FROM may be either a bare email or `"Display Name" <addr@host>`.
	// The header takes the full string; the SMTP envelope sender (MAIL FROM)
	// must be the bare address only — Postmark and most SMTP servers reject
	// envelope senders with display-name syntax.
	fromHeader := m.config.From
	envelopeFrom := m.config.From
	if strings.Contains(m.config.From, "<") {
		if parsed, err := netmail.ParseAddress(m.config.From); err == nil {
			envelopeFrom = parsed.Address
		}
	}

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
</div>`, b.name(), name, link, b.color(), link)

	return m.Send(to, fmt.Sprintf("%s — Verify your email", b.name()), body)
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

	return m.Send(to, fmt.Sprintf("%s — Reset your password", b.name()), body)
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

	return m.Send(to, fmt.Sprintf("%s — Review requested: %s v%s", b.name(), docID, version), body)
}

// SendReviewDecision notifies the document author that a review was approved or rejected.
func (m *Mailer) SendReviewDecision(to, authorName, reviewer, docID, title, version, decision, baseURL string) error {
	return m.SendReviewDecisionBranded(to, authorName, reviewer, docID, title, version, decision, baseURL, Branding{})
}

// SendReviewDecisionBranded sends a review decision with org branding.
func (m *Mailer) SendReviewDecisionBranded(to, authorName, reviewer, docID, title, version, decision, baseURL string, b Branding) error {
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

	return m.Send(to, fmt.Sprintf("%s — Review %s: %s v%s", b.name(), icon, docID, version), body)
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
</div>`, assigneeName, actor, taskTitle, priority, link, b.color())

	return m.Send(to, fmt.Sprintf("%s — Task assigned: %s", b.name(), taskTitle), body)
}

// SendOTPCode sends a one-time login code via email (magic link alternative).
func (m *Mailer) SendOTPCode(to, name, code string) error {
	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>Login Code</h2>
<p>Hi %s,</p>
<p>Your login code is:</p>
<p style="font-size: 32px; font-weight: bold; letter-spacing: 8px; padding: 16px; background: #f1f5f9; border-radius: 8px; text-align: center;">%s</p>
<p style="color: #666; font-size: 12px;">This code expires in 10 minutes.</p>
</div>`, name, code)

	return m.Send(to, "Login code", body)
}
