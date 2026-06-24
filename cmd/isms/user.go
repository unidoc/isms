package main

import (
	"fmt"
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

	cmd.AddCommand(userCreateCmd(), userListCmd(), userSetPasswordCmd(), userVerifyCmd())
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
