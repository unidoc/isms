package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// Documents deleted before versions were archived left their versions active, and
// they would still feed reviews, overdue tasks and the trust center if the id were
// ever used again. This archives them once per organization, at server start:
// every document id that has active versions but is ABSENT at the default branch's
// head gets its versions archived and its reviews closed (the same transaction as
// every other archive).
//
// Like every archive it is irreversible, so it acts only on strict reads: the head
// is read with DefaultBranchHash and the ids at that exact commit with
// DocumentPathsAtCommit, which fails on any error. Any error skips the
// organization without recording it as done, and the next start tries again. It
// runs in the background and never blocks the listener.

// StartDocumentVersionBackfill runs the backfill for every organization in the
// background.
func (s *Server) StartDocumentVersionBackfill(ctx context.Context) {
	go func() {
		orgs, err := s.db.ListOrganizations(ctx)
		if err != nil {
			log.Printf("documents: version backfill: listing organizations: %v", err)
			return
		}
		total := 0
		for _, org := range orgs {
			if ctx.Err() != nil {
				return
			}
			n, err := s.backfillOrgVersions(ctx, org.ID)
			if err != nil {
				log.Printf("documents: version backfill: organization %d skipped, will retry at the next start: %v", org.ID, err)
				continue
			}
			total += n
		}
		if total > 0 {
			log.Printf("documents: version backfill: archived the versions of %d deleted document(s)", total)
		}
	}()
}

// backfillNow is the database clock; a test replaces it through s.backfillClock.
func (s *Server) backfillNow(ctx context.Context) (time.Time, error) {
	if s.backfillClock != nil {
		return s.backfillClock(ctx)
	}
	return s.db.Now(ctx)
}

// backfillOrgVersions does the backfill for one organization and returns how many
// document ids it archived. It records completion only when everything was read
// and archived.
func (s *Server) backfillOrgVersions(ctx context.Context, orgID int) (int, error) {
	done, err := s.db.VersionBackfillDone(ctx, orgID)
	if err != nil {
		return 0, err
	}
	if done {
		return 0, nil
	}
	// Only versions that already existed when the repository was read are
	// archived: a document created and approved meanwhile is live. The time comes
	// from the database, before the repository is read, because it is compared with
	// created_at, which the database sets: the application host's clock may be
	// ahead of it. A version's row is written after its document is committed, so
	// one created before this time is for a document that the read below sees.
	readAt, err := s.backfillNow(ctx)
	if err != nil {
		return 0, fmt.Errorf("reading the database clock: %w", err)
	}
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("opening the repository: %w", err)
	}
	head, empty, err := st.DefaultBranchHash()
	if err != nil {
		return 0, fmt.Errorf("reading the default branch: %w", err)
	}
	present := map[string]string{}
	if !empty {
		if present, err = st.DocumentPathsAtCommit(head); err != nil {
			return 0, fmt.Errorf("reading the documents at %s: %w", head, err)
		}
	}
	withVersions, err := s.db.ActiveVersionDocumentIDs(ctx, orgID, readAt)
	if err != nil {
		return 0, err
	}
	// A repository with no commits next to recorded versions is a repository that
	// was initialised again, or the wrong one, not a set of deletions: archive
	// nothing and do not record completion.
	if empty && len(withVersions) > 0 {
		return 0, fmt.Errorf("the repository has no commits but %d document(s) have recorded versions", len(withVersions))
	}
	var gone []string
	for _, id := range withVersions {
		if _, ok := present[strings.ToLower(id)]; !ok {
			gone = append(gone, id)
		}
	}
	if err := s.archiveIDsBefore(ctx, orgID, st, gone, readAt, "backfill of documents deleted before versions were archived"); err != nil {
		return 0, err
	}
	if err := s.db.MarkVersionBackfillDone(ctx, orgID); err != nil {
		return 0, err
	}
	return len(gone), nil
}
