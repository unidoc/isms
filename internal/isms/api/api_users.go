package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	git "github.com/go-git/go-git/v5"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/scaffold"
	"isms.sh/internal/isms/store"
)

// slugRe enforces strict org slug format to prevent path traversal and other abuse.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

// handleMe returns the current user's profile and role.
// Creates a reader account if the user doesn't exist yet.
func (s *Server) handleMe(c echo.Context) error {
	email := getUserEmail(c)
	if email == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"email":         "",
			"name":          "Anonymous",
			"role":          "reader",
			"authenticated": false,
		})
	}

	orgID := getOrgID(c)
	ctx := c.Request().Context()

	// Find existing user — never auto-create
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}
	// Update last_seen
	s.db.TouchUser(ctx, email, user.Name)

	// Determine role: from org membership if orgID is set, otherwise fallback
	var role string
	if orgID > 0 {
		if orgRole, err := s.db.GetUserRole(ctx, orgID, user.ID); err == nil {
			role = orgRole
		}
	}
	if role == "" {
		role = "reader" // default fallback
	}

	// Check AI enabled for this org
	aiEnabled := true
	if orgID > 0 {
		if v, _ := s.db.GetOrgSetting(ctx, orgID, "ai_enabled"); v == "false" {
			aiEnabled = false
		}
	}

	resp := map[string]interface{}{
		"id":              user.ID,
		"email":           user.Email,
		"name":            user.Name,
		"role":            role,
		"is_agent":        user.IsAgent,
		"active":          user.Active,
		"authenticated":   true,
		"has_password":    user.HasPassword(),
		"otp_enabled":     user.HasOTP(),
		"email_verified":  user.EmailVerified,
		"organization_id": orgID,
		"ai_enabled":      aiEnabled,
	}

	// Include org slug for CLI sync support.
	if orgID > 0 {
		if org, err := s.db.GetOrganization(ctx, orgID); err == nil {
			resp["organization_uuid"] = org.UUID
			resp["organization_slug"] = org.Slug
			resp["organization_name"] = org.Name
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// handleListUsers returns all users in the current organization.
func (s *Server) handleListUsers(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	if orgID > 0 {
		users, err := s.db.ListOrgUsers(ctx, orgID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if users == nil {
			users = []db.UserWithRole{}
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"data": users})
	}

	// No org context — return empty array (never leak cross-org user data)
	return c.JSON(http.StatusOK, map[string]interface{}{"data": []db.UserWithRole{}})
}

// upsertUserRequest extends user creation with a separate role field for org membership.
type upsertUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// handleUpsertUser creates or updates a user and adds them to the current organization.
func (s *Server) handleUpsertUser(c echo.Context) error {
	orgID := getOrgID(c)

	// Only admin or manager can invite/update users (not contributor or reader)
	callerRole, _ := c.Get("user_role").(string)
	if callerRole != "admin" && callerRole != "manager" {
		return echo.NewHTTPError(http.StatusForbidden, "admin or manager role required to manage users")
	}

	var req upsertUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Email == "" || req.Name == "" || req.Role == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email, name, and role are required")
	}
	validRoles := map[string]bool{"admin": true, "manager": true, "contributor": true, "reader": true}
	if !validRoles[req.Role] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
	}

	// Only admin can assign admin or manager roles
	if (req.Role == "admin" || req.Role == "manager") && callerRole != "admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only admins can assign admin or manager roles")
	}

	ctx := c.Request().Context()

	// Upsert the user record (org-agnostic)
	u := &db.User{Email: req.Email, Name: req.Name, Active: true}
	if err := s.db.UpsertUser(ctx, u); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Add/update org membership with the specified role
	if orgID > 0 {
		if err := s.db.AddOrgMember(ctx, orgID, u.ID, req.Role); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "adding to organization: "+err.Error())
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":    u.ID,
		"email": u.Email,
		"name":  u.Name,
		"role":  req.Role,
	})
}

// handleMyOrganizations returns all organizations the current user belongs to.
func (s *Server) handleMyOrganizations(c echo.Context) error {
	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}
	orgs, err := s.db.ListUserOrgs(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if orgs == nil {
		orgs = []db.Organization{}
	}
	// Include role per org
	type orgWithRole struct {
		db.Organization
		Role string `json:"role"`
	}
	var result []orgWithRole
	for _, o := range orgs {
		role, _ := s.db.GetUserRole(ctx, o.ID, user.ID)
		result = append(result, orgWithRole{Organization: o, Role: role})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": result})
}

// handleCreateOrganization creates a new org and adds the current user as admin.
func (s *Server) handleCreateOrganization(c echo.Context) error {
	var req struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Template string `json:"template"` // optional: iso27001, soc2, nis2
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" || req.Slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and slug are required")
	}
	if !slugRe.MatchString(req.Slug) {
		return echo.NewHTTPError(http.StatusBadRequest, "slug must be 2-63 chars, lowercase letters/digits/hyphens only")
	}

	// Restrict org creation: if user already belongs to any org, they must be admin in at least one.
	ctx := c.Request().Context()
	email := getUserEmail(c)
	existingUser, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}
	existingOrgs, err := s.db.ListUserOrgs(ctx, existingUser.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if len(existingOrgs) > 0 {
		hasAdmin := false
		for _, o := range existingOrgs {
			if role, err := s.db.GetUserRole(ctx, o.ID, existingUser.ID); err == nil && role == "admin" {
				hasAdmin = true
				break
			}
		}
		if !hasAdmin {
			return echo.NewHTTPError(http.StatusForbidden, "only admins can create additional organizations")
		}
	}

	// Reserved slugs that conflict with subdomains/paths
	reserved := map[string]bool{
		"api": true, "app": true, "www": true, "mail": true, "smtp": true,
		"docs": true, "doc": true, "help": true, "support": true, "status": true,
		"admin": true, "git": true, "ssh": true, "ftp": true, "cdn": true,
		"static": true, "assets": true, "login": true, "auth": true, "oauth": true,
		"blog": true, "about": true, "pricing": true, "terms": true, "privacy": true,
		"dashboard": true, "overview": true, "organization": true, "organizations": true, "settings": true, "test": true, "staging": true, "dev": true,
		"ns1": true, "ns2": true, "mx": true, "autoconfig": true, "autodiscover": true,
	}
	if reserved[req.Slug] {
		return echo.NewHTTPError(http.StatusBadRequest, "this slug is reserved")
	}

	// Determine repo path — ISMS_DATA_DIR is required and resolved to absolute at startup.
	dataDir := os.Getenv("ISMS_DATA_DIR")
	if dataDir == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "ISMS_DATA_DIR not configured")
	}
	repoPath := filepath.Join(dataDir, "repos", req.Slug+".git")

	// Create org in DB
	org := &db.Organization{Name: req.Name, Slug: req.Slug, RepoPath: repoPath}
	if err := s.db.CreateOrganization(ctx, org); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating organization: "+err.Error())
	}

	// Init bare git repo on disk (skip if already exists)
	if _, err := git.PlainOpen(repoPath); err != nil {
		if _, err := git.PlainInit(repoPath, true); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "initializing repo: "+err.Error())
		}
	}

	// Bootstrap repo with a marker file (bare repos need at least one commit).
	// All runtime config comes from PostgreSQL — isms.yaml is not read by the API.
	st, stErr := store.NewBare(repoPath)
	if stErr != nil {
		log.Printf("ERROR: failed to open bare repo %s: %v", repoPath, stErr)
	} else {
		marker := fmt.Sprintf("# %s\n\nISMS repository initialized.\n", req.Name)
		if _, commitErr := st.CommitFile("README.md", []byte(marker),
			existingUser.Name, existingUser.Email, "chore: initialize ISMS repository"); commitErr != nil {
			log.Printf("ERROR: failed to commit README.md: %v", commitErr)
		}

		// Scaffold template if requested
		if req.Template != "" && scaffold.IsValidTemplate(req.Template) {
			if scaffoldErr := scaffold.ScaffoldToRepo(st, req.Template, existingUser.Name, existingUser.Email); scaffoldErr != nil {
				log.Printf("ERROR: failed to scaffold template %s: %v", req.Template, scaffoldErr)
			}
		}
	}

	// Add current user as admin
	if err := s.db.AddOrgMember(ctx, org.ID, existingUser.ID, "admin"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "adding member: "+err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"uuid": org.UUID,
		"name": org.Name,
		"slug": org.Slug,
	})
}

// emailToName converts "alfred.hall@command.is" to "Alfred Hall".
func emailToName(email string) string {
	at := 0
	for i, c := range email {
		if c == '@' {
			at = i
			break
		}
	}
	if at == 0 {
		return email
	}
	local := email[:at]
	// Replace dots and underscores with spaces, title case
	name := ""
	upper := true
	for _, c := range local {
		if c == '.' || c == '_' || c == '-' {
			name += " "
			upper = true
			continue
		}
		if upper {
			if c >= 'a' && c <= 'z' {
				c -= 32
			}
			upper = false
		}
		name += string(c)
	}
	return name
}

// handleListAvailableTemplates returns all templates available in the registry.
// GET /api/v1/templates/available
func (s *Server) handleListAvailableTemplates(c echo.Context) error {
	templates, err := scaffold.ListTemplates()
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": templates})
}

// handleAddTemplate adds a template's document scaffolding to an existing organization.
// POST /api/v1/templates
func (s *Server) handleAddTemplate(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}

	orgID := getOrgID(c)
	var req struct {
		Template string `json:"template"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if !scaffold.IsValidTemplate(req.Template) {
		available, _ := scaffold.ListTemplates()
		ids := make([]string, len(available))
		for i, t := range available {
			ids[i] = t.ID
		}
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid template: %s (available: %v)", req.Template, ids))
	}

	ctx := c.Request().Context()
	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	email := getUserEmail(c)
	user, _ := s.db.GetUserByEmail(ctx, email)
	authorName := email
	if user != nil && user.Name != "" {
		authorName = user.Name
	}

	if _, err := s.db.GetOrganization(ctx, orgID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}

	if err := scaffold.ScaffoldToRepo(st, req.Template, authorName, email); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("scaffolding template: %v", err))
	}

	// Invalidate store cache to pick up new files
	s.stores.Delete(orgID)
	// Invalidate search index — scaffolded documents need to be indexed
	s.searchIndex.Invalidate(orgID)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  email,
		Action: "template_added",
		Detail: fmt.Sprintf("Added %s template scaffolding", req.Template),
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"template": req.Template,
		"status":   "scaffolded",
	})
}

// handleRemoveTemplate removes a template's documents from the repository.
// DELETE /api/v1/templates/:name
func (s *Server) handleRemoveTemplate(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}

	name := c.Param("name")
	if !scaffold.IsValidTemplate(name) {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid template: %s", name))
	}

	orgID := getOrgID(c)
	ctx := c.Request().Context()

	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	email := getUserEmail(c)
	user, _ := s.db.GetUserByEmail(ctx, email)
	authorName := email
	if user != nil && user.Name != "" {
		authorName = user.Name
	}

	// The template documents live under documents/<template>/
	dirPath := filepath.Join(org.RepoPath, "documents", name)
	commitHash, err := st.DeleteDirectory(dirPath, authorName, email,
		fmt.Sprintf("chore: remove %s template documents", name))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("removing template: %v", err))
	}

	// Invalidate store cache
	s.stores.Delete(orgID)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  email,
		Action: "template_removed",
		Detail: fmt.Sprintf("Removed %s template documents", name),
	})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"template": name,
		"status":   "removed",
		"commit":   commitHash,
	})
}
