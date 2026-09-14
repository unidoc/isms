package api

import (
	"context"
	"fmt"

	"isms.sh/internal/isms/db"
)

// supplierContractLink deep-links to the supplier itself. The register view is
// routed as both /suppliers and /suppliers/:id (web/src/router.js:43-44) and
// opens the matching record from its route param, so this lands the reader on
// the contract rather than on a list to search.
func supplierContractLink(identifier string) string {
	return "/suppliers/" + identifier
}

// NotifySupplierContractExpiry sends in-app notifications for supplier contracts
// sitting exactly on the 30-day, 7-day or expiry-day mark today (#44, Annex A
// 5.20). It returns how many notifications it created.
//
// Called from the `isms server manager` cron, not from an HTTP handler — the
// wire keys are declared in this package and must not leak into db or cmd (see
// notification_keys.go), so the job lives here and the cron calls in.
//
// Nothing fires after expiry: an expired contract is carried by the overdue
// summary's SupplierContracts slice from then on, which is persistent UI state
// rather than a repeated nag.
//
// Recipients are the supplier's owner when set, and every admin/manager
// otherwise — the same fallback the incident notifications use, because an
// unowned expiring contract is exactly the case someone has to pick up.
func NotifySupplierContractExpiry(ctx context.Context, d *db.DB, orgID int) (int, error) {
	due, err := d.SupplierContractsAtMarks(ctx, orgID)
	if err != nil {
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}

	// Loaded once, not per supplier: the fallback is the same list every time.
	var fallback []string
	members, _ := d.ListOrgUsers(ctx, orgID)
	for _, m := range members {
		if m.Role == "admin" || m.Role == "manager" {
			fallback = append(fallback, m.Email)
		}
	}

	sent := 0
	for _, c := range due {
		params := map[string]any{"title": c.Name, "id": c.Identifier}

		recipients := fallback
		if c.Owner != "" {
			recipients = []string{c.Owner}
		}

		// The wire key is named directly in each branch (rather than resolved
		// through a shared variable) so every emission is unconditionally tied
		// to its own constant — the same shape the notification param checks
		// already read at every other multi-key notification in this package.
		if c.DaysUntil == 30 {
			title := fmt.Sprintf("Contract expires in 30 days: %s", c.Name)
			body := fmt.Sprintf("The contract for %s (%s) expires in 30 days.", c.Name, c.Identifier)
			for _, email := range recipients {
				// `isms server manager` is documented as an hourly cron, so the
				// same mark is reached many times on the day it lands. The row
				// the first run wrote is the record that it already fired.
				already, err := d.NotificationExistsToday(ctx, orgID, email, NotifyKeySupplierContract30, c.Identifier)
				if err != nil || already {
					continue
				}
				if err := d.CreateNotificationContentByEmail(ctx, orgID, email, db.NotificationContent{
					Title:    title,
					TitleKey: NotifyKeySupplierContract30,
					Body:     body,
					BodyKey:  NotifyKeySupplierContract30Body,
					Params:   params,
					Link:     supplierContractLink(c.Identifier),
				}); err == nil {
					sent++
				}
			}
		} else if c.DaysUntil == 7 {
			title := fmt.Sprintf("Contract expires in 7 days: %s", c.Name)
			body := fmt.Sprintf("The contract for %s (%s) expires in 7 days.", c.Name, c.Identifier)
			for _, email := range recipients {
				already, err := d.NotificationExistsToday(ctx, orgID, email, NotifyKeySupplierContract7, c.Identifier)
				if err != nil || already {
					continue
				}
				if err := d.CreateNotificationContentByEmail(ctx, orgID, email, db.NotificationContent{
					Title:    title,
					TitleKey: NotifyKeySupplierContract7,
					Body:     body,
					BodyKey:  NotifyKeySupplierContract7Body,
					Params:   params,
					Link:     supplierContractLink(c.Identifier),
				}); err == nil {
					sent++
				}
			}
		} else {
			title := fmt.Sprintf("Contract expires today: %s", c.Name)
			body := fmt.Sprintf("The contract for %s (%s) expires today.", c.Name, c.Identifier)
			for _, email := range recipients {
				already, err := d.NotificationExistsToday(ctx, orgID, email, NotifyKeySupplierContractToday, c.Identifier)
				if err != nil || already {
					continue
				}
				if err := d.CreateNotificationContentByEmail(ctx, orgID, email, db.NotificationContent{
					Title:    title,
					TitleKey: NotifyKeySupplierContractToday,
					Body:     body,
					BodyKey:  NotifyKeySupplierContractTodayBody,
					Params:   params,
					Link:     supplierContractLink(c.Identifier),
				}); err == nil {
					sent++
				}
			}
		}
	}
	return sent, nil
}
