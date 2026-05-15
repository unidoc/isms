package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
)

const gitRemoteName = "origin"

func syncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync local ISMS repository with the server",
		Long: `Sync pushes and pulls the local git repo to/from the ISMS server.

All configuration comes from your env file:
  ISMS_BASE_URL       — server URL
  ISMS_API_TOKEN      — API token (also used for git auth)
  ISMS_ORGANIZATION   — organization UUID

On first run, sync auto-configures the git remote.
After that, sync does fetch + fast-forward then push.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseURL, token, orgSlug, err := syncEnv()
			if err != nil {
				return err
			}

			// Verify we are inside a git repo
			root, err := gitRoot()
			if err != nil {
				return fmt.Errorf("not in a git repository — run 'isms init' first or cd into your ISMS repo")
			}

			repo, err := git.PlainOpen(root)
			if err != nil {
				return fmt.Errorf("could not open git repository: %w", err)
			}

			// Auto-setup remote if not configured yet
			if !hasRemote(repo) {
				fmt.Println("Setting up isms remote...")
				if err := setupRemote(repo, baseURL, token, orgSlug); err != nil {
					return err
				}
			}

			return syncRun(repo, root, token)
		},
	}

	return cmd
}

// syncEnv resolves base URL, token, and org slug from environment.
func syncEnv() (baseURL, token, orgSlug string, err error) {
	token = os.Getenv("ISMS_API_TOKEN")
	if token == "" {
		return "", "", "", fmt.Errorf("ISMS_API_TOKEN not set. Configure it in your env file.")
	}

	baseURL = os.Getenv("ISMS_BASE_URL")
	if baseURL == "" {
		apiURL := os.Getenv("ISMS_API_URL")
		if apiURL != "" {
			baseURL = strings.TrimRight(apiURL, "/")
			baseURL = strings.TrimSuffix(baseURL, "/api")
		}
	}
	if baseURL == "" {
		return "", "", "", fmt.Errorf("ISMS_BASE_URL not set. Configure it in your env file.")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// ISMS_ORGANIZATION is the org UUID; resolve slug from API
	c := apiClient()
	if c != nil {
		info, err := c.WhoAmI()
		if err == nil && info.OrganizationSlug != "" {
			orgSlug = info.OrganizationSlug
		}
	}
	if orgSlug == "" {
		return "", "", "", fmt.Errorf("ISMS_ORGANIZATION not set, or could not resolve org from API. Configure it in your env file.")
	}

	return baseURL, token, orgSlug, nil
}

// hasRemote checks if the isms git remote exists.
func hasRemote(repo *git.Repository) bool {
	remotes, err := repo.Remotes()
	if err != nil {
		return false
	}
	for _, r := range remotes {
		if r.Config().Name == gitRemoteName {
			return true
		}
	}
	return false
}

// setupRemote configures the git remote using org UUID in the URL.
func setupRemote(repo *git.Repository, baseURL, token, orgSlug string) error {
	// Get org UUID from API
	c := apiClient()
	if c == nil {
		return fmt.Errorf("API client not configured")
	}
	info, err := c.WhoAmI()
	if err != nil {
		return fmt.Errorf("could not get org info: %w", err)
	}
	if info.OrganizationUUID == "" {
		return fmt.Errorf("token is not scoped to an organization")
	}

	// Git URL uses UUID (not guessable)
	remoteURL := baseURL + "/git/" + info.OrganizationUUID

	// Remove existing origin if it exists
	_ = repo.DeleteRemote(gitRemoteName)

	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: gitRemoteName,
		URLs: []string{remoteURL},
	})
	if err != nil {
		return fmt.Errorf("git remote add failed: %w", err)
	}

	// Store credential config for future operations
	cfg, gerr := repo.Config()
	if gerr == nil {
		credHelper := fmt.Sprintf("!f() { echo username=x-token-auth; echo password=%s; }; f", token)
		cfg.Raw.Section("credential").Subsection(remoteURL).SetOption("helper", credHelper)
		_ = repo.SetConfig(cfg)
	}

	fmt.Printf("Remote configured: %s → %s (%s)\n\n", gitRemoteName, info.OrganizationName, info.OrganizationSlug)
	return nil
}

// syncRun does the actual sync: fetch, fast-forward, push.
func syncRun(repo *git.Repository, root, token string) error {
	auth := &githttp.BasicAuth{
		Username: "x-token-auth",
		Password: token,
	}

	// Fetch
	fmt.Println("Fetching...")
	err := repo.Fetch(&git.FetchOptions{
		RemoteName: gitRemoteName,
		Auth:       auth,
		Progress:   os.Stdout,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("git fetch failed: %w", err)
	}

	// Determine current branch. If HEAD is detached, attach to the remote's
	// default branch — otherwise fast-forward and push both silently no-op on
	// the "HEAD" pseudo-ref and the user sees no working-tree update.
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("could not determine current branch — do you have any commits?")
	}

	// ISMS workflow is always on main. If HEAD is detached, or on any other
	// branch, snap back to main from the remote tip.
	if headRef.Name() != plumbing.NewBranchReferenceName("main") {
		attached, attachErr := attachToMain(repo)
		if attachErr != nil {
			return fmt.Errorf("could not attach local HEAD to main: %w", attachErr)
		}
		fmt.Println("Attached local HEAD to main.")
		headRef = attached
	}

	branch := "main"

	// Check if remote branch exists and fast-forward if needed
	remoteRefName := plumbing.NewRemoteReferenceName(gitRemoteName, branch)
	remoteRef, err := repo.Reference(remoteRefName, true)
	if err == nil && remoteRef != nil {
		localHash := headRef.Hash()
		remoteHash := remoteRef.Hash()

		if localHash != remoteHash {
			fmt.Printf("Pulling %s/%s...\n", gitRemoteName, branch)

			// Check if remote is ahead — try fast-forward
			// We do this by checking if the local HEAD is an ancestor of the remote
			localCommit, err := repo.CommitObject(localHash)
			if err != nil {
				return fmt.Errorf("could not read local commit: %w", err)
			}
			remoteCommit, err := repo.CommitObject(remoteHash)
			if err != nil {
				return fmt.Errorf("could not read remote commit: %w", err)
			}

			// Check if local is ancestor of remote (can fast-forward)
			isAncestor, err := localCommit.IsAncestor(remoteCommit)
			if err != nil {
				return fmt.Errorf("could not check ancestry: %w", err)
			}

			if isAncestor {
				// Fast-forward: update worktree to remote HEAD
				wt, err := repo.Worktree()
				if err != nil {
					return fmt.Errorf("could not get worktree: %w", err)
				}
				err = wt.Checkout(&git.CheckoutOptions{
					Hash: remoteHash,
				})
				if err != nil {
					return fmt.Errorf("fast-forward failed: %w", err)
				}
				// Update branch ref to point to new hash
				ref := plumbing.NewHashReference(headRef.Name(), remoteHash)
				err = repo.Storer.SetReference(ref)
				if err != nil {
					return fmt.Errorf("could not update branch ref: %w", err)
				}
				fmt.Println("Fast-forwarded to remote.")
			} else {
				// Check if remote is ancestor of local (we're ahead — push will handle it)
				isRemoteAncestor, _ := remoteCommit.IsAncestor(localCommit)
				if !isRemoteAncestor {
					// Diverged — cannot auto-resolve
					fmt.Fprintf(os.Stderr, "\nSync conflict: local and remote have diverged.\n")
					fmt.Fprintf(os.Stderr, "Resolve manually:\n")
					fmt.Fprintf(os.Stderr, "  git fetch origin\n")
					fmt.Fprintf(os.Stderr, "  git rebase origin/%s\n", branch)
					fmt.Fprintf(os.Stderr, "  isms sync\n")
					return fmt.Errorf("sync conflict — local and remote have diverged")
				}
				// else: local is ahead of remote, push will handle it
			}
		}
	}

	// Pre-push validation: check for duplicate document_ids
	if err := validateLocalDocIDs(root); err != nil {
		return fmt.Errorf("pre-push validation failed: %w", err)
	}

	// Push
	fmt.Printf("Pushing %s...\n", branch)
	err = repo.Push(&git.PushOptions{
		RemoteName: gitRemoteName,
		Auth:       auth,
		Progress:   os.Stdout,
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("git push failed: %w", err)
	}

	fmt.Println("Sync complete.")
	return nil
}

// attachToMain recovers a detached HEAD by creating (or updating) the local
// main branch from refs/remotes/origin/main and pointing HEAD at it, then
// resetting the working tree to the remote tip. ISMS workflow is always on
// main — no other branches, ever.
func attachToMain(repo *git.Repository) (*plumbing.Reference, error) {
	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(gitRemoteName, "main"), true)
	if err != nil || remoteRef == nil {
		return nil, fmt.Errorf("could not resolve origin/main — has the remote been fetched?")
	}

	localBranchRef := plumbing.NewBranchReferenceName("main")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(localBranchRef, remoteRef.Hash())); err != nil {
		return nil, fmt.Errorf("could not update local main: %w", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, localBranchRef)); err != nil {
		return nil, fmt.Errorf("could not update HEAD: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("could not get worktree: %w", err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: localBranchRef, Force: true}); err != nil {
		return nil, fmt.Errorf("could not checkout main: %w", err)
	}

	newHead, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("could not re-read HEAD after attach: %w", err)
	}
	return newHead, nil
}

// validateLocalDocIDs scans all .md files in the local repo for duplicate document_ids.
func validateLocalDocIDs(root string) error {
	docsDir := filepath.Join(root, "documents")
	if _, err := os.Stat(docsDir); err != nil {
		return nil // no documents directory
	}

	seen := map[string]string{} // lowercase id -> file path
	return filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(raw)

		// Quick extract document_id from frontmatter
		if !strings.HasPrefix(content, "---\n") {
			return nil
		}
		end := strings.Index(content[4:], "\n---\n")
		if end < 0 {
			return nil
		}
		fm := content[4 : 4+end]
		for _, line := range strings.Split(fm, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "document_id:") {
				val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				val = strings.Trim(val, "\"' ")
				if val == "" {
					break
				}
				id := strings.ToLower(val)
				rel, _ := filepath.Rel(root, path)
				if existing, ok := seen[id]; ok {
					return fmt.Errorf("duplicate document_id %q:\n  %s\n  %s", id, existing, rel)
				}
				seen[id] = rel
				break
			}
		}
		return nil
	})
}
