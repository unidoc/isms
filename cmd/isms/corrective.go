package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func correctiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "corrective",
		Aliases: []string{"ca"},
		Short:   "Corrective actions management",
	}

	cmd.AddCommand(
		correctiveAddCmd(),
		correctiveListCmd(),
		correctiveShowCmd(),
		correctiveUpdateCmd(),
		correctiveStatusCmd(),
		correctiveRmCmd(),
	)
	return cmd
}

func correctiveAddCmd() *cobra.Command {
	var title, description, source, severity, assignee, due string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new corrective action (link to source entities via 'isms reference add' after create)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			ca := &db.CorrectiveAction{
				Title:       title,
				Description: description,
				Source:      source,
				Severity:    severity,
				Assignee:    assignee,
			}
			if due != "" {
				dd, err := parseEpochPtr(due)
				if err != nil {
					return err
				}
				ca.DueDate = dd
			}

			result, err := c.CreateCorrectiveAction(ca)
			if err != nil {
				return err
			}
			fmt.Printf("Corrective action #%d created: %s\n", result.ID, result.Title)
			fmt.Printf("  Source:   %s\n", result.Source)
			fmt.Printf("  Severity: %s\n", result.Severity)
			fmt.Printf("  Status:   %s\n", result.Status)
			if result.Assignee != "" {
				fmt.Printf("  Assignee: %s\n", result.Assignee)
			}
			if result.DueDate != nil {
				fmt.Printf("  Due:      %s\n", *result.DueDate)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Description (required)")
	cmd.Flags().StringVar(&source, "source", "other", "Source: internal_audit, external_audit, risk_assessment, security_incident, objective, feedback, other")
	cmd.Flags().StringVar(&severity, "severity", "observation", "Severity: major_nc, minor_nc, observation, opportunity")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee email")
	cmd.Flags().StringVar(&due, "due", "", "Due date (YYYY-MM-DD)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("description")
	return cmd
}

func correctiveListCmd() *cobra.Command {
	var status, severity, assignee string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List corrective actions",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			actions, err := c.ListCorrectiveActions(status, severity, assignee)
			if err != nil {
				return err
			}
			if len(actions) == 0 {
				fmt.Println("No corrective actions found.")
				return nil
			}
			fmt.Printf("  %-6s %-12s %-16s %-20s %-30s %-24s %s\n",
				"ID", "SEVERITY", "STATUS", "SOURCE", "TITLE", "ASSIGNEE", "DUE")
			fmt.Printf("  %s\n", strings.Repeat("-", 140))
			for _, ca := range actions {
				due := epochPtrStr(ca.DueDate)
				fmt.Printf("  %-6d %-12s %-16s %-20s %-30s %-24s %s\n",
					ca.ID,
					ca.Severity,
					ca.Status,
					ca.Source,
					truncate(ca.Title, 30),
					truncate(ca.Assignee, 24),
					due,
				)
			}
			fmt.Printf("\n%d corrective action(s)\n", len(actions))
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status: todo, assessment, awaiting_approval, implementation, monitoring, resolved")
	cmd.Flags().StringVar(&severity, "severity", "", "Filter by severity: major_nc, minor_nc, observation, opportunity")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Filter by assignee email")
	return cmd
}

func correctiveShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show corrective action details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid ID: %s", args[0])
			}

			c := requireAPI()
			ca, err := c.GetCorrectiveAction(id)
			if err != nil {
				return err
			}

			fmt.Printf("Corrective Action #%d\n", ca.ID)
			fmt.Printf("  Title:       %s\n", ca.Title)
			fmt.Printf("  Description: %s\n", ca.Description)
			fmt.Printf("  Source:      %s\n", ca.Source)
			fmt.Printf("  Severity:    %s\n", ca.Severity)
			fmt.Printf("  Status:      %s\n", ca.Status)
			fmt.Printf("  Created By:  %s\n", ca.CreatedBy)
			if ca.Assignee != "" {
				fmt.Printf("  Assignee:    %s\n", ca.Assignee)
			}
			if ca.DueDate != nil {
				fmt.Printf("  Due Date:    %s\n", *ca.DueDate)
			}
			if ca.RootCause != "" {
				fmt.Printf("  Root Cause:  %s\n", ca.RootCause)
			}
			if ca.Notes != "" {
				fmt.Printf("  Notes:       %s\n", ca.Notes)
			}
			if ca.ResolvedAt != nil {
				fmt.Printf("  Resolved:    %s by %s\n", ca.ResolvedAt.Format("2006-01-02 15:04"), ca.ResolvedBy)
			}
			fmt.Printf("  Created:     %s\n", ca.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Printf("  Updated:     %s\n", ca.UpdatedAt.Format("2006-01-02 15:04"))
			return nil
		},
	}
}

func correctiveUpdateCmd() *cobra.Command {
	var rootCause, notes string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update corrective action fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid ID: %s", args[0])
			}

			c := requireAPI()
			ca := &db.CorrectiveAction{
				RootCause: rootCause,
				Notes:     notes,
			}
			if err := c.UpdateCorrectiveAction(id, ca); err != nil {
				return err
			}
			fmt.Printf("Corrective action #%d updated.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&rootCause, "root-cause", "", "Root cause analysis")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes (action plan, implementation, verification, evidence)")
	return cmd
}

func correctiveStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id> <status>",
		Short: "Change corrective action status (todo, assessment, awaiting_approval, implementation, monitoring, resolved)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid ID: %s", args[0])
			}
			status := args[1]

			valid := map[string]bool{
				"todo": true, "assessment": true, "awaiting_approval": true,
				"implementation": true, "monitoring": true, "resolved": true,
			}
			if !valid[status] {
				return fmt.Errorf("invalid status: %s (valid: todo, assessment, awaiting_approval, implementation, monitoring, resolved)", status)
			}

			c := requireAPI()
			if err := c.UpdateCorrectiveActionStatus(id, status); err != nil {
				return err
			}
			fmt.Printf("Corrective action #%d status changed to %s.\n", id, status)
			return nil
		},
	}
}

func correctiveRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a corrective action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid ID: %s", args[0])
			}

			c := requireAPI()
			if err := c.DeleteCorrectiveAction(id); err != nil {
				return err
			}
			fmt.Printf("Corrective action #%d deleted.\n", id)
			return nil
		},
	}
}
