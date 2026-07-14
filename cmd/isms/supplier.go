package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func supplierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supplier",
		Short: "Manage supplier register",
	}

	cmd.AddCommand(supplierAddCmd(), supplierListCmd(), supplierEditCmd(),
		supplierRmCmd(), supplierReviewCmd(), supplierReviewedCmd(), supplierOverdueCmd())
	return cmd
}

func supplierAddCmd() *cobra.Command {
	var (
		name            string
		supplierType    string
		criticality     string
		dataAccess      bool
		contact         string
		contractRef     string
		notes           string
		confidentiality int
		integrity       int
		availability    int
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a supplier",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || supplierType == "" || criticality == "" {
				return fmt.Errorf("required: --name, --type, --criticality")
			}
			if err := validateCIA(cmd); err != nil {
				return err
			}

			c := requireAPI()
			sup := &db.Supplier{
				Name:         name,
				SupplierType: supplierType,
				Criticality:  criticality,
				DataAccess:   dataAccess,
				Contact:      contact,
				ContractRef:  contractRef,
				Notes:        notes,
			}
			// CIA ratings: set only when supplied, so an unset flag stays NULL
			// (not assessed) rather than writing 0.
			if cmd.Flags().Changed("confidentiality") {
				sup.Confidentiality = &confidentiality
			}
			if cmd.Flags().Changed("integrity") {
				sup.Integrity = &integrity
			}
			if cmd.Flags().Changed("availability") {
				sup.Availability = &availability
			}
			result, err := c.AddSupplier(sup)
			if err != nil {
				return err
			}
			fmt.Printf("Added supplier %s: %s\n", result.Identifier, result.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Supplier name")
	cmd.Flags().StringVar(&supplierType, "type", "", "Type: cloud, saas, consulting, hosting, infrastructure, software, contractor, other")
	cmd.Flags().StringVar(&criticality, "criticality", "", "Criticality: low, medium, high, critical")
	cmd.Flags().BoolVar(&dataAccess, "data-access", false, "Supplier has access to our data")
	cmd.Flags().StringVar(&contact, "contact", "", "Contact info")
	cmd.Flags().StringVar(&contractRef, "contract-ref", "", "Contract reference")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes (use ## Services heading to describe services)")
	cmd.Flags().IntVar(&confidentiality, "confidentiality", 0, "Confidentiality rating (0-5, unset = not assessed)")
	cmd.Flags().IntVar(&integrity, "integrity", 0, "Integrity rating (0-5, unset = not assessed)")
	cmd.Flags().IntVar(&availability, "availability", 0, "Availability rating (0-5, unset = not assessed)")
	return cmd
}

// validateCIA checks that any supplied CIA rating flag is within the DB's 0-5
// range, giving a fast, friendly error before the API round-trip.
func validateCIA(cmd *cobra.Command) error {
	for _, f := range []string{"confidentiality", "integrity", "availability"} {
		if !cmd.Flags().Changed(f) {
			continue
		}
		v, _ := cmd.Flags().GetInt(f)
		if v < 0 || v > 5 {
			return fmt.Errorf("--%s must be between 0 and 5", f)
		}
	}
	return nil
}

func supplierListCmd() *cobra.Command {
	var (
		filterCriticality string
		filterOverdue     bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List suppliers",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			suppliers, err := c.ListSuppliers()
			if err != nil {
				return err
			}
			fmt.Printf("%-10s %-30s %-10s %-10s %-8s %-12s %-12s\n",
				"ID", "NAME", "TYPE", "CRIT", "DATA", "LAST REV", "NEXT REV")
			fmt.Println(repeat("─", 96))
			now := time.Now()
			count := 0
			for _, sup := range suppliers {
				if filterCriticality != "" && sup.Criticality != filterCriticality {
					continue
				}
				if filterOverdue {
					if sup.NextReview == nil || sup.NextReview.IsZero() {
						continue
					}
					if !sup.NextReview.Before(now) {
						continue
					}
				}
				da := "no"
				if sup.DataAccess {
					da = "yes"
				}
				fmt.Printf("%-10s %-30s %-10s %-10s %-8s %-12s %-12s\n",
					sup.Identifier, truncate(sup.Name, 30), sup.SupplierType, sup.Criticality,
					da, sup.LastReview, sup.NextReview)
				count++
			}
			fmt.Printf("\n%d suppliers\n", count)
			return nil
		},
	}

	cmd.Flags().StringVar(&filterCriticality, "criticality", "", "Filter by criticality")
	cmd.Flags().BoolVar(&filterOverdue, "overdue", false, "Show only overdue suppliers")
	return cmd
}

// supplierEditPayload is the partial-update wire shape for `supplier edit`: every
// field is a pointer with omitempty, so an unset flag is omitted entirely rather
// than sent as a zero value the server would write over an existing value (#147).
// Mirrors the server's supplierUpdateRequest. data_access is *bool (not bool) so an
// explicit --data-access=false still turns it off instead of being dropped.
type supplierEditPayload struct {
	Name            *string `json:"name,omitempty"`
	SupplierType    *string `json:"supplier_type,omitempty"`
	Criticality     *string `json:"criticality,omitempty"`
	DataAccess      *bool   `json:"data_access,omitempty"`
	Contact         *string `json:"contact,omitempty"`
	Notes           *string `json:"notes,omitempty"`
	Confidentiality *int    `json:"confidentiality,omitempty"`
	Integrity       *int    `json:"integrity,omitempty"`
	Availability    *int    `json:"availability,omitempty"`
}

func supplierEditCmd() *cobra.Command {
	var (
		name            string
		supplierType    string
		criticality     string
		dataAccess      bool
		contact         string
		notes           string
		confidentiality int
		integrity       int
		availability    int
	)

	cmd := &cobra.Command{
		Use:   "edit <supplier-id>",
		Short: "Edit a supplier",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if err := validateCIA(cmd); err != nil {
				return err
			}

			c := requireAPI()
			// Only fields whose flag was set go on the wire; everything else is
			// omitted so a partial edit never blanks an untouched field (#147).
			update := &supplierEditPayload{}
			if cmd.Flags().Changed("name") {
				update.Name = &name
			}
			if cmd.Flags().Changed("type") {
				update.SupplierType = &supplierType
			}
			if cmd.Flags().Changed("criticality") {
				update.Criticality = &criticality
			}
			if cmd.Flags().Changed("data-access") {
				update.DataAccess = &dataAccess
			}
			if cmd.Flags().Changed("contact") {
				update.Contact = &contact
			}
			if cmd.Flags().Changed("notes") {
				update.Notes = &notes
			}
			if cmd.Flags().Changed("confidentiality") {
				update.Confidentiality = &confidentiality
			}
			if cmd.Flags().Changed("integrity") {
				update.Integrity = &integrity
			}
			if cmd.Flags().Changed("availability") {
				update.Availability = &availability
			}
			if _, err := c.UpdateSupplier(id, update); err != nil {
				return err
			}
			fmt.Printf("Updated %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Supplier name")
	cmd.Flags().StringVar(&supplierType, "type", "", "Type")
	cmd.Flags().StringVar(&criticality, "criticality", "", "Criticality")
	cmd.Flags().BoolVar(&dataAccess, "data-access", false, "Data access")
	cmd.Flags().StringVar(&contact, "contact", "", "Contact")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes (use ## Services heading)")
	cmd.Flags().IntVar(&confidentiality, "confidentiality", 0, "Confidentiality rating (0-5, unset = not assessed)")
	cmd.Flags().IntVar(&integrity, "integrity", 0, "Integrity rating (0-5, unset = not assessed)")
	cmd.Flags().IntVar(&availability, "availability", 0, "Availability rating (0-5, unset = not assessed)")
	return cmd
}

func supplierRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <supplier-id>",
		Short: "Remove a supplier",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			if err := c.DeleteSupplier(id); err != nil {
				return err
			}
			fmt.Printf("Removed %s\n", id)
			return nil
		},
	}
}

func supplierReviewCmd() *cobra.Command {
	var allOverdue bool

	cmd := &cobra.Command{
		Use:   "review [supplier-id]",
		Short: "Create review tasks for suppliers",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			if allOverdue {
				suppliers, err := c.ListSuppliers()
				if err != nil {
					return err
				}
				now := time.Now()
				count := 0
				for _, sup := range suppliers {
					if sup.NextReview == nil || sup.NextReview.IsZero() || !sup.NextReview.Before(now) {
						continue
					}
					task := &db.Task{
						Title:    fmt.Sprintf("Supplier review: %s (%s)", sup.Name, sup.Identifier),
						TaskType: "supplier_review",
						Status:   "open",
						Priority: supplierCriticalityToPriority(sup.Criticality),
						DueDate:  sup.NextReview,
					}
					if err := createReviewTask(c, task); err != nil {
						fmt.Printf("  Failed to create task for %s: %v\n", sup.Identifier, err)
						continue
					}
					fmt.Printf("  Created task: %s\n", task.Title)
					count++
				}
				if count == 0 {
					fmt.Println("No overdue supplier reviews.")
				} else {
					fmt.Printf("\nCreated %d review tasks.\n", count)
				}
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("provide a supplier ID or use --all-overdue")
			}

			// Single supplier review task
			suppliers, err := c.ListSuppliers()
			if err != nil {
				return err
			}
			for _, sup := range suppliers {
				if sup.Identifier == args[0] || fmt.Sprintf("%d", sup.ID) == args[0] {
					task := &db.Task{
						Title:    fmt.Sprintf("Supplier review: %s (%s)", sup.Name, sup.Identifier),
						TaskType: "supplier_review",
						Status:   "open",
						Priority: supplierCriticalityToPriority(sup.Criticality),
					}
					if err := createReviewTask(c, task); err != nil {
						return fmt.Errorf("failed to create task: %w", err)
					}
					fmt.Printf("Created task: %s\n", task.Title)
					return nil
				}
			}
			return fmt.Errorf("supplier %s not found", args[0])
		},
	}

	cmd.Flags().BoolVar(&allOverdue, "all-overdue", false, "Create tasks for all overdue suppliers")
	return cmd
}

func supplierCriticalityToPriority(criticality string) string {
	switch criticality {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func supplierReviewedCmd() *cobra.Command {
	var reviewDate string

	cmd := &cobra.Command{
		Use:   "reviewed <supplier-id>",
		Short: "Mark a supplier as reviewed (auto-calculates next review date)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			// Fetch current supplier so we can do a full PUT
			suppliers, err := c.ListSuppliers()
			if err != nil {
				return err
			}
			var sup *db.Supplier
			for _, s := range suppliers {
				if s.Identifier == id || fmt.Sprintf("%d", s.ID) == id {
					sup = &s
					break
				}
			}
			if sup == nil {
				return fmt.Errorf("supplier %s not found", id)
			}

			rd, err := parseEpochPtr(reviewDate)
			if err != nil {
				return err
			}
			if rd == nil {
				e := db.EpochNow()
				rd = &e
			}
			sup.LastReview = rd
			// next_review will be auto-calculated server-side

			result, err := c.UpdateSupplier(fmt.Sprintf("%d", sup.ID), sup)
			if err != nil {
				return err
			}
			fmt.Printf("%s reviewed on %s. Next review: %s\n",
				result.Identifier, epochPtrStr(result.LastReview), epochPtrStr(result.NextReview))
			return nil
		},
	}

	cmd.Flags().StringVar(&reviewDate, "date", "", "Review date (YYYY-MM-DD, default: today)")
	return cmd
}

func supplierOverdueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "overdue",
		Short: "List suppliers that need review",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			suppliers, err := c.ListSuppliers()
			if err != nil {
				return err
			}
			now := time.Now()
			count := 0
			for _, sup := range suppliers {
				if sup.NextReview == nil || sup.NextReview.IsZero() {
					continue
				}
				if sup.NextReview.Before(now) {
					fmt.Printf("  %s  %s  (due: %s, criticality: %s)\n",
						sup.Identifier, sup.Name, sup.NextReview.Format("2006-01-02"), sup.Criticality)
					count++
				}
			}
			if count == 0 {
				fmt.Println("No overdue supplier reviews.")
			} else {
				fmt.Printf("\n%d overdue supplier reviews.\n", count)
			}
			return nil
		},
	}
}
