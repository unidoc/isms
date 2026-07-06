package api

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"isms.sh/internal/isms/db"
)

// openCAsLinkedError is returned by the unified incident write path when an
// incident can't be resolved/closed because open corrective actions are still
// linked. The HTTP handler maps it to 409; suggestion-apply surfaces it as the
// apply failure. Same rule, one code path (#26).
type openCAsLinkedError struct {
	verb string
	n    int
}

func (e openCAsLinkedError) Error() string {
	return fmt.Sprintf("cannot %s incident: %d open corrective action(s) still linked", e.verb, e.n)
}

// enforceIncidentWriteTx is the SINGLE enforced incident mutation path (#26).
// Every writer — the HTTP update handler and suggestion-apply — calls this inside
// a transaction, so the business rules can't diverge:
//   - open-CA guard when transitioning to resolved/closed
//   - lifecycle timestamps (contained/resolved/closed_at) for the new status
//
// inc holds the desired post-update state; prevStatus is the status before the
// update, used to detect a status transition.
func enforceIncidentWriteTx(ctx context.Context, tx pgx.Tx, orgID int, inc *db.Incident, prevStatus string) error {
	statusChanged := inc.Status != prevStatus
	if statusChanged && (inc.Status == "resolved" || inc.Status == "closed") {
		n, err := db.CountOpenCAsByIncidentTx(ctx, tx, orgID, inc.Identifier)
		if err != nil {
			return err
		}
		if n > 0 {
			return openCAsLinkedError{verb: statusVerb(inc.Status), n: n}
		}
	}
	if err := db.UpdateIncidentTx(ctx, tx, orgID, inc); err != nil {
		return err
	}
	// Lifecycle timestamps only need touching on a status transition — skip the
	// extra UPDATE on plain field edits (title, notes, …).
	if !statusChanged {
		return nil
	}
	return db.SetIncidentLifecycleTx(ctx, tx, orgID, inc.ID, inc.Status)
}
