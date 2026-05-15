package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func orgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage organizations (multi-tenant)",
	}

	cmd.AddCommand(orgCreateCmd(), orgListCmd(), orgAddMemberCmd(), orgMembersCmd())
	return cmd
}

func orgCreateCmd() *cobra.Command {
	var name, slug, repoPath, domain string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if slug == "" {
				return fmt.Errorf("--slug is required")
			}
			reserved := map[string]bool{
				"api": true, "app": true, "www": true, "mail": true, "smtp": true,
				"docs": true, "admin": true, "git": true, "login": true, "auth": true,
				"blog": true, "about": true, "pricing": true, "overview": true, "organization": true, "organizations": true, "test": true, "staging": true, "dev": true,
			}
			if reserved[slug] {
				return fmt.Errorf("slug %q is reserved", slug)
			}
			if repoPath == "" {
				dataDir := os.Getenv("ISMS_DATA_DIR")
				if dataDir == "" {
					return fmt.Errorf("ISMS_DATA_DIR is required (or use --repo-path)")
				}
				if !filepath.IsAbs(dataDir) {
					abs, _ := filepath.Abs(dataDir)
					dataDir = abs
				}
				repoPath = filepath.Join(dataDir, "repos", slug+".git")
			}
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()
			org := &db.Organization{
				Name:     name,
				Slug:     slug,
				RepoPath: repoPath,
			}
			if domain != "" {
				org.Domain = &domain
			}
			if err := d.CreateOrganization(ctx, org); err != nil {
				return fmt.Errorf("creating organization: %w", err)
			}

			// Initialize bare git repo if it doesn't exist.
			if _, err := os.Stat(org.RepoPath); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(org.RepoPath), 0o755); err != nil {
					return fmt.Errorf("creating repo parent dir: %w", err)
				}
				if _, err := git.PlainInit(org.RepoPath, true); err != nil {
					return fmt.Errorf("initializing bare repo: %w", err)
				}
				fmt.Printf("  Bare repo initialized: %s\n\n", org.RepoPath)
			}

			fmt.Printf("Organization created:\n\n")
			fmt.Printf("  UUID:      %s\n", org.UUID)
			fmt.Printf("  Name:      %s\n", org.Name)
			fmt.Printf("  Slug:      %s\n", org.Slug)
			fmt.Printf("  Repo:      %s\n", org.RepoPath)
			if org.Domain != nil {
				fmt.Printf("  Domain:    %s\n", *org.Domain)
			}
			fmt.Printf("  Created:   %s\n\n", org.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Printf("Set in your env file:\n")
			fmt.Printf("  ISMS_ORGANIZATION=%s\n\n", org.UUID)
			fmt.Printf("Add members with: isms org add-member --org %s --email user@example.com --role manager\n", org.Slug)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Organization display name (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "URL-safe slug (required)")
	cmd.Flags().StringVar(&repoPath, "repo-path", "", "Path to bare git repo (default: $ISMS_DATA_DIR/repos/{slug}.git or ./data/repos/{slug}.git)")
	cmd.Flags().StringVar(&domain, "domain", "", "Custom domain (e.g. isms.mycompany.com)")

	return cmd
}

func orgListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all organizations",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			orgs, err := d.ListOrganizations(cmd.Context())
			if err != nil {
				return err
			}

			if len(orgs) == 0 {
				fmt.Println("No organizations.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "UUID\tNAME\tSLUG\tREPO PATH\tCREATED")
			for _, o := range orgs {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					o.UUID[:8]+"...", o.Name, o.Slug, o.RepoPath,
					o.CreatedAt.Format("2006-01-02 15:04"))
			}
			w.Flush()
			return nil
		},
	}
}

func orgAddMemberCmd() *cobra.Command {
	var orgSlug, email, role string

	cmd := &cobra.Command{
		Use:   "add-member",
		Short: "Add a user to an organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			if orgSlug == "" {
				return fmt.Errorf("--org is required")
			}
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			if role == "" {
				role = "reader"
			}

			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()

			org, err := d.GetOrganizationBySlug(ctx, orgSlug)
			if err != nil {
				return fmt.Errorf("organization %q not found: %w", orgSlug, err)
			}

			user, err := d.GetUserByEmail(ctx, email)
			if err != nil {
				return fmt.Errorf("user %q not found — create the user first with: isms user create --email %s", email, email)
			}

			if err := d.AddOrgMember(ctx, org.ID, user.ID, role); err != nil {
				return fmt.Errorf("adding member: %w", err)
			}

			fmt.Printf("Added %s (%s) to %s as %s.\n", user.Name, user.Email, org.Name, role)
			return nil
		},
	}

	cmd.Flags().StringVar(&orgSlug, "org", "", "Organization slug (required)")
	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&role, "role", "reader", "Role: admin, manager, contributor, reader")

	return cmd
}

func orgMembersCmd() *cobra.Command {
	var orgSlug string

	cmd := &cobra.Command{
		Use:   "members",
		Short: "List members of an organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			if orgSlug == "" {
				return fmt.Errorf("--org is required")
			}

			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()

			org, err := d.GetOrganizationBySlug(ctx, orgSlug)
			if err != nil {
				return fmt.Errorf("organization %q not found: %w", orgSlug, err)
			}

			members, err := d.ListOrgUsers(ctx, org.ID)
			if err != nil {
				return err
			}

			if len(members) == 0 {
				fmt.Printf("No members in %s.\n", org.Name)
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tEMAIL\tNAME\tROLE\tACTIVE")
			for _, u := range members {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%v\n",
					u.ID, u.Email, u.Name, u.Role, u.Active)
			}
			w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVar(&orgSlug, "org", "", "Organization slug (required)")

	return cmd
}
