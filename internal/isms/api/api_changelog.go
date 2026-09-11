package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
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
