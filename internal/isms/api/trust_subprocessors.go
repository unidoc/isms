package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// Trust center subprocessors (#522): the Suppliers register's data processors,
// published one by one, always at level public.

// trustPublicSubprocessorItem is a subprocessor row of GET /api/v1/trust/:slug/items.
// It is its own struct, never built from a supplier record, so a field added to
// suppliers later cannot reach visitors: the supplier id, identifier, notes,
// contact and contract reference stay out. All four texts are always present.
type trustPublicSubprocessorItem struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Section        string `json:"section"`
	SortOrder      int    `json:"sort_order"`
	AccessLevel    string `json:"access_level"`
	Name           string `json:"name"`
	Purpose        string `json:"purpose"`
	Location       string `json:"location"`
	DataCategories string `json:"data_categories"`
}

// trustPublicName is the name visitors see: the public name, else the supplier's.
func trustPublicName(t *db.TrustSubprocessor) string {
	if n := strings.TrimSpace(t.PublicName); n != "" {
		return n
	}
	return strings.TrimSpace(t.Name)
}

// hideSubprocessorItem hides a visible item whose supplier is no longer
// eligible. A failure is logged, never returned: the caller skips the row
// either way, so a failed hide cannot leak it.
func (s *Server) hideSubprocessorItem(ctx context.Context, orgID int, t *db.TrustSubprocessor) string {
	reason := t.IneligibleReason()
	if t.Item == nil || t.Item.HiddenAt != nil || reason == "" {
		return ""
	}
	s.hideTrustItem(ctx, orgID, t.Item.ID, reason, "system")
	return reason
}

// trustServableSubprocessors returns the subprocessors a visitor may be shown,
// by item id: item visible, supplier present, not deleted, active and with data
// access. A visible item whose supplier is not eligible (a writer bypassed the
// hook) is hidden here, as the document rows are.
func (s *Server) trustServableSubprocessors(ctx context.Context, orgID int) (map[int64]*db.TrustSubprocessor, error) {
	list, err := s.db.ListTrustSubprocessors(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := map[int64]*db.TrustSubprocessor{}
	for i := range list {
		t := &list[i]
		if t.Item == nil || t.Item.HiddenAt != nil {
			continue
		}
		if s.hideSubprocessorItem(ctx, orgID, t) != "" {
			continue
		}
		out[t.Item.ID] = t
	}
	return out, nil
}

// ---- manager ---------------------------------------------------------------

type trustSubprocessorItemJSON struct {
	ID           string `json:"id"`
	HiddenReason string `json:"hidden_reason"`
	SortOrder    int    `json:"sort_order"`
	PublishedAt  int64  `json:"published_at"`
}

// trustSubprocessorJSON is one supplier in GET /trust-center/subprocessors.
type trustSubprocessorJSON struct {
	SupplierID         int64                      `json:"supplier_id"`
	Identifier         string                     `json:"identifier"`
	Name               string                     `json:"name"`
	PublicName         string                     `json:"public_name"`
	PublicPurpose      string                     `json:"public_purpose"`
	ProcessingLocation string                     `json:"processing_location"`
	DataCategories     string                     `json:"data_categories"`
	Status             string                     `json:"status"`
	DataAccess         bool                       `json:"data_access"`
	Deleted            bool                       `json:"deleted"`
	Eligible           bool                       `json:"eligible"`
	IneligibleReason   string                     `json:"ineligible_reason"`
	Item               *trustSubprocessorItemJSON `json:"item"`
}

// trustManagerSubprocessor is the `subprocessor` object of a manager item row.
type trustManagerSubprocessor struct {
	SupplierID     int64  `json:"supplier_id"`
	Identifier     string `json:"identifier"`
	Name           string `json:"name"`
	PublicName     string `json:"public_name"`
	Purpose        string `json:"purpose"`
	Location       string `json:"location"`
	DataCategories string `json:"data_categories"`
	Status         string `json:"status"`
	DataAccess     bool   `json:"data_access"`
	Deleted        bool   `json:"deleted"`
	// DetailsMissing: purpose, location and data categories are all empty.
	DetailsMissing bool `json:"details_missing"`
}

func trustManagerSubprocessorFor(t *db.TrustSubprocessor) *trustManagerSubprocessor {
	return &trustManagerSubprocessor{
		SupplierID: t.SupplierID, Identifier: t.Identifier, Name: t.Name, PublicName: trustPublicName(t),
		Purpose: t.PublicPurpose, Location: t.ProcessingLocation, DataCategories: t.DataCategories,
		Status: t.Status, DataAccess: t.DataAccess, Deleted: t.Deleted,
		DetailsMissing: t.PublicPurpose == "" && t.ProcessingLocation == "" && t.DataCategories == "",
	}
}

func (s *Server) trustSubprocessorJSONFor(ctx context.Context, orgID int, t *db.TrustSubprocessor) trustSubprocessorJSON {
	reason := t.IneligibleReason()
	out := trustSubprocessorJSON{
		SupplierID: t.SupplierID, Identifier: t.Identifier, Name: t.Name, PublicName: t.PublicName,
		PublicPurpose: t.PublicPurpose, ProcessingLocation: t.ProcessingLocation, DataCategories: t.DataCategories,
		Status: t.Status, DataAccess: t.DataAccess, Deleted: t.Deleted,
		Eligible: reason == "", IneligibleReason: reason,
	}
	if t.Item != nil {
		hidden := t.Item.HiddenReason
		if hr := s.hideSubprocessorItem(ctx, orgID, t); hr != "" {
			hidden = hr
		}
		out.Item = &trustSubprocessorItemJSON{
			ID: t.Item.PublicID, HiddenReason: hidden, SortOrder: t.Item.SortOrder,
			PublishedAt: t.Item.PublishedAt.Unix(),
		}
	}
	return out
}

func trustSubprocessorError(c echo.Context, what string, err error) error {
	var inel *db.TrustSubprocessorIneligibleError
	switch {
	case errors.As(err, &inel):
		return apiError(http.StatusConflict, CodeTrustSubprocessorIneligible, Value(inel.Reason))
	case errors.Is(err, db.ErrTrustSubprocessorNotFound):
		return errNotFound("supplier")
	case errors.Is(err, db.ErrTrustItemAlreadyPublished):
		return apiError(http.StatusConflict, CodeTrustSubprocessorAlreadyPublished)
	case errors.Is(err, db.ErrTrustItemNotFound):
		return errNotFound("item")
	case errors.Is(err, db.ErrTrustCenterInvalid):
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	return trustInternalError(c, what, err)
}

// GET /api/v1/trust-center/subprocessors
func (s *Server) handleListTrustSubprocessors(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	list, err := s.db.ListTrustSubprocessors(ctx, orgID)
	if err != nil {
		return trustInternalError(c, "listing subprocessors", err)
	}
	changed, err := s.db.TrustSubprocessorsChangedAt(ctx, orgID)
	if err != nil {
		return trustInternalError(c, "reading last change", err)
	}
	out := struct {
		ChangedAt *int64                  `json:"changed_at"`
		Suppliers []trustSubprocessorJSON `json:"suppliers"`
	}{Suppliers: make([]trustSubprocessorJSON, 0, len(list))}
	if changed != nil {
		n := changed.Unix()
		out.ChangedAt = &n
	}
	for i := range list {
		out.Suppliers = append(out.Suppliers, s.trustSubprocessorJSONFor(ctx, orgID, &list[i]))
	}
	return c.JSON(http.StatusOK, out)
}

type trustSubprocessorPublishRequest struct {
	SupplierID int64 `json:"supplier_id"`
	// AccessLevel is accepted only as "public": subprocessors are always public.
	AccessLevel string `json:"access_level"`
}

// POST /api/v1/trust-center/subprocessors publishes a supplier and answers 201
// with its list entry.
func (s *Server) handlePublishTrustSubprocessor(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	var req trustSubprocessorPublishRequest
	if err := bindTrustJSON(c, &req); err != nil {
		return err
	}
	if req.SupplierID <= 0 || (req.AccessLevel != "" && req.AccessLevel != db.TrustAccessPublic) {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if _, err := s.db.PublishTrustSubprocessor(ctx, orgID, req.SupplierID, getUserEmail(c)); err != nil {
		return trustSubprocessorError(c, "publishing subprocessor", err)
	}
	t, err := s.db.GetTrustSubprocessor(ctx, orgID, req.SupplierID)
	if err != nil {
		return trustSubprocessorError(c, "reading subprocessor", err)
	}
	return c.JSON(http.StatusCreated, s.trustSubprocessorJSONFor(ctx, orgID, t))
}

// DELETE /api/v1/trust-center/subprocessors/:supplierId unpublishes.
func (s *Server) handleUnpublishTrustSubprocessor(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Param("supplierId"), 10, 64)
	if err != nil || id <= 0 {
		return errInvalidEntityID("supplier")
	}
	if err := s.db.UnpublishTrustSubprocessor(c.Request().Context(), getOrgID(c), id, getUserEmail(c)); err != nil {
		return trustSubprocessorError(c, "unpublishing subprocessor", err)
	}
	return c.NoContent(http.StatusNoContent)
}

// trustManagerSubprocessorRow fills the manager row of a subprocessor item and
// applies the read-time hide.
func (s *Server) trustManagerSubprocessorRow(c echo.Context, orgID int, it *db.TrustCenterItem, row *trustManagerItem) error {
	id, err := strconv.ParseInt(it.Reference, 10, 64)
	if err != nil {
		return nil
	}
	t, err := s.db.GetTrustSubprocessor(c.Request().Context(), orgID, id)
	if errors.Is(err, db.ErrTrustSubprocessorNotFound) {
		return nil
	}
	if err != nil {
		log.Printf("trust center: org %d item %d subprocessor %d: %v", orgID, it.ID, id, err)
		return trustInternalError(c, "reading subprocessor", err)
	}
	if hr := s.hideSubprocessorItem(c.Request().Context(), orgID, t); hr != "" {
		row.HiddenReason = hr
		it.HiddenAt = &db.Epoch{}
	}
	row.Subprocessor = trustManagerSubprocessorFor(t)
	return nil
}
