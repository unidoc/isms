package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

func (s *Server) handleListEntityComments(c echo.Context) error {
	orgID := getOrgID(c)
	entityType := c.Param("type")
	entityID := c.Param("id")

	comments, err := s.db.ListEntityComments(c.Request().Context(), orgID, entityType, entityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Enrich with reactions
	for i := range comments {
		reactions, _ := s.db.ListReactions(c.Request().Context(), orgID, "entity_comment", comments[i].ID)
		comments[i].Reactions = reactions
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": comments})
}

func (s *Server) handleCreateEntityComment(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		ParentID   *int64 `json:"parent_id"`
		Body       string `json:"body"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.EntityType == "" || req.EntityID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "entity_type and entity_id are required")
	}
	if req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "body is required")
	}

	comment := &db.EntityComment{
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
		ParentID:   req.ParentID,
		Author:     actor,
		Body:       req.Body,
	}
	if err := s.db.CreateEntityComment(ctx, orgID, comment); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "entity_comment_created",
		Detail: fmt.Sprintf("Comment on %s %s: %s", req.EntityType, req.EntityID, truncateStr(req.Body, 80)),
	})

	// Notify anyone @-mentioned in the comment (#4) — works on any entity type,
	// so change-request comments (and every other register) pull members in.
	s.notifyMentions(ctx, orgID, actor, req.Body,
		fmt.Sprintf("%s mentioned you in a comment", actor),
		entityLink(req.EntityType, req.EntityID))

	return c.JSON(http.StatusCreated, comment)
}

func (s *Server) handleResolveEntityComment(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := s.db.ResolveEntityComment(c.Request().Context(), orgID, id, getUserEmail(c)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "resolved"})
}

func (s *Server) handleDeleteEntityComment(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := s.db.DeleteEntityComment(c.Request().Context(), orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// ═══════════════════════════════════════════════════════════════════════
// REACTIONS
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleToggleReaction(c echo.Context) error {
	orgID := getOrgID(c)
	actor := getUserEmail(c)

	var req struct {
		TargetType string `json:"target_type"`
		TargetID   int64  `json:"target_id"`
		Emoji      string `json:"emoji"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Emoji == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "emoji is required")
	}

	added, err := s.db.ToggleReaction(c.Request().Context(), orgID, req.TargetType, req.TargetID, req.Emoji, actor)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	action := "removed"
	if added {
		action = "added"
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"action": action, "emoji": req.Emoji})
}

func (s *Server) handleListReactions(c echo.Context) error {
	orgID := getOrgID(c)
	targetType := c.Param("targetType")
	targetID, err := strconv.ParseInt(c.Param("targetId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid target id")
	}

	reactions, err := s.db.ListReactions(c.Request().Context(), orgID, targetType, targetID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": reactions})
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
