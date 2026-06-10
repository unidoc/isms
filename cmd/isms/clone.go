package main

import (
	"fmt"
	"os"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
)

func cloneCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "clone [directory]",
		Short: "Clone the ISMS repository from the server",
		Long: `Clone the organization's ISMS git repository from the server.

Directory defaults to {org-slug}-isms. All config comes from your env file:
  ISMS_BASE_URL, ISMS_API_TOKEN, ISMS_ORGANIZATION`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get base URL and token from env
			token := os.Getenv("ISMS_API_TOKEN")
			if token == "" {
				return fmt.Errorf("ISMS_API_TOKEN not set. Configure it in your env file.")
			}
			baseURL := os.Getenv("ISMS_BASE_URL")
			if baseURL == "" {
				apiURL := os.Getenv("ISMS_API_URL")
				if apiURL != "" {
					baseURL = strings.TrimSuffix(strings.TrimRight(apiURL, "/"), "/api")
				}
			}
			if baseURL == "" {
				return fmt.Errorf("ISMS_BASE_URL not set. Configure it in your env file.")
			}
			baseURL = strings.TrimRight(baseURL, "/")

			// Get org info from API (UUID + slug)
			c := requireAPI()
			info, err := c.WhoAmI()
			if err != nil {
				return fmt.Errorf("API connection failed: %w", err)
			}
			if info.OrganizationUUID == "" || info.OrganizationSlug == "" {
				return fmt.Errorf("token is not scoped to an organization")
			}

			// Target directory
			target := dir
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			if target == "" {
				target = info.OrganizationSlug + "-isms"
			}

			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("directory %s already exists", target)
			}

			// Git URL uses UUID (not guessable)
			remoteURL := baseURL + "/git/" + info.OrganizationUUID

			fmt.Printf("Cloning %s (%s) into %s...\n", info.OrganizationName, info.OrganizationSlug, target)
			repo, err := clonePinnedToMain(target, remoteURL, &githttp.BasicAuth{
				Username: "x-token-auth",
				Password: token,
			})
			if err != nil {
				return fmt.Errorf("git clone failed: %w", err)
			}

			// Set up the credential helper so future `git` invocations
			// (whether via `isms git`, `isms sync` auto-rebase, or direct git
			// CLI) authenticate against the ISMS server without prompting.
			if err := ensureCredentialHelper(repo, remoteURL, token); err != nil {
				return fmt.Errorf("credential helper setup: %w", err)
			}

			// Ensure remote URL is clean (no embedded credentials)
			_ = repo.DeleteRemote("origin")
			_, _ = repo.CreateRemote(&gitconfig.RemoteConfig{
				Name: "origin",
				URLs: []string{remoteURL},
			})

			fmt.Printf("\nDone. cd %s and start working.\n", target)

			// Check if repo is empty
			head, err := repo.Head()
			if err != nil || head == nil {
				fmt.Printf("\nRepo is empty. Initialize with:\n")
				fmt.Printf("  cd %s\n", target)
				fmt.Printf("  isms init %s\n", info.OrganizationSlug)
				fmt.Printf("  isms sync\n")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Target directory (default: {slug}-isms)")
	return cmd
}

// clonePinnedToMain clones remoteURL into target, pinning the checkout to the
// main branch so HEAD is never left detached — belt-and-suspenders independent
// of the server's HEAD advertisement. ISMS has exactly one branch (main).
func clonePinnedToMain(target, remoteURL string, auth *githttp.BasicAuth) (*git.Repository, error) {
	return git.PlainClone(target, false, &git.CloneOptions{
		URL:           remoteURL,
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		Auth:          auth,
		Progress:      os.Stdout,
	})
}
