package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// gitCmd is a thin passthrough to the system `git` CLI that runs in the ISMS
// repo root and inherits the credential helper that `isms sync` configured
// for the remote. The user can run `isms git status`, `isms git log`,
// `isms git rebase origin/main`, etc. without having to chdir into the repo
// or worry about authenticating with the server — the credential helper in
// .git/config presents the ISMS API token as the git password automatically.
func gitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "git [-- git-args...]",
		Short:              "Run a git command inside the ISMS repo (auth handled)",
		Long:               "Thin passthrough to the system `git` CLI. Runs in the ISMS repo root and uses the credential helper that `isms sync` configured for the remote, so the user does not have to manage tokens.\n\nNote: --root is NOT supported here (args are passed straight through to the system git CLI) — use ISMS_ROOT to point at a clone from another directory.",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := exec.LookPath("git"); err != nil {
				return fmt.Errorf("git CLI not found on PATH — install it")
			}
			root, err := resolveRepoRoot()
			if err != nil {
				return fmt.Errorf("no ISMS repo found — set ISMS_ROOT, or cd into your clone (or run `isms clone`)")
			}
			// Refresh credential helper for the current API token. Idempotent —
			// catches older repos that were cloned before the helper code worked.
			if err := refreshCredentialHelperForRepo(root); err != nil {
				// Non-fatal: if there's no env/token we still try the git op (it
				// may be a read-only local command like `status` that doesn't auth).
				fmt.Fprintf(os.Stderr, "warning: could not refresh credential helper: %v\n", err)
			}
			full := append([]string{"-C", root}, args...)
			c := exec.Command("git", full...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
	return cmd
}
