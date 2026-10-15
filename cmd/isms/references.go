package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/refs"
	"isms.sh/internal/isms/store"
)

// exitError asks main to exit with a code without printing anything: the
// command has already written its report, and "found broken references" is a
// result, not a crash.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func referencesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "references",
		Short: "Maintain entity references (the links between records)",
	}
	cmd.AddCommand(referencesCheckCmd())
	return cmd
}

func referencesCheckCmd() *cobra.Command {
	var orgSlug string
	var fix bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Find (and with --fix, delete) references that point at nothing",
		Long: `Check every stored reference with the same rule the API enforces when a link is
written, and list the ones that do not resolve: an id that is not in identifier
form (a bare row number), a record that does not exist, a record that was
deleted, or a document that is not in the org's repo.

A reference is a pair of rows, one per direction. A pair is broken when either
end fails to resolve. If a lookup itself fails (the org's repo is missing on
this machine, the database errors) the pair is reported as unchecked and is
never treated as broken.

Without --fix the command is read-only. With --fix it deletes every broken pair
after listing it; unchecked pairs are never deleted.

Run it on the server machine, with the server's environment: it needs
DATABASE_URL and read access to every org's git repo.

Exit status: 0 when everything resolves (with --fix: when every broken pair was
deleted and nothing was unchecked), 1 when there are broken or unchecked pairs
left, and non-zero on any error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Past argument parsing: from here a failure is not a usage mistake.
			cmd.SilenceUsage = true
			d, err := connectDB()
			if err != nil {
				return err
			}
			defer d.Close()

			res, err := refs.Check(cmd.Context(), d, os.Stdout, refs.CheckOptions{
				OrgSlug:   orgSlug,
				Fix:       fix,
				OpenStore: openOrgRepo,
			})
			if err != nil {
				return err
			}
			return referencesExit(res)
		},
	}
	// main prints returned errors; keep cobra from printing them a second time.
	cmd.SilenceErrors = true
	cmd.Flags().StringVar(&orgSlug, "org", "", "Only check this organization (slug)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Delete the broken pairs (default: report only)")
	return cmd
}

// referencesExit maps a check result to the exit status described in the help.
func referencesExit(res refs.CheckResult) error {
	remaining := res.Broken - res.Deleted
	if remaining > 0 || res.Unchecked > 0 {
		return &exitError{code: 1}
	}
	return nil
}

// openOrgRepo opens an org's bare repo read-only: PlainOpen never creates one,
// so a repo missing on this machine is an error, not an empty store.
func openOrgRepo(org db.Organization) (*store.Store, error) {
	path, fellBack, err := store.ResolveRepoPath(org.RepoPath, os.Getenv("ISMS_DATA_DIR"), org.Slug)
	if err != nil {
		return nil, err
	}
	if fellBack {
		fmt.Fprintf(os.Stderr, "warning: org %s: stored repo path %s cannot be opened; using %s under ISMS_DATA_DIR\n",
			org.Slug, org.RepoPath, path)
	}
	return store.NewBare(path)
}
