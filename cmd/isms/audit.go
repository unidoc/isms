package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"isms.sh/internal/isms/db"
	"github.com/spf13/cobra"
)

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Internal audit management",
	}

	cmd.AddCommand(
		auditProgrammeCmd(),
		auditCreateCmd(),
		auditListCmd(),
		auditShowCmd(),
		auditStartCmd(),
		auditAssessCmd(),
		auditFindingCmd(),
		auditCompleteCmd(),
	)
	return cmd
}

// --- Programme subcommands ---

func auditProgrammeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "programme",
		Short: "Manage audit programmes",
	}
	cmd.AddCommand(auditProgrammeCreateCmd(), auditProgrammeListCmd())
	return cmd
}

func auditProgrammeCreateCmd() *cobra.Command {
	var title string
	var year int
	var description string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an audit programme",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := &db.AuditProgramme{
				Title:       title,
				Year:        year,
				Description: description,
				Status:      "active",
				CreatedBy:   "", // server sets from token
			}

			c := requireAPI()
			result, err := c.CreateAuditProgramme(p)
			if err != nil {
				return err
			}
			fmt.Printf("Audit programme #%d created: %s (%d)\n", result.ID, result.Title, result.Year)
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Programme title")
	cmd.Flags().IntVar(&year, "year", time.Now().Year(), "Programme year")
	cmd.Flags().StringVar(&description, "description", "", "Programme description")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func auditProgrammeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List audit programmes",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			programmes, err := c.ListAuditProgrammes()
			if err != nil {
				return err
			}
			if len(programmes) == 0 {
				fmt.Println("No audit programmes found.")
				return nil
			}
			fmt.Printf("  %-6s %-6s %-10s %-40s %-20s %s\n",
				"ID", "YEAR", "STATUS", "TITLE", "CREATED BY", "DATE")
			fmt.Printf("  %s\n", strings.Repeat("-", 100))
			for _, p := range programmes {
				fmt.Printf("  %-6d %-6d %-10s %-40s %-20s %s\n",
					p.ID,
					p.Year,
					p.Status,
					truncate(p.Title, 40),
					truncate(p.CreatedBy, 20),
					p.CreatedAt.Format("2006-01-02"),
				)
			}
			fmt.Printf("\n%d programme(s)\n", len(programmes))
			return nil
		},
	}
}

// --- Audit CRUD ---

func auditCreateCmd() *cobra.Command {
	var programmeID int
	var title, scope, auditor, dateStr, endDateStr, auditType string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new audit",
		RunE: func(cmd *cobra.Command, args []string) error {
			if auditType == "" {
				auditType = "internal"
			}
			a := &db.Audit{
				Title:     title,
				Scope:     scope,
				Auditor:   auditor,
				AuditType: auditType,
				Status:    "planned",
			}
			if programmeID > 0 {
				a.ProgrammeID = &programmeID
			}
			if dateStr != "" {
				pd, err := parseEpochPtr(dateStr)
				if err != nil {
					return err
				}
				a.PlannedDate = pd
			}
			if endDateStr != "" {
				ed, err := parseEpochPtr(endDateStr)
				if err != nil {
					return err
				}
				a.EndDate = ed
			}

			c := requireAPI()
			result, err := c.CreateAudit(a)
			if err != nil {
				return err
			}
			fmt.Printf("Audit #%d created: %s\n", result.ID, result.Title)
			fmt.Printf("  Type:    %s\n", result.AuditType)
			fmt.Printf("  Scope:   %s\n", result.Scope)
			fmt.Printf("  Auditor: %s\n", result.Auditor)
			if result.PlannedDate != nil {
				fmt.Printf("  Date:    %s\n", result.PlannedDate.Format("2006-01-02"))
			}
			if result.EndDate != nil {
				fmt.Printf("  End:     %s\n", *result.EndDate)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&programmeID, "programme", 0, "Programme ID")
	cmd.Flags().StringVar(&title, "title", "", "Audit title")
	cmd.Flags().StringVar(&scope, "scope", "", "Audit scope (e.g. ISO27001-4.*,ISO27001-5.*,ISO27001-A.5.*)")
	cmd.Flags().StringVar(&auditor, "auditor", "", "Auditor email")
	cmd.Flags().StringVar(&auditType, "type", "internal", "Audit type: internal, external, surveillance, certification, recertification")
	cmd.Flags().StringVar(&dateStr, "date", "", "Planned date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&endDateStr, "end-date", "", "End date (YYYY-MM-DD)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("auditor")
	return cmd
}

func auditListCmd() *cobra.Command {
	var programmeID int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audits",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			audits, err := c.ListAudits(programmeID)
			if err != nil {
				return err
			}
			if len(audits) == 0 {
				fmt.Println("No audits found.")
				return nil
			}
			fmt.Printf("  %-6s %-14s %-36s %-24s %-6s %-6s %s\n",
				"ID", "STATUS", "TITLE", "AUDITOR", "ITEMS", "FINDS", "DATE")
			fmt.Printf("  %s\n", strings.Repeat("-", 110))
			for _, a := range audits {
				date := ""
				if a.PlannedDate != nil {
					date = a.PlannedDate.Format("2006-01-02")
				}
				fmt.Printf("  %-6d %-14s %-36s %-24s %-6d %-6d %s\n",
					a.ID,
					a.Status,
					truncate(a.Title, 36),
					truncate(a.Auditor, 24),
					a.ItemCount,
					a.FindingCount,
					date,
				)
			}
			fmt.Printf("\n%d audit(s)\n", len(audits))
			return nil
		},
	}

	cmd.Flags().IntVar(&programmeID, "programme", 0, "Filter by programme ID")
	return cmd
}

func auditShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <audit-id>",
		Short: "Show audit details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid audit ID: %s", args[0])
			}

			c := requireAPI()
			audit, err := c.GetAudit(id)
			if err != nil {
				return err
			}
			fmt.Printf("Audit #%d\n", audit.ID)
			fmt.Printf("  Title:       %s\n", audit.Title)
			fmt.Printf("  Type:        %s\n", audit.AuditType)
			fmt.Printf("  Scope:       %s\n", audit.Scope)
			fmt.Printf("  Auditor:     %s\n", audit.Auditor)
			fmt.Printf("  Status:      %s\n", audit.Status)
			if audit.PlannedDate != nil {
				fmt.Printf("  Planned:     %s\n", audit.PlannedDate.Format("2006-01-02"))
			}
			if audit.StartedAt != nil {
				fmt.Printf("  Started:     %s\n", audit.StartedAt.Format("2006-01-02 15:04"))
			}
			if audit.CompletedAt != nil {
				fmt.Printf("  Completed:   %s\n", audit.CompletedAt.Format("2006-01-02 15:04"))
			}
			if audit.Summary != "" {
				fmt.Printf("  Summary:     %s\n", audit.Summary)
			}
			fmt.Printf("  Items:       %d\n", audit.ItemCount)
			fmt.Printf("  Findings:    %d\n", audit.FindingCount)

			items, err := c.ListAuditItems(id)
			if err != nil {
				return fmt.Errorf("loading items: %w", err)
			}
			if len(items) > 0 {
				fmt.Printf("\nAudit Items:\n")
				fmt.Printf("  %-6s %-12s %-40s %s\n", "ID", "ITEM", "TITLE", "RESULT")
				fmt.Printf("  %s\n", strings.Repeat("-", 80))
				for _, item := range items {
					fmt.Printf("  %-6d %-12s %-40s %s\n",
						item.ID, item.ItemID, truncate(item.Title, 40), item.Result)
				}
			}

			findings, err := c.ListAuditFindings(id)
			if err != nil {
				return fmt.Errorf("loading findings: %w", err)
			}
			if len(findings) > 0 {
				fmt.Printf("\nFindings:\n")
				for i, f := range findings {
					fmt.Printf("  %d. [%s] %s (status: %s)\n",
						i+1, f.FindingType, f.Title, f.Status)
				}
			}
			return nil
		},
	}
}

// --- Audit workflow ---

func auditStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <audit-id>",
		Short: "Start an audit (sets status to in_progress)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid audit ID: %s", args[0])
			}

			c := requireAPI()
			if err := c.UpdateAuditStatus(id, "in_progress"); err != nil {
				return err
			}
			fmt.Printf("Audit #%d started.\n", id)
			return nil
		},
	}
}

func auditAssessCmd() *cobra.Command {
	var result, evidence, notes string

	cmd := &cobra.Command{
		Use:   "assess <item-id>",
		Short: "Record an assessment result for an audit item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid item ID: %s", args[0])
			}

			c := requireAPI()
			if err := c.AssessAuditItem(id, result, evidence, notes); err != nil {
				return err
			}
			fmt.Printf("Item #%d assessed: %s\n", id, result)
			return nil
		},
	}

	cmd.Flags().StringVar(&result, "result", "", "Result: conforming, minor_nc, major_nc, observation, opportunity")
	cmd.Flags().StringVar(&evidence, "evidence", "", "Evidence reviewed")
	cmd.Flags().StringVar(&notes, "notes", "", "Auditor notes")
	_ = cmd.MarkFlagRequired("result")
	return cmd
}

func auditFindingCmd() *cobra.Command {
	var findingType, title, description, dueDateStr string

	cmd := &cobra.Command{
		Use:   "finding <audit-id>",
		Short: "Record an audit finding (corrective action goes in description ## Corrective Action)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			auditID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid audit ID: %s", args[0])
			}

			c := requireAPI()
			finding := &db.AuditFinding{
				AuditID:     auditID,
				FindingType: findingType,
				Title:       title,
				Description: description,
				Status:      "open",
			}
			if dueDateStr != "" {
				dd, err := parseEpochPtr(dueDateStr)
				if err != nil {
					return err
				}
				finding.DueDate = dd
			}
			result, err := c.AddAuditFinding(finding)
			if err != nil {
				return err
			}
			fmt.Printf("Finding #%d created: %s\n", result.ID, result.Title)
			fmt.Printf("  Type:   %s\n", findingType)
			if result.DueDate != nil {
				fmt.Printf("  Due:    %s\n", result.DueDate.Format("2006-01-02"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&findingType, "type", "", "Finding type: minor_nc, major_nc, observation, opportunity")
	cmd.Flags().StringVar(&title, "title", "", "Finding title")
	cmd.Flags().StringVar(&description, "description", "", "Finding description (use ## Corrective Action heading)")
	cmd.Flags().StringVar(&dueDateStr, "due", "", "Due date (YYYY-MM-DD)")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("description")
	return cmd
}

func auditCompleteCmd() *cobra.Command {
	var summary string

	cmd := &cobra.Command{
		Use:   "complete <audit-id>",
		Short: "Complete an audit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid audit ID: %s", args[0])
			}

			c := requireAPI()
			// summary was silently dropped before #49; send it atomically with status.
			if _, err := c.UpdateAudit(id, map[string]interface{}{
				"status":  "completed",
				"summary": summary,
			}); err != nil {
				return err
			}
			fmt.Printf("Audit #%d completed.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&summary, "summary", "", "Auditor summary")
	_ = cmd.MarkFlagRequired("summary")
	return cmd
}

// formatResult converts a result code to a display-friendly string.
func formatResult(result string) string {
	switch result {
	case "conforming":
		return "Conforming"
	case "minor_nc":
		return "Minor NC"
	case "major_nc":
		return "Major NC"
	case "observation":
		return "Observation"
	case "opportunity":
		return "Opportunity"
	case "not_assessed":
		return "Not assessed"
	default:
		return result
	}
}

// formatFindingType converts a finding type to a display-friendly string.
func formatFindingType(ft string) string {
	switch ft {
	case "minor_nc":
		return "Minor nonconformity"
	case "major_nc":
		return "Major nonconformity"
	case "observation":
		return "Observation"
	case "opportunity":
		return "Opportunity for improvement"
	default:
		return ft
	}
}
