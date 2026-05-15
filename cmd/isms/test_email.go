package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/mail"
)

// testEmailCmd sends a test email using the current SMTP_* env vars, so an
// operator can verify mail delivery without going through the signup flow.
func testEmailCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test-email <recipient>",
		Short: "Send a test email to verify SMTP configuration",
		Long: `Send a test email to the given recipient using the SMTP_* environment
variables. Useful for confirming Postmark / SendGrid / SES credentials are correct
before relying on signup verification or review-notification emails.

Required env vars:
  SMTP_HOST       (e.g. smtp.postmarkapp.com)
  SMTP_PORT       (e.g. 587)
  SMTP_USER       (Postmark server token)
  SMTP_PASSWORD   (same as user for Postmark)
  SMTP_FROM       (verified sender; bare email or "Display Name <addr@host>")`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			to := args[0]

			cfg := mail.Config{
				Host:     os.Getenv("SMTP_HOST"),
				Port:     os.Getenv("SMTP_PORT"),
				User:     os.Getenv("SMTP_USER"),
				Password: os.Getenv("SMTP_PASSWORD"),
				From:     os.Getenv("SMTP_FROM"),
			}
			fmt.Printf("SMTP config: host=%s port=%s user=%s from=%q (password=%s)\n",
				cfg.Host, cfg.Port, cfg.User, cfg.From, maskedLen(cfg.Password))

			m := mail.New(cfg)
			if m == nil || !m.Enabled() {
				return fmt.Errorf("mailer not configured — set SMTP_HOST and SMTP_FROM at minimum")
			}

			subject := "ISMS test email"
			body := `<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>SMTP test successful</h2>
<p>If you can read this, your ISMS SMTP configuration is working.</p>
<p style="color: #666; font-size: 12px;">Sent by <code>isms server test-email</code>.</p>
</div>`

			if err := m.Send(to, subject, body); err != nil {
				return fmt.Errorf("send failed: %w", err)
			}
			fmt.Printf("Test email sent to %s\n", to)
			return nil
		},
	}
}

// maskedLen returns "(N chars)" or "(empty)" for masked secret display.
func maskedLen(s string) string {
	if s == "" {
		return "(empty)"
	}
	return fmt.Sprintf("(%d chars)", len(s))
}
