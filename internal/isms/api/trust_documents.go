package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// Publishing a document to the trust center never publishes git HEAD, which may
// hold an unapproved draft. Visitors get the "served version": the newest
// document_versions row whose file, read at that row's own commit, says
// `status: approved`. Merge always records such a row. The annual confirm
// endpoint records a row for whatever HEAD is, draft included, so "newest row"
// alone could serve unapproved text; the status check on the committed file is
// what tells the two apart.

// trustServedVersion is the content visitors may see for a document.
type trustServedVersion struct {
	Version     string
	ApprovedAt  db.Epoch
	ContentHash string
	CommitHash  string
	FilePath    string
	Title       string
	Body        string // markdown, frontmatter stripped
}

// trustDocHead is a document as it is at HEAD.
type trustDocHead struct {
	Deleted bool
	Status  string
	Version string
}

// trustDocumentHead reports the document's state at HEAD. Deleted is set only
// on a definitive not-found: HEAD reads fine and the document id resolves to
// nothing, and the file the newest recorded version points at is gone from HEAD
// (or no longer carries this id). Anything else that goes wrong is an error,
// never a "deleted", because deleted hides the item.
func (s *Server) trustDocumentHead(ctx context.Context, st *store.Store, orgID int, docID string) (*trustDocHead, error) {
	abs := st.FindDocumentByID(docID)
	if abs == "" {
		head, err := st.HeadHash()
		if err != nil {
			return nil, fmt.Errorf("reading HEAD: %w", err)
		}
		versions, err := s.db.ListVersions(ctx, orgID, docID)
		if err != nil {
			return nil, err
		}
		for _, v := range versions {
			raw, err := st.ReadFileAtRef(head, v.FilePath)
			if err != nil {
				continue // the file is not at HEAD
			}
			if doc, err := store.ParseDocument(v.FilePath, raw); err != nil || doc.Frontmatter.DocumentID == docID {
				return nil, fmt.Errorf("document %s is missing from the document index but present at %s", docID, v.FilePath)
			}
		}
		return &trustDocHead{Deleted: true}, nil
	}
	doc, err := st.LoadDocument(abs)
	if err != nil {
		return nil, fmt.Errorf("loading document %s: %w", docID, err)
	}
	return &trustDocHead{Status: doc.Frontmatter.Status, Version: doc.Frontmatter.Version}, nil
}

// trustServed resolves the served version of a document, or nil when no
// approved version exists.
func (s *Server) trustServed(ctx context.Context, st *store.Store, orgID int, docID string) (*trustServedVersion, error) {
	versions, err := s.db.ListVersions(ctx, orgID, docID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(versions, func(i, j int) bool {
		if !versions[i].CreatedAt.Equal(versions[j].CreatedAt.Time) {
			return versions[i].CreatedAt.After(versions[j].CreatedAt.Time)
		}
		return versions[i].ID > versions[j].ID
	})
	for _, v := range versions {
		raw, err := st.ReadFileAtRef(v.CommitHash, v.FilePath)
		if err != nil {
			return nil, fmt.Errorf("version %s of %s: %w", v.Version, docID, err)
		}
		doc, err := store.ParseDocument(v.FilePath, raw)
		if err != nil {
			return nil, fmt.Errorf("version %s of %s: %w", v.Version, docID, err)
		}
		if doc.Frontmatter.Status != "approved" {
			continue
		}
		return &trustServedVersion{
			Version: v.Version, ApprovedAt: v.CreatedAt, ContentHash: v.ContentHash,
			CommitHash: v.CommitHash, FilePath: v.FilePath,
			Title: doc.Frontmatter.Title, Body: doc.Body,
		}, nil
	}
	return nil, nil
}

// trustDocInfo is a published document item that visitors can be served.
type trustDocInfo struct {
	Item   db.TrustCenterItem
	Served *trustServedVersion
}

// trustServableDocuments checks every visible document item of the
// organization against the repository and returns the servable ones keyed by
// document id. An item whose document is retired or deleted at HEAD is hidden
// (D6) and left out; one with no approved version is left out. Computed once
// per request.
func (s *Server) trustServableDocuments(ctx context.Context, orgID int, items []db.TrustCenterItem) (map[string]*trustDocInfo, error) {
	out := map[string]*trustDocInfo{}
	var st *store.Store
	for _, it := range items {
		if it.Kind != db.TrustKindDocument || it.HiddenAt != nil {
			continue
		}
		if st == nil {
			var err error
			if st, err = s.storeForOrg(ctx, orgID); err != nil {
				return nil, err
			}
		}
		head, err := s.trustDocumentHead(ctx, st, orgID, it.Reference)
		if err != nil {
			return nil, err
		}
		if reason := trustHideReason(head); reason != "" {
			s.hideTrustItem(ctx, orgID, it.ID, reason, "system")
			continue
		}
		served, err := s.trustServed(ctx, st, orgID, it.Reference)
		if err != nil {
			return nil, err
		}
		if served != nil {
			out[it.Reference] = &trustDocInfo{Item: it, Served: served}
		}
	}
	return out, nil
}

func trustHideReason(h *trustDocHead) string {
	switch {
	case h.Deleted:
		return db.TrustHiddenDeleted
	case h.Status == "retired":
		return db.TrustHiddenRetired
	}
	return ""
}

// hideTrustItem hides an item; a failure is logged, never returned, because
// every caller either is answering a visitor or must not fail a user's save.
func (s *Server) hideTrustItem(ctx context.Context, orgID int, itemID int64, reason, actor string) {
	if err := s.db.HideTrustCenterItem(ctx, orgID, itemID, reason, actor); err != nil && !errors.Is(err, db.ErrTrustItemNotFound) {
		log.Printf("trust center: hiding item %d (%s): %v", itemID, reason, err)
	}
}

// hideTrustItemForDocument hides the document's trust item, if it has one. Used
// when the document is retired or deleted through the API, so the item is
// hidden at once instead of at the next read.
func (s *Server) hideTrustItemForDocument(ctx context.Context, orgID int, docID, reason, actor string) {
	it, err := s.db.GetTrustCenterItemByReference(ctx, orgID, db.TrustKindDocument, strings.ToLower(docID))
	if errors.Is(err, db.ErrTrustItemNotFound) {
		return
	}
	if err != nil {
		log.Printf("trust center: looking up item for document %s: %v", docID, err)
		return
	}
	s.hideTrustItem(ctx, orgID, it.ID, reason, actor)
}

// ---- manager endpoints -----------------------------------------------------

type trustServedJSON struct {
	Version     string `json:"version"`
	ApprovedAt  int64  `json:"approved_at"` // epoch seconds
	ContentHash string `json:"content_hash"`
}

// trustDocumentState is the response of the manager endpoints.
type trustDocumentState struct {
	Published           bool             `json:"published"`
	ID                  string           `json:"id,omitempty"`
	AccessLevel         string           `json:"access_level,omitempty"`
	HiddenReason        string           `json:"hidden_reason"`
	ServedVersion       *trustServedJSON `json:"served_version"`
	DocumentStatus      string           `json:"document_status"`
	DocumentVersion     string           `json:"document_version"`
	CanPublish          bool             `json:"can_publish"`
	CannotPublishReason string           `json:"cannot_publish_reason"`
}

func trustInternalError(c echo.Context, what string, err error) error {
	log.Printf("trust center: %s %s: %v", c.Request().Method, c.Request().URL.Path, fmt.Errorf("%s: %w", what, err))
	return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
}

// trustDocumentContext is what the three manager handlers share.
type trustDocumentContext struct {
	orgID int
	docID string
	st    *store.Store
	head  *trustDocHead
}

// loadTrustDocument resolves the document in the URL. A document that is not in
// the repository is the usual 404.
func (s *Server) loadTrustDocument(c echo.Context) (*trustDocumentContext, error) {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	docID := strings.ToLower(c.Param("docId"))
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return nil, trustInternalError(c, "opening repository", err)
	}
	if st.FindDocumentByID(docID) == "" {
		return nil, errNotFound("document")
	}
	head, err := s.trustDocumentHead(ctx, st, orgID, docID)
	if err != nil {
		return nil, trustInternalError(c, "reading document", err)
	}
	return &trustDocumentContext{orgID: orgID, docID: docID, st: st, head: head}, nil
}

// trustStateFor builds the state response, hiding the item first when its
// document is retired or deleted.
func (s *Server) trustStateFor(c echo.Context, d *trustDocumentContext) (*trustDocumentState, error) {
	ctx := c.Request().Context()
	item, err := s.db.GetTrustCenterItemByReference(ctx, d.orgID, db.TrustKindDocument, d.docID)
	if err != nil && !errors.Is(err, db.ErrTrustItemNotFound) {
		return nil, trustInternalError(c, "reading trust center item", err)
	}
	if item != nil && item.HiddenAt == nil {
		if reason := trustHideReason(d.head); reason != "" {
			s.hideTrustItem(ctx, d.orgID, item.ID, reason, getUserEmail(c))
			if item, err = s.db.GetTrustCenterItemByReference(ctx, d.orgID, db.TrustKindDocument, d.docID); err != nil {
				return nil, trustInternalError(c, "reading trust center item", err)
			}
		}
	}
	served, err := s.trustServed(ctx, d.st, d.orgID, d.docID)
	if err != nil {
		return nil, trustInternalError(c, "resolving served version", err)
	}
	out := &trustDocumentState{
		DocumentStatus: d.head.Status, DocumentVersion: d.head.Version,
	}
	if item != nil {
		out.ID = item.PublicID
		out.AccessLevel = item.AccessLevel
		out.HiddenReason = item.HiddenReason
		out.Published = item.HiddenAt == nil
	}
	if served != nil {
		out.ServedVersion = &trustServedJSON{Version: served.Version, ApprovedAt: served.ApprovedAt.Unix(), ContentHash: served.ContentHash}
	}
	switch {
	case d.head.Status == "retired":
		out.CannotPublishReason = "retired"
	case served == nil:
		out.CannotPublishReason = "no_approved_version"
	default:
		out.CanPublish = true
	}
	return out, nil
}

// GET /api/v1/documents/:docId/trust-center
func (s *Server) handleGetDocumentTrust(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	d, err := s.loadTrustDocument(c)
	if err != nil {
		return err
	}
	state, err := s.trustStateFor(c, d)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, state)
}

// POST /api/v1/documents/:docId/trust-center
func (s *Server) handlePublishDocumentTrust(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	var req struct {
		AccessLevel string `json:"access_level"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	d, err := s.loadTrustDocument(c)
	if err != nil {
		return err
	}
	level := req.AccessLevel
	if level == "" {
		settings, err := s.db.GetTrustCenterSettings(ctx, d.orgID)
		if err != nil {
			return trustInternalError(c, "reading settings", err)
		}
		level = settings.DefaultAccessLevel
	}
	switch level {
	case db.TrustAccessPublic, db.TrustAccessGated, db.TrustAccessRestricted:
	default:
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if d.head.Status == "retired" {
		return apiError(http.StatusConflict, CodeTrustDocumentRetired)
	}
	served, err := s.trustServed(ctx, d.st, d.orgID, d.docID)
	if err != nil {
		return trustInternalError(c, "resolving served version", err)
	}
	if served == nil {
		return apiError(http.StatusConflict, CodeTrustNoApprovedVersion)
	}

	actor := getUserEmail(c)
	status := http.StatusCreated
	item, err := s.db.GetTrustCenterItemByReference(ctx, d.orgID, db.TrustKindDocument, d.docID)
	switch {
	case errors.Is(err, db.ErrTrustItemNotFound):
		item = &db.TrustCenterItem{Kind: db.TrustKindDocument, Reference: d.docID, AccessLevel: level}
		err = s.db.PublishTrustCenterItem(ctx, d.orgID, item, actor)
	case err != nil:
	case item.HiddenAt == nil:
		return apiError(http.StatusConflict, CodeTrustAlreadyPublished)
	default:
		status = http.StatusOK
		err = s.db.RepublishTrustCenterItem(ctx, d.orgID, item.ID, level, actor)
	}
	if errors.Is(err, db.ErrTrustItemAlreadyPublished) {
		return apiError(http.StatusConflict, CodeTrustAlreadyPublished)
	}
	if err != nil {
		return trustInternalError(c, "publishing", err)
	}
	state, err := s.trustStateFor(c, d)
	if err != nil {
		return err
	}
	return c.JSON(status, state)
}

// DELETE /api/v1/documents/:docId/trust-center
func (s *Server) handleUnpublishDocumentTrust(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	item, err := s.db.GetTrustCenterItemByReference(ctx, orgID, db.TrustKindDocument, strings.ToLower(c.Param("docId")))
	if errors.Is(err, db.ErrTrustItemNotFound) {
		return errNotFound("item")
	}
	if err != nil {
		return trustInternalError(c, "reading trust center item", err)
	}
	if err := s.db.UnpublishTrustCenterItem(ctx, orgID, item.ID, getUserEmail(c)); err != nil {
		if errors.Is(err, db.ErrTrustItemNotFound) {
			return errNotFound("item")
		}
		return trustInternalError(c, "unpublishing", err)
	}
	return c.NoContent(http.StatusNoContent)
}
