package main

import (
	"isms.sh/internal/isms/tui"
	"github.com/spf13/cobra"
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
