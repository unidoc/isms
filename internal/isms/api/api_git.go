package api

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/store"
)

// resolveGitOrg looks up org by UUID from the URL param, verifies membership,
// and sets org_id + user_role on the context.
func (s *Server) resolveGitOrg(c echo.Context) (string, error) {
	uuid := c.Param("uuid")
	org, err := s.db.GetOrganizationByUUID(c.Request().Context(), uuid)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}
	if org.RepoPath == "" {
		return "", echo.NewHTTPError(http.StatusInternalServerError, "organization has no repo path")
	}

	// A single sign-on session and an org-scoped key are confined to one
	// organization, whatever the URL asks for.
	if sso := getSSOOrg(c); sso > 0 && sso != org.ID {
		return "", echo.NewHTTPError(http.StatusForbidden, "access denied")
	}
	if keyOrg, ok := c.Get("api_key_org").(int); ok && keyOrg != org.ID {
		return "", echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	// If middleware already resolved org, verify it matches
	if tokenOrgID, ok := c.Get("org_id").(int); ok && tokenOrgID > 0 && tokenOrgID != org.ID {
		return "", echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	// Resolve user membership and role from the git URL's org
	email := getUserEmail(c)
	if email == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	user, err := s.db.GetUserByEmail(c.Request().Context(), email)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}
	role, err := s.db.GetUserRole(c.Request().Context(), org.ID, user.ID)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
	}
	c.Set("org_id", org.ID)
	c.Set("user_role", role)

	st, err := s.storeForOrg(c.Request().Context(), org.ID)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return st.Root(), nil
}

// handleGitInfoRefs handles GET /git/:uuid/info/refs for the git smart HTTP protocol.
func (s *Server) handleGitInfoRefs(c echo.Context) error {
	service := c.QueryParam("service")
	if service != "git-upload-pack" && service != "git-receive-pack" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid service")
	}

	repoPath, err := s.resolveGitOrg(c)
	if err != nil {
		return err
	}

	// Check write permission for receive-pack.
	if service == "git-receive-pack" {
		role, _ := c.Get("user_role").(string)
		if role != "manager" && role != "admin" {
			return echo.NewHTTPError(http.StatusForbidden, "push requires manager or admin role")
		}
	}

	gitCmd := service[4:] // "upload-pack" or "receive-pack"
	cmd := exec.Command("git", gitCmd, "--stateless-rpc", "--advertise-refs", repoPath)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("git %s error: %v", gitCmd, err))
	}

	c.Response().Header().Set("Content-Type", "application/x-"+service+"-advertisement")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().WriteHeader(http.StatusOK)

	header := "# service=" + service + "\n"
	pktLine := fmt.Sprintf("%04x%s", len(header)+4, header)
	c.Response().Write([]byte(pktLine))
	c.Response().Write([]byte("0000"))
	c.Response().Write(out)
	c.Response().Flush()
	return nil
}

// handleGitUploadPack handles POST /git/:uuid/git-upload-pack (fetch/clone).
func (s *Server) handleGitUploadPack(c echo.Context) error {
	return s.handleGitRPC(c, "upload-pack")
}

// handleGitReceivePack handles POST /git/:uuid/git-receive-pack (push).
func (s *Server) handleGitReceivePack(c echo.Context) error {
	return s.handleGitRPC(c, "receive-pack")
}

// handleGitRPC runs the stateless-rpc git command and streams the result.
func (s *Server) handleGitRPC(c echo.Context, service string) error {
	repoPath, err := s.resolveGitOrg(c)
	if err != nil {
		return err
	}

	// Check write permission for push (after resolveGitOrg sets user_role)
	if service == "receive-pack" {
		role, _ := c.Get("user_role").(string)
		if role != "manager" && role != "admin" {
			return echo.NewHTTPError(http.StatusForbidden, "push requires manager or admin role")
		}
	}

	// Acquire write lock for receive-pack to prevent concurrent go-git writes
	if service == "receive-pack" {
		if orgID, ok := c.Get("org_id").(int); ok {
			if st, err := s.storeForOrg(c.Request().Context(), orgID); err == nil {
				st.WriteLock()
				defer func() {
					st.WriteUnlock()
					s.stores.Delete(orgID) // invalidate after push
				}()
			}
		}
	}

	// Snapshot refs before a push so we can reject + revert server-managed ref
	// changes (review/*) and history rewrites (non-fast-forward / deletions).
	var refsBefore map[string]string
	// The default branch before the push, read strictly (see
	// store.DefaultBranchHash); baseKnown is false when it could not be read,
	// which means nothing is archived afterwards.
	var base pushBase
	var baseKnown bool
	if service == "receive-pack" {
		if orgID, ok := c.Get("org_id").(int); ok {
			if st, err := s.storeForOrg(c.Request().Context(), orgID); err == nil {
				refsBefore, _ = st.ListRefHashes()
				if h, empty, hErr := st.DefaultBranchHash(); hErr != nil {
					log.Printf("documents: org %d: not archiving versions after this push: %v", orgID, hErr)
				} else {
					base, baseKnown = pushBase{hash: h, empty: empty}, true
				}
			}
		}
	}

	cmd := exec.Command("git", service, "--stateless-rpc", repoPath)
	cmd.Stdin = c.Request().Body
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("git %s error: %v", service, err))
	}

	// Ref-level protection: server-managed review refs are immutable via push, and
	// other refs are fast-forward-only (no history rewrite, no deletion). On a
	// violation, restore the pre-push ref state and reject.
	if service == "receive-pack" && refsBefore != nil {
		if orgID, ok := c.Get("org_id").(int); ok {
			if st, stErr := s.storeForOrg(c.Request().Context(), orgID); stErr == nil {
				// Read through a store opened now: the long-lived one caches the
				// pack index and may not see what the push (or a gc) wrote.
				fresh, freshErr := openFreshStore(st.Root())
				if freshErr != nil {
					fresh = st
				}
				refsAfter, _ := fresh.ListRefHashes()
				if reason := reviewRefViolation(refsBefore, refsAfter, fresh.IsAncestor); reason != "" {
					restoreRefs(st, refsBefore, refsAfter)
					fmt.Fprintf(os.Stderr, "REJECTED push for org %d: %s\n", orgID, reason)
					return echo.NewHTTPError(http.StatusForbidden, "push rejected: "+reason)
				}
			}
		}
	}

	// Post-receive validation: comprehensive repo protection
	// If validation fails, revert the push.
	if service == "receive-pack" {
		if orgID, ok := c.Get("org_id").(int); ok {
			if st, stErr := s.storeForOrg(c.Request().Context(), orgID); stErr == nil {
				// Everything after the push is read through a store opened now: the
				// long-lived one caches the pack index and may not see a big push.
				fresh, freshErr := openFreshStore(st.Root())
				if freshErr != nil {
					// Do not archive from a store that cannot be refreshed, and
					// never fall back to st: it is locked for this push and
					// archiving would take its lock again.
					log.Printf("documents: org %d: not archiving versions after this push: %v", orgID, freshErr)
					fresh = st
				}
				var rejectReason string
				if err := validateDocumentIDs(fresh); err != nil {
					rejectReason = err.Error()
				}
				if rejectReason == "" {
					if err := validateRepoContents(fresh); err != nil {
						rejectReason = err.Error()
					}
				}
				if rejectReason != "" {
					s.revertPush(st, refsBefore, base, baseKnown)
					fmt.Fprintf(os.Stderr, "REJECTED push for org %d: %s\n", orgID, rejectReason)
					return echo.NewHTTPError(http.StatusForbidden, "push rejected: "+rejectReason)
				}
				// The push passed validation and stays: archive the versions of
				// documents it removed, and of ids it added. Never for a rejected
				// push (reverted above). A delete and re-add of one id in a push is
				// an edit: present before and after.
				if baseKnown && freshErr == nil {
					if aerr := s.archiveAfterPush(c.Request().Context(), orgID, fresh, base); aerr != nil {
						log.Printf("documents: org %d: %v", orgID, aerr)
					}
				}
			}
		}
	}

	c.Response().Header().Set("Content-Type", "application/x-git-"+service+"-result")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Write(out)
	c.Response().Flush()
	return nil
}

// validateDocumentIDs scans all .md files in the repo and checks for duplicate document_ids (case-insensitive).
func validateDocumentIDs(st *store.Store) error {
	seen := map[string]string{} // lowercase id -> file path
	var dupeErr error

	st.WalkDir(st.DocsRoot(), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		pf, loadErr := st.LoadDocument(path)
		if loadErr != nil || pf == nil || pf.Frontmatter.DocumentID == "" {
			return nil
		}
		id := strings.ToLower(pf.Frontmatter.DocumentID)
		if existing, ok := seen[id]; ok {
			dupeErr = fmt.Errorf("duplicate document_id %q in %s and %s", id, existing, path)
			return filepath.SkipAll
		}
		seen[id] = path
		return nil
	})
	return dupeErr
}

const maxFileSizeBytes = 2 * 1024 * 1024 // 2 MB

// validateRepoContents enforces strict repo policy by walking the committed
// git tree (not the filesystem). In bare repos there is no working tree, so
// filesystem walks would never see pushed content.
func validateRepoContents(st *store.Store) error {
	return st.ValidateRepoContents(int64(maxFileSizeBytes))
}

// reviewRefViolation enforces ref-level push policy and returns a rejection
// reason (or "" if the push is allowed):
//   - refs/heads/review/* and refs/reviews/* are server-managed — a push must
//     not create, change, or delete them.
//   - every other ref is fast-forward-only (the old commit must be an ancestor
//     of the new one) and may not be deleted — so a push can't rewrite or erase
//     history (e.g. an approved document's commits or a reviewer's proposal).
//
// isAncestor reports whether old is an ancestor of new (a fast-forward).
func reviewRefViolation(before, after map[string]string, isAncestor func(old, new string) (bool, error)) string {
	serverManaged := func(name string) bool {
		return strings.HasPrefix(name, "refs/heads/review/") || strings.HasPrefix(name, "refs/reviews/")
	}
	// Created or changed refs.
	for name, newHash := range after {
		oldHash, existed := before[name]
		if serverManaged(name) {
			if !existed || oldHash != newHash {
				return "review refs are server-managed and cannot be pushed to: " + name
			}
			continue
		}
		if existed && oldHash != newHash {
			if ok, _ := isAncestor(oldHash, newHash); !ok {
				return "non-fast-forward push (history rewrite) rejected: " + name
			}
		}
	}
	// Deleted refs.
	for name := range before {
		if _, stillThere := after[name]; !stillThere {
			return "deleting refs via push is not allowed: " + name
		}
	}
	return ""
}

// restoreRefs reverts the repo's refs to the pre-push snapshot: restores changed
// and deleted refs to their old hash, and removes any refs the push created.
func restoreRefs(st *store.Store, before, after map[string]string) {
	for name, hash := range before {
		if err := st.SetRefHash(name, hash); err != nil {
			fmt.Fprintf(os.Stderr, "restoreRefs: could not restore %s to %.8s: %v\n", name, hash, err)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			if err := st.DeleteRef(name); err != nil {
				fmt.Fprintf(os.Stderr, "restoreRefs: could not delete new ref %s: %v\n", name, err)
			}
		}
	}
}

// openFreshStore opens the repository again, so what is read after a push is not
// served from the long-lived store's cached pack index. A variable so a test can
// make it fail.
var openFreshStoreFn = store.NewBare
var openFreshStoreMu sync.RWMutex

func openFreshStore(path string) (*store.Store, error) {
	openFreshStoreMu.RLock()
	f := openFreshStoreFn
	openFreshStoreMu.RUnlock()
	return f(path)
}

// setOpenFreshStore replaces the opener and returns a function that restores it
// (tests).
func setOpenFreshStore(f func(string) (*store.Store, error)) func() {
	openFreshStoreMu.Lock()
	old := openFreshStoreFn
	openFreshStoreFn = f
	openFreshStoreMu.Unlock()
	return func() { openFreshStoreMu.Lock(); openFreshStoreFn = old; openFreshStoreMu.Unlock() }
}

// revertPush puts every ref back where it was before a rejected push, the
// default branch included, so a push of several commits leaves nothing behind.
// Without a snapshot it falls back to resetting the branch one commit.
func (s *Server) revertPush(st *store.Store, refsBefore map[string]string, base pushBase, baseKnown bool) {
	if refsBefore == nil {
		if head, err := st.HeadCommit(); err == nil && head.NumParents() > 0 {
			st.ResetToCommit(head.ParentHashes[0].String())
		}
		return
	}
	before := map[string]string{}
	for k, v := range refsBefore {
		before[k] = v
	}
	if baseKnown {
		if ref, err := st.DefaultBranchRef(); err == nil {
			if base.empty {
				delete(before, ref)
			} else {
				before[ref] = base.hash
			}
		}
	}
	after, _ := st.ListRefHashes()
	restoreRefs(st, before, after)
}
