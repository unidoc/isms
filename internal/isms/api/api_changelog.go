package api

import (
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
	entityID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid entity id")
	}

	entries, err := s.db.ListEntityChangelog(c.Request().Context(), orgID, entityType, entityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": entries})
}
