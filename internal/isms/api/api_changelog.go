package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

func (s *Server) handleListChangelog(c echo.Context) error {
	orgID := getOrgID(c)
	entityType := c.QueryParam("type")
	limit := 50
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	entries, err := s.db.ListChangelog(c.Request().Context(), orgID, entityType, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": entries})
}

func (s *Server) handleEntityChangelog(c echo.Context) error {
	orgID := getOrgID(c)
	entityType := c.Param("type")
	if entityType == "document" {
		return s.handleDocumentHistory(c, orgID)
	}
	// Accept the numeric id OR the entity's display identifier (#201); which
	// forms are valid depends on the :type, so dispatch on it.
	entityID, hasIdentifier, err := s.resolveEntityID(c.Request().Context(), orgID, entityType, c.Param("id"))
	if err != nil {
		if !hasIdentifier {
			// No display identifier for this type, so the param could only ever
			// have been numeric — and entityType may have no entity_inline key
			// to name in the message.
			return apiError(http.StatusBadRequest, CodeInvalidID)
		}
		if errors.Is(err, errInvalidID) {
			return errInvalidEntityID(entityType)
		}
		return errNotFound(entityType)
	}

	entries, err := s.db.ListEntityChangelog(c.Request().Context(), orgID, entityType, entityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": entries})
}

// handleDocumentHistory serves GET /changelog/document/:id from git. A document's
// id is text (document_id, not a bigint), and its edits are committed by API
// handlers and by git push alike, so the repository is the only complete record.
func (s *Server) handleDocumentHistory(c echo.Context, orgID int) error {
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	path := st.FindDocumentByID(c.Param("id"))
	if path == "" {
		return errNotFound("document")
	}
	hist, err := st.FileHistory(path, 200)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Approvals, change requests, closes and confirmations make no git commit,
	// so they come from the decision log and are merged in by time.
	decisions, err := s.db.ListDecisionRecords(c.Request().Context(), orgID, strings.ToLower(c.Param("id")))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": mergeDocumentHistory(documentChangelog(hist), decisions)})
}

// mergeDocumentHistory adds decision-log rows to the git rows and orders the
// result newest first. Ties keep each source's own order, git rows first.
func mergeDocumentHistory(git []db.ChangelogEntry, decisions []db.DecisionRecord) []db.ChangelogEntry {
	out := git
	for _, d := range decisions {
		e := db.ChangelogEntry{
			EntityType: "document",
			Reason:     d.Comment,
			ChangedBy:  d.DecidedBy,
			CreatedAt:  d.CreatedAt,
		}
		// Set outside the literal for the same reason as in documentChangelog.
		e.Action = d.Decision
		if d.Version != "" {
			v := d.Version
			e.NewValue = &v
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt.Time) })
	return out
}

// documentChangelog maps git history rows onto the changelog row shape the
// History panel already renders.
func documentChangelog(hist []store.HistoryEntry) []db.ChangelogEntry {
	entries := make([]db.ChangelogEntry, 0, len(hist))
	for _, h := range hist {
		e := db.ChangelogEntry{
			EntityType: "document",
			Field:      h.Field,
			OldValue:   h.OldValue,
			NewValue:   h.NewValue,
			ChangedBy:  h.ChangedBy,
			CreatedAt:  db.NewEpoch(h.At),
		}
		// Set outside the literal: these rows are read-only views of git, never
		// written to entity_changelog, so the schema-action drift guard does not apply.
		e.Action = h.Action
		entries = append(entries, e)
	}
	return entries
}
