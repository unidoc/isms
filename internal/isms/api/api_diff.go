package api

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/store"
)

// validGitRef matches commit hashes (7-40 hex chars) or HEAD with optional suffixes.
var validGitRef = regexp.MustCompile(`^([0-9a-fA-F]{7,40}|HEAD(~\d+)?)$`)

// handleListVersions returns all recorded versions of a document.
func (s *Server) handleListVersions(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")
	versions, err := s.db.ListVersions(c.Request().Context(), orgID, docID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if versions == nil {
		versions = []db.DocumentVersion{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": versions})
}

// handleDocumentDiff returns a diff between two versions (or current vs a version).
// Query params: ?from=commit1&to=commit2 (default to=HEAD)
func (s *Server) handleDocumentDiff(c echo.Context) error {
	orgID := getOrgID(c)
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	docID := c.Param("docId")
	from := c.QueryParam("from")
	to := c.QueryParam("to")

	// Resolve version strings to commit hashes if needed.
	ctx := c.Request().Context()
	if from != "" && len(from) < 8 {
		// Looks like a version number, resolve to commit hash.
		v, err := s.db.GetVersion(ctx, orgID, docID, from)
		if err == nil {
			from = v.CommitHash
		}
	}
	if to == "" {
		to = "HEAD"
	}
	if to != "HEAD" && len(to) < 8 {
		v, err := s.db.GetVersion(ctx, orgID, docID, to)
		if err == nil {
			to = v.CommitHash
		}
	}

	// Validate git refs to prevent flag injection
	if from != "" && !validGitRef.MatchString(from) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid 'from' ref")
	}
	if !validGitRef.MatchString(to) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid 'to' ref")
	}

	// Get file path.
	filePath := resolveDocPathFromStore(st, docID)
	if filePath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	// Compute diff using go-git.
	diffText, err := st.DiffFiles(from, to, filePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to compute diff: "+err.Error())
	}
	diffOutput := []byte(diffText)

	// Return as JSON with structured diff lines.
	type DiffLine struct {
		Type string `json:"type"` // "add", "remove", "context", "header"
		Text string `json:"text"`
	}

	var lines []DiffLine
	for _, line := range strings.Split(string(diffOutput), "\n") {
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

	return c.JSON(http.StatusOK, map[string]interface{}{
		"document_id": docID,
		"from":        from,
		"to":          to,
		"lines":       lines,
	})
}

// resolveDocPathFromStore finds a document's relative file path from the store.
// Uses the cached document ID index for fast lookup.
func resolveDocPathFromStore(s *store.Store, docID string) string {
	absPath := s.FindDocumentByID(docID)
	if absPath == "" {
		return ""
	}
	rel, err := filepath.Rel(s.Root(), absPath)
	if err != nil {
		return ""
	}
	return rel
}
