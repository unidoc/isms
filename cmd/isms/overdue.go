package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func overdueCmd() *cobra.Command {
	var createTasks bool

	cmd := &cobra.Command{
		Use:   "overdue",
		Short: "Show overdue reviews and optionally create tasks for them",
		Long: `Lists all overdue review items across the ISMS: risks, suppliers, systems,
legal requirements, and tasks.

Use --create-tasks to auto-create ISMS tasks for all overdue items.
Deduplicates: won't create a task if one already exists.

Examples:
  isms overdue                  # list everything overdue
  isms overdue --create-tasks   # create review tasks for all overdue items`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			summary, err := c.GetOverdueSummary()
			if err != nil {
				return err
			}

			if summary.TotalCount == 0 {
				fmt.Println("Nothing overdue. All reviews are on schedule.")
				return nil
			}

			fmt.Printf("Overdue Reviews (%d)\n", summary.TotalCount)
			fmt.Println(repeat("═", 70))

			if len(summary.Risks) > 0 {
				fmt.Printf("\n  Risks (%d)\n", len(summary.Risks))
				fmt.Println("  " + repeat("─", 66))
				for _, r := range summary.Risks {
					fmt.Printf("  %-14s %-32s %3d days late  [%s]\n",
						r.EntityID, truncate(r.Title, 32), r.DaysLate, r.Criticality)
				}
			}

			if len(summary.Suppliers) > 0 {
				fmt.Printf("\n  Suppliers (%d)\n", len(summary.Suppliers))
				fmt.Println("  " + repeat("─", 66))
				for _, s := range summary.Suppliers {
					fmt.Printf("  %-14s %-32s %3d days late  [%s]\n",
						s.EntityID, truncate(s.Title, 32), s.DaysLate, s.Criticality)
				}
			}

			if len(summary.Systems) > 0 {
				fmt.Printf("\n  Systems — access review (%d)\n", len(summary.Systems))
				fmt.Println("  " + repeat("─", 66))
				for _, s := range summary.Systems {
					fmt.Printf("  %-14s %-32s %3d days late  [%s]\n",
						s.EntityID, truncate(s.Title, 32), s.DaysLate, s.Criticality)
				}
			}

			if len(summary.Legal) > 0 {
				fmt.Printf("\n  Legal requirements (%d)\n", len(summary.Legal))
				fmt.Println("  " + repeat("─", 66))
				for _, l := range summary.Legal {
					fmt.Printf("  %-14s %-32s %3d days late  [%s]\n",
						l.EntityID, truncate(l.Title, 32), l.DaysLate, l.Criticality)
				}
			}

			if len(summary.Tasks) > 0 {
				fmt.Printf("\n  Overdue tasks (%d)\n", len(summary.Tasks))
				fmt.Println("  " + repeat("─", 66))
				for _, t := range summary.Tasks {
					fmt.Printf("  %-14s %-32s %3d days late  [%s]\n",
						t.EntityID, truncate(t.Title, 32), t.DaysLate, t.Criticality)
				}
			}

			if !createTasks {
				overdueReviews := len(summary.Risks) + len(summary.Suppliers) +
					len(summary.Systems) + len(summary.Legal)
				if overdueReviews > 0 {
					fmt.Printf("\nRun `isms overdue --create-tasks` to create review tasks for %d overdue items.\n", overdueReviews)
				}
				return nil
			}

			// Create tasks
			fmt.Println()
			result, err := c.CreateOverdueTasks()
			if err != nil {
				return fmt.Errorf("failed to create tasks: %w", err)
			}

			if len(result.Created) > 0 {
				fmt.Printf("Created %d review tasks:\n", len(result.Created))
				for _, t := range result.Created {
					fmt.Printf("  #%-4d [%-8s] %s\n", t.ID, t.Priority, t.Title)
				}
			}
			if result.Skipped > 0 {
				fmt.Printf("Skipped %d (task already exists).\n", result.Skipped)
			}
			if len(result.Created) == 0 && result.Skipped > 0 {
				fmt.Println("All overdue items already have open tasks.")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&createTasks, "create-tasks", false, "Create ISMS tasks for all overdue review items")
	return cmd
}
