package api

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// AdminOnly returns middleware that restricts access to admin role only.
func (s *Server) AdminOnly() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, _ := c.Get("user_role").(string)
			if role != "admin" {
				return echo.NewHTTPError(http.StatusForbidden, "admin access required")
			}
			return next(c)
		}
	}
}

// --- Members ---

// handleAdminListMembers returns all members of the current organization.
func (s *Server) handleAdminListMembers(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	users, err := s.db.ListOrgUsers(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing members: "+err.Error())
	}
	if users == nil {
		users = []db.UserWithRole{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": users})
}

// handleAdminUpdateRole updates a member's role in the organization.
func (s *Server) handleAdminUpdateRole(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	userID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user ID")
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Role == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "role is required")
	}

	validRoles := map[string]bool{"admin": true, "manager": true, "contributor": true, "reader": true}
	if !validRoles[req.Role] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
	}

	// Prevent self-downgrade if last admin
	callerEmail := getUserEmail(c)
	callerUser, _ := s.db.GetUserByEmail(ctx, callerEmail)
	if callerUser != nil && callerUser.ID == userID && req.Role != "admin" {
		callerRole, _ := s.db.GetUserRole(ctx, orgID, callerUser.ID)
		if callerRole == "admin" {
			// Count admins in this org
			var adminCount int
			_ = s.db.Pool().QueryRow(ctx,
				`SELECT COUNT(*) FROM organization_members WHERE organization_id = $1 AND role = 'admin'`,
				orgID).Scan(&adminCount)
			if adminCount <= 1 {
				return echo.NewHTTPError(http.StatusBadRequest, "cannot downgrade yourself — you are the only admin")
			}
		}
	}

	// Verify user is already a member before updating role
	if _, err := s.db.GetOrgMember(ctx, orgID, userID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user is not a member of this organization")
	}
	if err := s.db.AddOrgMember(ctx, orgID, userID, req.Role); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "updating role: "+err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "member_role_updated",
		Detail: fmt.Sprintf("Updated user %d role to %s", userID, req.Role),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "role updated"})
}

// handleAdminRemoveMember removes a member from the organization.
func (s *Server) handleAdminRemoveMember(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	userID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user ID")
	}

	// Prevent removing yourself
	callerEmail := getUserEmail(c)
	callerUser, _ := s.db.GetUserByEmail(ctx, callerEmail)
	if callerUser != nil && callerUser.ID == userID {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot remove yourself — ask another admin")
	}

	if err := s.db.RemoveOrgMember(ctx, orgID, userID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "removing member: "+err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "member_removed",
		Detail: fmt.Sprintf("Removed user %d from organization", userID),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "member removed"})
}

// --- API Keys (admin read-only audit view) ---

// handleAdminListAPIKeys returns all API keys from users who are members of this org.
// This is a read-only audit view — token creation/revocation is done in user Settings.
func (s *Server) handleAdminListAPIKeys(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	keys, err := s.db.ListAllAPIKeysForOrg(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing API keys: "+err.Error())
	}
	if keys == nil {
		keys = []db.APIKey{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": keys})
}

// --- OIDC Providers ---

// handleAdminListOIDC returns all OIDC providers for the current organization.
func (s *Server) handleAdminListOIDC(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	providers, err := s.db.ListOIDCProviders(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing OIDC providers: "+err.Error())
	}
	if providers == nil {
		providers = []db.OIDCProvider{}
	}

	// Mask client secrets in response
	for i := range providers {
		if providers[i].ClientSecret != "" {
			providers[i].ClientSecret = "********"
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": providers})
}

// handleAdminCreateOIDC creates a new OIDC provider for the organization.
func (s *Server) handleAdminCreateOIDC(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var p db.OIDCProvider
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	p.OrganizationID = orgID

	if p.ProviderName == "" || p.ClientID == "" || p.DiscoveryURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "provider_name, client_id, and discovery_url are required")
	}
	if p.DefaultRole == "" {
		p.DefaultRole = "reader"
	}

	if err := s.db.CreateOIDCProvider(ctx, &p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating OIDC provider: "+err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "oidc_provider_created",
		Detail: fmt.Sprintf("Created OIDC provider %q (%s)", p.DisplayName, p.ProviderName),
	})

	// Mask secret in response
	p.ClientSecret = ""
	return c.JSON(http.StatusCreated, p)
}

// handleAdminUpdateOIDC updates an existing OIDC provider.
func (s *Server) handleAdminUpdateOIDC(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid provider ID")
	}

	// Fetch existing to verify ownership
	existing, err := s.db.GetOIDCProviderByID(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "OIDC provider not found")
	}
	if existing.OrganizationID != orgID {
		return echo.NewHTTPError(http.StatusForbidden, "provider belongs to another organization")
	}

	var p db.OIDCProvider
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	p.ID = id
	p.OrganizationID = orgID

	// If client_secret not provided in update, keep existing
	if p.ClientSecret == "" || p.ClientSecret == "********" {
		p.ClientSecret = existing.ClientSecret
	}

	if err := s.db.UpdateOIDCProvider(ctx, &p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "updating OIDC provider: "+err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "oidc_provider_updated",
		Detail: fmt.Sprintf("Updated OIDC provider %q", p.ProviderName),
	})

	p.ClientSecret = ""
	return c.JSON(http.StatusOK, p)
}

// handleAdminDeleteOIDC deletes an OIDC provider.
func (s *Server) handleAdminDeleteOIDC(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid provider ID")
	}

	// Verify ownership
	existing, err := s.db.GetOIDCProviderByID(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "OIDC provider not found")
	}
	if existing.OrganizationID != orgID {
		return echo.NewHTTPError(http.StatusForbidden, "provider belongs to another organization")
	}

	if err := s.db.DeleteOIDCProvider(ctx, orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "deleting OIDC provider: "+err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "oidc_provider_deleted",
		Detail: fmt.Sprintf("Deleted OIDC provider %q", existing.ProviderName),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "provider deleted"})
}

// handleAdminTestOIDC tests an OIDC provider's discovery URL.
func (s *Server) handleAdminTestOIDC(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid provider ID")
	}

	provider, err := s.db.GetOIDCProviderByID(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "OIDC provider not found")
	}
	if provider.OrganizationID != orgID {
		return echo.NewHTTPError(http.StatusForbidden, "provider belongs to another organization")
	}

	// Test OIDC discovery
	oidcProvider, err := oidc.NewProvider(ctx, provider.DiscoveryURL)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("discovery failed: %v", err),
		})
	}

	// Extract endpoint info
	endpoint := oidcProvider.Endpoint()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":       true,
		"auth_url":      endpoint.AuthURL,
		"token_url":     endpoint.TokenURL,
		"discovery_url": provider.DiscoveryURL,
		"scopes":        strings.Split(provider.Scopes, " "),
	})
}

// --- Settings ---

// handleAdminListSettings returns all settings for the current organization.
func (s *Server) handleAdminListSettings(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	settings, err := s.db.GetOrgSettings(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing settings: "+err.Error())
	}
	if settings == nil {
		settings = []db.OrgSetting{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": settings})
}

// handleAdminUpdateSetting updates a single setting for the organization.
func (s *Server) handleAdminUpdateSetting(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}

	// Validate branding values
	if req.Key == "branding_color" && req.Value != "" {
		if matched, _ := regexp.MatchString(`^#[0-9a-fA-F]{3,8}$`, req.Value); !matched {
			return echo.NewHTTPError(http.StatusBadRequest, "branding_color must be a valid hex color (e.g. #3b82f6)")
		}
	}
	if (req.Key == "terms_url" || req.Key == "privacy_url") && req.Value != "" {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.Value)), "javascript:") {
			return echo.NewHTTPError(http.StatusBadRequest, "URL cannot use javascript: protocol")
		}
	}

	if err := s.db.SetOrgSetting(ctx, orgID, req.Key, req.Value); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "updating setting: "+err.Error())
	}

	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "setting_updated",
		Detail: fmt.Sprintf("Updated setting %q", req.Key),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "setting updated"})
}

// handleBrandingUpload accepts an image file (PNG, SVG, or ICO) and stores it
// in the blob store. The "type" form field selects the asset: logo or favicon.
func (s *Server) handleBrandingUpload(c echo.Context) error {
	if err := requireRole(c, "admin"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
	}

	// Asset type: logo (default) or favicon
	assetType := c.FormValue("type")
	if assetType == "" {
		assetType = "logo"
	}
	if assetType != "logo" && assetType != "favicon" {
		return echo.NewHTTPError(http.StatusBadRequest, "type must be logo or favicon")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file required")
	}

	if file.Size > 2*1024*1024 {
		return echo.NewHTTPError(http.StatusBadRequest, "file too large (max 2 MB)")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExts := map[string]bool{".png": true, ".svg": true, ".ico": true}
	if !allowedExts[ext] {
		return echo.NewHTTPError(http.StatusBadRequest, "only PNG, SVG, and ICO files are allowed")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "reading file")
	}
	defer src.Close()
	content, err := io.ReadAll(src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "reading file content")
	}

	if ext == ".svg" {
		content = sanitizeSVG(content)
	}
	if ext == ".png" {
		if len(content) < 8 || string(content[:4]) != "\x89PNG" {
			return echo.NewHTTPError(http.StatusBadRequest, "file is not a valid PNG")
		}
	}

	targetName := assetType + ext
	filePath := "branding/" + targetName

	// Remove other variants of this asset type (e.g. switching PNG↔SVG)
	for _, oldExt := range []string{".png", ".svg", ".ico"} {
		if oldExt != ext {
			_ = s.blobs.Delete(ctx, org.UUID, "branding/"+assetType+oldExt)
		}
	}

	if err := s.blobs.Put(ctx, org.UUID, filePath, content); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "storing branding file: "+err.Error())
	}

	actor := getUserEmail(c)
	s.db.LogActivity(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "branding_uploaded",
		Detail: fmt.Sprintf("Uploaded %s as %s", assetType, targetName),
	})

	return c.JSON(http.StatusOK, map[string]string{
		"status": "uploaded",
		"file":   filePath,
	})
}

// handleBrandingDelete removes a branding file from the blob store.
func (s *Server) handleBrandingDelete(c echo.Context) error {
	if err := requireRole(c, "admin"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "organization not found")
	}

	name := c.Param("name")
	allowedNames := map[string]bool{
		"logo.png": true, "logo.svg": true,
		"favicon.ico": true, "favicon.png": true, "favicon.svg": true,
	}
	if !allowedNames[name] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid branding file name")
	}

	filePath := "branding/" + name
	if err := s.blobs.Delete(ctx, org.UUID, filePath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "deleting branding file: "+err.Error())
	}

	actor := getUserEmail(c)
	s.db.LogActivity(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "branding_deleted",
		Detail: fmt.Sprintf("Removed branding file: %s", name),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// sanitizeSVG strips dangerous content from SVG files using XML parsing.
// It removes dangerous elements (script, foreignObject, etc.) and attributes
// (event handlers, javascript: URIs) by walking the parsed XML tree.
func sanitizeSVG(content []byte) []byte {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	// Preserve original encoding and don't enforce strict namespace rules,
	// since SVGs in the wild are often loose with namespaces.
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	// Dangerous element names (lowercase for comparison).
	dangerousElements := map[string]bool{
		"script":        true,
		"style":         true,
		"foreignobject": true,
		"iframe":        true,
		"object":        true,
		"embed":         true,
		"applet":        true,
	}

	// Track depth inside dangerous elements so we skip their entire subtree.
	dangerousDepth := 0

	for {
		tok, err := decoder.Token()
		if err != nil {
			break // EOF or parse error — emit what we have
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localLower := strings.ToLower(t.Name.Local)
			if dangerousElements[localLower] {
				dangerousDepth++
				continue
			}
			if dangerousDepth > 0 {
				continue
			}

			// Filter attributes
			clean := make([]xml.Attr, 0, len(t.Attr))
			for _, attr := range t.Attr {
				attrLocal := strings.ToLower(attr.Name.Local)

				// Remove event handler attributes (on*)
				if strings.HasPrefix(attrLocal, "on") {
					continue
				}

				// Check for dangerous URI values in href-like attributes.
				// xlink:href is parsed by encoding/xml as Space="http://www.w3.org/1999/xlink" Local="href".
				valTrimmed := strings.TrimSpace(strings.ToLower(attr.Value))
				isHref := attrLocal == "href"
				if isHref {
					if strings.HasPrefix(valTrimmed, "javascript:") ||
						strings.HasPrefix(valTrimmed, "vbscript:") ||
						strings.HasPrefix(valTrimmed, "data:") {
						continue
					}
				}

				// Also check src, action, formaction, data attributes for dangerous URIs
				if attrLocal == "src" || attrLocal == "action" || attrLocal == "formaction" || attrLocal == "data" {
					if strings.HasPrefix(valTrimmed, "javascript:") ||
						strings.HasPrefix(valTrimmed, "vbscript:") ||
						strings.HasPrefix(valTrimmed, "data:text/html") {
						continue
					}
				}

				// Check for style attribute with dangerous content
				if attrLocal == "style" {
					if strings.Contains(valTrimmed, "expression(") ||
						strings.Contains(valTrimmed, "javascript:") ||
						strings.Contains(valTrimmed, "vbscript:") {
						continue
					}
				}

				clean = append(clean, attr)
			}
			t.Attr = clean
			encoder.EncodeToken(t)

		case xml.EndElement:
			localLower := strings.ToLower(t.Name.Local)
			if dangerousElements[localLower] {
				if dangerousDepth > 0 {
					dangerousDepth--
				}
				continue
			}
			if dangerousDepth > 0 {
				continue
			}
			encoder.EncodeToken(t)

		case xml.CharData, xml.Comment, xml.ProcInst, xml.Directive:
			if dangerousDepth > 0 {
				continue
			}
			encoder.EncodeToken(tok)
		}
	}

	encoder.Flush()
	if buf.Len() == 0 {
		// If parsing failed completely, return empty SVG rather than
		// passing through potentially dangerous raw content.
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	}
	return buf.Bytes()
}
