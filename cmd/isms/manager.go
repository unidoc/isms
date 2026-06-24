package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func managerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Run ISMS manager — automated housekeeping (cron-safe)",
		Long: `Runs automated ISMS housekeeping tasks for all organizations.
Safe to run from cron — idempotent, no duplicates, logs all actions.

Tasks performed:
  - Create review tasks for overdue risks, suppliers, systems, legal requirements
  - Create review tasks for documents past their review cycle
  - Create check-in tasks for objectives with overdue check-ins
  - Backfill missing next_review dates on suppliers and systems
  - Report summary of actions taken

Example cron (every hour):
  0 * * * * /usr/local/bin/isms server manager --quiet

Example cron (daily at 8am):
  0 8 * * * /usr/local/bin/isms server manager`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			return runManager(quiet)
		},
	}

	cmd.Flags().Bool("quiet", false, "Only output if actions were taken")
	return cmd
}

func runManager(quiet bool) error {
	dbURL := getDBURL()
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx := context.Background()
	d, err := db.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer d.Close()

	// Global cleanup: expired tokens and sessions
	d.CleanupExpired(ctx)

	orgs, err := d.ListOrganizations(ctx)
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	totalCreated := 0
	totalBackfilled := 0

	for _, org := range orgs {
		orgID := org.ID

		created, backfilled, err := manageOrg(ctx, d, orgID, quiet)
		if err != nil && !quiet {
			fmt.Printf("[%s] error: %v\n", org.Slug, err)
			continue
		}
		totalCreated += created
		totalBackfilled += backfilled

		if !quiet && (created > 0 || backfilled > 0) {
			fmt.Printf("[%s] created %d tasks, backfilled %d review dates\n",
				org.Slug, created, backfilled)
		}
	}

	if !quiet {
		fmt.Printf("\nISMS manager: %d org(s), %d tasks created, %d review dates backfilled\n",
			len(orgs), totalCreated, totalBackfilled)
	} else if totalCreated > 0 || totalBackfilled > 0 {
		fmt.Printf("isms manager: %d tasks created, %d backfilled [%s]\n",
			totalCreated, totalBackfilled, time.Now().Format("2006-01-02 15:04"))
	}

	return nil
}

func manageOrg(ctx context.Context, d *db.DB, orgID int, quiet bool) (created, backfilled int, err error) {
	// 1. Backfill missing next_review on suppliers
	suppliers, err := d.ListSuppliers(ctx, orgID)
	if err == nil {
		for _, s := range suppliers {
			if s.NextReview == nil || s.NextReview.IsZero() {
				before := s.ToChangeMap()
				s.CalculateNextReview()
				if err := d.UpdateSupplier(ctx, orgID, &s); err == nil {
					backfilled++
					if changes := db.DiffFields("supplier", int64(s.ID), "system", "automated next_review backfill", before, s.ToChangeMap()); len(changes) > 0 {
						_ = d.LogChanges(ctx, orgID, changes)
					}
				}
			}
		}
	}

	// 2. Backfill missing next_review on systems
	systems, err := d.ListSystems(ctx, orgID)
	if err == nil {
		for _, sys := range systems {
			if sys.NextReview == nil || sys.NextReview.IsZero() {
				before := sys.ToChangeMap()
				sys.CalculateNextReview()
				if err := d.UpdateSystem(ctx, orgID, &sys); err == nil {
					backfilled++
					if changes := db.DiffFields("system", int64(sys.ID), "system", "automated next_review backfill", before, sys.ToChangeMap()); len(changes) > 0 {
						_ = d.LogChanges(ctx, orgID, changes)
					}
				}
			}
		}
	}

	// 3. Create tasks for all overdue items
	// Use "system" as the actor since this is automated
	result, err := d.CreateOverdueReviewTasks(ctx, orgID, "system")
	if err != nil {
		return created, backfilled, err
	}
	created = len(result.Created)

	return created, backfilled, nil
}
