package main

import (
	"github.com/spf13/cobra"
	"isms.sh/internal/isms/tui"
)

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			return tui.Run(c)
		},
	}
}
