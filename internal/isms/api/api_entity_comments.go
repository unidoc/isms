package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// entityCommentTypes are the entity_type values entity_comments accepts; keep in
// step with entity_comments_entity_type_check in the migrations.
var entityCommentTypes = map[string]bool{
	"risk": true, "supplier": true, "incident": true, "legal_requirement": true,
	"change_request": true, "corrective_action": true, "objective": true, "task": true,
	"system": true, "asset": true, "audit": true, "audit_finding": true,
	"audit_programme": true, "program": true, "checkin": true, "access_review": true,
	"document": true, "review": true,
}

// canonicalCommentEntityID resolves the record a comment is on and returns the
// id its thread is stored under, and whether the record exists in the org. It is
// the id /references uses (AUDIT-<id>, FIND-<id>, AP-<id>, a program's key, an
// objective's display id), so one record has one thread whichever spelling the
// caller used (#438). Checkins, reviews and access reviews are not reference
// types: they are keyed by their integer row id.
func (s *Server) canonicalCommentEntityID(ctx context.Context, orgID int, viewer db.TaskViewer, entityType, entityID string) (string, bool) {
	switch entityType {
	case "checkin", "review", "access_review":
		id, err := strconv.ParseInt(entityID, 10, 64)
		if err != nil || id <= 0 {
			return entityID, false
		}
		var found bool
		switch entityType {
		case "checkin":
			_, err = s.db.GetCheckin(ctx, orgID, id)
			found = err == nil
		case "review":
			_, err = s.db.GetReview(ctx, orgID, int(id))
			found = err == nil
		default:
			found, err = s.db.AccessReviewExists(ctx, orgID, id)
			found = found && err == nil
		}
		return strconv.FormatInt(id, 10), found
	}
	return s.canonicalReferenceID(ctx, orgID, viewer, entityType, entityID)
}

// privateTask returns the task a comment is on when entityType is "task" and the
// task is private, else nil. An id that doesn't resolve is not this check's job.
func (s *Server) privateTask(ctx context.Context, orgID int, entityType, entityID string) *db.Task {
	if entityType != "task" {
		return nil
	}
	id, err := s.resolveTaskID(ctx, orgID, entityID)
	if err != nil {
		return nil
	}
	t, err := s.db.GetTask(ctx, orgID, id)
	if err != nil || !t.Private {
		return nil
	}
	return t
}

// canViewCommentEntity applies a private task's visibility rule to its
// comments: a member who can't GET the task must not read, post or react to
// them. Every other type, and an id that doesn't resolve, is allowed.
func (s *Server) canViewCommentEntity(c echo.Context, entityType, entityID string) bool {
	t := s.privateTask(c.Request().Context(), getOrgID(c), entityType, entityID)
	return t == nil || canViewTask(c, t)
}

func (s *Server) handleListEntityComments(c echo.Context) error {
	orgID := getOrgID(c)
	entityType := c.Param("type")
	entityID := c.Param("id")

	// A hidden private task answers like a missing one: an empty thread.
	if !s.canViewCommentEntity(c, entityType, entityID) {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []db.EntityComment{}})
	}

	// Read the thread under its canonical id; if the id doesn't resolve (a
	// deleted record, or a legacy row) list the literal id so those still show.
	if canonicalID, found := s.canonicalCommentEntityID(c.Request().Context(), orgID, taskViewer(c), entityType, entityID); found {
		entityID = canonicalID
	}

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
	// Contributors and managers comment on register entities; readers are
	// read-only (#23). Mirrors handleCreateEntitySuggestion — the two are the
	// contributor's write surface and had drifted apart.
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
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
		return errRequired("body")
	}
	if !entityCommentTypes[req.EntityType] {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unknown entity_type: %s", req.EntityType))
	}
	// Validate the record and store the thread under its canonical id, so AUDIT-3
	// and 3 are one thread (#438). Everything below uses entityID, not req.EntityID.
	viewer := taskViewer(c)
	entityID, found := s.canonicalCommentEntityID(ctx, orgID, viewer, req.EntityType, req.EntityID)
	if !found {
		return apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity(req.EntityType))
	}
	if !s.canViewCommentEntity(c, req.EntityType, entityID) {
		return apiError(http.StatusBadRequest, CodeNotFoundInOrg, Entity("task"))
	}

	// A reply stays on its parent's record. Unlike document comments the client
	// has to name the entity anyway, so a mismatch is an error, not rewritten.
	if req.ParentID != nil {
		parent, err := s.db.GetEntityComment(ctx, orgID, *req.ParentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apiError(http.StatusNotFound, CodeNotFound, Entity("comment"))
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if parent.EntityType != req.EntityType || parent.EntityID != entityID {
			return apiError(http.StatusBadRequest, CodeInvalidRequest)
		}
	}

	comment := &db.EntityComment{
		EntityType: req.EntityType,
		EntityID:   entityID,
		ParentID:   req.ParentID,
		Author:     actor,
		Body:       req.Body,
	}
	// A #RISK-1 mention links this record to RISK-1, written with the comment
	// so neither lands without the other (#194).
	mentions := s.resolveCommentMentions(ctx, orgID, viewer, req.EntityType, entityID, req.Body)
	if err := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateEntityCommentTx(ctx, tx, orgID, comment); err != nil {
			return err
		}
		return db.AddCommentReferencesTx(ctx, tx, orgID, db.CommentTypeEntityComment, comment.ID, actor, mentions)
	}); err != nil {
		return pgxHTTPError(err)
	}

	// The activity feed and chat webhooks are org-wide, so a private task's
	// comment is logged without its body.
	priv := s.privateTask(ctx, orgID, req.EntityType, entityID)
	detail := fmt.Sprintf("Comment on %s %s: %s", req.EntityType, entityID, truncateStr(req.Body, 80))
	if priv != nil {
		detail = fmt.Sprintf("Comment on private task %s", entityID)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "entity_comment_created",
		Detail: detail,
	})

	// Notify anyone @-mentioned in the comment (#4) — works on any entity type,
	// so change-request comments (and every other register) pull members in.
	// On a private task only members who can see it are notified.
	var canSee []func(email, role string) bool
	if priv != nil {
		canSee = append(canSee, func(email, role string) bool { return taskVisibleTo(priv, email, role) })
	}
	s.notifyMentions(ctx, orgID, actor, req.Body,
		fmt.Sprintf("%s mentioned you in a comment", actor),
		NotifyKeyMentionComment,
		entityLink(req.EntityType, entityID), canSee...)

	return c.JSON(http.StatusCreated, comment)
}

func (s *Server) handleResolveEntityComment(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, CodeInvalidID)
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
		return apiError(http.StatusBadRequest, CodeInvalidID)
	}
	ctx := c.Request().Context()
	comment, err := s.db.GetEntityComment(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apiError(http.StatusNotFound, CodeNotFound, Entity("comment"))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	err = s.db.DeleteEntityComment(ctx, orgID, id)
	var hasReplies *db.CommentHasRepliesError
	switch {
	case errors.As(err, &hasReplies):
		return apiError(http.StatusConflict, CodeCommentHasReplies, Count(hasReplies.Count))
	case errors.Is(err, pgx.ErrNoRows): // deleted by someone else between the read and the lock
		return apiError(http.StatusNotFound, CodeNotFound, Entity("comment"))
	case err != nil:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	deleted := fmt.Sprintf("Deleted comment on %s %s: %s", comment.EntityType, comment.EntityID, truncateStr(comment.Body, 80))
	if s.privateTask(ctx, orgID, comment.EntityType, comment.EntityID) != nil {
		deleted = fmt.Sprintf("Deleted comment on private task %s", comment.EntityID)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "entity_comment_deleted",
		Detail: deleted,
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// ═══════════════════════════════════════════════════════════════════════
// REACTIONS
// ═══════════════════════════════════════════════════════════════════════

// canViewCommentTarget is false for an entity_comment on a private task the
// caller can't see, so reactions to it read as if the comment doesn't exist.
func (s *Server) canViewCommentTarget(c echo.Context, targetType string, targetID int64) bool {
	if targetType != "entity_comment" {
		return true
	}
	cm, err := s.db.GetEntityComment(c.Request().Context(), getOrgID(c), targetID)
	return err != nil || s.canViewCommentEntity(c, cm.EntityType, cm.EntityID)
}

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

	if !s.canViewCommentTarget(c, req.TargetType, req.TargetID) {
		return apiError(http.StatusNotFound, CodeNotFound, Entity("comment"))
	}
	added, err := s.db.ToggleReaction(c.Request().Context(), orgID, req.TargetType, req.TargetID, req.Emoji, actor)
	if err != nil {
		if errors.Is(err, db.ErrReactionTargetNotFound) {
			return apiError(http.StatusNotFound, CodeNotFound, Entity("comment"))
		}
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

	if !s.canViewCommentTarget(c, targetType, targetID) {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []db.ReactionSummary{}})
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
