package api

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

type trustSettingsJSON struct {
	Enabled            bool     `json:"enabled"`
	Title              string   `json:"title"`
	OverviewDocumentID *string  `json:"overview_document_id"`
	SecurityContact    string   `json:"security_contact"`
	DefaultAccessLevel string   `json:"default_access_level"`
	UpdatedAt          db.Epoch `json:"updated_at"`
	UpdatedBy          *string  `json:"updated_by"`
}

type trustSettingsRequest struct {
	Enabled            *bool            `json:"enabled"`
	Title              *string          `json:"title"`
	SecurityContact    *string          `json:"security_contact"`
	DefaultAccessLevel *string          `json:"default_access_level"`
	OverviewDocumentID Optional[string] `json:"overview_document_id"`
}

type trustManagerDocument struct {
	DocumentID      string           `json:"document_id"`
	Title           string           `json:"title"`
	Classification  string           `json:"classification"`
	Status          string           `json:"status"`
	DocumentStatus  string           `json:"document_status"`
	DocumentVersion string           `json:"document_version"`
	ServedVersion   *trustServedJSON `json:"served_version"`
}

type trustManagerItem struct {
	ID           string                `json:"id"`
	Kind         string                `json:"kind"`
	Section      string                `json:"section"`
	AccessLevel  string                `json:"access_level"`
	SortOrder    int                   `json:"sort_order"`
	HiddenReason string                `json:"hidden_reason"`
	PublishedAt  int64                 `json:"published_at"`
	Document     *trustManagerDocument `json:"document"`
}

type trustOrderRequest struct {
	Kind string   `json:"kind"`
	IDs  []string `json:"ids"`
}

func trustSettingsResponse(st *db.TrustCenterSettings) trustSettingsJSON {
	return trustSettingsJSON{
		Enabled: st.Enabled, Title: st.Title, OverviewDocumentID: st.OverviewDocumentID,
		SecurityContact: st.SecurityContact, DefaultAccessLevel: st.DefaultAccessLevel,
		UpdatedAt: st.UpdatedAt, UpdatedBy: st.UpdatedBy,
	}
}

// GET /api/v1/trust-center/settings
func (s *Server) handleGetTrustSettings(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	st, err := s.db.GetTrustCenterSettings(c.Request().Context(), getOrgID(c))
	if err != nil {
		return trustInternalError(c, "reading settings", err)
	}
	return c.JSON(http.StatusOK, trustSettingsResponse(st))
}

// PUT /api/v1/trust-center/settings
func (s *Server) handlePutTrustSettings(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	var req trustSettingsRequest
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	patch := db.TrustCenterSettingsPatch{
		Enabled: req.Enabled, Title: req.Title, SecurityContact: req.SecurityContact,
		DefaultAccessLevel: req.DefaultAccessLevel, SetOverview: req.OverviewDocumentID.Set,
	}
	if req.OverviewDocumentID.Set && req.OverviewDocumentID.Value != nil {
		if id := strings.ToLower(strings.TrimSpace(*req.OverviewDocumentID.Value)); id != "" {
			if err := s.checkTrustOverviewDocument(c, orgID, id); err != nil {
				return err
			}
			patch.OverviewDocumentID = &id
		}
	}
	st, err := s.db.UpdateTrustCenterSettings(ctx, orgID, patch, getUserEmail(c))
	if errors.Is(err, db.ErrTrustCenterInvalid) {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if err != nil {
		return trustInternalError(c, "updating settings", err)
	}
	return c.JSON(http.StatusOK, trustSettingsResponse(st))
}

// checkTrustOverviewDocument: the overview document must exist and not be
// retired. One with no approved version is allowed; the public page leaves it
// out until there is one.
func (s *Server) checkTrustOverviewDocument(c echo.Context, orgID int, docID string) error {
	ctx := c.Request().Context()
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return trustInternalError(c, "opening repository", err)
	}
	if st.FindDocumentByID(docID) == "" {
		return errNotFound("document")
	}
	head, err := s.trustDocumentHead(ctx, st, orgID, docID)
	if err != nil {
		return trustInternalError(c, "reading document", err)
	}
	if head.Deleted {
		return errNotFound("document")
	}
	if head.Status == "retired" {
		return apiError(http.StatusConflict, CodeTrustDocumentRetired)
	}
	return nil
}

// trustManagerItems builds the manager list: every item of the organization,
// hidden ones included, in sort_order, id order. An item whose document is
// retired or deleted at HEAD is hidden first, as trustStateFor does. A document
// that cannot be read makes that row status "error"; it never fails the list.
func (s *Server) trustManagerItems(c echo.Context) ([]trustManagerItem, error) {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	rows, err := s.db.ListPublishedTrustCenterItems(ctx, orgID)
	if err != nil {
		return nil, trustInternalError(c, "listing items", err)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SortOrder != rows[j].SortOrder {
			return rows[i].SortOrder < rows[j].SortOrder
		}
		return rows[i].ID < rows[j].ID
	})
	out := make([]trustManagerItem, 0, len(rows))
	var st *store.Store
	for i := range rows {
		row, err := s.trustManagerRow(c, orgID, &st, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, nil
}

// trustManagerRow builds one manager row. st is opened on first use and shared
// across rows of one request.
func (s *Server) trustManagerRow(c echo.Context, orgID int, st **store.Store, it *db.TrustCenterItem) (*trustManagerItem, error) {
	row := &trustManagerItem{
		ID: it.PublicID, Kind: it.Kind, Section: it.Section, AccessLevel: it.AccessLevel,
		SortOrder: it.SortOrder, HiddenReason: it.HiddenReason, PublishedAt: it.PublishedAt.Unix(),
	}
	if it.Kind == db.TrustKindDocument {
		if *st == nil {
			opened, err := s.storeForOrg(c.Request().Context(), orgID)
			if err != nil {
				return nil, trustInternalError(c, "opening repository", err)
			}
			*st = opened
		}
		row.Document = s.trustManagerDocumentFor(c, *st, orgID, it, row)
	}
	return row, nil
}

func (s *Server) trustManagerDocumentFor(c echo.Context, st *store.Store, orgID int, it *db.TrustCenterItem, row *trustManagerItem) *trustManagerDocument {
	ctx := c.Request().Context()
	doc := &trustManagerDocument{DocumentID: it.Reference, Title: it.Reference}
	failed := false
	head, err := s.trustDocumentHead(ctx, st, orgID, it.Reference)
	if err != nil {
		log.Printf("trust center: org %d item %d document %s: %v", orgID, it.ID, it.Reference, err)
		failed = true
	} else {
		if reason := trustHideReason(head); reason != "" && it.HiddenAt == nil {
			s.hideTrustItem(ctx, orgID, it.ID, reason, getUserEmail(c))
			row.HiddenReason = reason
			it.HiddenAt = &db.Epoch{}
		}
		if !head.Deleted {
			doc.Title, doc.Classification = head.Title, head.Classification
			doc.DocumentStatus, doc.DocumentVersion = head.Status, head.Version
		}
	}
	served, err := s.trustServed(ctx, st, orgID, it.Reference)
	if err != nil {
		log.Printf("trust center: org %d item %d document %s: %v", orgID, it.ID, it.Reference, err)
		failed = true
	}
	if served != nil {
		doc.ServedVersion = &trustServedJSON{Version: served.Version, ApprovedAt: served.ApprovedAt.Unix(), ContentHash: served.ContentHash}
		if failed || head == nil || head.Deleted {
			doc.Title = served.Title
		}
	}
	switch {
	case it.HiddenAt != nil:
		doc.Status = "hidden"
	case failed:
		doc.Status = "error"
	case served == nil:
		doc.Status = "cannot_publish"
	case head.Status != "approved" || head.Version != served.Version:
		doc.Status = "draft_in_progress"
	default:
		doc.Status = "published"
	}
	return doc
}

// GET /api/v1/trust-center/items
func (s *Server) handleListTrustItems(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	items, err := s.trustManagerItems(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// PUT /api/v1/trust-center/items/order answers 200 with the new list, so the
// client needs no second request.
func (s *Server) handleReorderTrustItems(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	var req trustOrderRequest
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	err := s.db.ReorderTrustCenterItems(c.Request().Context(), getOrgID(c), req.Kind, req.IDs)
	switch {
	case errors.Is(err, db.ErrTrustCenterInvalid):
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	case errors.Is(err, db.ErrTrustOrderStale):
		return apiError(http.StatusConflict, CodeTrustOrderStale)
	case err != nil:
		return trustInternalError(c, "reordering items", err)
	}
	return s.handleListTrustItems(c)
}

type trustItemPatchRequest struct {
	AccessLevel           string `json:"access_level"`
	ConfirmClassification bool   `json:"confirm_classification"`
}

// PATCH /api/v1/trust-center/items/:id changes an item's access level and
// answers with its manager row. A hidden item keeps its hidden state. Making a
// confidential or restricted document public needs confirm_classification.
func (s *Server) handlePatchTrustItem(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	var req trustItemPatchRequest
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	it, err := s.db.GetTrustCenterItem(ctx, orgID, c.Param("id"))
	if errors.Is(err, db.ErrTrustItemNotFound) {
		return errNotFound("item")
	}
	if err != nil {
		return trustInternalError(c, "reading trust center item", err)
	}
	switch req.AccessLevel {
	case db.TrustAccessPublic, db.TrustAccessGated, db.TrustAccessRestricted:
	default:
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	var st *store.Store
	if it.Kind == db.TrustKindDocument && req.AccessLevel == db.TrustAccessPublic &&
		it.AccessLevel != db.TrustAccessPublic && !req.ConfirmClassification {
		if st, err = s.storeForOrg(ctx, orgID); err != nil {
			return trustInternalError(c, "opening repository", err)
		}
		head, err := s.trustDocumentHead(ctx, st, orgID, it.Reference)
		if err != nil {
			return trustInternalError(c, "reading document", err)
		}
		served, err := s.trustServed(ctx, st, orgID, it.Reference)
		if err != nil {
			return trustInternalError(c, "resolving served version", err)
		}
		classes := []string{head.Classification}
		if served != nil {
			classes = append(classes, served.Classification)
		}
		if cls := sensitiveTrustClassification(classes...); cls != "" {
			return apiError(http.StatusConflict, CodeTrustClassificationConfirm, Value(cls))
		}
	}
	if err := s.db.SetTrustCenterItemAccessLevel(ctx, orgID, it.ID, req.AccessLevel, getUserEmail(c)); err != nil {
		if errors.Is(err, db.ErrTrustItemNotFound) {
			return errNotFound("item")
		}
		return trustInternalError(c, "changing access level", err)
	}
	if it, err = s.db.GetTrustCenterItem(ctx, orgID, it.PublicID); err != nil {
		return trustInternalError(c, "reading trust center item", err)
	}
	row, err := s.trustManagerRow(c, orgID, &st, it)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, row)
}
