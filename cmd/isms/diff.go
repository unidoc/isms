package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func diffCmd() *cobra.Command {
	var sinceCommit string

	cmd := &cobra.Command{
		Use:   "diff <document-id>",
		Short: "Show changes to a document since last commit",
		Long: `Show what changed in a document. Useful before sending for review.

Examples:
  isms diff ISO27001-4.1              # diff against last commit
  isms diff ISO27001-4.1 --since HEAD~3  # diff against 3 commits ago
  isms diff ISO27001-4.1 --since abc123   # diff against specific commit`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			docID := args[0]
			diff, err := c.DocumentDiff(docID, sinceCommit)
			if err != nil {
				return err
			}
			if diff == "" {
				fmt.Println("No changes.")
				return nil
			}
			fmt.Print(diff)
			return nil
		},
	}

	cmd.Flags().StringVar(&sinceCommit, "since", "", "Compare against this commit/ref (default: HEAD)")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show ISMS status: documents, overdue reviews, and pending tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			// Fetch all documents via unified API.
			allDocs, _ := c.FlattenAllDocs()

			type statusCount struct {
				draft, inReview, approved, total int
			}

			// Group by folder
			byFolder := map[string]*statusCount{}
			for _, d := range allDocs {
				folder := d.Folder
				if folder == "" {
					folder = "other"
				}
				sc, ok := byFolder[folder]
				if !ok {
					sc = &statusCount{}
					byFolder[folder] = sc
				}
				sc.total++
				switch d.Status {
				case "draft":
					sc.draft++
				case "in_review":
					sc.inReview++
				case "approved":
					sc.approved++
				}
			}

			fmt.Println("Documents")
			fmt.Println(repeat("─", 50))
			fmt.Printf("  %-12s %5s %5s %5s %5s\n", "FOLDER", "DRAFT", "REVIEW", "APPR", "TOTAL")

			var totalDraft, totalReview, totalApproved, totalAll int
			for folder, sc := range byFolder {
				fmt.Printf("  %-12s %5d %5d %5d %5d\n",
					truncate(folder, 12), sc.draft, sc.inReview, sc.approved, sc.total)
				totalDraft += sc.draft
				totalReview += sc.inReview
				totalApproved += sc.approved
				totalAll += sc.total
			}

			fmt.Println(repeat("─", 50))
			pct := 0
			if totalAll > 0 {
				pct = (totalApproved * 100) / totalAll
			}
			fmt.Printf("  %-12s %5d %5d %5d %5d  (%d%% approved)\n", "Total",
				totalDraft, totalReview, totalApproved, totalAll, pct)

			// Overdue reviews
			summary, err := c.GetOverdueSummary()
			if err == nil && summary.TotalCount > 0 {
				fmt.Println()
				fmt.Printf("Overdue Reviews (%d)\n", summary.TotalCount)
				fmt.Println(repeat("─", 70))
				if len(summary.Risks) > 0 {
					fmt.Printf("  Risks (%d)\n", len(summary.Risks))
					for _, r := range summary.Risks {
						fmt.Printf("    %-12s %-35s %d days late  [%s]\n",
							r.EntityID, truncate(r.Title, 35), r.DaysLate, r.Criticality)
					}
				}
				if len(summary.Suppliers) > 0 {
					fmt.Printf("  Suppliers (%d)\n", len(summary.Suppliers))
					for _, s := range summary.Suppliers {
						fmt.Printf("    %-12s %-35s %d days late  [%s]\n",
							s.EntityID, truncate(s.Title, 35), s.DaysLate, s.Criticality)
					}
				}
				if len(summary.Systems) > 0 {
					fmt.Printf("  Systems (%d)\n", len(summary.Systems))
					for _, s := range summary.Systems {
						fmt.Printf("    %-12s %-35s %d days late  [%s]\n",
							s.EntityID, truncate(s.Title, 35), s.DaysLate, s.Criticality)
					}
				}
				if len(summary.Legal) > 0 {
					fmt.Printf("  Legal (%d)\n", len(summary.Legal))
					for _, l := range summary.Legal {
						fmt.Printf("    %-12s %-35s %d days late  [%s]\n",
							l.EntityID, truncate(l.Title, 35), l.DaysLate, l.Criticality)
					}
				}
				if len(summary.Tasks) > 0 {
					fmt.Printf("  Tasks (%d)\n", len(summary.Tasks))
					for _, t := range summary.Tasks {
						fmt.Printf("    %-12s %-35s %d days late  [%s]\n",
							t.EntityID, truncate(t.Title, 35), t.DaysLate, t.Criticality)
					}
				}
			} else if err == nil {
				fmt.Println()
				fmt.Println("No overdue reviews.")
			}

			return nil
		},
	}
}
