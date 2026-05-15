package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func programCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "program",
		Short: "Manage objective programs",
	}

	cmd.AddCommand(
		programCreateCmd(),
		programListCmd(),
	)
	return cmd
}

func programCreateCmd() *cobra.Command {
	var key, title, description string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new program",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			p := &db.Program{
				Key:         strings.ToUpper(key),
				Title:       title,
				Description: description,
			}

			result, err := c.CreateProgram(p)
			if err != nil {
				return err
			}
			fmt.Printf("Program created: %s (%s)\n", result.Key, result.Identifier)
			fmt.Printf("  Title: %s\n", result.Title)
			if result.Description != "" {
				fmt.Printf("  Description: %s\n", result.Description)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Program key (uppercase, e.g. AWARE, SEC)")
	cmd.Flags().StringVar(&title, "title", "", "Program title")
	cmd.Flags().StringVar(&description, "description", "", "Program description")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func programListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List programs",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			programs, err := c.ListPrograms()
			if err != nil {
				return err
			}

			if len(programs) == 0 {
				fmt.Println("No programs found.")
				return nil
			}

			fmt.Printf("%-10s %-12s %s\n", "KEY", "IDENTIFIER", "TITLE")
			fmt.Println(repeat("-", 50))
			for _, p := range programs {
				fmt.Printf("%-10s %-12s %s\n", p.Key, p.Identifier, p.Title)
			}
			return nil
		},
	}
}

func objectiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objective",
		Short: "Manage ISMS objectives",
	}

	cmd.AddCommand(
		objectiveCreateCmd(),
		objectiveListCmd(),
		objectiveShowCmd(),
	)
	return cmd
}

func objectiveCreateCmd() *cobra.Command {
	var program, title, description, owner, source, unit, method, operator string
	var targetValue float64
	var hasTarget bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an objective",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			// Resolve program by key
			programs, err := c.ListPrograms()
			if err != nil {
				return err
			}
			var programID int64
			for _, p := range programs {
				if strings.EqualFold(p.Key, program) {
					programID = p.ID
					break
				}
			}
			if programID == 0 {
				return fmt.Errorf("program '%s' not found", program)
			}

			o := &db.Objective{
				ProgramID:         programID,
				Title:             title,
				Description:       description,
				Owner:             owner,
				Source:            source,
				MeasurementMethod: method,
				Unit:              unit,
				TargetOperator:    operator,
			}
			if hasTarget {
				o.TargetValue = &targetValue
			}

			result, err := c.CreateObjective(o)
			if err != nil {
				return err
			}
			fmt.Printf("Objective created: %s\n", result.DisplayID)
			fmt.Printf("  Title:  %s\n", result.Title)
			fmt.Printf("  Status: %s\n", result.Status)
			if result.TargetValue != nil {
				op := operatorSymbol(result.TargetOperator)
				fmt.Printf("  Target: %s %g %s\n", op, *result.TargetValue, result.Unit)
			}
			if result.Owner != "" {
				fmt.Printf("  Owner:  %s\n", result.Owner)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&program, "program", "", "Program key (e.g. AWARE)")
	cmd.Flags().StringVar(&title, "title", "", "Objective title")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&owner, "owner", "", "Owner (email or name)")
	cmd.Flags().StringVar(&source, "source", "", "Source of requirement")
	cmd.Flags().StringVar(&method, "method", "", "Measurement method")
	cmd.Flags().Float64Var(&targetValue, "target-value", 0, "Target value")
	cmd.Flags().StringVar(&operator, "target-operator", "gte", "Target operator (gte, lte, eq, gt, lt)")
	cmd.Flags().StringVar(&unit, "unit", "", "Unit of measurement (%, minutes, count, etc.)")
	_ = cmd.MarkFlagRequired("program")
	_ = cmd.MarkFlagRequired("title")

	// Track if --target-value was explicitly set
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		hasTarget = cmd.Flags().Changed("target-value")
		return nil
	}

	return cmd
}

func objectiveListCmd() *cobra.Command {
	var program, status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List objectives",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			var programID int64
			if program != "" {
				programs, err := c.ListPrograms()
				if err != nil {
					return err
				}
				for _, p := range programs {
					if strings.EqualFold(p.Key, program) {
						programID = p.ID
						break
					}
				}
				if programID == 0 {
					return fmt.Errorf("program '%s' not found", program)
				}
			}

			objectives, err := c.ListObjectives(programID, status)
			if err != nil {
				return err
			}

			if len(objectives) == 0 {
				fmt.Println("No objectives found.")
				return nil
			}

			fmt.Printf("%-12s %-10s %-40s %s\n", "ID", "STATUS", "TITLE", "TARGET")
			fmt.Println(repeat("-", 80))
			for _, o := range objectives {
				target := ""
				if o.TargetValue != nil {
					target = fmt.Sprintf("%s %g %s", operatorSymbol(o.TargetOperator), *o.TargetValue, o.Unit)
				}
				fmt.Printf("%-12s %-10s %-40s %s\n",
					o.DisplayID,
					o.Status,
					truncate(o.Title, 40),
					target,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&program, "program", "", "Filter by program key")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	return cmd
}

func objectiveShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <display-id>",
		Short: "Show objective details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			// List all and find by display_id
			objectives, err := c.ListObjectives(0, "")
			if err != nil {
				return err
			}

			var found *db.Objective
			for _, o := range objectives {
				if strings.EqualFold(o.DisplayID, args[0]) {
					match := o
					found = &match
					break
				}
			}
			if found == nil {
				return fmt.Errorf("objective '%s' not found", args[0])
			}

			fmt.Printf("Objective: %s\n", found.DisplayID)
			fmt.Printf("  Title:       %s\n", found.Title)
			fmt.Printf("  Status:      %s\n", found.Status)
			if found.Description != "" {
				fmt.Printf("  Description: %s\n", found.Description)
			}
			if found.Owner != "" {
				fmt.Printf("  Owner:       %s\n", found.Owner)
			}
			if found.Source != "" {
				fmt.Printf("  Source:      %s\n", found.Source)
			}
			if found.MeasurementMethod != "" {
				fmt.Printf("  Method:      %s\n", found.MeasurementMethod)
			}
			if found.TargetValue != nil {
				fmt.Printf("  Target:      %s %g %s\n", operatorSymbol(found.TargetOperator), *found.TargetValue, found.Unit)
			}
			if found.StartedAt != nil {
				fmt.Printf("  Started:     %s\n", found.StartedAt.Format("2006-01-02"))
			}
			if found.ArchivedAt != nil {
				fmt.Printf("  Archived:    %s\n", found.ArchivedAt.Format("2006-01-02"))
			}

			// Show recent checkins
			checkins, err := c.ListCheckins(found.ID, 10)
			if err == nil && len(checkins) > 0 {
				fmt.Println("\n  Recent checkins:")
				for _, ci := range checkins {
					status := " "
					if ci.Success != nil {
						if *ci.Success {
							status = "PASS"
						} else {
							status = "FAIL"
						}
					}
					val := ""
					if ci.ValueNumeric != nil {
						val = fmt.Sprintf(" value=%g", *ci.ValueNumeric)
					}
					msg := ""
					if ci.Message != "" {
						msg = " — " + ci.Message
					}
					fmt.Printf("    [%s] %s %-4s%s%s\n",
						ci.OccurredAt.Format("2006-01-02 15:04"),
						ci.CreatedBy,
						status,
						val,
						msg,
					)
				}
			}

			return nil
		},
	}
}

func checkinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkin",
		Short: "Record a measurement checkin",
	}

	cmd.AddCommand(
		checkinAddCmd(),
		checkinListCmd(),
	)
	return cmd
}

func checkinAddCmd() *cobra.Command {
	var value float64
	var success, fail bool
	var message, publicNote string
	var hasValue bool

	cmd := &cobra.Command{
		Use:   "add <display-id>",
		Short: "Record a checkin for an objective",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			// Resolve display_id to objective ID
			objectives, err := c.ListObjectives(0, "")
			if err != nil {
				return err
			}
			var objID int64
			for _, o := range objectives {
				if strings.EqualFold(o.DisplayID, args[0]) {
					objID = o.ID
					break
				}
			}
			if objID == 0 {
				return fmt.Errorf("objective '%s' not found", args[0])
			}

			ci := &db.Checkin{
				Message:    message,
				PublicNote: publicNote,
			}
			if hasValue {
				ci.ValueNumeric = &value
			}
			if success {
				b := true
				ci.Success = &b
			} else if fail {
				b := false
				ci.Success = &b
			}

			result, err := c.CreateCheckin(objID, ci)
			if err != nil {
				return err
			}

			fmt.Printf("Checkin #%d recorded for %s\n", result.ID, args[0])
			if result.ValueNumeric != nil {
				fmt.Printf("  Value: %g\n", *result.ValueNumeric)
			}
			if result.Success != nil {
				if *result.Success {
					fmt.Println("  Result: PASS")
				} else {
					fmt.Println("  Result: FAIL")
				}
			}
			if result.Message != "" {
				fmt.Printf("  Note: %s\n", result.Message)
			}
			return nil
		},
	}

	cmd.Flags().Float64Var(&value, "value", 0, "Measured value")
	cmd.Flags().BoolVar(&success, "success", false, "Mark as pass")
	cmd.Flags().BoolVar(&fail, "fail", false, "Mark as fail")
	cmd.Flags().StringVar(&message, "message", "", "Internal note")
	cmd.Flags().StringVarP(&publicNote, "public-note", "p", "", "Stakeholder-facing note")

	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		hasValue = cmd.Flags().Changed("value")
		return nil
	}

	return cmd
}

func checkinListCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list <display-id>",
		Short: "List checkins for an objective",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			// Resolve display_id
			objectives, err := c.ListObjectives(0, "")
			if err != nil {
				return err
			}
			var objID int64
			for _, o := range objectives {
				if strings.EqualFold(o.DisplayID, args[0]) {
					objID = o.ID
					break
				}
			}
			if objID == 0 {
				return fmt.Errorf("objective '%s' not found", args[0])
			}

			checkins, err := c.ListCheckins(objID, limit)
			if err != nil {
				return err
			}

			if len(checkins) == 0 {
				fmt.Println("No checkins found.")
				return nil
			}

			fmt.Printf("%-6s %-20s %-6s %-10s %s\n", "ID", "DATE", "RESULT", "VALUE", "NOTE")
			fmt.Println(repeat("-", 70))
			for _, ci := range checkins {
				result := "-"
				if ci.Success != nil {
					if *ci.Success {
						result = "PASS"
					} else {
						result = "FAIL"
					}
				}
				val := "-"
				if ci.ValueNumeric != nil {
					val = fmt.Sprintf("%g", *ci.ValueNumeric)
				}
				fmt.Printf("%-6s %-20s %-6s %-10s %s\n",
					strconv.FormatInt(ci.ID, 10),
					ci.OccurredAt.Format("2006-01-02 15:04"),
					result,
					val,
					truncate(ci.Message, 30),
				)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Max checkins to show")
	return cmd
}

func operatorSymbol(op string) string {
	switch op {
	case "gte":
		return ">="
	case "lte":
		return "<="
	case "eq":
		return "="
	case "gt":
		return ">"
	case "lt":
		return "<"
	default:
		return op
	}
}
