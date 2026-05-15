package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"isms.sh/internal/isms/db"
	"github.com/spf13/cobra"
)

// connectDB connects to Postgres using DATABASE_URL.
// Used only for API key management which requires direct DB access.
func connectDB() (*db.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL not set")
	}
	return db.New(context.Background(), dbURL)
}

func apiKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api-key",
		Short: "Manage personal access tokens for CLI and AI agent access",
	}

	cmd.AddCommand(apiKeyCreateCmd(), apiKeyListCmd(), apiKeyRevokeCmd())
	return cmd
}

func apiKeyCreateCmd() *cobra.Command {
	var name, email, permissions, orgSlug string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new personal access token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if email == "" {
				return fmt.Errorf("--email is required")
			}

			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()

			// Resolve org scope if --org is provided
			var orgScope *int
			if orgSlug != "" {
				org, err := d.GetOrganizationBySlug(ctx, orgSlug)
				if err != nil {
					return fmt.Errorf("organization %q not found: %w", orgSlug, err)
				}
				orgScope = &org.ID
			}

			// Generate random key
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return fmt.Errorf("generating API key: %w", err)
			}
			token := "isms_" + hex.EncodeToString(raw)

			// Hash for storage
			hash := sha256.Sum256([]byte(token))
			tokenHash := hex.EncodeToString(hash[:])

			tok, err := d.CreateAPIKey(ctx, name, tokenHash, email, permissions, orgScope, nil)
			if err != nil {
				return fmt.Errorf("creating API key: %w", err)
			}

			fmt.Printf("Personal access token created successfully:\n\n")
			fmt.Printf("  Name:        %s\n", tok.Name)
			fmt.Printf("  Email:       %s\n", tok.UserEmail)
			fmt.Printf("  Permissions: %s\n", tok.Permissions)
			if tok.OrganizationID != nil {
				fmt.Printf("  Org scope:   %d\n", *tok.OrganizationID)
			} else {
				fmt.Printf("  Org scope:   global (all orgs)\n")
			}
			fmt.Printf("  Key:         %s\n\n", token)
			fmt.Printf("Save this key — it will not be shown again.\n")
			fmt.Printf("Use it with: ISMS_API_KEY=%s\n", token)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Token name (e.g. \"claude-agent\")")
	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&permissions, "permissions", "read-write", "Permissions: read, write, read-write")
	cmd.Flags().StringVar(&orgSlug, "org", "", "Organization slug to scope the token to (omit for global)")

	return cmd
}

func apiKeyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			keys, err := d.ListAPIKeys(cmd.Context())
			if err != nil {
				return err
			}

			if len(keys) == 0 {
				fmt.Println("No API keys.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tEMAIL\tPERMISSIONS\tORG SCOPE\tCREATED\tLAST USED\tSTATUS")
			for _, t := range keys {
				status := "active"
				if t.RevokedAt != nil {
					status = "revoked"
				}
				lastUsed := "-"
				if t.LastUsedAt != nil {
					lastUsed = t.LastUsedAt.Format("2006-01-02 15:04")
				}
				orgScope := "global"
				if t.OrganizationID != nil {
					orgScope = fmt.Sprintf("%d", *t.OrganizationID)
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					t.ID, t.Name, t.UserEmail, t.Permissions, orgScope,
					t.CreatedAt.Format("2006-01-02 15:04"),
					lastUsed, status)
			}
			w.Flush()
			return nil
		},
	}
}

func apiKeyRevokeCmd() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "revoke <key-id>",
		Short: "Revoke an API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid API key ID: %s", args[0])
			}
			if email == "" {
				return fmt.Errorf("--email is required (owner of the token)")
			}

			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()

			// Look up user by email to get user ID
			user, err := d.GetUserByEmail(ctx, email)
			if err != nil {
				return fmt.Errorf("user %q not found: %w", email, err)
			}

			if err := d.RevokeAPIKey(ctx, user.ID, id); err != nil {
				return fmt.Errorf("revoking API key: %w", err)
			}

			fmt.Printf("API key %d revoked.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Token owner email (required)")
	return cmd
}
