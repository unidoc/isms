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
	var orgSlug string
	cmd := &cobra.Command{
		Use:   "test-email <recipient>",
		Short: "Send a test email to verify SMTP configuration",
		Long: `Send a test email to the given recipient using the SMTP_* environment
variables. Useful for confirming Postmark / SendGrid / SES credentials are correct
before relying on signup verification or review-notification emails.

With --org <slug>, the email is sent the way a tenant's real mail is: the org's
name becomes the From display name (e.g. "STS" <noreply@host>) while the envelope
sender stays SMTP_FROM, so SPF/DKIM alignment is preserved. Without --org, the
raw SMTP_FROM is used as-is. --org needs DATABASE_URL set.

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

			// Resolve org branding when --org is given. SendBranded/PreviewFrom
			// only consume Branding.Name (the From display name) — the generic
			// test body isn't a colored template — so we set just the name.
			var branding mail.Branding
			if orgSlug != "" {
				d, err := connectDB()
				if err != nil {
					return err
				}
				defer d.Close()
				ctx := cmd.Context()
				org, err := d.GetOrganizationBySlug(ctx, orgSlug)
				if err != nil || org == nil {
					return fmt.Errorf("organization %q not found", orgSlug)
				}
				branding.Name = org.Name
				header, envelope := mail.PreviewFrom(cfg.From, branding.Name)
				fmt.Printf("Org context: %s (%s)\n", org.Name, org.Slug)
				fmt.Printf("From header: %s\n", header)
				fmt.Printf("Envelope:    %s\n", envelope)
			}

			subject := "ISMS test email"
			body := `<div style="font-family: sans-serif; max-width: 500px; margin: 0 auto;">
<h2>SMTP test successful</h2>
<p>If you can read this, your ISMS SMTP configuration is working.</p>
<p style="color: #666; font-size: 12px;">Sent by <code>isms server test-email</code>.</p>
</div>`

			var sendErr error
			if orgSlug != "" {
				sendErr = m.SendBranded(to, subject, body, branding)
			} else {
				sendErr = m.Send(to, subject, body)
			}
			if sendErr != nil {
				return fmt.Errorf("send failed: %w", sendErr)
			}
			fmt.Printf("Test email sent to %s\n", to)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgSlug, "org", "", "Send as this org (branded From display name); needs DATABASE_URL")
	return cmd
}

// maskedLen returns "(N chars)" or "(empty)" for masked secret display.
func maskedLen(s string) string {
	if s == "" {
		return "(empty)"
	}
	return fmt.Sprintf("(%d chars)", len(s))
}
