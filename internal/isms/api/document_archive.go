package api

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// The versions of a document that is gone are archived, so they are never read
// again and a new document that reuses the id starts with no history; its open
// reviews are closed, because a review belongs to one incarnation of a document.
// Neither can be undone, and a FALSE archive (an id that is still in the
// repository) would empty a live document's history and everything served from
// it. Every function here therefore acts on an id only when it has read the
// repository AT AN EXPLICIT COMMIT and the id is absent from it; any error
// archives nothing.
//
// Id sets are never taken from "whatever HEAD resolves to right now": a ref that
// is being rewritten can read as empty or as an older commit, and a set built
// from that would name every live document as gone. Callers pass the commits
// they know: the commit a delete returned and its parent, the default branch hash
// before and after a push (store.DefaultBranchHash, which does not trust a ref
// that is mid-write), or one resolved branch hash for "is this id absent now".
//
// Disappearance (the API delete, removing a template, a git push that removed
// documents) archives the ids that left. Appearance (POST /documents, adding a
// template, a git push that added documents) archives the active versions of an
// id that was absent before the write: the backfill for documents deleted before
// archiving existed, and a backstop for any path that removes a document without
// knowing it did.

// archiveIDs archives the active versions of each id and closes its live
// reviews, both in ONE transaction per id: either both happen or neither. The
// review branches and the activity entries follow the commit. It never decides
// whether an id is absent: callers do that first.
func (s *Server) archiveIDs(ctx context.Context, orgID int, st *store.Store, ids []string, why string) error {
	return s.archiveIDsBefore(ctx, orgID, st, ids, time.Now().Add(24*time.Hour), why)
}

// archiveIDsBefore is archiveIDs limited to versions and reviews created before
// the given time.
func (s *Server) archiveIDsBefore(ctx context.Context, orgID int, st *store.Store, ids []string, before time.Time, why string) error {
	sort.Strings(ids)
	for _, id := range ids {
		n, closed, err := s.db.ArchiveDocumentAndCloseReviewsBefore(ctx, orgID, id, before)
		if err != nil {
			return fmt.Errorf("archiving %s: %w", id, err)
		}
		if n > 0 {
			log.Printf("documents: org %d: archived %d version(s) of %s (%s)", orgID, n, id, why)
		}
		for _, rid := range closed {
			rid := rid
			if aerr := st.ArchiveReviewBranch(rid); aerr != nil {
				log.Printf("documents: org %d: archiving the branch of review %d: %v", orgID, rid, aerr)
			}
			s.logAndNotify(ctx, orgID, &db.Activity{
				DocumentID: id,
				ReviewID:   &rid,
				Actor:      "system",
				Action:     "review_status_changed",
				Detail:     fmt.Sprintf("Review #%d closed: document %s no longer exists (%s)", rid, id, why),
			})
		}
	}
	return nil
}

// idsAbsentFrom returns the ids in from that are not in present.
func idsAbsentFrom(from, present map[string]bool) []string {
	var out []string
	for id := range from {
		if !present[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// archiveBetween archives the ids that are in the commit beforeHash and not in
// afterHash. When scope is not nil only ids in scope are archived.
//
// There is no "empty set" brake here: the ids are read at explicit commits by
// DocumentPathsAtCommit, which fails on every read error and treats only a commit
// with no documents entry as having no documents, so an empty set read this way
// is true (a repository emptied for real). Refusing it would lose the archive for
// good: once the ids are gone, nothing would ever archive them again.
func (s *Server) archiveBetween(ctx context.Context, orgID int, st *store.Store, beforeHash, afterHash string, scope map[string]bool, why string) error {
	before, err := st.DocumentIDsAtCommit(beforeHash)
	if err != nil {
		return err
	}
	after, err := st.DocumentIDsAtCommit(afterHash)
	if err != nil {
		return err
	}
	ids := idsAbsentFrom(before, after)
	if scope != nil {
		kept := ids[:0]
		for _, id := range ids {
			if scope[id] {
				kept = append(kept, id)
			}
		}
		ids = kept
	}
	return s.archiveIDs(ctx, orgID, st, ids, why)
}

// archiveIfAbsent archives the active versions of the given ids that are absent
// from the default branch right now. It is called BEFORE a write that creates
// documents, so an id that is present (an edit, a re-scaffold over existing
// documents) is never touched. The branch is resolved once; a failure to read it
// archives nothing and is returned, and the caller must then not write.
func (s *Server) archiveIfAbsent(ctx context.Context, orgID int, st *store.Store, ids []string, why string) error {
	present, err := st.DocumentIDsAtHead()
	if err != nil {
		return fmt.Errorf("reading document ids at HEAD: %w", err)
	}
	var absent []string
	for _, id := range ids {
		if id = strings.ToLower(strings.TrimSpace(id)); id != "" && !present[id] {
			absent = append(absent, id)
		}
	}
	return s.archiveIDs(ctx, orgID, st, absent, why)
}

// pushBase is the state of the default branch before a push.
type pushBase struct {
	hash  string
	empty bool // a repository with no refs at all
}

// archiveAfterPush archives for a push that was accepted: ids that left the
// default branch, and the active versions of ids that appeared. fresh must be a
// store opened after the push (the long-lived one may not see a new pack).
func (s *Server) archiveAfterPush(ctx context.Context, orgID int, fresh *store.Store, base pushBase) error {
	afterHash, afterEmpty, err := fresh.DefaultBranchHash()
	if err != nil {
		return fmt.Errorf("reading the default branch after the push: %w", err)
	}
	if afterEmpty {
		return nil
	}
	after, err := fresh.DocumentIDsAtCommit(afterHash)
	if err != nil {
		return err
	}
	before := map[string]bool{}
	if !base.empty {
		if before, err = fresh.DocumentIDsAtCommit(base.hash); err != nil {
			return err
		}
	}
	// Both legs, with no empty-set brake (see archiveBetween): a push that removes
	// every document archives them all, and a later push that adds them back
	// archives nothing that is live.
	ids := append(idsAbsentFrom(before, after), idsAbsentFrom(after, before)...)
	return s.archiveIDs(ctx, orgID, fresh, ids, "git push")
}
