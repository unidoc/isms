package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Set via -ldflags at build time.
// Use: -ldflags "-X main.version=... -X main.commitHash=... -X main.commitCount=..."
var (
	version     = "dev"
	commitHash  = "unknown"
	commitCount = "0"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("isms %s.%s (%s)\n", version, commitCount, commitHash)
		},
	}
}
