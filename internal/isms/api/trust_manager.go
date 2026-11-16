package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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
	// BlockedEmailDomains is the list in effect, never null: the built-in list
	// when BlockedEmailDomainsDefault, else the organization's own (possibly empty).
	BlockedEmailDomains        []string `json:"blocked_email_domains"`
	BlockedEmailDomainsDefault bool     `json:"blocked_email_domains_default"`
	// DefaultBlockedEmailDomains is the built-in list, for "Restore defaults".
	DefaultBlockedEmailDomains []string `json:"default_blocked_email_domains"`
	PrivacyNoticeURL           string   `json:"privacy_notice_url"`
	// SignInPausedUntil is set while the organization's hourly sign-in link cap
	// (a backstop) is reached: when the first slot frees up. Null otherwise.
	SignInPausedUntil *db.Epoch `json:"sign_in_paused_until"`
	// OverviewWithheld is true when an overview document is set but is also
	// published at a gated or restricted level, so the page shows no overview.
	OverviewWithheld bool `json:"overview_withheld"`
	// EmailConfigured is false when the server cannot send mail, in which case
	// visitors cannot sign in.
	EmailConfigured bool `json:"email_configured"`
}

type trustSettingsRequest struct {
	Enabled            *bool            `json:"enabled"`
	Title              *string          `json:"title"`
	SecurityContact    *string          `json:"security_contact"`
	DefaultAccessLevel *string          `json:"default_access_level"`
	OverviewDocumentID Optional[string] `json:"overview_document_id"`
	// BlockedEmailDomains: absent leaves it alone, null restores the built-in
	// list, a list (possibly empty) is the organization's own.
	BlockedEmailDomains Optional[[]string] `json:"blocked_email_domains"`
	PrivacyNoticeURL    *string            `json:"privacy_notice_url"`
}

type trustManagerDocument struct {
	DocumentID     string `json:"document_id"`
	Title          string `json:"title"`
	Classification string `json:"classification"`
	// ClassificationSensitive: the classification at HEAD or in the served
	// version is confidential or restricted (the guard's rule).
	ClassificationSensitive bool             `json:"classification_sensitive"`
	Status                  string           `json:"status"`
	DocumentStatus          string           `json:"document_status"`
	DocumentVersion         string           `json:"document_version"`
	ServedVersion           *trustServedJSON `json:"served_version"`
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
	// Certification is set for kind certification; null when the certification
	// row is missing, which the list tolerates.
	Certification *trustManagerCertification `json:"certification"`
	// Subprocessor is set for kind subprocessor (#522); null for other kinds
	// and when the supplier row is gone.
	Subprocessor *trustManagerSubprocessor `json:"subprocessor"`
}

type trustOrderRequest struct {
	Kind string   `json:"kind"`
	IDs  []string `json:"ids"`
}

func (s *Server) trustSettingsResponse(ctx context.Context, st *db.TrustCenterSettings) (trustSettingsJSON, error) {
	out := trustSettingsJSON{
		BlockedEmailDomains: st.EffectiveBlockedEmailDomains(), BlockedEmailDomainsDefault: st.BlockedEmailDomains == nil,
		DefaultBlockedEmailDomains: db.TrustBuiltinBlockedEmailDomains(), PrivacyNoticeURL: st.PrivacyNoticeURL,
		EmailConfigured: s.mailer != nil,
		Enabled:         st.Enabled, Title: st.Title, OverviewDocumentID: st.OverviewDocumentID,
		SecurityContact: st.SecurityContact, DefaultAccessLevel: st.DefaultAccessLevel,
		UpdatedAt: st.UpdatedAt, UpdatedBy: st.UpdatedBy,
	}
	until, err := s.db.TrustVisitorLinkPausedUntil(ctx, st.OrganizationID, trustSignInOrgHourly(), time.Hour)
	if err != nil {
		return out, err
	}
	if until != nil {
		out.SignInPausedUntil = &db.Epoch{Time: *until}
	}
	if st.OverviewDocumentID != nil && *st.OverviewDocumentID != "" {
		items, err := s.db.ListPublishedTrustCenterItems(ctx, st.OrganizationID)
		if err != nil {
			return out, err
		}
		out.OverviewWithheld = trustOverviewWithheld(items, *st.OverviewDocumentID)
	}
	return out, nil
}

// GET /api/v1/trust-center/settings
func (s *Server) handleGetTrustSettings(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	st, err := s.db.GetTrustCenterSettings(ctx, getOrgID(c))
	if err != nil {
		return trustInternalError(c, "reading settings", err)
	}
	resp, err := s.trustSettingsResponse(ctx, st)
	if err != nil {
		return trustInternalError(c, "reading settings", err)
	}
	return c.JSON(http.StatusOK, resp)
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
	if req.BlockedEmailDomains.Set {
		patch.SetBlockedEmailDomains = true
		if req.BlockedEmailDomains.Value != nil {
			norm, err := db.NormalizeBlockedEmailDomains(*req.BlockedEmailDomains.Value)
			if err != nil {
				return apiError(http.StatusBadRequest, CodeInvalidRequest)
			}
			patch.BlockedEmailDomains = norm
		}
	}
	patch.PrivacyNoticeURL = req.PrivacyNoticeURL
	if req.OverviewDocumentID.Set && req.OverviewDocumentID.Value != nil {
		if id := strings.ToLower(strings.TrimSpace(*req.OverviewDocumentID.Value)); id != "" {
			if err := s.checkTrustOverviewDocument(c, orgID, id); err != nil {
				return err
			}
			// A document published at a level that is not public would leak
			// through the overview, which is a public element.
			items, err := s.db.ListPublishedTrustCenterItems(ctx, orgID)
			if err != nil {
				return trustInternalError(c, "reading items", err)
			}
			if trustOverviewWithheld(items, id) {
				return apiError(http.StatusConflict, CodeTrustOverviewNotPublic)
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
	resp, err := s.trustSettingsResponse(ctx, st)
	if err != nil {
		return trustInternalError(c, "reading settings", err)
	}
	return c.JSON(http.StatusOK, resp)
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
	if it.Kind == db.TrustKindCertification {
		if id, err := strconv.ParseInt(it.Reference, 10, 64); err == nil {
			cert, err := s.db.GetTrustCertification(c.Request().Context(), orgID, id)
			switch {
			case err == nil:
				row.Certification = trustManagerCertificationFor(cert)
			case errors.Is(err, db.ErrTrustCertificationNotFound):
			default:
				return nil, trustInternalError(c, "reading certification", err)
			}
		}
	}
	if it.Kind == db.TrustKindSubprocessor {
		if err := s.trustManagerSubprocessorRow(c, orgID, it, row); err != nil {
			return nil, err
		}
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
	servedClass := ""
	if served != nil {
		servedClass = served.Classification
	}
	doc.ClassificationSensitive = sensitiveTrustClassification(doc.Classification, servedClass) != ""
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
	// Subprocessors are always public (#522).
	if it.Kind == db.TrustKindSubprocessor && req.AccessLevel != db.TrustAccessPublic {
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
