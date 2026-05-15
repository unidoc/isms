package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/mcp"
)

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run MCP server for AI agents (stdio)",
		Long: `Runs a Model Context Protocol (MCP) server on stdin/stdout.

Exposes ISMS entities, documents, and suggestions as MCP tools
that AI agents can use to read operational data and propose changes.

Requires ISMS_API_URL and ISMS_API_TOKEN (or ISMS_API_KEY) environment variables
pointing at a running ISMS API server.

Example usage with Claude Code:
  {
    "mcpServers": {
      "isms": {
        "command": "isms",
        "args": ["server", "mcp"],
        "env": {
          "ISMS_API_URL": "https://isms.example.com",
          "ISMS_API_TOKEN": "tok_xxx"
        }
      }
    }
  }`,
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := os.Getenv("ISMS_API_URL")
			if apiURL == "" {
				if base := os.Getenv("ISMS_BASE_URL"); base != "" {
					apiURL = base
				}
			}
			apiToken := os.Getenv("ISMS_API_TOKEN")
			if apiToken == "" {
				apiToken = os.Getenv("ISMS_API_KEY")
			}
			if apiURL == "" || apiToken == "" {
				return fmt.Errorf("ISMS_API_URL and ISMS_API_TOKEN are required")
			}
			return mcp.Serve(apiURL, apiToken)
		},
	}
}
