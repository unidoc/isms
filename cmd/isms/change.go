package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/client"
	"isms.sh/internal/isms/db"
)

func changeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change",
		Short: "Manage change requests",
	}
	cmd.AddCommand(changeListCmd(), changeShowCmd(), changeCreateCmd(), changeUpdateCmd(), changeStatusCmd())
	return cmd
}

// resolveChangeID accepts either a numeric database id or a per-org identifier
// (e.g. CR-1) and returns the numeric id. Identifiers are resolved via the list,
// since identifier and id genuinely diverge (identifier is a per-org sequence).
func resolveChangeID(c *client.Client, arg string) (int, error) {
	if id, err := strconv.Atoi(arg); err == nil {
		return id, nil
	}
	changes, _, err := c.ListChanges("")
	if err != nil {
		return 0, err
	}
	for _, ch := range changes {
		if strings.EqualFold(ch.Identifier, arg) {
			return ch.ID, nil
		}
	}
	return 0, fmt.Errorf("no change request with id or identifier %q", arg)
}

func changeListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List change requests",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			changes, total, err := c.ListChanges(status)
			if err != nil {
				return err
			}
			fmt.Printf("%-5s %-8s %-32s %-9s %-8s %-12s %-14s\n",
				"ID", "REF", "TITLE", "PRIORITY", "RISK", "STATUS", "ASSIGNED")
			fmt.Println(repeat("-", 96))
			for _, ch := range changes {
				fmt.Printf("%-5d %-8s %-32s %-9s %-8s %-12s %-14s\n",
					ch.ID, ch.Identifier, truncate(ch.Title, 32),
					truncate(ch.Priority, 9), truncate(ch.RiskLevel, 8),
					truncate(ch.Status, 12), truncate(ch.AssignedTo, 14))
			}
			if total > len(changes) {
				fmt.Printf("\n%d of %d change requests (raise the server page size to see all)\n", len(changes), total)
			} else {
				fmt.Printf("\n%d change requests\n", len(changes))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status ("+strings.Join(db.ChangeStatuses, ", ")+")")
	return cmd
}

func changeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id|ref>",
		Short: "Show a change request in detail (numeric id or CR- reference)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			id, err := resolveChangeID(c, args[0])
			if err != nil {
				return err
			}
			ch, err := c.GetChange(id)
			if err != nil {
				return err
			}
			fmt.Printf("%s  %s\n", ch.Identifier, ch.Title)
			fmt.Println(repeat("-", 60))
			fmt.Printf("Type:          %s\n", ch.Type)
			fmt.Printf("Status:        %s\n", ch.Status)
			fmt.Printf("Priority:      %s\n", ch.Priority)
			fmt.Printf("Risk level:    %s\n", ch.RiskLevel)
			fmt.Printf("Category:      %s\n", ch.Category)
			fmt.Printf("Requested by:  %s\n", ch.RequestedBy)
			if ch.AssignedTo != "" {
				fmt.Printf("Assigned to:   %s\n", ch.AssignedTo)
			}
			if ch.ApprovedBy != "" {
				fmt.Printf("Approved by:   %s\n", ch.ApprovedBy)
			}
			if ch.Description != "" {
				fmt.Printf("\nDescription:\n%s\n", ch.Description)
			}
			if ch.Justification != "" {
				fmt.Printf("\nJustification:\n%s\n", ch.Justification)
			}
			if ch.RollbackPlan != "" {
				fmt.Printf("\nRollback plan:\n%s\n", ch.RollbackPlan)
			}
			if ch.Notes != "" {
				fmt.Printf("\nNotes:\n%s\n", ch.Notes)
			}
			return nil
		},
	}
}

func changeCreateCmd() *cobra.Command {
	var cr db.ChangeRequest
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a change request (starts as 'proposed'; transition with 'change status')",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cr.Title == "" {
				return fmt.Errorf("--title is required")
			}
			c := requireAPI()
			out, err := c.CreateChange(&cr)
			if err != nil {
				return err
			}
			fmt.Printf("Created %s (%s) — status %s\n", out.Identifier, out.Title, out.Status)
			return nil
		},
	}
	// No --status: a change is born "proposed" and transitioned via
	// `change status`, so approved_by / the follow-up task / the changelog
	// transition are never skipped.
	cmd.Flags().StringVar(&cr.Type, "type", "", "Type ("+strings.Join(db.ChangeTypes, ", ")+"; default change)")
	cmd.Flags().StringVar(&cr.Title, "title", "", "Change title (required)")
	cmd.Flags().StringVar(&cr.Description, "desc", "", "What is changing and why")
	cmd.Flags().StringVar(&cr.Justification, "justification", "", "Business justification")
	cmd.Flags().StringVar(&cr.Priority, "priority", "medium", "Priority ("+strings.Join(db.ChangePriorities, ", ")+")")
	cmd.Flags().StringVar(&cr.Category, "category", "", "Category ("+strings.Join(db.ChangeCategories, ", ")+")")
	cmd.Flags().StringVar(&cr.RiskLevel, "risk", "", "Risk level ("+strings.Join(db.ChangeRiskLevels, ", ")+")")
	cmd.Flags().StringVar(&cr.RollbackPlan, "rollback", "", "Rollback plan")
	cmd.Flags().StringVar(&cr.Notes, "notes", "", "Notes")
	cmd.Flags().StringVar(&cr.AssignedTo, "assigned-to", "", "Assignee email")
	return cmd
}

func changeUpdateCmd() *cobra.Command {
	var title, desc, justification, priority, category, risk, rollback, notes, assignedTo string
	cmd := &cobra.Command{
		Use:   "update <id|ref>",
		Short: "Update change request fields (only the flags you pass are changed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			id, err := resolveChangeID(c, args[0])
			if err != nil {
				return err
			}
			// Send only changed fields — the server contract is nil = leave alone.
			fields := map[string]interface{}{}
			f := cmd.Flags()
			for flag, val := range map[string]*string{
				"title": &title, "desc": &desc, "justification": &justification,
				"priority": &priority, "category": &category, "risk": &risk,
				"rollback": &rollback, "notes": &notes, "assigned-to": &assignedTo,
			} {
				if f.Changed(flag) {
					fields[changeFieldJSON[flag]] = *val
				}
			}
			if len(fields) == 0 {
				return fmt.Errorf("no fields to update — pass at least one flag")
			}
			out, err := c.UpdateChange(id, fields)
			if err != nil {
				return err
			}
			fmt.Printf("Updated %s\n", out.Identifier)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Change title")
	cmd.Flags().StringVar(&desc, "desc", "", "Description")
	cmd.Flags().StringVar(&justification, "justification", "", "Business justification")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority ("+strings.Join(db.ChangePriorities, ", ")+")")
	cmd.Flags().StringVar(&category, "category", "", "Category ("+strings.Join(db.ChangeCategories, ", ")+")")
	cmd.Flags().StringVar(&risk, "risk", "", "Risk level ("+strings.Join(db.ChangeRiskLevels, ", ")+")")
	cmd.Flags().StringVar(&rollback, "rollback", "", "Rollback plan")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes")
	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "Assignee email")
	return cmd
}

// changeFieldJSON maps update flag names to the API's JSON field names.
var changeFieldJSON = map[string]string{
	"title": "title", "desc": "description", "justification": "justification",
	"priority": "priority", "category": "category", "risk": "risk_level",
	"rollback": "rollback_plan", "notes": "notes", "assigned-to": "assigned_to",
}

func changeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id|ref> <new-status>",
		Short: "Transition a change request's status (" + strings.Join(db.ChangeStatuses, ", ") + ")",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			id, err := resolveChangeID(c, args[0])
			if err != nil {
				return err
			}
			status, err := c.UpdateChangeStatus(id, args[1])
			if err != nil {
				return err
			}
			fmt.Printf("Change #%d → %s\n", id, status)
			return nil
		},
	}
}
