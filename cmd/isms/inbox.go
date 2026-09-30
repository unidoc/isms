package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func inboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "View and manage incoming review items",
	}

	cmd.AddCommand(inboxListCmd(), inboxDumpCmd(), inboxResolveCmd())
	return cmd
}

func inboxListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show all open items needing your attention",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			items, err := c.InboxList()
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("Inbox empty — nothing needs your attention.")
				return nil
			}
			fmt.Printf("Inbox (%d items)\n\n", len(items))
			fmt.Printf("  %-6s %-10s %-14s %-36s %-20s %s\n",
				"ID", "TYPE", "DOCUMENT", "TITLE", "FROM", "STATUS")
			fmt.Printf("  %s\n", strings.Repeat("-", 100))
			for _, item := range items {
				status := item.Status
				if item.Role != "" {
					status += " (" + item.Role + ")"
				}
				fmt.Printf("  %-6d %-10s %-14s %-36s %-20s %s\n",
					item.ID,
					item.Type,
					item.DocumentID,
					truncate(item.Title, 36),
					truncate(item.From, 20),
					status,
				)
			}
			return nil
		},
	}
}

func inboxDumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dump",
		Short: "Dump all open items as JSON (for Claude Code to read)",
		Long:  "Outputs every review, comment, and task you are involved in as structured JSON that Claude can process. Each row has needs_action: true when it is waiting on you, false when you are only waiting on someone else.",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			data, err := c.InboxDump()
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		},
	}
}

func inboxResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <comment-id> [comment-id...]",
		Short: "Mark comments as resolved",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			for _, arg := range args {
				id := 0
				fmt.Sscanf(arg, "%d", &id)
				if id == 0 {
					fmt.Fprintf(os.Stderr, "  invalid comment ID: %s\n", arg)
					continue
				}
				if err := c.ResolveComment(id); err != nil {
					fmt.Fprintf(os.Stderr, "  error resolving #%d: %v\n", id, err)
				} else {
					fmt.Printf("  Resolved comment #%d\n", id)
				}
			}
			return nil
		},
	}
}
