package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// errProvisionFailed distinguishes a failed JIT user creation (DB error → 500)
// from a genuine "user not found / provisioning off" (401), so an operator
// debugging a broken DB doesn't chase a misleading 401 down the CF JWT path.
var errProvisionFailed = errors.New("auto-provision failed")

// cfProvisionConfig controls just-in-time user provisioning on Cloudflare Access
// login. Off by default — opt in via ISMS_CF_AUTO_PROVISION. Safe ONLY when the
// CF Access policy is the source of truth for who may reach the app (#98).
//
// JIT creates the user row ONLY. Organization membership and role stay an
// explicit admin/CLI action — there is intentionally no "default org": which
// org a person belongs to, and at what role, is a governance decision, not
// something to guess from an env var.
type cfProvisionConfig struct {
	Enabled bool
}

// deriveNameFromEmail produces a fallback display name when the CF JWT carries
// no name claim (e.g. "ari.bjarna" from "ari.bjarna@acme.com").
func deriveNameFromEmail(email string) string {
	if at := strings.IndexByte(email, '@'); at > 0 {
		return email[:at]
	}
	return email
}

// resolveCFUser returns the ISMS user for a Cloudflare-Access-authenticated
// email. When the user doesn't exist and auto-provisioning is enabled, it
// creates the user row (active) — and nothing else. Org membership and role are
// granted separately by an admin (Admin → Members) or the CLI; until then the
// user has no org and can authenticate but not load an org's data.
//
// Shared by the auth middleware (API/CLI/git) and the cf-session handler (web)
// so both behave identically. Returns (user, created, error); created is true on
// JIT provisioning.
func resolveCFUser(ctx context.Context, d *db.DB, email, name string, pc cfProvisionConfig) (*db.User, bool, error) {
	if u, err := d.GetUserByEmail(ctx, email); err == nil {
		return u, false, nil
	}
	if !pc.Enabled {
		return nil, false, fmt.Errorf("user not found")
	}

	u := &db.User{Email: email, Name: name, Active: true}
	if u.Name == "" {
		u.Name = deriveNameFromEmail(email)
	}
	if err := d.UpsertUser(ctx, u); err != nil {
		return nil, false, fmt.Errorf("%w: %v", errProvisionFailed, err)
	}
	return u, true, nil
}

// handleCFSession mints an ISMS session token for a user already authenticated
// by Cloudflare Access (#98). The SPA calls this on load when it has no local
// token: behind CF Access the proxy adds identity headers, we validate the JWT
// (not just the email header — never trust the header alone), resolve/provision
// the user, and return a session like a password login. Public route; only
// succeeds when CF Access is configured and the JWT validates.
func (s *Server) handleCFSession(c echo.Context) error {
	if s.cfKeyCache == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Cloudflare Access is not configured")
	}

	jwt := c.Request().Header.Get("Cf-Access-Jwt-Assertion")
	if jwt == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "no Cloudflare Access token")
	}
	claims, err := s.cfKeyCache.VerifyJWT(jwt)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("invalid Cloudflare Access token: %v", err))
	}

	email := claims.Email
	if hdr := c.Request().Header.Get("Cf-Access-Authenticated-User-Email"); hdr != "" {
		if email != "" && !strings.EqualFold(email, hdr) {
			return echo.NewHTTPError(http.StatusUnauthorized, "token email mismatch")
		}
		if email == "" {
			email = hdr
		}
	}
	if email == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "no email in Cloudflare Access identity")
	}

	ctx := c.Request().Context()
	user, created, err := resolveCFUser(ctx, s.db, email, claims.Name, s.cfProvision)
	if err != nil {
		if errors.Is(err, errProvisionFailed) {
			log.Printf("[cf-access] auto-provision failed for %s: %v", email, err)
			return echo.NewHTTPError(http.StatusInternalServerError, "auto-provision failed")
		}
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}
	if created {
		log.Printf("[cf-access] auto-provisioned user %s", email)
	}

	// Resolve org + role — mirror handleLogin: subdomain/domain context first,
	// then auto-select when the user belongs to exactly one org.
	var orgID int
	var orgName, orgSlug string
	if slug, ok := c.Get("org_slug").(string); ok && slug != "" {
		org, oerr := s.db.GetOrganizationBySlug(ctx, slug)
		if oerr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "organization not found")
		}
		if _, merr := s.db.GetOrgMember(ctx, org.ID, user.ID); merr != nil {
			// JIT-provisioned but not yet a member — an admin must add them.
			return echo.NewHTTPError(http.StatusForbidden, "no access to this organization yet — ask an admin to add you")
		}
		orgID, orgName, orgSlug = org.ID, org.Name, org.Slug
	} else {
		orgs, _ := s.db.ListUserOrgs(ctx, user.ID)
		switch {
		case len(orgs) == 1:
			orgID, orgName, orgSlug = orgs[0].ID, orgs[0].Name, orgs[0].Slug
		case len(orgs) == 0:
			// Don't mint an org-less session here: it would persist a stale
			// token so that adding the user to an org later wouldn't take
			// effect on refresh (the CF probe only runs when there's no token).
			// 403 instead → after an admin adds them, the next load re-probes
			// and resolves the org cleanly.
			return echo.NewHTTPError(http.StatusForbidden, "no organization yet — ask an admin to add you")
			// len(orgs) > 1: leave orgID 0 → the SPA shows the org picker.
		}
	}

	role := "reader"
	if orgID > 0 {
		if r, rerr := s.db.GetUserRole(ctx, orgID, user.ID); rerr == nil && r != "" {
			role = r
		}
	}

	token, err := s.createSessionJWT(user, orgID, role, orgSlug, orgName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating session token")
	}

	return c.JSON(http.StatusOK, loginResponse{
		Token:            token,
		Email:            user.Email,
		Name:             user.Name,
		Role:             role,
		OrganizationID:   orgID,
		OrganizationName: orgName,
		OrganizationSlug: orgSlug,
	})
}
