package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/yuin/goldmark"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/notify"
	"isms.sh/internal/isms/store"
)

// logAndNotify logs an activity entry and dispatches notifications to configured channels.
func (s *Server) logAndNotify(ctx context.Context, orgID int, a *db.Activity) {
	a.OrganizationID = orgID
	s.db.LogActivity(ctx, orgID, a)
	if s.notifier != nil {
		// Read per-org notification channels from Postgres
		slackWH, _ := s.db.GetOrgSetting(ctx, orgID, "slack_webhook")
		matrixRoom, _ := s.db.GetOrgSetting(ctx, orgID, "matrix_room_id")
		matrixToken, _ := s.db.GetOrgSetting(ctx, orgID, "matrix_token")
		matrixServer, _ := s.db.GetOrgSetting(ctx, orgID, "matrix_server")
		if slackWH != "" || matrixRoom != "" {
			s.notifier.Send(notify.Event{
				Actor:  a.Actor,
				Action: a.Action,
				Detail: a.Detail,
				Link:   docLink(a.DocumentID),
				Channels: notify.OrgChannels{
					SlackWebhook: slackWH,
					MatrixRoomID: matrixRoom,
					MatrixToken:  matrixToken,
					MatrixServer: matrixServer,
				},
			})
		}
	}
}

// docLink returns a web UI link for a document ID.
func docLink(docID string) string {
	if docID == "" {
		return ""
	}
	switch {
	case docID == "suppliers":
		return "/suppliers"
	case docID == "risks":
		return "/risks"
	default:
		// All documents are resolved by document_id.
		return "/documents?doc=" + docID
	}
}

// getUserEmail extracts the authenticated user email from context or headers.
func getUserEmail(c echo.Context) string {
	// Set by token auth middleware
	if email, ok := c.Get("user_email").(string); ok && email != "" {
		return email
	}
	// Set by Cloudflare Zero Trust
	if email := c.Request().Header.Get("Cf-Access-Authenticated-User-Email"); email != "" {
		return email
	}
	return ""
}

// getOrgID returns the organization ID set by AuthMiddleware, or 0 if not set.
func getOrgID(c echo.Context) int {
	if id, ok := c.Get("org_id").(int); ok {
		return id
	}
	return 0
}

// getAPIKeyID returns the API key ID from context, or nil if using JWT session.
func getAPIKeyID(c echo.Context) *int {
	if id, ok := c.Get("api_key_id").(int); ok && id > 0 {
		return &id
	}
	return nil
}

// --- Reviews ---

func (s *Server) handleListReviews(c echo.Context) error {
	orgID := getOrgID(c)
	// Server-side filter / search / sort / pagination — match the gold pattern.
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.ReviewListParams{
		Page:   page,
		Limit:  limit,
		Sort:   c.QueryParam("sort"),
		Search: c.QueryParam("q"),
		Status: c.QueryParam("status"),
		Phase:  c.QueryParam("phase"),
	}
	items, total, err := s.db.PaginatedReviews(c.Request().Context(), orgID, params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit < 1 {
		params.Limit = 50
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      items,
		"total":     total,
		"page":      params.Page,
		"page_size": params.Limit,
	})
}

func (s *Server) handleReviewStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.ReviewStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

// reviewCreateRequest is the body schema for POST /reviews. Status is NEVER
// settable from the body: the review state machine is owned by dedicated
// transition endpoints (approve / merge / resubmit / status). System-managed
// fields (RequestedBy, Round, OrganizationID, ID, MergeCommit, timestamps)
// are derived server-side and ignored if present in the body.
type reviewCreateRequest struct {
	DocumentID   string `json:"document_id"`
	DocumentType string `json:"document_type"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Message      string `json:"message"`
	Version      string `json:"version"`
}

func (s *Server) handleCreateReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req reviewCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.DocumentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "document_id is required")
	}

	// Capture current HEAD as the snapshot the review is anchored to. Best-effort —
	// not all stores may expose a head (e.g. fresh repo); fall back to empty string.
	var commitHash, sentHead string
	if st, stErr := s.storeForOrg(ctx, orgID); stErr == nil {
		if h, herr := st.HeadHash(); herr == nil {
			commitHash = h
			sentHead = h
		}
	}

	// Body's Description maps to legacy Message field for backward compat.
	message := req.Message
	if message == "" {
		message = req.Description
	}

	r := db.Review{
		DocumentID:   req.DocumentID,
		DocumentType: req.DocumentType,
		Title:        req.Title,
		Version:      req.Version,
		CommitHash:   commitHash,
		SentHead:     sentHead,
		Message:      message,
		// Server-side enforced: identity, state, round.
		RequestedBy: getUserEmail(c),
		Status:      "open",
	}
	if err := s.db.CreateReview(ctx, orgID, &r); err != nil {
		return pgxHTTPError(err)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: r.DocumentID,
		Actor:      getUserEmail(c),
		Action:     "review_created",
		Detail:     fmt.Sprintf("Created review for %s", r.DocumentID),
	})
	return c.JSON(http.StatusCreated, r)
}

func (s *Server) handleGetReview(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	review, err := s.db.GetReview(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}
	// Return flat review with author_is_agent enrichment
	type reviewWithAgent struct {
		*db.Review
		AuthorIsAgent bool `json:"author_is_agent"`
	}
	return c.JSON(http.StatusOK, &reviewWithAgent{
		Review:        review,
		AuthorIsAgent: s.db.IsUserAgent(c.Request().Context(), review.RequestedBy),
	})
}

func (s *Server) handleListReviewAssignments(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	assignments, err := s.db.ListAssignmentsForReview(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Enrich with is_agent flag for AI review detection
	type enrichedAssignment struct {
		db.ReviewAssignment
		IsAgent bool `json:"is_agent"`
	}
	enriched := make([]enrichedAssignment, len(assignments))
	for i, a := range assignments {
		enriched[i] = enrichedAssignment{
			ReviewAssignment: a,
			IsAgent:          s.db.IsUserAgent(c.Request().Context(), a.Reviewer),
		}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": enriched})
}

func (s *Server) handleUpdateReviewStatus(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// This endpoint only allows closing a review. All other transitions
	// happen through dedicated handlers (approve, merge, resubmit).
	if req.Status != "closed" {
		return echo.NewHTTPError(http.StatusBadRequest, "this endpoint only supports closing reviews; use the dedicated approve/merge/resubmit endpoints for other transitions")
	}

	ctx := c.Request().Context()
	actor := getUserEmail(c)

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}
	if review.Status == "closed" || review.Status == "merged" {
		return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("review is already %s", review.Status))
	}

	if err := s.db.UpdateReviewStatus(ctx, orgID, id, "closed"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		ReviewID: &id,
		Actor:    actor,
		Action:   "review_status_changed",
		Detail:   fmt.Sprintf("Review #%d closed by %s", id, actor),
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "closed"})
}

// handleForwardReview adds new reviewers to an existing review. Manager-only.
func (s *Server) handleForwardReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	var req struct {
		Reviewers []string `json:"reviewers"`
		Message   string   `json:"message"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req.Reviewers) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one reviewer required")
	}

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	// Add assignments + status update in a single transaction with RLS
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		for _, reviewer := range req.Reviewers {
			if err := db.AddReviewAssignmentTx(ctx, tx, orgID, id, reviewer); err != nil {
				return fmt.Errorf("assigning reviewer %s: %w", reviewer, err)
			}
		}
		if review.Status != "open" {
			if err := db.UpdateReviewStatusTx(ctx, tx, orgID, id, "open"); err != nil {
				return fmt.Errorf("updating review status: %w", err)
			}
		}
		return nil
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	// Post-commit: notifications and emails (fire-and-forget)
	var assignments []db.ReviewAssignment
	for _, reviewer := range req.Reviewers {
		assignments = append(assignments, db.ReviewAssignment{ReviewID: id, Reviewer: reviewer, Status: "pending"})

		notifBody := fmt.Sprintf("%s forwarded review of %s (%s v%s) to you", actor, review.DocumentID, review.Title, review.Version)
		if req.Message != "" {
			notifBody += "\n\nNote: " + req.Message
		}
		s.db.CreateNotificationByEmail(ctx, orgID, reviewer,
			fmt.Sprintf("Review forwarded: %s", review.Title),
			notifBody, "/inbox/reviews")

		// Send email notification to reviewer
		if s.mailer != nil && s.mailer.Enabled() {
			baseURL := os.Getenv("ISMS_BASE_URL")
			_ = s.mailer.SendReviewRequest(reviewer, reviewer, actor, review.DocumentID, review.Title, review.Version, baseURL, id, req.Message)
		}
	}

	detail := fmt.Sprintf("Forwarded review #%d (%s) to %s", id, review.DocumentID, strings.Join(req.Reviewers, ", "))
	if req.Message != "" {
		detail += " — " + req.Message
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: review.DocumentID,
		ReviewID:   &id,
		Actor:      actor,
		Action:     "review_forwarded",
		Detail:     detail,
	})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":      "forwarded",
		"reviewers":   req.Reviewers,
		"assignments": assignments,
	})
}

// --- Review Timeline ---

// handleReviewTimeline returns a chronological timeline of all activity for a review:
// activity log entries, comments, and approvals merged and sorted by time.
func (s *Server) handleReviewTimeline(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()

	// Fetch all data sources (sequential but fast).
	activities, _ := s.db.ActivityForReview(ctx, orgID, id, 200)
	comments, _ := s.db.CommentsForReview(ctx, orgID, id)
	approvals, _ := s.db.ApprovalsForReview(ctx, orgID, id)
	assignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, id)
	decisions, _ := s.db.GetReviewDecisions(ctx, orgID, id)

	// Build unified timeline entries.
	type TimelineEntry struct {
		Type        string      `json:"type"`       // "activity", "comment", "approval", "assignment", "decision"
		Actor       string      `json:"actor"`
		Action      string      `json:"action"`
		Detail      string      `json:"detail,omitempty"`
		Body        string      `json:"body,omitempty"`
		Quote       string      `json:"quote,omitempty"`
		Decision    string      `json:"decision,omitempty"`
		Status      string      `json:"status,omitempty"`
		Reviewer    string      `json:"reviewer,omitempty"`
		ContentHash string      `json:"content_hash,omitempty"`
		Round       int         `json:"round"`
		CreatedAt   string      `json:"created_at"`
		Data        interface{} `json:"data,omitempty"`
	}

	var timeline []TimelineEntry

	for _, a := range activities {
		timeline = append(timeline, TimelineEntry{
			Type:      "activity",
			Actor:     a.Actor,
			Action:    a.Action,
			Detail:    a.Detail,
			CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	for _, c := range comments {
		timeline = append(timeline, TimelineEntry{
			Type:      "comment",
			Actor:     c.Author,
			Action:    "comment",
			Body:      c.Body,
			Quote:     c.Quote,
			Status:    c.Status,
			CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Data:      c,
		})
	}
	for _, a := range approvals {
		timeline = append(timeline, TimelineEntry{
			Type:      "approval",
			Actor:     a.ApprovedBy,
			Action:    a.Decision,
			Detail:    a.Comment,
			Decision:  a.Decision,
			CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Data:      a,
		})
	}
	for _, a := range assignments {
		timeline = append(timeline, TimelineEntry{
			Type:      "assignment",
			Reviewer:  a.Reviewer,
			Action:    "assigned",
			Status:    a.Status,
			CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Data:      a,
		})
	}
	for _, d := range decisions {
		timeline = append(timeline, TimelineEntry{
			Type:        "decision",
			Actor:       d.DecidedBy,
			Action:      d.Decision,
			Detail:      d.Comment,
			Decision:    d.Decision,
			ContentHash: d.ContentHash,
			CreatedAt:   d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Data:        d,
		})
	}

	// Sort by created_at ascending.
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].CreatedAt < timeline[j].CreatedAt
	})

	if timeline == nil {
		timeline = []TimelineEntry{}
	}

	// Annotate each entry with its round number.
	// Walk the sorted timeline; increment round when we hit a "review_resubmitted" entry.
	{
		round := 1
		for i := range timeline {
			if timeline[i].Type == "activity" && timeline[i].Action == "review_resubmitted" {
				round++
			}
			timeline[i].Round = round
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": timeline})
}

// handleReviewDiff returns the diff for a review (between commit_hash stored on review and current HEAD).
func (s *Server) handleReviewDiff(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	filePath := resolveDocPathFromStore(st, review.DocumentID)
	if filePath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document file not found")
	}

	// Use sent_head as the diff baseline: "what changed since this was sent for review" (current round).
	// For first review (commit_hash empty = never approved), use empty baseline so the whole document shows as new.
	// The ?from= query param allows overriding the baseline (used for "all changes in review" view).
	from := c.QueryParam("from")
	if from == "" {
		if review.CommitHash == "" {
			// First review — no prior approved version. Show entire document as new content.
			from = ""
		} else {
			from = review.SentHead
			if from == "" {
				from = review.CommitHash
			}
		}
	}
	var diffText string

	// Check if a review branch exists with edits
	branchName := fmt.Sprintf("review/%d", id)
	toRef := "HEAD"
	branchDiff, branchErr := st.DiffSuggestion(filePath, branchName)
	hasBranch := branchErr == nil && branchDiff != ""
	customFrom := c.QueryParam("from") != "" // explicit baseline override

	if hasBranch && !customFrom {
		// Review branch has edits, no explicit baseline — show diff between main and review branch
		diffText = branchDiff
	} else if hasBranch && customFrom {
		// Review branch has edits + explicit baseline — diff from custom baseline to branch
		diffText, _ = st.DiffDocumentBodies(from, branchName, filePath)
	} else if from == "" {
		// First review — no base commit. Show entire current content as "added".
		raw, readErr := st.ReadFile(filePath)
		if readErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "reading document: "+readErr.Error())
		}
		body := store.StripFrontmatter(string(raw))
		var addedLines []string
		for _, line := range strings.Split(body, "\n") {
			addedLines = append(addedLines, "+"+line)
		}
		diffText = "@@ -0,0 +1," + strconv.Itoa(len(addedLines)) + " @@\n" + strings.Join(addedLines, "\n")
	} else {
		// Use body-only diff that strips frontmatter from both sides
		var err error
		diffText, err = st.DiffDocumentBodies(from, "HEAD", filePath)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to compute diff: "+err.Error())
		}
	}
	if hasBranch {
		toRef = branchName
	}

	// Parse into structured lines, same as handleDocumentDiff.
	type DiffLine struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	var lines []DiffLine
	for _, line := range strings.Split(diffText, "\n") {
		if line == "" {
			continue
		}
		dl := DiffLine{Text: line}
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			dl.Type = "add"
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			dl.Type = "remove"
		case strings.HasPrefix(line, "@@"):
			dl.Type = "header"
		default:
			dl.Type = "context"
		}
		lines = append(lines, dl)
	}

	// Include current HEAD and last file modification info for context.
	// updated_since_sent: true when THIS SPECIFIC FILE was modified after the review was sent.
	// Compares file content at sent_head vs current file content (not repo-level HEAD).
	currentHead, _ := st.HeadHash()
	_, fileLastTime, fileLastMsg, fileLastAuthor, _ := st.FileLastCommit(filePath)
	updatedSinceSent := false
	if review.SentHead != "" {
		relPathCheck := strings.TrimPrefix(filePath, st.Root()+"/")
		relPathCheck = strings.ReplaceAll(relPathCheck, string(filepath.Separator), "/")
		sentContent, sentErr := st.ReadFileAtRef(review.SentHead, relPathCheck)
		curContent, curErr := st.ReadFile(filePath)
		if sentErr == nil && curErr == nil {
			updatedSinceSent = string(sentContent) != string(curContent)
		}
	}

	// Return old and new body for full track-changes rendering
	var oldBody, newBody string
	relPath := strings.TrimPrefix(filePath, st.Root()+"/")
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	// Old body: from sent_head (the snapshot sent for review)
	if from != "" {
		if raw, err := st.ReadFileAtRef(from, relPath); err == nil {
			oldBody = store.StripFrontmatter(string(raw))
		}
	}
	// New body: from review branch if exists, otherwise current HEAD
	if hasBranch {
		if raw, err := st.ReadFileAtRef(branchName, relPath); err == nil {
			newBody = store.StripFrontmatter(string(raw))
		}
	} else {
		if raw, err := st.ReadFile(filePath); err == nil {
			newBody = store.StripFrontmatter(string(raw))
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"document_id":        review.DocumentID,
		"from":               from,
		"to":                 toRef,
		"has_branch":         hasBranch,
		"current_head":       currentHead,
		"sent_head":          review.SentHead,
		"updated_since_sent": updatedSinceSent,
		"last_modified":      fileLastTime.Format("2006-01-02T15:04:05Z07:00"),
		"last_modified_by":   fileLastAuthor,
		"last_commit_msg":    fileLastMsg,
		"diff":               diffText,
		"lines":              lines,
		"old_body":           oldBody,
		"new_body":           newBody,
		"round":              review.Round,
		"commit_hash":        review.CommitHash,
	})
}

// handleAddReviewComment adds a comment tied to a specific review.
func (s *Server) handleAddReviewComment(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()

	// Verify review exists.
	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	// Authorization: admin/manager always, others must be assigned
	actor := getUserEmail(c)
	role, _ := c.Get("user_role").(string)
	if role != "admin" && role != "manager" {
		assignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, id)
		isAssigned := false
		for _, a := range assignments {
			if a.Reviewer == actor {
				isAssigned = true
				break
			}
		}
		if !isAssigned && review.RequestedBy != actor {
			return echo.NewHTTPError(http.StatusForbidden, "not authorized to comment on this review")
		}
	}

	var req struct {
		Body           string  `json:"body"`
		Quote          string  `json:"quote"`
		ParagraphIndex *int    `json:"paragraph_index"`
		ParagraphHash  string  `json:"paragraph_hash"`
		SuggestionBody *string `json:"suggestion_body"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Body == "" && (req.SuggestionBody == nil || *req.SuggestionBody == "") {
		return echo.NewHTTPError(http.StatusBadRequest, "comment body or suggestion required")
	}

	comment := &db.Comment{
		ReviewID:       &id,
		DocumentID:     review.DocumentID,
		Author:         getUserEmail(c),
		Body:           req.Body,
		Quote:          req.Quote,
		ParagraphIndex: req.ParagraphIndex,
		ParagraphHash:  req.ParagraphHash,
		Status:         "open",
	}
	if req.SuggestionBody != nil && *req.SuggestionBody != "" {
		comment.SuggestionBody = req.SuggestionBody
		pending := "pending"
		comment.SuggestionStatus = &pending
		if comment.Body == "" {
			comment.Body = "Suggested replacement for this paragraph"
		}
	}
	if err := s.db.AddComment(ctx, orgID, comment); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	action := "comment_added"
	detail := fmt.Sprintf("Comment on review #%d (%s)", id, review.DocumentID)
	if comment.SuggestionBody != nil {
		action = "suggestion_added"
		detail = fmt.Sprintf("Suggested edit on review #%d (%s)", id, review.DocumentID)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: review.DocumentID,
		ReviewID:   &id,
		Actor:      getUserEmail(c),
		Action:     action,
		Detail:     detail,
	})

	return c.JSON(http.StatusCreated, comment)
}

// handleReviewApprove records an approval/changes_requested decision on a review
// and auto-transitions the review status based on all assignment statuses.
func (s *Server) handleReviewApprove(c echo.Context) error {
	// Anyone assigned to the review can approve/request changes.
	// Admin/manager can always approve. Assignment check is below.

	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	var req struct {
		Decision string `json:"decision"` // "approved" or "changes_requested"
		Comment  string `json:"comment"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Decision != "approved" && req.Decision != "changes_requested" && req.Decision != "proposed_revision" {
		return echo.NewHTTPError(http.StatusBadRequest, "decision must be 'approved', 'changes_requested', or 'proposed_revision'")
	}

	actor := getUserEmail(c)

	// Self-approval blocked — author cannot approve their own review
	if actor == review.RequestedBy {
		return echo.NewHTTPError(http.StatusForbidden, "you cannot approve your own review")
	}

	// Verify the actor is an assigned reviewer or has admin/manager role.
	// Also check if the reviewer already acted in this round (non-pending = already acted).
	role, _ := c.Get("user_role").(string)
	assignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, id)
	if role != "admin" && role != "manager" {
		isAssigned := false
		for _, a := range assignments {
			if a.Reviewer == actor {
				isAssigned = true
				break
			}
		}
		if !isAssigned {
			return echo.NewHTTPError(http.StatusForbidden, "you are not assigned as a reviewer on this review")
		}
	}
	// Block duplicate action in the same round — reviewer already acted
	for _, a := range assignments {
		if a.Reviewer == actor && a.Status != "pending" {
			return echo.NewHTTPError(http.StatusConflict,
				fmt.Sprintf("you already submitted '%s' for this round — wait for the next round", a.Status))
		}
	}

	// Prepare approval and decision record before starting the transaction.
	approval := &db.Approval{
		ReviewID:   &id,
		DocumentID: review.DocumentID,
		Version:    review.Version,
		Round:      review.Round,
		Decision:   req.Decision,
		ApprovedBy: actor,
		Comment:    req.Comment,
	}

	decRec := &db.DecisionRecord{
		ReviewID:   &id,
		DocumentID: review.DocumentID,
		Decision:   req.Decision,
		DecidedBy:  actor,
		CommitRef:  review.CommitHash,
		Version:    review.Version,
		Comment:    req.Comment,
	}
	if u, _ := s.db.GetUserByEmail(ctx, actor); u != nil {
		decRec.DecidedByID = &u.ID
	}
	// Compute content hash — use review branch if exists, otherwise main.
	if st, stErr := s.storeForOrg(ctx, orgID); stErr == nil {
		if docPath := resolveDocPathFromStore(st, review.DocumentID); docPath != "" {
			branchName := fmt.Sprintf("review/%d", id)
			relPath := strings.TrimPrefix(docPath, st.Root()+"/")
			relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")
			raw, readErr := st.ReadFileAtRef(branchName, relPath)
			if readErr != nil {
				raw, readErr = st.ReadFile(docPath)
			}
			if readErr == nil {
				h := sha256.Sum256(raw)
				decRec.ContentHash = hex.EncodeToString(h[:])
			}
		}
	}

	// All DB writes in a single transaction with RLS: approval, decision record, assignment status, review status.
	var newStatus string
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.AddApprovalTx(ctx, tx, orgID, approval); err != nil {
			// Unique constraint on (org, review, approved_by, round) catches concurrent duplicate approvals.
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("CONFLICT:you already submitted a decision for this review round")
			}
			return err
		}

		if err := db.CreateDecisionRecordTx(ctx, tx, orgID, decRec); err != nil {
			return fmt.Errorf("failed to record decision: %w", err)
		}

		// Update the reviewer's assignment status.
		assignments, _ = db.ListAssignmentsForReviewTx(ctx, tx, orgID, id)
		for _, a := range assignments {
			if a.Reviewer == actor {
				if err := db.UpdateAssignmentStatusTx(ctx, tx, orgID, a.ID, req.Decision); err != nil {
					return fmt.Errorf("failed to update assignment: %w", err)
				}
				break
			}
		}

		// Auto-transition review status based on state machine.
		// Re-fetch assignments after update within the same tx.
		assignments, _ = db.ListAssignmentsForReviewTx(ctx, tx, orgID, id)
		newStatus = review.Status
		if req.Decision == "changes_requested" || req.Decision == "proposed_revision" {
			newStatus = "changes_requested"
		} else {
			// Check if ALL reviewers have approved.
			allApproved := true
			for _, a := range assignments {
				if a.Status != "approved" {
					allApproved = false
					break
				}
			}
			if allApproved && len(assignments) > 0 {
				newStatus = "approved"
			}
		}

		if newStatus != review.Status {
			if err := db.UpdateReviewStatusTx(ctx, tx, orgID, id, newStatus); err != nil {
				return fmt.Errorf("failed to update review status: %w", err)
			}
		}

		return nil
	})
	if txErr != nil {
		msg := txErr.Error()
		if strings.HasPrefix(msg, "CONFLICT:") {
			return echo.NewHTTPError(http.StatusConflict, strings.TrimPrefix(msg, "CONFLICT:"))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, msg)
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: review.DocumentID,
		ReviewID:   &id,
		Actor:      actor,
		Action:     "review_" + req.Decision,
		Detail:     fmt.Sprintf("%s %s review #%d (%s)", actor, req.Decision, id, review.DocumentID),
	})

	if newStatus != review.Status {
		s.logAndNotify(ctx, orgID, &db.Activity{
			DocumentID: review.DocumentID,
			ReviewID:   &id,
			Actor:      "system",
			Action:     "review_status_changed",
			Detail:     fmt.Sprintf("Review #%d auto-transitioned to %s", id, newStatus),
		})
	}

	// Agent-to-agent detection: suppress human notifications when agents are still iterating.
	reviewerIsAgent := s.db.IsUserAgent(ctx, actor)
	authorIsAgent := s.db.IsUserAgent(ctx, review.RequestedBy)
	maxRounds := s.db.GetOrgSettingInt(ctx, orgID, "ai_review_max_rounds", 3)

	agentLoop := reviewerIsAgent && authorIsAgent
	escalated := false

	if agentLoop && (req.Decision == "changes_requested" || req.Decision == "proposed_revision") {
		if review.Round >= maxRounds {
			// Escalation: max rounds exceeded, notify human owner
			escalated = true
			// Find document owner for escalation
			if st, stErr := s.storeForOrg(ctx, orgID); stErr == nil {
				if filePath := resolveDocPathFromStore(st, review.DocumentID); filePath != "" {
					if doc, loadErr := st.LoadDocument(filePath); loadErr == nil && doc != nil && doc.Frontmatter.Owner != "" {
						_ = s.db.CreateNotificationByEmail(ctx, orgID, doc.Frontmatter.Owner,
							"AI review escalated",
							fmt.Sprintf("AI review of %s reached round %d without agreement. Please review and decide.", review.DocumentID, review.Round),
							fmt.Sprintf("/reviews/%d", id))
					}
				}
			}
			s.logAndNotify(ctx, orgID, &db.Activity{
				DocumentID: review.DocumentID,
				ReviewID:   &id,
				Actor:      "system",
				Action:     "ai_review_escalated",
				Detail:     fmt.Sprintf("AI review of %s escalated after %d rounds — human decision needed", review.DocumentID, review.Round),
			})
		} else {
			// Agent loop continues: notify author agent, suppress human notifications
			_ = s.db.CreateAgentNotification(ctx, orgID, review.RequestedBy,
				"Review changes requested",
				fmt.Sprintf("Reviewer %s requested changes on %s (round %d). Address comments and resubmit.", actor, review.DocumentID, review.Round),
				fmt.Sprintf("/reviews/%d", id))
		}
	} else {
		// Normal flow: email the review author (human or mixed)
		if s.mailer != nil && s.mailer.Enabled() {
			baseURL := os.Getenv("ISMS_BASE_URL")
			_ = s.mailer.SendReviewDecision(review.RequestedBy, review.RequestedBy, actor, review.DocumentID, review.Title, review.Version, req.Decision, baseURL)
		}
	}

	// Auto-merge check: if policy allows and all requirements met, trigger merge
	autoMerged := false
	if newStatus == "approved" {
		if st, stErr := s.storeForOrg(ctx, orgID); stErr == nil {
			docPath := resolveDocPathFromStore(st, review.DocumentID)
			if docPath != "" {
				autoMergeApprovals, _ := s.db.ApprovalsForReview(ctx, orgID, id)
				policyStatus, _ := s.db.CheckReviewPolicy(ctx, orgID, docPath, assignments, autoMergeApprovals...)
				if policyStatus != nil && policyStatus.CanAutoMerge {
					// Trigger merge as system actor
					if mergeErr := s.performMerge(ctx, orgID, id, review, "system (auto-merge)"); mergeErr == nil {
						autoMerged = true
					} else {
						s.logAndNotify(ctx, orgID, &db.Activity{
							DocumentID: review.DocumentID,
							ReviewID:   &id,
							Actor:      "system",
							Action:     "auto_merge_failed",
							Detail:     fmt.Sprintf("Auto-merge failed for review #%d: %v", id, mergeErr),
						})
					}
					if autoMerged {
						s.logAndNotify(ctx, orgID, &db.Activity{
							DocumentID: review.DocumentID,
							ReviewID:   &id,
							Actor:      "system",
							Action:     "review_auto_merged",
							Detail:     fmt.Sprintf("Review #%d auto-merged: policy '%s' satisfied", id, policyStatus.PolicyName),
						})
					}
				}
			}
		}
	}

	// Build response with approval context
	totalAssigned := len(assignments)
	approvedCount := 0
	var pendingReviewers []string
	for _, a := range assignments {
		if a.Status == "approved" {
			approvedCount++
		} else {
			pendingReviewers = append(pendingReviewers, a.Reviewer)
		}
	}

	resp := map[string]interface{}{
		"approval":           approval,
		"review_status":      newStatus,
		"round":              review.Round,
		"total_reviewers":    totalAssigned,
		"approved_count":     approvedCount,
		"pending_reviewers":  pendingReviewers,
	}
	if autoMerged {
		resp["auto_merged"] = true
		resp["review_status"] = "merged"
	}
	if agentLoop {
		resp["agent_loop"] = true
		resp["escalated"] = escalated
	}

	return c.JSON(http.StatusOK, resp)
}

// performMerge executes the merge logic for a review. Shared by manual merge and auto-merge.
func (s *Server) performMerge(ctx context.Context, orgID int, id int, review *db.Review, actor string) error {
	// Resolve actor name
	actorName := actor
	if user, _ := s.db.GetUserByEmail(ctx, actor); user != nil && user.Name != "" {
		actorName = user.Name
	}

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("opening repository: %w", err)
	}

	// Git merge
	branchName := fmt.Sprintf("review/%d", id)
	docPath := resolveDocPathFromStore(st, review.DocumentID)
	if docPath != "" {
		baseForMerge := review.SentHead
		if baseForMerge == "" {
			baseForMerge = review.CommitHash
		}
		if _, mergeErr := st.MergeSuggestion(docPath, branchName, actorName, actor, baseForMerge); mergeErr != nil {
			if mergeErr == store.ErrConflict {
				return fmt.Errorf("document was modified since review was sent")
			}
			// "reference not found" means no review branch exists — content was edited
			// directly on main (e.g. no reviewer edits). This is a valid no-op merge.
			errMsg := mergeErr.Error()
			if !strings.Contains(errMsg, "reference not found") && !strings.Contains(errMsg, "not found") {
				return fmt.Errorf("merging review branch: %w", mergeErr)
			}
		}
	}

	// Update document status
	if err := setDocumentStatusAndVersion(st, review.DocumentID, "approved", "", actorName, actor); err != nil {
		return fmt.Errorf("updating document status: %w", err)
	}

	// Prepare data for DB operations (read-only git access, no DB writes yet)
	mergeHead, _ := st.HeadHash()

	decRec := &db.DecisionRecord{
		ReviewID:   &id,
		DocumentID: review.DocumentID,
		Decision:   "merged",
		DecidedBy:  actor,
		CommitRef:  review.CommitHash,
		Version:    review.Version,
	}
	if user, _ := s.db.GetUserByEmail(ctx, actor); user != nil {
		decRec.DecidedByID = &user.ID
	}
	if dp := resolveDocPathFromStore(st, review.DocumentID); dp != "" {
		if raw, readErr := st.ReadFile(dp); readErr == nil {
			h := sha256.Sum256(raw)
			decRec.ContentHash = hex.EncodeToString(h[:])
		}
	}

	filePath := resolveDocPathFromStore(st, review.DocumentID)
	var ver *db.DocumentVersion
	if mergeHead != "" && filePath != "" {
		ver = &db.DocumentVersion{
			DocumentID: review.DocumentID,
			Version:    review.Version,
			CommitHash: mergeHead,
			FilePath:   filePath,
			Message:    review.Message,
			CreatedBy:  actor,
		}
		if raw, readErr := st.ReadFile(filePath); readErr == nil {
			h := sha256.Sum256(raw)
			ver.ContentHash = hex.EncodeToString(h[:])
		}
		if doc, loadErr := st.LoadDocument(filePath); loadErr == nil && doc != nil {
			ver.Owner = doc.Frontmatter.Owner
			if doc.Frontmatter.ReviewCycle > 0 {
				rc := doc.Frontmatter.ReviewCycle
				ver.ReviewCycleMonths = &rc
			}
		}
	}

	// All DB writes in a single transaction — if any fails, all roll back.
	return s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		// Store merge commit hash — needed for diff baseline on subsequent reviews
		if mergeHead != "" {
			if err := db.SetMergeCommitTx(ctx, tx, orgID, id, mergeHead); err != nil {
				return fmt.Errorf("storing merge commit: %w", err)
			}
		}

		// Update review status
		if err := db.UpdateReviewStatusTx(ctx, tx, orgID, id, "merged"); err != nil {
			return fmt.Errorf("updating review status: %w", err)
		}

		// Decision record
		if err := db.CreateDecisionRecordTx(ctx, tx, orgID, decRec); err != nil {
			return fmt.Errorf("creating merge decision record: %w", err)
		}

		// Record version history
		if ver != nil {
			if err := db.RecordVersionTx(ctx, tx, orgID, ver); err != nil {
				return fmt.Errorf("recording version: %w", err)
			}
		}

		return nil
	})
}

// handleMergeReview merges a review — marks it as merged (final state).
func (s *Server) handleMergeReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	if review.Status != "approved" {
		return echo.NewHTTPError(http.StatusBadRequest, "review must be approved before merging")
	}

	// Enforce approval policies before manual merge
	// Enforce approval policies using canonical check (same as auto-merge)
	if st, stErr := s.storeForOrg(ctx, orgID); stErr == nil {
		policyDocPath := resolveDocPathFromStore(st, review.DocumentID)
		if policyDocPath == "" {
			policyDocPath = review.DocumentID
		}
		assignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, id)
		mergeApprovals, _ := s.db.ApprovalsForReview(ctx, orgID, id)
		policyResult, _ := s.db.CheckReviewPolicy(ctx, orgID, policyDocPath, assignments, mergeApprovals...)
		if policyResult != nil && !policyResult.Met {
			var details []string
			if policyResult.Approvals < policyResult.MinApprovals {
				details = append(details, fmt.Sprintf("need %d approval(s), have %d", policyResult.MinApprovals, policyResult.Approvals))
			}
			if policyResult.RequireHuman && !policyResult.HumanApproved {
				details = append(details, "at least one human approval is required")
			}
			return c.JSON(http.StatusForbidden, map[string]interface{}{
				"error":   "approval policy requirements not met",
				"details": details,
			})
		}
	}

	if err := s.performMerge(ctx, orgID, id, review, actor); err != nil {
		if strings.Contains(err.Error(), "modified since") || strings.Contains(err.Error(), "conflict") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: review.DocumentID,
		ReviewID:   &id,
		Actor:      actor,
		Action:     "review_merged",
		Detail:     fmt.Sprintf("Review #%d merged — %s v%s published", id, review.DocumentID, review.Version),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "merged"})
}

// handleAcceptAndMerge lets the author accept a proposed revision and merge in one step.
// The reviewer already edited the document — author agrees, so we convert the
// proposed_revision to an approval and merge immediately.
func (s *Server) handleAcceptAndMerge(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	// Only the author (requester) can accept a proposed revision
	if review.RequestedBy != actor {
		return echo.NewHTTPError(http.StatusForbidden, "only the review author can accept and publish")
	}

	if review.Status != "changes_requested" {
		return echo.NewHTTPError(http.StatusBadRequest, "review is not in changes_requested state")
	}

	// Verify there's actually a proposed revision
	assignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, id)
	hasProposed := false
	for _, a := range assignments {
		if a.Status == "proposed_revision" {
			hasProposed = true
			break
		}
	}
	if !hasProposed {
		return echo.NewHTTPError(http.StatusBadRequest, "no proposed revision to accept")
	}

	// All DB writes in a single transaction with RLS: assignment updates, review status, approval record.
	rid := id
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		for _, a := range assignments {
			if a.Status == "proposed_revision" {
				if err := db.UpdateAssignmentStatusTx(ctx, tx, orgID, a.ID, "approved"); err != nil {
					return fmt.Errorf("failed to update assignment status: %w", err)
				}
			}
		}

		if err := db.UpdateReviewStatusTx(ctx, tx, orgID, id, "approved"); err != nil {
			return fmt.Errorf("failed to update review status: %w", err)
		}

		if err := db.AddApprovalTx(ctx, tx, orgID, &db.Approval{
			ReviewID:   &rid,
			DocumentID: review.DocumentID,
			Version:    review.Version,
			Round:      review.Round,
			Decision:   "approved",
			ApprovedBy: actor,
			Comment:    "Accepted proposed revision and published",
		}); err != nil {
			return fmt.Errorf("failed to record approval: %w", err)
		}

		return nil
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	// Now merge (performMerge has its own transaction)
	review.Status = "approved" // update in-memory for performMerge
	if err := s.performMerge(ctx, orgID, id, review, actor); err != nil {
		if strings.Contains(err.Error(), "modified since") || strings.Contains(err.Error(), "conflict") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: review.DocumentID,
		ReviewID:   &id,
		Actor:      actor,
		Action:     "review_merged",
		Detail:     fmt.Sprintf("Review #%d — accepted proposed revision and published %s v%s", id, review.DocumentID, review.Version),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "merged"})
}

// handleUpdateReviewContent writes document content to the review branch.
// Only assigned reviewers, the review requester, managers, and admins can edit.
func (s *Server) handleUpdateReviewContent(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()
	email := getUserEmail(c)

	// Check authorization: must be assigned reviewer, review owner, or manager/admin
	role, _ := c.Get("user_role").(string)
	if role != "admin" && role != "manager" {
		assignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, id)
		isAssigned := false
		for _, a := range assignments {
			if a.Reviewer == email {
				isAssigned = true
				break
			}
		}
		if !isAssigned {
			rev, _ := s.db.GetReview(ctx, orgID, id)
			if rev == nil || rev.RequestedBy != email {
				return echo.NewHTTPError(http.StatusForbidden, "not authorized to edit this review")
			}
		}
	}

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}
	if review.Status == "merged" || review.Status == "closed" {
		return echo.NewHTTPError(http.StatusBadRequest, "review is already "+review.Status)
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	docPath := resolveDocPathFromStore(st, review.DocumentID)
	if docPath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document file not found")
	}

	// Preserve frontmatter — read from review branch first, fall back to main
	branchName := fmt.Sprintf("review/%d", id)
	relPath := strings.TrimPrefix(docPath, st.Root()+"/")
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")
	raw, err := st.ReadFileAtRef(branchName, relPath)
	if err != nil {
		raw, err = st.ReadFile(docPath)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "reading document: "+err.Error())
	}
	var newContent string
	lines := strings.Split(string(raw), "\n")
	if len(lines) >= 2 && strings.TrimSpace(lines[0]) == "---" {
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				endIdx = i
				break
			}
		}
		if endIdx >= 0 {
			newContent = strings.Join(lines[:endIdx+1], "\n") + "\n" + req.Content
		} else {
			newContent = req.Content
		}
	} else {
		newContent = req.Content
	}

	user, _ := s.db.GetUserByEmail(ctx, email)
	authorName := email
	if user != nil && user.Name != "" {
		authorName = user.Name
	}

	branchName = fmt.Sprintf("review/%d", id)
	message := fmt.Sprintf("review(%s): edit by %s", review.DocumentID, email)

	commitHash, err := st.CreateSuggestion(docPath, branchName, []byte(newContent), authorName, email, message)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "saving to review branch: "+err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: review.DocumentID,
		ReviewID:   &id,
		Actor:      email,
		Action:     "review_edited",
		Detail:     fmt.Sprintf("Edited document in review #%d", id),
	})

	return c.JSON(http.StatusOK, map[string]string{"commit": commitHash, "branch": branchName})
}

// handleGetReviewContent returns the document content from the review branch (or main if no branch edits).
func (s *Server) handleGetReviewContent(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	ctx := c.Request().Context()

	review, err := s.db.GetReview(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	docPath := resolveDocPathFromStore(st, review.DocumentID)
	if docPath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document file not found")
	}

	relPath := strings.TrimPrefix(docPath, st.Root()+"/")
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	branchName := fmt.Sprintf("review/%d", id)
	// Try reading from review branch first
	content, err := st.ReadFileAtRef(branchName, relPath)
	fromBranch := true
	if err != nil {
		// No review branch — read from main
		content, err = st.ReadFile(docPath)
		fromBranch = false
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "reading document: "+err.Error())
		}
	}

	body := store.StripFrontmatter(string(content))
	return c.JSON(http.StatusOK, map[string]interface{}{
		"body":        body,
		"from_branch": fromBranch,
		"branch":      branchName,
	})
}

// --- Comments (DB-backed) ---

func (s *Server) handleAllOpenComments(c echo.Context) error {
	orgID := getOrgID(c)
	comments, err := s.db.AllOpenComments(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if comments == nil {
		comments = []db.Comment{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": comments})
}

func (s *Server) handleListCommentsDB(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")
	var reviewFilter []int
	if rid := c.QueryParam("review_id"); rid != "" {
		if id, err := strconv.Atoi(rid); err == nil {
			reviewFilter = append(reviewFilter, id)
		}
	}
	comments, err := s.db.CommentsForDocument(c.Request().Context(), orgID, docID, reviewFilter...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": comments})
}

func (s *Server) handleAddCommentDB(c echo.Context) error {
	orgID := getOrgID(c)
	var comment db.Comment
	if err := c.Bind(&comment); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	comment.Author = getUserEmail(c) // always use authenticated user
	// If suggestion_body is provided, set suggestion_status to pending
	if comment.SuggestionBody != nil && *comment.SuggestionBody != "" && comment.SuggestionStatus == nil {
		pending := "pending"
		comment.SuggestionStatus = &pending
	}
	if err := s.db.AddComment(c.Request().Context(), orgID, &comment); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	detail := fmt.Sprintf("Comment on %s", comment.DocumentID)
	if comment.Quote != "" {
		snippet := comment.Quote
		if len(snippet) > 60 {
			snippet = snippet[:60] + "..."
		}
		detail = fmt.Sprintf("Comment on %s (re: %q)", comment.DocumentID, snippet)
	}
	s.db.LogActivity(c.Request().Context(), orgID, &db.Activity{
		DocumentID: comment.DocumentID,
		Actor:      getUserEmail(c),
		Action:     "comment_added",
		Detail:     detail,
	})
	if s.notifier != nil && s.notifier.Enabled() {
		link := docLink(comment.DocumentID)
		if comment.ParagraphIndex != nil {
			link += fmt.Sprintf("#p%d", *comment.ParagraphIndex)
		}
		s.notifier.Send(notify.Event{
			Actor:   getUserEmail(c),
			Action:  "comment_added",
			Detail:  detail,
			Body:    comment.Body,
			BaseURL: os.Getenv("ISMS_BASE_URL"),
			Link:    link,
		})
	}
	return c.JSON(http.StatusCreated, comment)
}

func (s *Server) handleResolveCommentDB(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment id")
	}
	resolvedBy := getUserEmail(c) // always use authenticated user
	ctx := c.Request().Context()
	// Get document_id from comment before resolving
	docID, _ := s.db.GetCommentDocumentID(ctx, orgID, id)
	if err := s.db.ResolveComment(ctx, orgID, id, resolvedBy); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: docID,
		Actor:      getUserEmail(c),
		Action:     "comment_resolved",
		Detail:     fmt.Sprintf("Resolved comment #%d", id),
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "resolved"})
}

// --- Suggestions ---

// handleAcceptSuggestion applies a suggestion's proposed text to the document on the review branch.
func (s *Server) handleAcceptSuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	commentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment id")
	}
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	comment, err := s.db.GetComment(ctx, orgID, commentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "comment not found")
	}
	if comment.SuggestionBody == nil || *comment.SuggestionBody == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "comment is not a suggestion")
	}
	if comment.SuggestionStatus == nil || *comment.SuggestionStatus != "pending" {
		return echo.NewHTTPError(http.StatusBadRequest, "suggestion is not pending")
	}
	if comment.ReviewID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "suggestion not attached to a review")
	}

	review, err := s.db.GetReview(ctx, orgID, *comment.ReviewID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "review not found")
	}
	if review.Status == "merged" || review.Status == "closed" {
		return echo.NewHTTPError(http.StatusBadRequest, "review is "+review.Status)
	}

	// Auth: only review author or admin/manager can accept suggestions
	role, _ := c.Get("user_role").(string)
	if role != "admin" && role != "manager" && review.RequestedBy != actor {
		return echo.NewHTTPError(http.StatusForbidden, "only the document author can accept suggestions")
	}

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	docPath := resolveDocPathFromStore(st, review.DocumentID)
	if docPath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	// Read from review branch or HEAD
	branchName := fmt.Sprintf("review/%d", *comment.ReviewID)
	relPath := strings.TrimPrefix(docPath, st.Root()+"/")
	relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	var rawContent []byte
	if data, readErr := st.ReadFileAtRef(branchName, relPath); readErr == nil {
		rawContent = data
	} else if data, readErr := st.ReadFile(docPath); readErr == nil {
		rawContent = data
	} else {
		return echo.NewHTTPError(http.StatusInternalServerError, "reading document")
	}

	// Split frontmatter + body, find target paragraph
	fullContent := string(rawContent)
	fmEnd := findFrontmatterEnd(fullContent)
	var prefix, body string
	if fmEnd > 0 {
		prefix = fullContent[:fmEnd]
		body = fullContent[fmEnd:]
	} else {
		body = fullContent
	}

	paragraphs := splitMarkdownParagraphs(body)
	targetIdx := -1

	// Match by index + quote verification
	if comment.ParagraphIndex != nil {
		idx := *comment.ParagraphIndex
		if idx >= 0 && idx < len(paragraphs) && (comment.Quote == "" || strings.Contains(paragraphs[idx], comment.Quote[:min(len(comment.Quote), 50)])) {
			targetIdx = idx
		}
	}
	// Fallback: scan by quote content
	if targetIdx == -1 && comment.Quote != "" {
		needle := comment.Quote
		if len(needle) > 50 {
			needle = needle[:50]
		}
		for i, p := range paragraphs {
			if strings.Contains(p, needle) {
				targetIdx = i
				break
			}
		}
	}
	if targetIdx == -1 {
		return echo.NewHTTPError(http.StatusConflict, "target paragraph not found — document may have changed")
	}

	// Replace paragraph and commit to review branch
	paragraphs[targetIdx] = *comment.SuggestionBody
	newBody := strings.Join(paragraphs, "\n\n")
	newContent := prefix + newBody

	user, _ := s.db.GetUserByEmail(ctx, actor)
	authorName := actor
	if user != nil && user.Name != "" {
		authorName = user.Name
	}
	message := fmt.Sprintf("review(%s): accept suggestion #%d by %s", review.DocumentID, commentID, comment.Author)
	commitHash, commitErr := st.CreateSuggestion(docPath, branchName, []byte(newContent), authorName, actor, message)
	if commitErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "committing suggestion: "+commitErr.Error())
	}

	if err := s.db.AcceptSuggestion(ctx, orgID, commentID, actor); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: review.DocumentID,
		ReviewID:   comment.ReviewID,
		Actor:      actor,
		Action:     "suggestion_accepted",
		Detail:     fmt.Sprintf("Accepted suggestion #%d by %s", commentID, comment.Author),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "accepted", "commit": commitHash, "branch": branchName})
}

func (s *Server) handleRejectSuggestion(c echo.Context) error {
	orgID := getOrgID(c)
	commentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment id")
	}
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	comment, err := s.db.GetComment(ctx, orgID, commentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "comment not found")
	}
	if comment.SuggestionBody == nil || comment.SuggestionStatus == nil || *comment.SuggestionStatus != "pending" {
		return echo.NewHTTPError(http.StatusBadRequest, "not a pending suggestion")
	}

	// Auth: review author or admin/manager, and review must not be merged/closed
	if comment.ReviewID != nil {
		review, _ := s.db.GetReview(ctx, orgID, *comment.ReviewID)
		if review != nil {
			if review.Status == "merged" || review.Status == "closed" {
				return echo.NewHTTPError(http.StatusBadRequest, "review is "+review.Status)
			}
			role, _ := c.Get("user_role").(string)
			if role != "admin" && role != "manager" && review.RequestedBy != actor {
				return echo.NewHTTPError(http.StatusForbidden, "only the document author can reject suggestions")
			}
		}
	}

	if err := s.db.RejectSuggestion(ctx, orgID, commentID, actor); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: comment.DocumentID,
		ReviewID:   comment.ReviewID,
		Actor:      actor,
		Action:     "suggestion_rejected",
		Detail:     fmt.Sprintf("Rejected suggestion #%d by %s", commentID, comment.Author),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "rejected"})
}

func (s *Server) handleListSuggestions(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	comments, err := s.db.CommentsForReview(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	var suggestions []db.Comment
	for _, c := range comments {
		if c.SuggestionBody != nil {
			suggestions = append(suggestions, c)
		}
	}
	if suggestions == nil {
		suggestions = []db.Comment{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": suggestions})
}

// Helper: find end of frontmatter (position after closing "---\n")
func findFrontmatterEnd(content string) int {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			pos := 0
			for j := 0; j <= i; j++ {
				pos += len(lines[j]) + 1 // +1 for newline
			}
			return pos
		}
	}
	return 0
}

// splitMarkdownParagraphs splits markdown body on double-newlines.
func splitMarkdownParagraphs(body string) []string {
	parts := regexp.MustCompile(`\n\n+`).Split(strings.TrimSpace(body), -1)
	var result []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			result = append(result, p)
		}
	}
	return result
}

// --- Approvals ---

func (s *Server) handleListApprovals(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")
	approvals, err := s.db.ApprovalsForDocument(c.Request().Context(), orgID, docID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": approvals})
}

// --- Decision Log ---

// handleListDocumentDecisions returns all decision records for a document.
func (s *Server) handleListDocumentDecisions(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")
	records, err := s.db.ListDecisionRecords(c.Request().Context(), orgID, docID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if records == nil {
		records = []db.DecisionRecord{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": records})
}

// handleListReviewDecisions returns all decision records for a specific review.
func (s *Server) handleListReviewDecisions(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid review id")
	}
	records, err := s.db.GetReviewDecisions(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if records == nil {
		records = []db.DecisionRecord{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": records})
}

// --- Tasks ---

func (s *Server) handleListTasks(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.TaskListParams{
		Page:     page,
		Limit:    limit,
		Sort:     c.QueryParam("sort"),
		Search:   c.QueryParam("q"),
		Status:   c.QueryParam("status"),
		Priority: c.QueryParam("priority"),
		TaskType: c.QueryParam("task_type"),
		Assignee: c.QueryParam("assignee"),
	}
	items, total, err := s.db.PaginatedTasks(c.Request().Context(), orgID, params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit < 1 {
		params.Limit = 50
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      items,
		"total":     total,
		"page":      params.Page,
		"page_size": params.Limit,
	})
}

func (s *Server) handleTaskStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.TaskStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleCreateTask(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	var req taskCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	t := db.Task{
		Title:          req.Title,
		Description:    req.Description,
		TaskType:       req.TaskType,
		Assignee:       req.Assignee,
		Status:         req.Status,
		Priority:       req.Priority,
		DueDate:        req.DueDate,
		RecurrenceDays: req.RecurrenceDays,
		Notes:          req.Notes,
	}
	t.CreatedBy = getUserEmail(c) // always use authenticated user
	if t.Assignee == "" {
		t.Assignee = t.CreatedBy // default: assign to yourself
	}
	if t.Status == "" {
		t.Status = "open"
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.TaskType == "" {
		t.TaskType = "general"
	}
	// due_date stays optional — no auto-default. Users can leave it empty.
	if err := validateEnum("status", t.Status, db.TaskStatuses); err != nil {
		return err
	}
	if err := validateEnum("priority", t.Priority, db.TaskPriorities); err != nil {
		return err
	}
	if err := validateEnum("task_type", t.TaskType, db.TaskTypes); err != nil {
		return err
	}
	if err := s.validateOrgMember(c, t.Assignee); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := s.db.CreateTask(ctx, orgID, &t); err != nil {
		return pgxHTTPError(err)
	}

	s.createReferencesForEntity(ctx, orgID, "task", t.Identifier, t.CreatedBy, req.References)

	s.searchUpsert(orgID, "task", t.Identifier, t.Title, t.Identifier+" "+t.Title+" "+t.Description)

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "task",
		EntityID:   int64(t.ID),
		Action:     "create",
		ChangedBy:  t.CreatedBy,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  t.CreatedBy,
		Action: "task_created",
		Detail: fmt.Sprintf("Task: %s assigned to %s", t.Title, t.Assignee),
	})

	// Email assignee if set
	if t.Assignee != "" && s.mailer != nil && s.mailer.Enabled() {
		baseURL := os.Getenv("ISMS_BASE_URL")
		_ = s.mailer.SendTaskAssigned(t.Assignee, t.Assignee, t.CreatedBy, t.Title, t.Priority, baseURL)
	}

	return c.JSON(http.StatusCreated, t)
}

func (s *Server) handleUpdateTaskStatus(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid task id")
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := validateEnum("status", req.Status, db.TaskStatuses); err != nil {
		return err
	}
	before, err := s.db.GetTask(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "task not found")
	}
	if err := s.db.UpdateTaskStatus(ctx, orgID, id, req.Status); err != nil {
		return pgxHTTPError(err)
	}
	after, err := s.db.GetTask(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	actor := getUserEmail(c)
	if changes := db.DiffFields("task", int64(id), actor, "", before.ToChangeMap(), after.ToChangeMap()); len(changes) > 0 {
		_ = s.db.LogChanges(ctx, orgID, changes)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "task_status_changed",
		Detail: fmt.Sprintf("Task #%d status changed to %s", id, req.Status),
	})
	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

func (s *Server) handleGetTask(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid task id")
	}
	task, err := s.db.GetTask(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "task not found")
	}
	return c.JSON(http.StatusOK, task)
}

func (s *Server) handleUpdateTask(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid task id")
	}

	old, err := s.db.GetTask(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "task not found")
	}

	var req taskUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.TaskStatuses); err != nil {
			return err
		}
	}
	if req.Priority != nil {
		if err := validateEnum("priority", *req.Priority, db.TaskPriorities); err != nil {
			return err
		}
	}
	if req.TaskType != nil {
		if err := validateEnum("task_type", *req.TaskType, db.TaskTypes); err != nil {
			return err
		}
	}
	if req.Assignee != nil && *req.Assignee != "" {
		if err := s.validateOrgMember(c, *req.Assignee); err != nil {
			return err
		}
	}
	t := *old
	t.ID = id
	if req.Title != nil {
		t.Title = *req.Title
	}
	if req.Description != nil {
		t.Description = *req.Description
	}
	if req.TaskType != nil {
		t.TaskType = *req.TaskType
	}
	if req.Assignee != nil {
		t.Assignee = *req.Assignee
	}
	if req.Status != nil {
		t.Status = *req.Status
	}
	if req.Priority != nil {
		t.Priority = *req.Priority
	}
	if req.DueDate != nil {
		t.DueDate = *req.DueDate
	}
	if req.RecurrenceDays != nil {
		t.RecurrenceDays = *req.RecurrenceDays
	}
	if req.Notes != nil {
		t.Notes = *req.Notes
	}

	if err := s.db.UpdateTask(ctx, orgID, &t); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	diffs := db.DiffFields("task", int64(id), user, "", old.ToChangeMap(), t.ToChangeMap())
	_ = s.db.LogChanges(ctx, orgID, diffs)

	s.searchUpsert(orgID, "task", old.Identifier, t.Title, old.Identifier+" "+t.Title+" "+t.Description)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "task_updated",
		Detail: fmt.Sprintf("Task #%d: %s", id, t.Title),
	})

	// Reload to get computed fields
	updated, _ := s.db.GetTask(ctx, orgID, id)
	if updated != nil {
		return c.JSON(http.StatusOK, updated)
	}
	return c.JSON(http.StatusOK, t)
}

func (s *Server) handleDeleteTask(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid task id")
	}

	task, err := s.db.GetTask(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "task not found")
	}

	if err := s.db.DeleteTask(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "task",
		EntityID:   int64(id),
		Action:     "delete",
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "task_deleted",
		Detail: fmt.Sprintf("%s: %s", task.Identifier, task.Title),
	})

	s.searchRemove(orgID, "task", task.Identifier)

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Tasks (DTOs) ---

type taskCreateRequest struct {
	Title          string           `json:"title"`
	Description    string           `json:"description"`
	TaskType       string           `json:"task_type"`
	Assignee       string           `json:"assignee"`
	Status         string           `json:"status"`
	Priority       string           `json:"priority"`
	DueDate        *db.Epoch        `json:"due_date"`
	RecurrenceDays *int             `json:"recurrence_days"`
	Notes          string           `json:"notes"`
	References     []ReferenceInput `json:"references"`
}

type taskUpdateRequest struct {
	Title          *string    `json:"title"`
	Description    *string    `json:"description"`
	TaskType       *string    `json:"task_type"`
	Assignee       *string    `json:"assignee"`
	Status         *string    `json:"status"`
	Priority       *string    `json:"priority"`
	DueDate        **db.Epoch `json:"due_date"`
	RecurrenceDays **int      `json:"recurrence_days"`
	Notes          *string    `json:"notes"`
}

// --- Change Requests ---

// changeCreateRequest is the API contract for creating a change request.
type changeCreateRequest struct {
	Title         string           `json:"title"`
	Description   string           `json:"description"`
	Justification string           `json:"justification"`
	Priority      string           `json:"priority"`
	Category      string           `json:"category"`
	RiskLevel     string           `json:"risk_level"`
	RollbackPlan  string           `json:"rollback_plan"`
	Notes         string           `json:"notes"`
	AssignedTo    string           `json:"assigned_to"`
	Status        string           `json:"status"`
	PlannedAt     *db.Epoch        `json:"planned_at"`
	References    []ReferenceInput `json:"references"`
}

// changeUpdateRequest is the API contract for updating a change request. nil = leave alone.
// Status, when present, is routed through UpdateChangeRequestStatus so closure
// metadata (approved_at, approved_by, implemented_at) is cleared correctly on
// reverse transitions — never inline-set via UpdateChangeRequest.
type changeUpdateRequest struct {
	Title         *string    `json:"title"`
	Description   *string    `json:"description"`
	Justification *string    `json:"justification"`
	Priority      *string    `json:"priority"`
	Category      *string    `json:"category"`
	RiskLevel     *string    `json:"risk_level"`
	Status        *string    `json:"status"`
	RollbackPlan  *string    `json:"rollback_plan"`
	Notes         *string    `json:"notes"`
	AssignedTo    *string    `json:"assigned_to"`
	PlannedAt     **db.Epoch `json:"planned_at"`
}

func (s *Server) handleListChanges(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.ChangeRequestListParams{
		Page:     page,
		Limit:    limit,
		Sort:     c.QueryParam("sort"),
		Search:   c.QueryParam("q"),
		Status:   c.QueryParam("status"),
		Priority: c.QueryParam("priority"),
		Category: c.QueryParam("category"),
		Assignee: c.QueryParam("assignee"),
	}
	items, total, err := s.db.PaginatedChangeRequests(c.Request().Context(), orgID, params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit < 1 {
		params.Limit = 50
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      items,
		"total":     total,
		"page":      params.Page,
		"page_size": params.Limit,
	})
}

func (s *Server) handleChangeStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.ChangeRequestStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleGetChange(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid change id")
	}
	cr, err := s.db.GetChangeRequest(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "change request not found")
	}
	return c.JSON(http.StatusOK, cr)
}

func (s *Server) handleCreateChange(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	var req changeCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	cr := db.ChangeRequest{
		Title:         req.Title,
		Description:   req.Description,
		Justification: req.Justification,
		Priority:      req.Priority,
		Category:      req.Category,
		RiskLevel:     req.RiskLevel,
		RollbackPlan:  req.RollbackPlan,
		Notes:         req.Notes,
		AssignedTo:    req.AssignedTo,
		Status:        req.Status,
		PlannedAt:     req.PlannedAt,
	}
	cr.RequestedBy = getUserEmail(c) // always use authenticated user
	if cr.AssignedTo == "" {
		cr.AssignedTo = cr.RequestedBy
	}
	if cr.Status == "" {
		cr.Status = "proposed"
	}
	if cr.Priority == "" {
		cr.Priority = "medium"
	}
	if cr.Category == "" {
		cr.Category = "process"
	}
	if cr.RiskLevel == "" {
		cr.RiskLevel = "low"
	}
	if err := validateEnum("status", cr.Status, db.ChangeStatuses); err != nil {
		return err
	}
	if err := validateEnum("priority", cr.Priority, db.ChangePriorities); err != nil {
		return err
	}
	if err := validateEnum("category", cr.Category, db.ChangeCategories); err != nil {
		return err
	}
	if err := validateEnum("risk_level", cr.RiskLevel, db.ChangeRiskLevels); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := s.db.CreateChangeRequest(ctx, orgID, &cr); err != nil {
		return pgxHTTPError(err)
	}

	s.createReferencesForEntity(ctx, orgID, "change_request", cr.Identifier, cr.RequestedBy, req.References)
	if out, err := s.db.GetChangeRequest(ctx, orgID, cr.ID); err == nil {
		cr = *out
	}

	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "change_request",
		EntityID:   int64(cr.ID),
		Action:     "create",
		ChangedBy:  cr.RequestedBy,
	})

	s.searchUpsert(orgID, "change", cr.Identifier, cr.Title, cr.Identifier+" "+cr.Title+" "+cr.Description)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  cr.RequestedBy,
		Action: "change_requested",
		Detail: fmt.Sprintf("Change request: %s", cr.Title),
	})
	return c.JSON(http.StatusCreated, cr)
}

func (s *Server) handleUpdateChange(c echo.Context) error {
	if err := requireRole(c, "admin", "manager", "contributor"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid change id")
	}

	old, err := s.db.GetChangeRequest(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "change request not found")
	}
	oldMap := old.ToChangeMap()

	var req changeUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Priority != nil {
		if err := validateEnum("priority", *req.Priority, db.ChangePriorities); err != nil {
			return err
		}
	}
	if req.Category != nil {
		if err := validateEnum("category", *req.Category, db.ChangeCategories); err != nil {
			return err
		}
	}
	if req.RiskLevel != nil {
		if err := validateEnum("risk_level", *req.RiskLevel, db.ChangeRiskLevels); err != nil {
			return err
		}
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.ChangeStatuses); err != nil {
			return err
		}
	}
	if req.AssignedTo != nil && *req.AssignedTo != "" {
		if err := s.validateOrgMember(c, *req.AssignedTo); err != nil {
			return err
		}
	}
	// Route status changes through the dedicated transition function so that
	// approved_at / implemented_at are cleared correctly on reverse transitions.
	if req.Status != nil && *req.Status != old.Status {
		if err := s.db.UpdateChangeRequestStatus(ctx, orgID, id, *req.Status, getUserEmail(c)); err != nil {
			return pgxHTTPError(err)
		}
	}
	cr := *old
	if req.Title != nil {
		cr.Title = *req.Title
	}
	if req.Description != nil {
		cr.Description = *req.Description
	}
	if req.Justification != nil {
		cr.Justification = *req.Justification
	}
	if req.Priority != nil {
		cr.Priority = *req.Priority
	}
	if req.Category != nil {
		cr.Category = *req.Category
	}
	if req.RiskLevel != nil {
		cr.RiskLevel = *req.RiskLevel
	}
	if req.RollbackPlan != nil {
		cr.RollbackPlan = *req.RollbackPlan
	}
	if req.Notes != nil {
		cr.Notes = *req.Notes
	}
	if req.AssignedTo != nil {
		cr.AssignedTo = *req.AssignedTo
	}
	if req.PlannedAt != nil {
		cr.PlannedAt = *req.PlannedAt
	}
	if err := s.db.UpdateChangeRequest(ctx, orgID, id, &cr); err != nil {
		return pgxHTTPError(err)
	}
	updated, _ := s.db.GetChangeRequest(ctx, orgID, id)
	if updated == nil {
		return echo.NewHTTPError(http.StatusNotFound, "change request not found")
	}

	actor := getUserEmail(c)
	diffs := db.DiffFields("change_request", int64(id), actor, "", oldMap, updated.ToChangeMap())
	if len(diffs) > 0 {
		_ = s.db.LogChanges(ctx, orgID, diffs)
	}

	s.searchUpsert(orgID, "change", updated.Identifier, updated.Title, updated.Identifier+" "+updated.Title+" "+updated.Description)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "change_updated",
		Detail: fmt.Sprintf("%s updated", updated.Identifier),
	})

	return c.JSON(http.StatusOK, updated)
}

func (s *Server) handleUpdateChangeStatus(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid change request id")
	}

	old, _ := s.db.GetChangeRequest(ctx, orgID, id)
	oldStatus := ""
	if old != nil {
		oldStatus = old.Status
	}

	var req struct {
		Status     string `json:"status"`
		ApprovedBy string `json:"approved_by"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := validateEnum("status", req.Status, db.ChangeStatuses); err != nil {
		return err
	}
	req.ApprovedBy = getUserEmail(c) // always use authenticated user
	if err := s.db.UpdateChangeRequestStatus(ctx, orgID, id, req.Status, req.ApprovedBy); err != nil {
		return pgxHTTPError(err)
	}
	updated, _ := s.db.GetChangeRequest(ctx, orgID, id)
	if updated == nil {
		return echo.NewHTTPError(http.StatusNotFound, "change request not found")
	}

	actor := getUserEmail(c)
	if oldStatus != req.Status {
		ov := oldStatus
		nv := req.Status
		_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
			EntityType: "change_request",
			EntityID:   int64(id),
			Action:     "update",
			Field:      "status",
			OldValue:   &ov,
			NewValue:   &nv,
			ChangedBy:  actor,
		})
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "change_status_changed",
		Detail: fmt.Sprintf("Change #%d status changed to %s", id, req.Status),
	})

	// Auto-create implementation task on approval, assigned to the change requester
	if req.Status == "approved" && oldStatus != "approved" {
		assignee := updated.AssignedTo
		if assignee == "" {
			assignee = updated.RequestedBy
		}
		due := db.NewEpoch(time.Now().AddDate(0, 0, 14))
		t := &db.Task{
			Title:       fmt.Sprintf("Implement %s: %s", updated.Identifier, updated.Title),
			Description: fmt.Sprintf("Auto-created from approved change %s. Verify implementation and mark this task done.", updated.Identifier),
			TaskType:    "change_followup",
			Assignee:    assignee,
			CreatedBy:   actor,
			Status:      "open",
			Priority:    "medium",
			DueDate:     &due,
		}
		if err := s.db.CreateTask(ctx, orgID, t); err == nil {
			_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
				EntityType: "task",
				EntityID:   int64(t.ID),
				Action:     "create",
				ChangedBy:  actor,
			})
			s.searchUpsert(orgID, "task", t.Identifier, t.Title, t.Identifier+" "+t.Title+" "+t.Description)
			s.logAndNotify(ctx, orgID, &db.Activity{
				Actor:  actor,
				Action: "task_created",
				Detail: fmt.Sprintf("Auto-task %s for approved change %s", t.Identifier, updated.Identifier),
			})
			if assignee != "" && s.mailer != nil && s.mailer.Enabled() {
				baseURL := os.Getenv("ISMS_BASE_URL")
				_ = s.mailer.SendTaskAssigned(assignee, assignee, actor, t.Title, t.Priority, baseURL)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": req.Status})
}

func (s *Server) handleDeleteChange(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid change request id")
	}

	cr, err := s.db.GetChangeRequest(ctx, orgID, id)
	if err != nil || cr == nil {
		return echo.NewHTTPError(http.StatusNotFound, "change request not found")
	}

	if err := s.db.DeleteChangeRequest(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}

	user := getUserEmail(c)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "change_request",
		EntityID:   int64(id),
		Action:     "delete",
		ChangedBy:  user,
	})
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user,
		Action: "change_deleted",
		Detail: fmt.Sprintf("%s: %s", cr.Identifier, cr.Title),
	})

	s.searchRemove(orgID, "change", cr.Identifier)

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Implementation Status ---

func (s *Server) handleListImplementation(c echo.Context) error {
	orgID := getOrgID(c)
	itemType := c.QueryParam("type")
	status := c.QueryParam("status")
	items, err := s.db.ListImplementationStatus(c.Request().Context(), orgID, itemType, status)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": items})
}

// implementationUpdateRequest is the body schema for PUT
// /documents/:itemId/implementation. Server-managed fields (ID,
// OrganizationID, ItemID, UpdatedAt) are deliberately omitted — ItemID
// comes from the URL param, and the rest are derived in the DB.
type implementationUpdateRequest struct {
	ItemType   string    `json:"item_type"`
	Status     string    `json:"status"`
	Owner      string    `json:"owner"`
	TargetDate *db.Epoch `json:"target_date"`
	Notes      string    `json:"notes"`
}

// implementationStatuses are the allowed values for ImplementationStatus.Status.
// Mirrors the schema CHECK constraint so we can return 400 instead of 23514.
var implementationStatuses = []string{"not_started", "in_progress", "implemented", "verified"}

func (s *Server) handleUpdateImplementation(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	itemID := c.Param("itemId")
	if itemID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "itemId is required")
	}

	// Verify the item_id resolves to an actual document in this org's repo.
	// Falls through silently if the store can't be opened — implementation
	// rows are still org-scoped via the (org_id, item_id) WHERE clause.
	if st, stErr := s.storeForOrg(ctx, orgID); stErr == nil {
		if path := st.FindDocumentByID(itemID); path == "" {
			return echo.NewHTTPError(http.StatusNotFound, "document not found: "+itemID)
		}
	}

	var req implementationUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := validateEnum("status", req.Status, implementationStatuses); err != nil {
		return err
	}
	if req.Owner != "" {
		if err := s.validateOrgMember(c, req.Owner); err != nil {
			return err
		}
	}

	impl := db.ImplementationStatus{
		ItemID:     itemID, // from URL, not body
		ItemType:   req.ItemType,
		Status:     req.Status,
		Owner:      req.Owner,
		TargetDate: req.TargetDate,
		Notes:      req.Notes,
	}
	if err := s.db.UpsertImplementationStatus(ctx, orgID, &impl); err != nil {
		return pgxHTTPError(err)
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: itemID,
		Actor:      getUserEmail(c),
		Action:     "implementation_updated",
		Detail:     fmt.Sprintf("%s implementation status: %s", itemID, impl.Status),
	})
	return c.JSON(http.StatusOK, impl)
}

// statusCounts holds progress counts for a single document type.
type statusCounts struct {
	Total       int `json:"total"`
	NotStarted  int `json:"not_started"`
	InProgress  int `json:"in_progress"`
	Implemented int `json:"implemented"`
	Verified    int `json:"verified"`
}

// mapFrontmatterStatus converts git frontmatter status to implementation status.
func mapFrontmatterStatus(s string) string {
	switch s {
	case "draft", "":
		return "not_started"
	case "in_review":
		return "in_progress"
	case "approved":
		return "implemented"
	case "verified":
		return "verified"
	default:
		return "not_started"
	}
}

func (sc *statusCounts) add(status string) {
	sc.Total++
	switch status {
	case "not_started":
		sc.NotStarted++
	case "in_progress":
		sc.InProgress++
	case "implemented":
		sc.Implemented++
	case "verified":
		sc.Verified++
	default:
		sc.NotStarted++
	}
}

func (s *Server) handleImplementationProgress(c echo.Context) error {
	orgID := getOrgID(c)
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Build a map of DB overrides keyed by item_id.
	dbOverrides := map[string]string{}
	items, err := s.db.ListImplementationStatus(c.Request().Context(), orgID, "", "")
	if err == nil {
		for _, item := range items {
			dbOverrides[item.ItemID] = item.Status
		}
	}

	// Walk all document folders dynamically from git store.
	folderCounts := map[string]statusCounts{}
	for _, folder := range st.ListDocFolders() {
		docs, _ := st.LoadDocumentsFromDir(folder)
		sc := statusCounts{}
		for _, d := range docs {
			status := mapFrontmatterStatus(d.Frontmatter.Status)
			if override, ok := dbOverrides[d.Frontmatter.DocumentID]; ok {
				status = override
			}
			sc.add(status)
		}
		folderCounts[folder] = sc
	}

	// Aggregate totals.
	var total, notStarted, inProgress, implemented, verified int
	for _, sc := range folderCounts {
		total += sc.Total
		notStarted += sc.NotStarted
		inProgress += sc.InProgress
		implemented += sc.Implemented
		verified += sc.Verified
	}

	pct := 0
	if total > 0 {
		pct = ((implemented + verified) * 100) / total
	}

	// Build by_type map using folder names as keys.
	byType := map[string]statusCounts{}
	for folder, sc := range folderCounts {
		// Use singular form as key (strip trailing 's' if present)
		key := strings.TrimSuffix(folder, "s")
		byType[key] = sc
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total":       total,
		"not_started": notStarted,
		"in_progress": inProgress,
		"implemented": implemented,
		"verified":    verified,
		"percent":     pct,
		"by_type":     byType,
	})
}

// handleLogo serves the org's logo from blob store.
func (s *Server) handleLogo(c echo.Context) error {
	return s.serveBrandingFileWithFallback(c, "branding/logo.svg", "branding/logo.png")
}

func (s *Server) handleFavicon(c echo.Context) error {
	return s.serveBrandingFileWithFallback(c, "branding/favicon.png", "branding/favicon.ico")
}

func (s *Server) serveBrandingFileWithFallback(c echo.Context, paths ...string) error {
	for _, p := range paths {
		if err := s.serveBrandingFile(c, p); err == nil {
			return nil
		}
	}
	return c.NoContent(http.StatusNotFound)
}

func (s *Server) serveBrandingFile(c echo.Context, blobPath string) error {
	orgUUID := s.resolveOrgUUID(c)
	if orgUUID == "" {
		return c.NoContent(http.StatusNotFound)
	}
	ctx := c.Request().Context()
	data, err := s.blobs.Get(ctx, orgUUID, blobPath)
	if err != nil || len(data) == 0 {
		return fmt.Errorf("not found: %s", blobPath)
	}
	ct := "application/octet-stream"
	if strings.HasSuffix(blobPath, ".png") {
		ct = "image/png"
	} else if strings.HasSuffix(blobPath, ".ico") {
		ct = "image/x-icon"
	} else if strings.HasSuffix(blobPath, ".svg") {
		ct = "image/svg+xml"
	} else if strings.HasSuffix(blobPath, ".jpg") || strings.HasSuffix(blobPath, ".jpeg") {
		ct = "image/jpeg"
	}
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	return c.Blob(http.StatusOK, ct, data)
}

// resolveOrgUUID returns the org UUID from auth context, query param, or single-org auto-detect.
func (s *Server) resolveOrgUUID(c echo.Context) string {
	ctx := c.Request().Context()
	// 1. From auth context (soft-auth or full auth)
	if orgID := getOrgID(c); orgID > 0 {
		if org, err := s.db.GetOrganization(ctx, orgID); err == nil {
			return org.UUID
		}
	}
	// 2. From query param ?org=slug
	if slug := c.QueryParam("org"); slug != "" {
		if org, err := s.db.GetOrganizationBySlug(ctx, slug); err == nil {
			return org.UUID
		}
	}
	// 3. Single-org auto-detect
	if orgs, err := s.db.ListOrganizations(ctx); err == nil && len(orgs) == 1 {
		return orgs[0].UUID
	}
	return ""
}

func (s *Server) handleTerms(c echo.Context) error {
	return s.serveLegalDoc(c, s.termsFile, "Terms of Service")
}

func (s *Server) handlePrivacy(c echo.Context) error {
	return s.serveLegalDoc(c, s.privacyFile, "Privacy Policy")
}

func (s *Server) serveLegalDoc(c echo.Context, filePath, fallbackTitle string) error {
	if filePath == "" {
		return c.HTML(http.StatusNotFound, "<h1>Not configured</h1>")
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return c.HTML(http.StatusNotFound, "<h1>Not found</h1>")
	}

	content := string(raw)
	title := fallbackTitle
	version := ""
	effectiveDate := ""

	// Strip YAML frontmatter and extract metadata
	if strings.HasPrefix(content, "---\n") {
		if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
			fm := content[4 : 4+end]
			content = content[4+end+5:]
			// Parse frontmatter fields
			for _, line := range strings.Split(fm, "\n") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(strings.Trim(parts[1], "\"'"))
				switch key {
				case "title":
					title = val
				case "version":
					version = val
				case "effective_date":
					effectiveDate = val
				}
			}
		}
	}

	// Render markdown to HTML
	var buf bytes.Buffer
	md := goldmark.New()
	if err := md.Convert([]byte(content), &buf); err != nil {
		return c.HTML(http.StatusInternalServerError, "<h1>Render error</h1>")
	}

	// Version/date banner
	meta := ""
	if version != "" || effectiveDate != "" {
		parts := []string{}
		if version != "" {
			parts = append(parts, "Version "+version)
		}
		if effectiveDate != "" {
			parts = append(parts, "Effective "+effectiveDate)
		}
		meta = fmt.Sprintf(`<p style="color:#64748b;font-size:0.875rem;margin-bottom:2rem;">%s</p>`, strings.Join(parts, " · "))
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 800px; margin: 40px auto; padding: 0 20px; line-height: 1.6; color: #e2e8f0; background: #0f172a; }
h1, h2, h3 { color: #f1f5f9; }
a { color: #60a5fa; }
table { border-collapse: collapse; width: 100%%; }
th, td { border: 1px solid #334155; padding: 8px 12px; text-align: left; }
th { background: #1e293b; }
code { background: #1e293b; padding: 2px 6px; border-radius: 4px; }
</style>
</head>
<body>%s%s</body>
</html>`, title, meta, buf.String())

	return c.HTML(http.StatusOK, html)
}

// --- Notifications ---

func (s *Server) handleListNotifications(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "user not found")
	}
	unreadOnly := c.QueryParam("unread") == "true"
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}
	notifications, err := s.db.ListNotifications(ctx, orgID, user.ID, unreadOnly, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": notifications})
}

func (s *Server) handleMarkRead(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid notification id")
	}
	// Resolve authenticated user's ID for recipient check
	email := getUserEmail(c)
	user, userErr := s.db.GetUserByEmail(c.Request().Context(), email)
	if userErr != nil || user == nil {
		return echo.NewHTTPError(http.StatusForbidden, "user not found")
	}
	if err := s.db.MarkRead(c.Request().Context(), orgID, id, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "read"})
}

func (s *Server) handleMarkAllRead(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "user not found")
	}
	if err := s.db.MarkAllRead(ctx, orgID, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleUnreadCount(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "user not found")
	}
	count, err := s.db.UnreadCount(ctx, orgID, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// --- Activity ---

func (s *Server) handleListActivity(c echo.Context) error {
	orgID := getOrgID(c)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}
	activities, err := s.db.RecentActivity(c.Request().Context(), orgID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": activities})
}

func (s *Server) handleDocumentActivity(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}
	activities, err := s.db.ActivityForDocument(c.Request().Context(), orgID, docID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": activities})
}

// --- Inbox ---

// inboxItem is the unified inbox item returned by handleInbox.
type inboxItem struct {
	Type       string   `json:"type"`
	ID         int      `json:"id"`
	DocumentID string   `json:"document_id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	From       string   `json:"from"`
	CreatedAt  db.Epoch `json:"created_at"`
}

func (s *Server) handleInbox(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)
	if actor == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user email required")
	}

	var items []inboxItem

	// Open reviews requested by this user with open comments
	reviews, _ := s.db.ListReviews(ctx, orgID, "open", 50)
	for _, r := range reviews {
		if r.RequestedBy == actor && r.OpenComments > 0 {
			items = append(items, inboxItem{
				Type: "review", ID: r.ID, DocumentID: r.DocumentID,
				Title: r.Title, Status: r.Status, From: r.RequestedBy, CreatedAt: r.CreatedAt,
			})
		}
	}

	// Changes-requested reviews owned by this user
	changesRequested, _ := s.db.ListReviews(ctx, orgID, "changes_requested", 50)
	for _, r := range changesRequested {
		if r.RequestedBy == actor {
			items = append(items, inboxItem{
				Type: "review", ID: r.ID, DocumentID: r.DocumentID,
				Title: r.Title, Status: r.Status, From: r.RequestedBy, CreatedAt: r.CreatedAt,
			})
		}
	}

	// Open tasks assigned to this user
	tasks, _ := s.db.ListTasks(ctx, orgID, actor, "open", 50)
	inProgressTasks, _ := s.db.ListTasks(ctx, orgID, actor, "in_progress", 50)
	tasks = append(tasks, inProgressTasks...)
	for _, t := range tasks {
		items = append(items, inboxItem{
			Type: "task", ID: t.ID,
			Title: t.Title, Status: t.Status, From: t.CreatedBy, CreatedAt: t.CreatedAt,
		})
	}

	// All open comments on documents the user has reviewed
	allComments, _ := s.db.AllOpenComments(ctx, orgID)
	for _, cm := range allComments {
		items = append(items, inboxItem{
			Type: "comment", ID: cm.ID, DocumentID: cm.DocumentID,
			Title: cm.Body, Status: cm.Status, From: cm.Author, CreatedAt: cm.CreatedAt,
		})
	}

	if items == nil {
		items = []inboxItem{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": items})
}

// handleInboxDump returns the same JSON structure as `isms inbox dump` for Claude consumption.
func (s *Server) handleInboxDump(c echo.Context) error {
	orgID := getOrgID(c)
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	actor := getUserEmail(c)
	if actor == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user email required")
	}

	type replyInfo struct {
		ID     int    `json:"id"`
		Author string `json:"author"`
		Body   string `json:"body"`
	}

	type commentInfo struct {
		ID             int         `json:"id"`
		DocumentID     string      `json:"document_id"`
		DocumentTitle  string      `json:"document_title,omitempty"`
		FilePath       string      `json:"file_path,omitempty"`
		Author         string      `json:"author"`
		Body           string      `json:"body"`
		Section        string      `json:"section,omitempty"`
		ParagraphIndex *int        `json:"paragraph_index,omitempty"`
		Quote          string      `json:"quote,omitempty"`
		Replies        []replyInfo `json:"replies,omitempty"`
	}

	type reviewInfo struct {
		ID           int    `json:"id"`
		DocumentID   string `json:"document_id"`
		Title        string `json:"title"`
		Version      string `json:"version"`
		Status       string `json:"status"`
		OpenComments int    `json:"open_comments"`
	}

	type taskInfo struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
		TaskType    string `json:"task_type"`
		Priority    string `json:"priority"`
		Status      string `json:"status"`
	}

	type inboxDump struct {
		User     string        `json:"user"`
		Reviews  []reviewInfo  `json:"reviews"`
		Comments []commentInfo `json:"comments"`
		Tasks    []taskInfo    `json:"tasks"`
	}

	dump := inboxDump{User: actor}

	// All open comments — enriched with file paths, titles, and replies
	allOpenComments, _ := s.db.AllOpenComments(ctx, orgID)

	// Load all comments per document to find replies
	allDocComments := map[string][]db.Comment{}
	for _, cm := range allOpenComments {
		if _, loaded := allDocComments[cm.DocumentID]; !loaded {
			docComments, _ := s.db.CommentsForDocument(ctx, orgID, cm.DocumentID)
			allDocComments[cm.DocumentID] = docComments
		}
	}

	for _, cm := range allOpenComments {
		// Skip replies — they'll be nested under their parent
		if cm.ParentID != nil {
			continue
		}

		ci := commentInfo{
			ID: cm.ID, DocumentID: cm.DocumentID, Author: cm.Author,
			Body: cm.Body, Section: cm.Section, Quote: cm.Quote,
		}
		if cm.ParagraphIndex != nil {
			ci.ParagraphIndex = cm.ParagraphIndex
		}

		// Resolve file path and title from store
		ci.FilePath = resolveDocPathFromStore(st, cm.DocumentID)
		ci.DocumentTitle = resolveDocTitle(st, cm.DocumentID)

		// Find replies to this comment (thread)
		for _, reply := range allDocComments[cm.DocumentID] {
			if reply.ParentID != nil && *reply.ParentID == cm.ID {
				ci.Replies = append(ci.Replies, replyInfo{
					ID: reply.ID, Author: reply.Author, Body: reply.Body,
				})
			}
		}

		dump.Comments = append(dump.Comments, ci)
	}

	// Reviews with open/changes_requested status owned by actor
	reviews, _ := s.db.ListReviews(ctx, orgID, "", 100)
	for _, r := range reviews {
		if r.RequestedBy != actor {
			continue
		}
		if r.Status != "open" && r.Status != "changes_requested" {
			continue
		}
		dump.Reviews = append(dump.Reviews, reviewInfo{
			ID: r.ID, DocumentID: r.DocumentID, Title: r.Title,
			Version: r.Version, Status: r.Status, OpenComments: r.OpenComments,
		})
	}

	// Tasks assigned to actor
	tasks, _ := s.db.ListTasks(ctx, orgID, actor, "open", 50)
	inProgress, _ := s.db.ListTasks(ctx, orgID, actor, "in_progress", 50)
	tasks = append(tasks, inProgress...)
	for _, t := range tasks {
		dump.Tasks = append(dump.Tasks, taskInfo{
			ID: t.ID, Title: t.Title, Description: t.Description,
			TaskType: t.TaskType,
			Priority: t.Priority, Status: t.Status,
		})
	}

	// Ensure non-nil slices for clean JSON
	if dump.Reviews == nil {
		dump.Reviews = []reviewInfo{}
	}
	if dump.Comments == nil {
		dump.Comments = []commentInfo{}
	}
	if dump.Tasks == nil {
		dump.Tasks = []taskInfo{}
	}

	return c.JSON(http.StatusOK, dump)
}

// resolveDocTitle returns a human-readable title for a document ID from the store.
func resolveDocTitle(st *store.Store, docID string) string {
	switch docID {
	case "suppliers":
		return "Supplier Register"
	case "risks":
		return "Risk Register"
	case "assets":
		return "Asset Register"
	}

	// Search all document folders for matching document_id.
	docPath := st.FindDocumentByID(docID)
	if docPath != "" {
		if pf, err := st.LoadDocument(docPath); err == nil && pf != nil {
			return pf.Frontmatter.Title
		}
	}
	return docID
}

// --- Agent Pending Actions ---

// handleAgentPendingActions returns reviews and tasks the authenticated user should act on.
// For writer agents: reviews in changes_requested where they are the author.
// For reviewer agents: reviews with pending assignments for them.
func (s *Server) handleAgentPendingActions(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)

	// Reviews where I'm author and status is changes_requested (I need to resubmit)
	authorReviews, _ := s.db.ListReviewsByAuthorStatus(ctx, orgID, actor, "changes_requested")

	// Reviews where I'm assigned reviewer and my assignment is pending (I need to review)
	reviewerPending, _ := s.db.ListPendingAssignmentsForReviewer(ctx, orgID, actor)

	// Suggestions I created that are awaiting review
	mySuggestions, _ := s.db.ListSuggestions(ctx, orgID, db.SuggestionFilters{Status: "open", SuggestedBy: actor, Limit: 20})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"reviews_awaiting_resubmit": authorReviews,
		"reviews_awaiting_review":   reviewerPending,
		"my_open_suggestions":       mySuggestions,
	})
}

// --- Confirm Document Review (lightweight annual review evidence) ---

// handleConfirmDocumentReview records that a document was reviewed and found still valid.
// Creates audit evidence without requiring a full review cycle.
// Used for annual/periodic reviews where no changes are needed.
func (s *Server) handleConfirmDocumentReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		// Contributors can confirm only if they are the document owner
		if err2 := requireRole(c, "contributor"); err2 != nil {
			return err
		}
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)
	docID := c.Param("docId")

	// Verify actor is document owner or admin/manager
	role, _ := c.Get("user_role").(string)
	if role == "contributor" {
		// Contributors must be the document owner
		if st, stErr := s.storeForOrg(ctx, orgID); stErr == nil {
			if filePath := resolveDocPathFromStore(st, docID); filePath != "" {
				if doc, loadErr := st.LoadDocument(filePath); loadErr == nil && doc != nil {
					if doc.Frontmatter.Owner != "" && doc.Frontmatter.Owner != actor {
						return echo.NewHTTPError(http.StatusForbidden, "only the document owner or a manager can confirm review")
					}
				}
			}
		}
	}

	var req struct {
		Comment string `json:"comment"`
	}
	_ = c.Bind(&req)
	if req.Comment == "" {
		req.Comment = "Reviewed — no changes required"
	}

	// Load document from git to get current version and path
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "opening repository")
	}

	filePath := resolveDocPathFromStore(st, docID)
	if filePath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	doc, err := st.LoadDocument(filePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "loading document")
	}

	version := doc.Frontmatter.Version
	if version == "" {
		version = "1.0"
	}

	// Prepare records before starting the transaction.
	approval := &db.Approval{
		DocumentID: docID,
		Version:    version,
		Round:      1,
		Decision:   "confirmed",
		ApprovedBy: actor,
		Comment:    req.Comment,
	}

	decRec := &db.DecisionRecord{
		DocumentID: docID,
		Decision:   "confirmed",
		DecidedBy:  actor,
		Version:    version,
		Comment:    req.Comment,
	}
	if u, _ := s.db.GetUserByEmail(ctx, actor); u != nil {
		decRec.DecidedByID = &u.ID
	}
	if raw, readErr := st.ReadFile(filePath); readErr == nil {
		h := sha256.Sum256(raw)
		decRec.ContentHash = hex.EncodeToString(h[:])
	}

	// Prepare version snapshot
	commitHash, _ := st.HeadHash()
	var ver *db.DocumentVersion
	if commitHash != "" {
		ver = &db.DocumentVersion{
			DocumentID: docID,
			Version:    version,
			CommitHash: commitHash,
			FilePath:   filePath,
			Message:    "Annual review: " + req.Comment,
			CreatedBy:  actor,
		}
		if raw, readErr := st.ReadFile(filePath); readErr == nil {
			h := sha256.Sum256(raw)
			ver.ContentHash = hex.EncodeToString(h[:])
		}
		if doc.Frontmatter.Owner != "" {
			ver.Owner = doc.Frontmatter.Owner
		}
		if doc.Frontmatter.ReviewCycle > 0 {
			rc := doc.Frontmatter.ReviewCycle
			ver.ReviewCycleMonths = &rc
		}
	}

	// All DB writes in a single transaction with RLS: approval, decision record, version snapshot.
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.AddApprovalTx(ctx, tx, orgID, approval); err != nil {
			return err
		}

		if err := db.CreateDecisionRecordTx(ctx, tx, orgID, decRec); err != nil {
			return fmt.Errorf("failed to create decision record: %w", err)
		}

		if ver != nil {
			if err := db.RecordVersionTx(ctx, tx, orgID, ver); err != nil {
				return fmt.Errorf("failed to record version: %w", err)
			}
		}

		return nil
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	// Activity log
	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: docID,
		Actor:      actor,
		Action:     "document_review_confirmed",
		Detail:     fmt.Sprintf("%s confirmed review of %s v%s: %s", actor, docID, version, req.Comment),
	})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":      "confirmed",
		"document_id": docID,
		"version":     version,
		"reviewed_by": actor,
		"comment":     req.Comment,
	})
}

// --- Review Send (compound operation) ---

func (s *Server) handleReviewSend(c echo.Context) error {
	orgID := getOrgID(c)

	// Only manager/admin can send documents for review
	role, _ := c.Get("user_role").(string)
	if role != "manager" && role != "admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only managers and admins can send documents for review")
	}

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	docID := c.Param("docId")
	actor := getUserEmail(c)
	if actor == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user email required")
	}

	var req struct {
		Reviewers []string `json:"reviewers"`
		Message   string   `json:"message"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// a. Resolve document from store (title, version, type)
	title, version, docType, err := resolveDocument(st, docID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// b. Find the base commit for diff — last merged review's merge_commit, or empty for first review
	commitHash := ""
	lastApproved, _ := s.db.GetLastApprovedReview(ctx, orgID, docID)
	if lastApproved != nil {
		// Prefer merge_commit (post-merge state) over commit_hash (pre-review state)
		if lastApproved.MergeCommit != "" {
			commitHash = lastApproved.MergeCommit
		} else if lastApproved.CommitHash != "" {
			commitHash = lastApproved.CommitHash
		}
	}

	// c. Check if there's already an open review for this document
	existingReview, existErr := s.db.GetOpenReviewForDocument(ctx, orgID, docID)
	if existErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("checking existing reviews: %v", existErr))
	}
	if existingReview != nil {
		if existingReview.Status == "changes_requested" {
			// Resubmission after changes_requested: new review round on the same review.
			// Update document status back to in_review.
			if err := setDocumentStatusAndVersion(st, docID, "in_review", "", actor, actor); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("setting document status: %v", err))
			}
			// Capture new HEAD as the resubmission snapshot.
			newSentHead, _ := st.HeadHash()

			// Steps 1-3 must be atomic with RLS: update review, reset assignments, add new reviewers.
			txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
				// 1. Mark comments outdated, update review: new sent_head, status back to open, round++.
				if err := db.MarkCommentsOutdatedTx(ctx, tx, orgID, existingReview.ID); err != nil {
					return fmt.Errorf("marking comments outdated: %w", err)
				}
				if err := db.ResubmitReviewTx(ctx, tx, orgID, existingReview.ID, newSentHead); err != nil {
					return fmt.Errorf("updating review: %w", err)
				}
				// 2. Reset all existing assignment statuses back to pending.
				if err := db.ResetAssignmentsTx(ctx, tx, orgID, existingReview.ID); err != nil {
					return fmt.Errorf("resetting assignments: %w", err)
				}
				// 3. Add any new reviewers that weren't already assigned.
				for _, reviewer := range req.Reviewers {
					if err := db.AddReviewAssignmentTx(ctx, tx, orgID, existingReview.ID, reviewer); err != nil {
						return fmt.Errorf("assigning reviewer %s: %w", reviewer, err)
					}
				}
				return nil
			})
			if txErr != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
			}
			// Notify all assigned reviewers about the resubmission.
			allAssignments, _ := s.db.ListAssignmentsForReview(ctx, orgID, existingReview.ID)
			for _, a := range allAssignments {
				notifBody := fmt.Sprintf("%s resubmitted %s (%s v%s) for review", actor, docID, title, version)
				if req.Message != "" {
					notifBody += "\n\nNote: " + req.Message
				}
				s.db.CreateNotificationByEmail(ctx, orgID, a.Reviewer,
					fmt.Sprintf("Review resubmitted: %s", title),
					notifBody, fmt.Sprintf("/reviews/%d", existingReview.ID))
				if s.mailer != nil && s.mailer.Enabled() {
					baseURL := os.Getenv("ISMS_BASE_URL")
					_ = s.mailer.SendReviewRequest(a.Reviewer, a.Reviewer, actor, docID, title, version, baseURL, existingReview.ID, req.Message)
				}
			}
			s.logAndNotify(ctx, orgID, &db.Activity{
				DocumentID: docID,
				ReviewID:   &existingReview.ID,
				Actor:      actor,
				Action:     "review_resubmitted",
				Detail:     fmt.Sprintf("Resubmitted %s v%s for review (review #%d)", docID, version, existingReview.ID),
			})
			return c.JSON(http.StatusOK, map[string]interface{}{
				"review_id": existingReview.ID,
				"version":   version,
				"round":     existingReview.Round + 1,
				"message":   "review resubmitted with changes",
			})
		}

		// Review is still open — add all new reviewers atomically with RLS.
		txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
			for _, reviewer := range req.Reviewers {
				if err := db.AddReviewAssignmentTx(ctx, tx, orgID, existingReview.ID, reviewer); err != nil {
					return fmt.Errorf("assigning reviewer %s: %w", reviewer, err)
				}
			}
			return nil
		})
		if txErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
		}
		// Post-commit: notifications (fire-and-forget)
		for _, reviewer := range req.Reviewers {
			notifBody := fmt.Sprintf("%s wants to publish %s v%s and requested your review", actor, title, version)
			if req.Message != "" {
				notifBody += "\n\nNote: " + req.Message
			}
			s.db.CreateNotificationByEmail(ctx, orgID, reviewer,
				fmt.Sprintf("Review: %s v%s", title, version),
				notifBody, fmt.Sprintf("/reviews/%d", existingReview.ID))
			if s.mailer != nil && s.mailer.Enabled() {
				baseURL := os.Getenv("ISMS_BASE_URL")
				_ = s.mailer.SendReviewRequest(reviewer, reviewer, actor, docID, title, version, baseURL, existingReview.ID, req.Message)
			}
		}
		s.logAndNotify(ctx, orgID, &db.Activity{
			DocumentID: docID,
			ReviewID:   &existingReview.ID,
			Actor:      actor,
			Action:     "reviewers_added",
			Detail:     fmt.Sprintf("Added reviewers %s to existing review #%d", strings.Join(req.Reviewers, ", "), existingReview.ID),
		})
		return c.JSON(http.StatusOK, map[string]interface{}{
			"review_id": existingReview.ID,
			"version":   existingReview.Version,
			"message":   "reviewers added to existing review",
		})
	}

	// Set document status to in_review (version unchanged — only user edits change version)
	if err := setDocumentStatusAndVersion(st, docID, "in_review", "", actor, actor); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("setting document status: %v", err))
	}

	// Capture HEAD AFTER status change — this is the immutable snapshot of what was sent
	sentHead, _ := st.HeadHash()

	// d. Create review record + assignments in a single transaction with RLS
	review := &db.Review{
		DocumentID:   docID,
		DocumentType: docType,
		Title:        title,
		Version:      version,
		CommitHash:   commitHash,
		SentHead:     sentHead,
		RequestedBy:  actor,
		Message:      req.Message,
		Status:       "open",
	}
	txErr := s.db.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		if err := db.CreateReviewTx(ctx, tx, orgID, review); err != nil {
			return fmt.Errorf("creating review: %w", err)
		}
		// f. Create review assignments for each reviewer
		for _, reviewer := range req.Reviewers {
			if err := db.AddReviewAssignmentTx(ctx, tx, orgID, review.ID, reviewer); err != nil {
				return fmt.Errorf("assigning reviewer %s: %w", reviewer, err)
			}
		}
		return nil
	})
	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, txErr.Error())
	}

	// Post-commit: notifications and emails (fire-and-forget, outside transaction)
	for _, reviewer := range req.Reviewers {
		notifBody := fmt.Sprintf("%s wants to publish %s v%s and requested your review", actor, title, version)
		if req.Message != "" {
			notifBody += "\n\nNote: " + req.Message
		}
		s.db.CreateNotificationByEmail(ctx, orgID, reviewer,
			fmt.Sprintf("Review: %s v%s", title, version),
			notifBody, fmt.Sprintf("/reviews/%d", review.ID))

		// Send email notification to reviewer
		if s.mailer != nil && s.mailer.Enabled() {
			baseURL := os.Getenv("ISMS_BASE_URL")
			_ = s.mailer.SendReviewRequest(reviewer, reviewer, actor, docID, title, version, baseURL, review.ID, req.Message)
		}
	}

	// h. Log activity
	detail := fmt.Sprintf("Created review for %s v%s", docID, version)
	if len(req.Reviewers) > 0 {
		detail += fmt.Sprintf(", assigned to %s", strings.Join(req.Reviewers, ", "))
	}
	s.logAndNotify(ctx, orgID, &db.Activity{
		DocumentID: docID,
		ReviewID:   &review.ID,
		Actor:      actor,
		Action:     "review_requested",
		Detail:     detail,
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"review_id": review.ID,
		"version":   version,
	})
}

// resolveDocument looks up a document by ID from the store and returns title, version, and type.
// Uses the cached document index for fast lookup.
func resolveDocument(st *store.Store, docID string) (title, version, docType string, err error) {
	absPath := st.FindDocumentByID(docID)
	if absPath == "" {
		return "", "", "", fmt.Errorf("document not found: %s", docID)
	}
	pf, loadErr := st.LoadDocument(absPath)
	if loadErr != nil || pf == nil {
		return "", "", "", fmt.Errorf("document not found: %s", docID)
	}
	title = pf.Frontmatter.Title
	version = pf.Frontmatter.Version

	// Determine type from the top-level folder name under documents/
	docsRoot := st.DocsRoot()
	relPath, _ := filepath.Rel(docsRoot, absPath)
	parts := strings.SplitN(relPath, string(filepath.Separator), 2)
	if len(parts) > 0 {
		// Use singular form of the folder name (strip trailing 's')
		docType = strings.TrimSuffix(parts[0], "s")
	}
	if docType == "" {
		docType = "document"
	}
	return
}

// bumpMinorVersion increments the minor version: "0.1" -> "0.2", "1.3" -> "1.4".
func bumpMinorVersion(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) == 2 {
		minor := 0
		fmt.Sscanf(parts[1], "%d", &minor)
		return parts[0] + "." + fmt.Sprintf("%d", minor+1)
	}
	if len(parts) == 1 {
		major := 0
		fmt.Sscanf(parts[0], "%d", &major)
		return fmt.Sprintf("%d.1", major)
	}
	return v + ".1"
}

// setDocumentStatusAndVersion updates frontmatter status and version for a document
// atomically in a single commit, preventing TOCTOU races.
func setDocumentStatusAndVersion(st *store.Store, docID, status, version, authorName, authorEmail string) error {
	// Find document path
	docPath := st.FindDocumentByID(docID)
	if docPath == "" {
		return fmt.Errorf("document not found: %s", docID)
	}

	fields := map[string]string{"status": status}
	if version != "" {
		fields["version"] = version
	}
	_, err := st.UpdateDocumentMetadataMulti(docPath, fields, authorName, authorEmail)
	return err
}
