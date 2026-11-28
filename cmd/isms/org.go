package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/timefmt"
)

func orgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage organizations (multi-tenant)",
	}

	cmd.AddCommand(orgCreateCmd(), orgListCmd(), orgAddMemberCmd(), orgMembersCmd(), orgSetRepoPathCmd(), orgSetTrustDomainCmd())
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
				if errors.Is(err, db.ErrOrgHostTaken) {
					return fmt.Errorf("creating organization: %s is already used by an organization's app or trust domain", domain)
				}
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
			if org.TrustDomain != nil {
				fmt.Printf("  Trust domain: %s\n", *org.TrustDomain)
			}
			fmt.Printf("  Created:   %s\n\n", timefmt.LocalTime(org.CreatedAt))
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

// validateRepoPath absolutises path and checks that it opens as a git repo.
func validateRepoPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving --path: %w", err)
	}
	if _, err := git.PlainOpen(abs); err != nil {
		return "", fmt.Errorf("%s is not a git repository: %w", abs, err)
	}
	return abs, nil
}

func orgSetRepoPathCmd() *cobra.Command {
	var slug, path string

	cmd := &cobra.Command{
		Use:   "set-repo-path",
		Short: "Update where an organization's git repo is stored (e.g. after moving the data dir)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if slug == "" {
				return fmt.Errorf("--org is required")
			}
			if path == "" {
				return fmt.Errorf("--path is required")
			}
			abs, err := validateRepoPath(path)
			if err != nil {
				return err
			}
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()
			org, err := d.GetOrganizationBySlug(ctx, slug)
			if err != nil {
				return fmt.Errorf("organization %q not found", slug)
			}
			if err := d.SetOrganizationRepoPath(ctx, org.ID, abs); err != nil {
				return fmt.Errorf("updating repo path: %w", err)
			}
			fmt.Printf("Repo path for %s updated:\n", org.Slug)
			fmt.Printf("  Old: %s\n", org.RepoPath)
			fmt.Printf("  New: %s\n", abs)
			fmt.Printf("\nRestart a running server to pick up the change.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&slug, "org", "", "Organization slug (required)")
	cmd.Flags().StringVar(&path, "path", "", "Path to the bare git repo (required)")
	return cmd
}

func orgSetTrustDomainCmd() *cobra.Command {
	var slug, domain string
	var clear bool

	cmd := &cobra.Command{
		Use:   "set-trust-domain",
		Short: "Serve an organization's trust center on its own host (e.g. trust.company.com), or stop doing so",
		RunE: func(cmd *cobra.Command, args []string) error {
			if slug == "" {
				return fmt.Errorf("--org is required")
			}
			if clear == (domain != "") {
				return fmt.Errorf("give exactly one of --domain or --clear")
			}
			var newDomain *string
			if !clear {
				norm, err := db.NormalizeTrustDomain(domain, db.ApexHost(os.Getenv("ISMS_DOMAIN"), os.Getenv("ISMS_BASE_URL")))
				if err != nil {
					return err
				}
				newDomain = &norm
			}
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			ctx := cmd.Context()
			org, err := d.GetOrganizationBySlug(ctx, slug)
			if err != nil {
				return fmt.Errorf("organization %q not found", slug)
			}
			if err := d.SetOrganizationTrustDomain(ctx, org.ID, newDomain); err != nil {
				if errors.Is(err, db.ErrOrgHostTaken) {
					return fmt.Errorf("%s is already used by an organization's app or trust domain", *newDomain)
				}
				return fmt.Errorf("updating trust domain: %w", err)
			}
			old, now := "(none)", "(none)"
			if org.TrustDomain != nil {
				old = *org.TrustDomain
			}
			if newDomain != nil {
				now = *newDomain
			}
			fmt.Printf("Trust domain for %s updated:\n", org.Slug)
			fmt.Printf("  Old: %s\n", old)
			fmt.Printf("  New: %s\n", now)
			return nil
		},
	}

	cmd.Flags().StringVar(&slug, "org", "", "Organization slug (required)")
	cmd.Flags().StringVar(&domain, "domain", "", "Host name to serve the trust center on (e.g. trust.example.com)")
	cmd.Flags().BoolVar(&clear, "clear", false, "Remove the organization's trust domain")
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
			fmt.Fprintln(w, "UUID\tNAME\tSLUG\tREPO PATH\tTRUST DOMAIN\tCREATED")
			for _, o := range orgs {
				trust := "-"
				if o.TrustDomain != nil {
					trust = *o.TrustDomain
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					o.UUID[:8]+"...", o.Name, o.Slug, o.RepoPath, trust,
					timefmt.LocalTime(o.CreatedAt))
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
