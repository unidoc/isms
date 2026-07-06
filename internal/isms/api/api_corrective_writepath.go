package api

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"isms.sh/internal/isms/db"
)

// openTasksLinkedError is returned by the unified CA write path when a corrective
// action can't be resolved because open implementation tasks are still linked.
// The HTTP handler maps it to 409; suggestion-apply surfaces it as the apply
// failure. Same rule, one code path (#26).
type openTasksLinkedError struct {
	identifier string
	n          int
}

func (e openTasksLinkedError) Error() string {
	return fmt.Sprintf("cannot resolve %s: %d open implementation task(s) still linked", e.identifier, e.n)
}

// applyCorrectiveActionDefaults sets the server-side defaults for a new corrective
// action. Both the HTTP create handler and suggestion-apply call it, so a CA
// created either way starts in the SAME state — previously suggestion-apply used
// different defaults (status "assessment" vs "todo", etc.), the "wrong starting
// state" bug #26 names. Caller sets CreatedBy/Assignee (identity) beforehand.
func applyCorrectiveActionDefaults(ca *db.CorrectiveAction) {
	if ca.Status == "" {
		ca.Status = "todo"
	}
	if ca.Severity == "" {
		ca.Severity = "observation"
	}
	if ca.Source == "" {
		ca.Source = "other"
	}
	if ca.DueDate == nil {
		d := db.NewEpoch(time.Now().AddDate(0, 0, 30))
		ca.DueDate = &d
	}
	if ca.Notes == "" {
		ca.Notes = "## Action plan\n\n## Implementation\n\n## Verification\n\n## Evidence\n"
	}
}

// enforceCorrectiveActionWriteTx is the SINGLE enforced CA update path (#26).
// HTTP handler and suggestion-apply both call it inside a transaction:
//   - open-task guard when transitioning to resolved
//   - resolved_at / resolved_by_id closure metadata on →resolved
//
// ca holds the desired post-update state; prevStatus is the status before the
// update; actor stamps resolved_by_id.
func enforceCorrectiveActionWriteTx(ctx context.Context, tx pgx.Tx, orgID int, ca *db.CorrectiveAction, prevStatus, actor string) error {
	resolving := ca.Status == "resolved" && prevStatus != "resolved"
	if resolving {
		// An empty identifier is corrupt data — the open-task LIKE query would
		// otherwise degrade to '%%' and match every open ca_followup task in the
		// org, either false-blocking the resolve or silently disabling the guard.
		if ca.Identifier == "" {
			return fmt.Errorf("corrective action %d has no identifier", ca.ID)
		}
		n, err := db.CountOpenTasksByCATx(ctx, tx, orgID, ca.Identifier)
		if err != nil {
			return err
		}
		if n > 0 {
			return openTasksLinkedError{identifier: ca.Identifier, n: n}
		}
	}
	if err := db.UpdateCorrectiveActionTx(ctx, tx, orgID, ca); err != nil {
		return err
	}
	if resolving {
		return db.SetCorrectiveActionResolvedTx(ctx, tx, orgID, ca.ID, actor)
	}
	return nil
}
