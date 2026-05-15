package main

import (
	"fmt"

	"isms.sh/internal/isms/db"
	"github.com/spf13/cobra"
)

func systemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "system",
		Aliases: []string{"systems"},
		Short:   "Manage IT systems register",
	}

	cmd.AddCommand(systemAddCmd(), systemListCmd(), systemEditCmd(), systemRmCmd(), systemReviewCmd())
	return cmd
}

func systemAddCmd() *cobra.Command {
	var (
		name            string
		description     string
		supplierID      int64
		department      string
		classification  string
		criticality     string
		rpoHours        int
		rtoHours        int
		confidentiality int
		integrity       int
		availability    int
		owner           string
		notes           string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a system to the register",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("required: --name")
			}

			c := requireAPI()
			sys := &db.System{
				Name:            name,
				Description:     description,
				Department:      department,
				Classification:  classification,
				Criticality:     criticality,
				RPOHours:        rpoHours,
				RTOHours:        rtoHours,
				Confidentiality: &confidentiality,
				Integrity:       &integrity,
				Availability:    &availability,
				Owner:           owner,
				Notes:           notes,
			}
			if cmd.Flags().Changed("supplier-id") {
				sys.SupplierID = &supplierID
			}
			result, err := c.CreateSystem(sys)
			if err != nil {
				return err
			}
			fmt.Printf("Added system %s: %s\n", result.Identifier, result.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "System/service name")
	cmd.Flags().StringVar(&description, "desc", "", "Description (use ## Purpose heading)")
	cmd.Flags().Int64Var(&supplierID, "supplier-id", 0, "Supplier ID (FK)")
	cmd.Flags().StringVar(&department, "department", "", "Department (IT, Development, Marketing, etc.)")
	cmd.Flags().StringVar(&classification, "classification", "confidential", "Classification: public, internal, confidential, restricted")
	cmd.Flags().StringVar(&criticality, "criticality", "low", "Criticality: low, medium, high, critical")
	cmd.Flags().IntVar(&rpoHours, "rpo", 0, "Recovery Point Objective in hours")
	cmd.Flags().IntVar(&rtoHours, "rto", 0, "Recovery Time Objective in hours")
	cmd.Flags().IntVar(&confidentiality, "cia-c", 0, "Confidentiality impact (0-5)")
	cmd.Flags().IntVar(&integrity, "cia-i", 0, "Integrity impact (0-5)")
	cmd.Flags().IntVar(&availability, "cia-a", 0, "Availability impact (0-5)")
	cmd.Flags().StringVar(&owner, "owner", "", "System owner")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes (use ## Access control heading)")
	return cmd
}

func systemListCmd() *cobra.Command {
	var filterCriticality string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List systems",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			systems, err := c.ListSystems()
			if err != nil {
				return err
			}
			fmt.Printf("%-10s %-24s %-10s %-14s %-8s %-8s %-12s %s\n",
				"ID", "NAME", "CRIT", "CLASS", "RPO", "RTO", "DEPT", "OWNER")
			fmt.Println(repeat("─", 100))
			count := 0
			for _, sys := range systems {
				if filterCriticality != "" && sys.Criticality != filterCriticality {
					continue
				}
				fmt.Printf("%-10s %-24s %-10s %-14s %-8s %-8s %-12s %s\n",
					sys.Identifier,
					truncate(sys.Name, 24),
					sys.Criticality,
					sys.Classification,
					fmt.Sprintf("%dh", sys.RPOHours),
					fmt.Sprintf("%dh", sys.RTOHours),
					truncate(sys.Department, 12),
					truncate(sys.Owner, 15))
				count++
			}
			fmt.Printf("\n%d systems\n", count)
			return nil
		},
	}

	cmd.Flags().StringVar(&filterCriticality, "criticality", "", "Filter by criticality")
	return cmd
}

func systemEditCmd() *cobra.Command {
	var (
		name            string
		description     string
		supplierID      int64
		department      string
		classification  string
		criticality     string
		rpoHours        int
		rtoHours        int
		confidentiality int
		integrity       int
		availability    int
		owner           string
		notes           string
	)

	cmd := &cobra.Command{
		Use:   "edit <system-id>",
		Short: "Edit a system",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			update := &db.System{}
			if cmd.Flags().Changed("name") {
				update.Name = name
			}
			if cmd.Flags().Changed("desc") {
				update.Description = description
			}
			if cmd.Flags().Changed("supplier-id") {
				update.SupplierID = &supplierID
			}
			if cmd.Flags().Changed("department") {
				update.Department = department
			}
			if cmd.Flags().Changed("classification") {
				update.Classification = classification
			}
			if cmd.Flags().Changed("criticality") {
				update.Criticality = criticality
			}
			if cmd.Flags().Changed("rpo") {
				update.RPOHours = rpoHours
			}
			if cmd.Flags().Changed("rto") {
				update.RTOHours = rtoHours
			}
			if cmd.Flags().Changed("cia-c") {
				update.Confidentiality = &confidentiality
			}
			if cmd.Flags().Changed("cia-i") {
				update.Integrity = &integrity
			}
			if cmd.Flags().Changed("cia-a") {
				update.Availability = &availability
			}
			if cmd.Flags().Changed("owner") {
				update.Owner = owner
			}
			if cmd.Flags().Changed("notes") {
				update.Notes = notes
			}
			if _, err := c.UpdateSystem(id, update); err != nil {
				return err
			}
			fmt.Printf("Updated %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "System/service name")
	cmd.Flags().StringVar(&description, "desc", "", "Description (use ## Purpose heading)")
	cmd.Flags().Int64Var(&supplierID, "supplier-id", 0, "Supplier ID (FK)")
	cmd.Flags().StringVar(&department, "department", "", "Department")
	cmd.Flags().StringVar(&classification, "classification", "", "Classification: public, internal, confidential, restricted")
	cmd.Flags().StringVar(&criticality, "criticality", "", "Criticality: low, medium, high, critical")
	cmd.Flags().IntVar(&rpoHours, "rpo", 0, "Recovery Point Objective in hours")
	cmd.Flags().IntVar(&rtoHours, "rto", 0, "Recovery Time Objective in hours")
	cmd.Flags().IntVar(&confidentiality, "cia-c", 0, "Confidentiality impact (0-5)")
	cmd.Flags().IntVar(&integrity, "cia-i", 0, "Integrity impact (0-5)")
	cmd.Flags().IntVar(&availability, "cia-a", 0, "Availability impact (0-5)")
	cmd.Flags().StringVar(&owner, "owner", "", "System owner")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes (use ## Access control heading)")
	return cmd
}

func systemRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <system-id>",
		Short: "Remove a system from the register",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			if err := c.DeleteSystem(id); err != nil {
				return err
			}
			fmt.Printf("Removed %s\n", id)
			return nil
		},
	}
}

func systemReviewCmd() *cobra.Command {
	var (
		usersAdded   int
		usersRemoved int
		notes        string
		reviewedBy   string
	)

	cmd := &cobra.Command{
		Use:   "review <system-id>",
		Short: "Record an access review for a system",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			systemID := args[0]

			c := requireAPI()
			ar := &db.AccessReview{
				UsersAdded:   usersAdded,
				UsersRemoved: usersRemoved,
				Notes:        notes,
				ReviewedBy:   reviewedBy,
				ReviewedAt:   db.EpochNow(),
			}
			if reviewedBy == "" {
				// Will be filled server-side from auth
				ar.ReviewedBy = ""
			}

			// Parse the system ID number from SYSTEM-N format or raw number
			id := systemID
			if len(id) > 7 && id[:7] == "SYSTEM-" {
				id = id[7:]
			}

			result, err := c.CreateAccessReview(id, ar)
			if err != nil {
				return err
			}
			fmt.Printf("Access review recorded (ID %d) for system %s\n", result.ID, systemID)
			if usersAdded > 0 || usersRemoved > 0 {
				fmt.Printf("  +%d added, -%d removed\n", usersAdded, usersRemoved)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&usersAdded, "users-added", 0, "Number of users added")
	cmd.Flags().IntVar(&usersRemoved, "users-removed", 0, "Number of users removed")
	cmd.Flags().StringVar(&notes, "notes", "", "Review notes")
	cmd.Flags().StringVar(&reviewedBy, "reviewed-by", "", "Reviewer email (default: current user)")
	return cmd
}
