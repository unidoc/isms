package main

import (
	"fmt"
	"net/url"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"isms.sh/internal/isms/db"
)

func userCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
	}

	cmd.AddCommand(userCreateCmd(), userListCmd(), userSetPasswordCmd(), userVerifyCmd(), userTestAuthCmd(), userResetOTPCmd())
	return cmd
}

// userResetOTPCmd clears a user's OTP so they can log in and re-enroll. The
// primary use case: an OTP secret encrypted with a different ISMS_SECRET (e.g. a
// DB copied to a new install with its own secret) can no longer be decrypted, so
// GetUserByEmail errors and login fails with "invalid credentials" and no OTP
// prompt. Clearing OTP lets the user in with just their password to re-enroll.
func userResetOTPCmd() *cobra.Command {
	var email string
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset-otp",
		Short: "Remove a user's OTP (2FA) so they can log in and re-enroll",
		Long: `Clears otp_secret and otp_verified for the user.

Use this when the OTP secret can no longer be decrypted — typically after a DB
was copied to a new install that has its own ISMS_SECRET. The symptom is login
failing with "invalid credentials" and NO OTP prompt (GetUserByEmail errors on
the undecryptable OTP secret before the password check completes). After reset,
the user logs in with just their password and can re-enroll 2FA, which is then
encrypted with this install's ISMS_SECRET.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()
			user, err := d.GetUserByEmail(ctx, email)
			if err != nil || user == nil {
				return fmt.Errorf("user %q not found", email)
			}
			// This disables a security control (2FA), so confirm unless --yes —
			// cheap insurance against a fat-fingered --email.
			if !yes {
				fmt.Printf("Clear OTP (2FA) for %s? [y/N] ", user.Email)
				var resp string
				fmt.Scanln(&resp)
				if resp != "y" && resp != "Y" {
					return fmt.Errorf("aborted")
				}
			}
			if err := d.ClearOTP(ctx, user.ID); err != nil {
				return fmt.Errorf("clearing OTP: %w", err)
			}
			fmt.Printf("OTP cleared for %s. Log in with just the password, then re-enroll 2FA.\n", user.Email)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the confirmation prompt")
	return cmd
}

func userCreateCmd() *cobra.Command {
	var email, name, password string
	var isAgent bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new user",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()

			existing, _ := d.GetUserByEmail(ctx, email)
			if existing != nil {
				return fmt.Errorf("user %s already exists", email)
			}

			user := &db.User{Email: email, Name: name, IsAgent: isAgent, Active: true}
			if err := d.UpsertUser(ctx, user); err != nil {
				return fmt.Errorf("creating user: %w", err)
			}

			agentLabel := ""
			if isAgent {
				agentLabel = " [agent]"
			}

			if password != "" {
				if len(password) < 7 {
					return fmt.Errorf("password must be at least 7 characters")
				}
				hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if err != nil {
					return fmt.Errorf("hashing password: %w", err)
				}
				if err := d.SetPassword(ctx, user.ID, string(hash)); err != nil {
					return fmt.Errorf("saving password: %w", err)
				}
				fmt.Printf("User created: %s (%s)%s with password\n", name, email, agentLabel)
			} else {
				fmt.Printf("User created: %s (%s)%s\n", name, email, agentLabel)
			}

			fmt.Printf("\nNext steps:\n")
			if password == "" {
				fmt.Printf("  isms server user set-password --email %s --password <pw>\n", email)
			}
			fmt.Printf("  isms server org add-member --org <slug> --email %s --role manager\n", email)
			fmt.Printf("  isms server api-key create --name cli --email %s --org <slug>\n", email)

			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&name, "name", "", "User display name (required)")
	cmd.Flags().StringVar(&password, "password", "", "Set password (optional, can also use set-password)")
	cmd.Flags().BoolVar(&isAgent, "agent", false, "Mark user as an AI agent account")

	return cmd
}

func userListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			users, err := d.ListUsers(cmd.Context())
			if err != nil {
				return err
			}

			if len(users) == 0 {
				fmt.Println("No users.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tEMAIL\tNAME\tACTIVE\tLAST SEEN")
			for _, u := range users {
				lastSeen := "-"
				if u.LastSeen != nil {
					lastSeen = u.LastSeen.Format("2006-01-02 15:04")
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%v\t%s\n",
					u.ID, u.Email, u.Name, u.Active, lastSeen)
			}
			w.Flush()
			return nil
		},
	}
}

func userSetPasswordCmd() *cobra.Command {
	var email, password string

	cmd := &cobra.Command{
		Use:   "set-password",
		Short: "Set password for a user (enables local login)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}

			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()
			user, err := d.GetUserByEmail(ctx, email)
			if err != nil {
				return fmt.Errorf("user %q not found", email)
			}

			var pw []byte
			if password != "" {
				pw = []byte(password)
			} else {
				fmt.Printf("Setting password for %s (%s)\n", user.Name, user.Email)
				fmt.Print("Password: ")
				pw, err = term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err != nil {
					return fmt.Errorf("reading password: %w", err)
				}
			}
			if len(pw) < 7 {
				return fmt.Errorf("password must be at least 7 characters")
			}

			hash, err := bcrypt.GenerateFromPassword(pw, bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hashing password: %w", err)
			}

			if err := d.SetPassword(ctx, user.ID, string(hash)); err != nil {
				return fmt.Errorf("saving password: %w", err)
			}

			fmt.Printf("Password set for %s. Local login enabled.\n", user.Email)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&password, "password", "", "Password (optional, prompts if not provided)")
	return cmd
}

// userVerifyCmd marks a user's email as verified and activates the account.
// Useful when the verification email didn't arrive (transient SMTP failure,
// junk folder, sender reputation, etc.) and the user is stuck.
func userVerifyCmd() *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Mark a user's email as verified and activate the account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()
			user, err := d.GetUserByEmail(ctx, email)
			if err != nil || user == nil {
				return fmt.Errorf("user %q not found", email)
			}
			if err := d.SetEmailVerified(ctx, user.ID); err != nil {
				return fmt.Errorf("marking verified: %w", err)
			}
			if !user.Active {
				user.Active = true
				if err := d.UpsertUser(ctx, user); err != nil {
					return fmt.Errorf("activating user: %w", err)
				}
			}
			fmt.Printf("User %s verified and activated.\n", user.Email)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	return cmd
}

// userTestAuthCmd runs the exact credential checks that POST /auth/login
// performs — read-only, against this box's own DATABASE_URL. It isolates a
// login failure as either bad data (wrong DB / stale hash / inactive account)
// or transport (something altering the request before it reaches the API).
func userTestAuthCmd() *cobra.Command {
	var email, password string

	cmd := &cobra.Command{
		Use:   "test-auth",
		Short: "Test whether an email + password would authenticate (read-only, mirrors login)",
		Long: `Runs the same credential checks as POST /auth/login — user lookup, active
check, password-set check, and bcrypt comparison — against this box's own
DATABASE_URL. Read-only: records no login attempt and issues no token.

Use it to tell apart a data problem (this box points at a different DB, the row's
hash differs, or the account is inactive) from a transport problem (a proxy
altering the request before it reaches the API). OTP is reported but not required
here — the "invalid username or password" error happens before the OTP step.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}

			var pw []byte
			if password != "" {
				pw = []byte(password)
			} else {
				fmt.Print("Password: ")
				var err error
				pw, err = term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err != nil {
					return fmt.Errorf("reading password: %w", err)
				}
			}

			// Show which DB we're actually talking to (credentials redacted) —
			// the whole point is to confirm it's the DB you think it is.
			if raw := os.Getenv("DATABASE_URL"); raw != "" {
				if u, perr := url.Parse(raw); perr == nil {
					fmt.Printf("DB:    %s%s\n\n", u.Host, u.Path)
				}
			}

			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()

			user, err := d.GetUserByEmail(ctx, email)
			if err != nil || user == nil {
				fmt.Printf("✗ user %q NOT FOUND in this database\n\n", email)
				fmt.Println("VERDICT: FAIL — no such user here. This box points at a different")
				fmt.Println("         database than the one where the account exists (check DATABASE_URL).")
				return nil
			}
			fmt.Printf("✓ user found: id=%d name=%q agent=%v\n", user.ID, user.Name, user.IsAgent)

			if user.Active {
				fmt.Println("✓ account active")
			} else {
				fmt.Println("✗ account INACTIVE")
			}
			fmt.Printf("• email_verified=%v (not required for login)\n", user.EmailVerified)

			if !user.HasPassword() {
				fmt.Println("✗ no local password set (external-auth-only account)")
				fmt.Println()
				fmt.Println("VERDICT: FAIL — login returns \"invalid credentials\" (no local password).")
				return nil
			}
			hash := *user.PasswordHash
			prefix := hash
			if len(prefix) > 7 {
				prefix = prefix[:7]
			}
			fmt.Printf("✓ password hash present: %s… (len %d)\n", prefix, len(hash))

			bcryptErr := bcrypt.CompareHashAndPassword([]byte(hash), pw)
			if bcryptErr != nil {
				fmt.Printf("✗ bcrypt does NOT match: %v\n", bcryptErr)
			} else {
				fmt.Println("✓ bcrypt password MATCHES")
			}

			if user.HasOTP() {
				fmt.Println("• OTP is ENABLED — a valid TOTP code is also required at login")
			}

			// Final verdict mirrors handleLogin's decision order.
			fmt.Println()
			switch {
			case !user.Active:
				fmt.Println("VERDICT: FAIL — account is INACTIVE; login returns \"invalid credentials\"")
				fmt.Printf("         even with the right password. Fix: isms server user verify --email %s\n", email)
			case bcryptErr != nil:
				fmt.Println("VERDICT: FAIL — password does NOT match the stored hash in THIS database.")
				fmt.Println("         Either the password differs, or this row's hash differs from the working box")
				fmt.Println("         (a DB copy rather than the same DB).")
			default:
				fmt.Println("VERDICT: PASS — these credentials authenticate against this database.")
				fmt.Println("         If the web login still fails, the problem is between the browser and the")
				fmt.Println("         API (proxy/transport), not the credentials or the DB.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&password, "password", "", "Password (optional, prompts if not provided)")
	return cmd
}
