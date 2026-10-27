package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// External evidence links (#316): a URL, label, type and optional dates
// attached to a register record. Only suppliers are wired up; the table
// already accepts the other register types.
//
// A stored URL is data. Nothing here (or anywhere) fetches it: validation is
// parsing only, so a link can never make the server connect to an address the
// submitter chose.

const (
	externalLinkMaxURL   = 2048
	externalLinkMaxLabel = 200
)

// flexID reads a JSON string or number as its text, so entity_id can be
// "SUPPLIER-3" or 3.
type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexID(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexID(n.String())
	return nil
}

// externalLinkBody is the writable shape of a link. PUT is a full replace:
// every field is read from the body, and an absent or null date is NULL.
type externalLinkBody struct {
	EntityType string    `json:"entity_type"`
	EntityID   flexID    `json:"entity_id"`
	URL        string    `json:"url"`
	Label      string    `json:"label"`
	LinkType   string    `json:"link_type"`
	ValidUntil *db.Epoch `json:"valid_until"`
	VerifiedAt *db.Epoch `json:"verified_at"`
}

// dayOf turns a request date into a UTC-midnight time for a DATE column; nil or
// the zero time (an empty string) is NULL.
func dayOf(e *db.Epoch) *time.Time {
	if e == nil || e.IsZero() {
		return nil
	}
	u := e.UTC()
	d := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	return &d
}

// validateExternalLinkURL accepts only absolute http(s) URLs with a host.
func validateExternalLinkURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > externalLinkMaxURL {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
		return "", false
	}
	if u.Hostname() == "" {
		return "", false
	}
	return raw, true
}

// externalLinkInput validates the editable fields of a request body.
func externalLinkInput(req *externalLinkBody) (db.ExternalLinkInput, error) {
	if strings.TrimSpace(req.URL) == "" {
		return db.ExternalLinkInput{}, errRequired("url")
	}
	rawURL, ok := validateExternalLinkURL(req.URL)
	if !ok {
		return db.ExternalLinkInput{}, apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return db.ExternalLinkInput{}, errRequired("label")
	}
	if n := len([]rune(label)); n < 1 || n > externalLinkMaxLabel {
		return db.ExternalLinkInput{}, apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	valid := false
	for _, t := range db.ExternalLinkTypes {
		if req.LinkType == t {
			valid = true
		}
	}
	if !valid {
		return db.ExternalLinkInput{}, apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	return db.ExternalLinkInput{
		URL: rawURL, Label: label, LinkType: req.LinkType,
		ValidUntil: dayOf(req.ValidUntil), VerifiedAt: dayOf(req.VerifiedAt),
	}, nil
}

// externalLinkSupplier resolves a supplier by identifier (SUPPLIER-n) or row id
// and confirms it is live in this organization.
func (s *Server) externalLinkSupplier(ctx context.Context, orgID int, param string) (*db.Supplier, error) {
	id, err := s.resolveSupplierID(ctx, orgID, param)
	if errors.Is(err, errInvalidID) {
		return nil, errInvalidEntityID("supplier")
	} else if err != nil {
		return nil, errNotFound("supplier")
	}
	sup, err := s.db.GetSupplier(ctx, orgID, id)
	if err != nil {
		return nil, errNotFound("supplier")
	}
	return sup, nil
}

// externalLinkChange is the parent's changelog row for a link write. It is
// always an "update" of the supplier: a "delete" row on a live supplier would
// read as the supplier itself being deleted.
func externalLinkChange(supplierID int64, actor string, old, new *db.ExternalLink) *db.ChangelogEntry {
	e := &db.ChangelogEntry{EntityType: "supplier", EntityID: supplierID, Action: "update", Field: "external_link", ChangedBy: actor}
	if old != nil {
		v := old.ChangeSummary()
		e.OldValue = &v
	}
	if new != nil {
		v := new.ChangeSummary()
		e.NewValue = &v
	}
	return e
}

// externalLinkID parses the :id path param; anything unusable is a plain 404.
func externalLinkID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound("external_link")
	}
	return id, nil
}

// handleListExternalLinks returns the live links of one record.
func (s *Server) handleListExternalLinks(c echo.Context) error {
	orgID := getOrgID(c)
	entityType, entityID := c.QueryParam("type"), c.QueryParam("id")
	if entityType == "" || entityID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "type and id query params required")
	}
	if entityType != "supplier" {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	ctx := c.Request().Context()
	sup, err := s.externalLinkSupplier(ctx, orgID, entityID)
	if err != nil {
		return err
	}
	links, err := s.db.ListExternalLinks(ctx, orgID, "supplier", sup.ID)
	if err != nil {
		return pgxHTTPError(err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": links})
}

// handleCreateExternalLink adds a link to a supplier.
func (s *Server) handleCreateExternalLink(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	actor := getUserEmail(c)
	var req externalLinkBody
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if req.EntityType != "supplier" {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	if req.EntityID == "" {
		return errRequired("entity_id")
	}
	in, err := externalLinkInput(&req)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	sup, err := s.externalLinkSupplier(ctx, orgID, string(req.EntityID))
	if err != nil {
		return err
	}

	var link *db.ExternalLink
	err = s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.LockLiveSupplierTx(ctx, tx, orgID, sup.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotFound("supplier")
			}
			return err
		}
		var err error
		link, err = db.CreateExternalLinkTx(ctx, tx, orgID, "supplier", sup.ID, in, actor)
		if err != nil {
			return err
		}
		return db.LogChangeTx(ctx, tx, orgID, externalLinkChange(sup.ID, actor, nil, link))
	})
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he
		}
		return pgxHTTPError(err)
	}
	return c.JSON(http.StatusCreated, link)
}

// handleUpdateExternalLink replaces every editable field of a link. A
// verified_at that changes to a different date makes the caller the verifier;
// an unchanged one keeps the previous verifier; null clears both.
func (s *Server) handleUpdateExternalLink(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	actor := getUserEmail(c)
	id, err := externalLinkID(c)
	if err != nil {
		return err
	}
	var req externalLinkBody
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidRequest)
	}
	in, err := externalLinkInput(&req)
	if err != nil {
		return err
	}

	var link *db.ExternalLink
	err = s.db.WithOrgTx(c.Request().Context(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		old, err := db.GetExternalLinkTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		keep := in.VerifiedAt != nil && dayOf(old.VerifiedAt) != nil && dayOf(old.VerifiedAt).Equal(*in.VerifiedAt)
		link, err = db.UpdateExternalLinkTx(ctx, tx, orgID, id, in, keep, actor)
		if err != nil {
			return err
		}
		return db.LogChangeTx(ctx, tx, orgID, externalLinkChange(old.EntityID, actor, old, link))
	})
	if err != nil {
		return externalLinkWriteError(err)
	}
	return c.JSON(http.StatusOK, link)
}

// handleVerifyExternalLink stamps verified_at = today and the caller.
func (s *Server) handleVerifyExternalLink(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	actor := getUserEmail(c)
	id, err := externalLinkID(c)
	if err != nil {
		return err
	}
	var link *db.ExternalLink
	err = s.db.WithOrgTx(c.Request().Context(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		old, err := db.GetExternalLinkTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		link, err = db.VerifyExternalLinkTx(ctx, tx, orgID, id, actor)
		if err != nil {
			return err
		}
		return db.LogChangeTx(ctx, tx, orgID, externalLinkChange(old.EntityID, actor, old, link))
	})
	if err != nil {
		return externalLinkWriteError(err)
	}
	return c.JSON(http.StatusOK, link)
}

// handleDeleteExternalLink soft-deletes a link.
func (s *Server) handleDeleteExternalLink(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	actor := getUserEmail(c)
	id, err := externalLinkID(c)
	if err != nil {
		return err
	}
	err = s.db.WithOrgTx(c.Request().Context(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		old, err := db.GetExternalLinkTx(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if err := db.DeleteExternalLinkTx(ctx, tx, orgID, id); err != nil {
			return err
		}
		return db.LogChangeTx(ctx, tx, orgID, externalLinkChange(old.EntityID, actor, old, nil))
	})
	if err != nil {
		return externalLinkWriteError(err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// externalLinkWriteError maps a missing (or other-organization) link to 404.
func externalLinkWriteError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound("external_link")
	}
	return pgxHTTPError(err)
}
