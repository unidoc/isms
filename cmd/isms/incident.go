package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func incidentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "incident",
		Short: "Incident management",
	}

	cmd.AddCommand(
		incidentAddCmd(),
		incidentListCmd(),
		incidentShowCmd(),
		incidentUpdateCmd(),
		incidentResolveCmd(),
		incidentCloseCmd(),
	)
	return cmd
}

func incidentAddCmd() *cobra.Command {
	var title, description, severity, assignee string
	var incidentType, source string
	var dataBreach bool
	var affectsC, affectsI, affectsA bool
	var notes, gdprRole string
	var risks []string
	var assets, systems []string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Report a new incident",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			inc := &db.Incident{
				Title:        title,
				Description:  description,
				Severity:     severity,
				AffectsC:     affectsC,
				AffectsI:     affectsI,
				AffectsA:     affectsA,
				IncidentType: incidentType,
				Source:       source,
				Notes:        notes,
				DataBreach:   dataBreach,
				GDPRRole:     gdprRole,
				Assignee:     assignee,
			}

			result, err := c.CreateIncident(inc, buildRefs(
				refSpec{"risk", risks},
				refSpec{"asset", assets},
				refSpec{"system", systems},
			))
			if err != nil {
				return err
			}
			fmt.Printf("Incident #%d created: %s\n", result.ID, result.Title)
			fmt.Printf("  Severity: %s\n", result.Severity)
			fmt.Printf("  Type:     %s\n", result.IncidentType)
			fmt.Printf("  Source:   %s\n", result.Source)
			fmt.Printf("  Status:   %s\n", result.Status)
			if result.DataBreach {
				fmt.Printf("  Data Breach: yes\n")
			}
			if result.Assignee != "" {
				fmt.Printf("  Assignee: %s\n", result.Assignee)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Incident title")
	cmd.Flags().StringVar(&description, "description", "", "Incident description")
	cmd.Flags().StringVar(&severity, "severity", "medium", "Severity: critical, high, medium, low")
	cmd.Flags().BoolVar(&affectsC, "affects-c", false, "Affects confidentiality")
	cmd.Flags().BoolVar(&affectsI, "affects-i", false, "Affects integrity")
	cmd.Flags().BoolVar(&affectsA, "affects-a", false, "Affects availability")
	cmd.Flags().StringVar(&incidentType, "type", "event", "Type: incident, event, weakness")
	cmd.Flags().StringVar(&source, "source", "internal", "Source: internal, external, 'internal and external'")
	cmd.Flags().BoolVar(&dataBreach, "data-breach", false, "Flag as data breach")
	cmd.Flags().StringVar(&notes, "notes", "", "Additional notes")
	cmd.Flags().StringVar(&gdprRole, "gdpr-role", "", "GDPR role: controller, processor")
	cmd.Flags().StringSliceVar(&risks, "risks", nil, "Linked risk IDs (comma-separated, e.g. RISK-001,RISK-003)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee email")
	cmd.Flags().StringSliceVar(&assets, "assets", nil, "Affected asset IDs (comma-separated)")
	cmd.Flags().StringSliceVar(&systems, "systems", nil, "Affected system IDs (comma-separated)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("description")
	return cmd
}

func incidentListCmd() *cobra.Command {
	var status, severity string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List incidents",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			incidents, err := c.ListIncidents(status, severity)
			if err != nil {
				return err
			}
			if len(incidents) == 0 {
				fmt.Println("No incidents found.")
				return nil
			}
			fmt.Printf("  %-6s %-10s %-10s %-10s %-14s %-40s %-24s %s\n",
				"ID", "SEVERITY", "TYPE", "SOURCE", "STATUS", "TITLE", "REPORTER", "DATE")
			fmt.Printf("  %s\n", strings.Repeat("-", 140))
			for _, inc := range incidents {
				fmt.Printf("  %-6d %-10s %-10s %-10s %-14s %-40s %-24s %s\n",
					inc.ID,
					inc.Severity,
					inc.IncidentType,
					inc.Source,
					inc.Status,
					truncate(inc.Title, 40),
					truncate(inc.Reporter, 24),
					inc.CreatedAt.Format("2006-01-02 15:04"),
				)
			}
			fmt.Printf("\n%d incident(s)\n", len(incidents))
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status: open, investigating, contained, resolved, closed")
	cmd.Flags().StringVar(&severity, "severity", "", "Filter by severity: critical, high, medium, low")
	return cmd
}

func incidentShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show incident details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid incident ID: %s", args[0])
			}

			c := requireAPI()
			inc, err := c.GetIncident(id)
			if err != nil {
				return err
			}

			fmt.Printf("Incident #%d\n", inc.ID)
			fmt.Printf("  Title:       %s\n", inc.Title)
			fmt.Printf("  Description: %s\n", inc.Description)
			fmt.Printf("  Severity:    %s\n", inc.Severity)
			fmt.Printf("  Type:        %s\n", inc.IncidentType)
			fmt.Printf("  Source:       %s\n", inc.Source)
			fmt.Printf("  Status:      %s\n", inc.Status)
			cia := ""
			if inc.AffectsC {
				cia += "C"
			}
			if inc.AffectsI {
				cia += "I"
			}
			if inc.AffectsA {
				cia += "A"
			}
			if cia != "" {
				fmt.Printf("  Affects:     %s\n", cia)
			}
			if inc.DataBreach {
				fmt.Printf("  Data Breach: yes\n")
			}
			if inc.GDPRRole != "" {
				fmt.Printf("  GDPR Role:   %s\n", inc.GDPRRole)
			}
			if inc.AuthorityNotified != "" && inc.AuthorityNotified != "not_required" {
				fmt.Printf("  Authority:   %s\n", inc.AuthorityNotified)
			}
			if inc.SubjectsNotified != "" && inc.SubjectsNotified != "not_required" {
				fmt.Printf("  Subjects:    %s\n", inc.SubjectsNotified)
			}
			if inc.Notes != "" {
				fmt.Printf("  Notes:       %s\n", inc.Notes)
			}
			fmt.Printf("  Reporter:    %s\n", inc.Reporter)
			if inc.Assignee != "" {
				fmt.Printf("  Assignee:    %s\n", inc.Assignee)
			}
			fmt.Printf("  Detected:    %s\n", inc.DetectedAt.Format("2006-01-02 15:04"))
			if inc.ContainedAt != nil {
				fmt.Printf("  Contained:   %s\n", inc.ContainedAt.Format("2006-01-02 15:04"))
			}
			if inc.ResolvedAt != nil {
				fmt.Printf("  Resolved:    %s\n", inc.ResolvedAt.Format("2006-01-02 15:04"))
			}
			if inc.ClosedAt != nil {
				fmt.Printf("  Closed:      %s\n", inc.ClosedAt.Format("2006-01-02 15:04"))
			}
			if inc.RootCause != "" {
				fmt.Printf("  Root Cause:  %s\n", inc.RootCause)
			}
			if inc.LessonsLearned != "" {
				fmt.Printf("  Lessons:     %s\n", inc.LessonsLearned)
			}
			return nil
		},
	}
}

func incidentUpdateCmd() *cobra.Command {
	var assignee, rootCause, lessons, severity string
	var incidentType, source string
	var dataBreach bool
	var affectsC, affectsI, affectsA bool
	var notes, gdprRole, authorityNotified, subjectsNotified string
	var risks []string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an incident",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid incident ID: %s", args[0])
			}

			c := requireAPI()
			inc := &db.Incident{
				Assignee:          assignee,
				RootCause:         rootCause,
				LessonsLearned:    lessons,
				Severity:          severity,
				AffectsC:          affectsC,
				AffectsI:          affectsI,
				AffectsA:          affectsA,
				IncidentType:      incidentType,
				Source:            source,
				Notes:             notes,
				DataBreach:        dataBreach,
				GDPRRole:          gdprRole,
				AuthorityNotified: authorityNotified,
				SubjectsNotified:  subjectsNotified,
			}
			if err := c.UpdateIncident(id, inc); err != nil {
				return err
			}
			fmt.Printf("Incident #%d updated.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee email")
	cmd.Flags().StringVar(&rootCause, "root-cause", "", "Root cause analysis")
	cmd.Flags().StringVar(&lessons, "lessons", "", "Lessons learned")
	cmd.Flags().StringVar(&severity, "severity", "", "Severity: critical, high, medium, low")
	cmd.Flags().BoolVar(&affectsC, "affects-c", false, "Affects confidentiality")
	cmd.Flags().BoolVar(&affectsI, "affects-i", false, "Affects integrity")
	cmd.Flags().BoolVar(&affectsA, "affects-a", false, "Affects availability")
	cmd.Flags().StringVar(&incidentType, "type", "", "Type: incident, event, weakness")
	cmd.Flags().StringVar(&source, "source", "", "Source: internal, external, 'internal and external'")
	cmd.Flags().BoolVar(&dataBreach, "data-breach", false, "Flag as data breach")
	cmd.Flags().StringVar(&notes, "notes", "", "Additional notes")
	cmd.Flags().StringVar(&gdprRole, "gdpr-role", "", "GDPR role: controller, processor")
	cmd.Flags().StringVar(&authorityNotified, "authority-notified", "", "Authority notification status: not_required, pending, notified")
	cmd.Flags().StringVar(&subjectsNotified, "subjects-notified", "", "Subjects notification status: not_required, pending, notified")
	cmd.Flags().StringSliceVar(&risks, "risks", nil, "Linked risk IDs (comma-separated)")
	return cmd
}

func incidentResolveCmd() *cobra.Command {
	var rootCause string

	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Resolve an incident",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid incident ID: %s", args[0])
			}

			c := requireAPI()
			if err := c.UpdateIncidentStatus(id, "resolved"); err != nil {
				return err
			}

			// Update root cause if provided
			if rootCause != "" {
				inc := &db.Incident{RootCause: rootCause}
				c.UpdateIncident(id, inc)
			}

			fmt.Printf("Incident #%d resolved.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&rootCause, "root-cause", "", "Root cause analysis")
	return cmd
}

func incidentCloseCmd() *cobra.Command {
	var lessons string

	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Close an incident",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid incident ID: %s", args[0])
			}

			c := requireAPI()
			if err := c.UpdateIncidentStatus(id, "closed"); err != nil {
				return err
			}

			// Update lessons learned if provided
			if lessons != "" {
				inc := &db.Incident{LessonsLearned: lessons}
				c.UpdateIncident(id, inc)
			}

			fmt.Printf("Incident #%d closed.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&lessons, "lessons", "", "Lessons learned")
	return cmd
}
