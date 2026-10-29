package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/api"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/mail"
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
  - Create renewal tasks for supplier contracts and supplier evidence links
    that are expiring or expired
  Tasks are assigned to the item's owner; an item with no owner is skipped and
  reported as unassigned. A task that cannot be created is reported as failed.
  - Backfill missing next_review dates on suppliers and systems
  - Notify on supplier contracts 30 days, 7 days, and 0 days from expiry
  - Email each member their weekly digest of pending work (Monday 08:00 UTC
    onward, once per ISO week; needs SMTP_HOST, SMTP_FROM and ISMS_BASE_URL)
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
	totalNotified := 0
	totalUnassigned := 0
	totalFailed := 0

	for _, org := range orgs {
		orgID := org.ID

		created, backfilled, notified, unassigned, failed, err := manageOrg(ctx, d, orgID, quiet)
		// Printed even under --quiet: cron mails any output, which is what a
		// failure should do. The counts still go into the totals, because what
		// manageOrg did before failing (e.g. notifications) did happen.
		if err != nil {
			fmt.Printf("[%s] error: %v\n", org.Slug, err)
		}
		totalCreated += created
		totalBackfilled += backfilled
		totalNotified += notified
		totalUnassigned += unassigned
		totalFailed += failed

		if !quiet && (created > 0 || backfilled > 0 || notified > 0 || unassigned > 0 || failed > 0) {
			fmt.Printf("[%s] created %d tasks (%d unassigned, %d failed), backfilled %d review dates, sent %d notifications\n",
				org.Slug, created, unassigned, failed, backfilled, notified)
		}
	}

	// Weekly digests run after the housekeeping above, so a failure here never
	// costs the run its primary output. Mail settings come from the same env
	// vars `serve` reads; without SMTP or ISMS_BASE_URL the step is skipped
	// (a digest without links is not worth sending), with a line unless quiet.
	digests, derr := api.SendDigests(ctx, d, mail.New(mail.ConfigFromEnv()), os.Getenv("ISMS_BASE_URL"),
		os.Getenv("ISMS_SUBDOMAIN_ROUTING") == "1", time.Now())
	if derr != nil {
		fmt.Printf("[warn] weekly digests failed: %v\n", derr)
	}
	if digests.Disabled != "" && !quiet {
		fmt.Printf("weekly digests skipped: %s\n", digests.Disabled)
	}

	if !quiet {
		fmt.Printf("\nISMS manager: %d org(s), %d tasks created (%d unassigned, %d failed), %d review dates backfilled, %d notifications sent, %d digests sent, %d digests failed\n",
			len(orgs), totalCreated, totalUnassigned, totalFailed, totalBackfilled, totalNotified, digests.Sent, digests.Failed)
	} else if totalCreated > 0 || totalBackfilled > 0 || totalNotified > 0 || totalFailed > 0 || digests.Sent > 0 || digests.Failed > 0 {
		// Unassigned alone does not print: an item without an owner stays
		// unassigned on every run, and --quiet is for cron, which mails any output.
		fmt.Printf("isms manager: %d tasks created (%d unassigned, %d failed), %d backfilled, %d notified, %d digests sent, %d digests failed [%s]\n",
			totalCreated, totalUnassigned, totalFailed, totalBackfilled, totalNotified, digests.Sent, digests.Failed, time.Now().Format("2006-01-02 15:04"))
	}

	return nil
}

func manageOrg(ctx context.Context, d *db.DB, orgID int, quiet bool) (created, backfilled, notified, unassigned, failed int, err error) {
	// 1. Backfill missing next_review on suppliers
	suppliers, err := d.ListSuppliers(ctx, orgID)
	if err == nil {
		// Resolved once per org, not per supplier: SupplierReviewCycles is four
		// GetOrgSetting queries and the value cannot change mid-loop.
		cycles := d.SupplierReviewCycles(ctx, orgID)
		for _, s := range suppliers {
			if s.NextReview == nil || s.NextReview.IsZero() {
				before := s.ToChangeMap()
				s.CalculateNextReview(cycles)
				if err := d.UpdateSupplier(ctx, orgID, &s, nil); err == nil {
					backfilled++
					if changes := db.DiffFields("supplier", int64(s.ID), "system", "automated next_review backfill", before, s.ToChangeMap()); len(changes) > 0 {
						// Unlike an HTTP handler (which has a response as a signal),
						// the cron's only signal is stdout — surface a silent
						// audit-trail failure instead of swallowing it.
						if err := d.LogChanges(ctx, orgID, changes); err != nil && !quiet {
							fmt.Printf("[warn] changelog write failed for supplier %d: %v\n", s.ID, err)
						}
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
				if err := d.UpdateSystem(ctx, orgID, &sys, nil); err == nil {
					backfilled++
					if changes := db.DiffFields("system", int64(sys.ID), "system", "automated next_review backfill", before, sys.ToChangeMap()); len(changes) > 0 {
						if err := d.LogChanges(ctx, orgID, changes); err != nil && !quiet {
							fmt.Printf("[warn] changelog write failed for system %d: %v\n", sys.ID, err)
						}
					}
				}
			}
		}
	}

	// 3. Create tasks for all overdue items
	// Use "system" as the actor since this is automated
	// "system" is not a user, so items without an owner are skipped and reported
	// (unassigned) rather than assigned to it; the dashboard button, which has a
	// real user to fall back on, handles those.
	// A failure is returned at the end, not here: step 4 does not depend on it.
	result, terr := d.CreateOverdueReviewTasks(ctx, orgID, "system", "")
	if terr == nil {
		created = len(result.Created)
		unassigned = result.Unassigned
		failed = result.Failed
	}

	// 4. Supplier contract expiry notifications (#44).
	// Non-fatal: a notification failure must not stop the cron from having done
	// the task creation above, which is the run's primary output.
	notified, nerr := api.NotifySupplierContractExpiry(ctx, d, orgID)
	if nerr != nil && !quiet {
		fmt.Printf("[warn] supplier contract expiry notifications failed: %v\n", nerr)
	}

	return created, backfilled, notified, unassigned, failed, terr
}
