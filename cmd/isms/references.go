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
		Short: "Find (and with --fix, delete or merge) references that point at nothing or use the wrong id",
		Long: `Check every stored reference with the same rule the API enforces when a link is
written, and list the ones that do not resolve: an id that is not in identifier
form (a bare row number), a record that does not exist, a record that was
deleted, or a document that is not in the org's repo.

A reference is a pair of rows, one per direction. A pair is broken when either
end fails to resolve. If a lookup itself fails (the org's repo is missing on
this machine, the database errors) the pair is reported as unchecked and is
never treated as broken.

A second check, listed after the broken links, finds links that resolve but are
stored under another spelling of the same record than the one the API writes
now: a program by row id instead of its key, an objective by row id instead of
its display id, an audit, finding or audit programme by bare number instead of
AUDIT-, FIND- or AP-. The same record can then be linked twice. Each such pair is
shown with the id it should have and the action --fix would take.

Without --fix the command is read-only. With --fix it deletes every broken pair
after listing it, then rewrites each such link to the canonical id, or, when the
record is already linked under that id, merges the two (the link stays manual if
either one was). Comment mentions stored under the old spelling are rewritten
too. Unchecked pairs are never touched.

Run it on the server machine, with the server's environment: it needs
DATABASE_URL and read access to every org's git repo.

Exit status: 0 when everything resolves and uses its canonical id (with --fix:
when every broken pair was deleted, every non-canonical link was rewritten or
merged, and nothing was unchecked), 1 when broken, unchecked or non-canonical
links are left, and non-zero on any error.`,
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
	cmd.Flags().BoolVar(&fix, "fix", false, "Delete the broken pairs and rewrite or merge non-canonical ones (default: report only)")
	return cmd
}

// referencesExit maps a check result to the exit status described in the help.
func referencesExit(res refs.CheckResult) error {
	remaining := res.Broken - res.Deleted
	nonCanonical := res.NonCanonical - res.Fixed
	comments := res.NonCanonicalComments - res.FixedComments
	if remaining > 0 || res.Unchecked > 0 || nonCanonical > 0 || comments > 0 {
		return &exitError{code: 1}
	}
	return nil
}

// openOrgRepo opens an org's bare repo read-only: PlainOpen never creates one,
// so a repo missing on this machine is an error, not an empty store.
func openOrgRepo(org db.Organization) (*store.Store, error) {
	return store.NewBare(org.RepoPath)
}
