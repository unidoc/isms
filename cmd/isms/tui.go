package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/tui"
)

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive terminal reader for the local clone (offline)",
		Long:  "Browse and read the documents in your local ISMS clone, offline. Reads the clone at --root / ISMS_ROOT, or walks up from the current directory — run `isms clone` first if you don't have one.",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRepoRoot()
			if err != nil {
				return fmt.Errorf("no ISMS repo found — set ISMS_ROOT or cd into your clone (run `isms clone` first)")
			}
			return tui.Run(root)
		},
	}
}
