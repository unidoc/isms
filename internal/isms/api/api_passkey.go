package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// ---------------------------------------------------------------------------
// webauthn.User adapter
// ---------------------------------------------------------------------------

// webAuthnUser wraps a db.User and its stored credentials to satisfy
// the webauthn.User interface required by the go-webauthn library.
type webAuthnUser struct {
	user  *db.User
	creds []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte                         { return []byte(fmt.Sprintf("%d", u.user.ID)) }
func (u *webAuthnUser) WebAuthnName() string                       { return u.user.Email }
func (u *webAuthnUser) WebAuthnDisplayName() string                { return u.user.Name }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// dbCredToWebAuthn converts a stored DB credential to the library's Credential type.
func dbCredToWebAuthn(c *db.WebAuthnCredential) webauthn.Credential {
	var transports []protocol.AuthenticatorTransport
	for _, t := range c.Transport {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transport:       transports,
		Authenticator: webauthn.Authenticator{
			SignCount: uint32(c.SignCount),
		},
	}
}

// dbCredsToWebAuthn converts a slice of DB credentials.
func dbCredsToWebAuthn(creds []db.WebAuthnCredential) []webauthn.Credential {
	out := make([]webauthn.Credential, len(creds))
	for i := range creds {
		out[i] = dbCredToWebAuthn(&creds[i])
	}
	return out
}

// ---------------------------------------------------------------------------
// Registration (authenticated)
// ---------------------------------------------------------------------------

// handlePasskeyRegisterBegin starts the WebAuthn registration ceremony.
// POST /auth/passkey/register/begin
func (s *Server) handlePasskeyRegisterBegin(c echo.Context) error {
	if s.webauthn == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "WebAuthn not configured (set ISMS_BASE_URL)")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)

	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	// Load existing credentials so the authenticator can exclude them.
	dbCreds, err := s.db.ListWebAuthnCredentials(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing credentials: "+err.Error())
	}

	wanUser := &webAuthnUser{user: user, creds: dbCredsToWebAuthn(dbCreds)}

	// Exclude existing credentials from registration.
	excludeList := make([]protocol.CredentialDescriptor, len(dbCreds))
	for i, dc := range dbCreds {
		excludeList[i] = protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: dc.CredentialID,
		}
	}

	creation, session, err := s.webauthn.BeginRegistration(wanUser,
		webauthn.WithExclusions(excludeList),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "begin registration: "+err.Error())
	}

	s.passkeyRegistrations.Store(email, session)

	return c.JSON(http.StatusOK, creation)
}

// handlePasskeyRegisterComplete finishes the WebAuthn registration ceremony.
// POST /auth/passkey/register/complete
func (s *Server) handlePasskeyRegisterComplete(c echo.Context) error {
	if s.webauthn == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "WebAuthn not configured")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)

	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	raw, ok := s.passkeyRegistrations.LoadAndDelete(email)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "no registration in progress")
	}
	session := raw.(*webauthn.SessionData)

	// Load existing creds for the user object.
	dbCreds, err := s.db.ListWebAuthnCredentials(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing credentials: "+err.Error())
	}

	wanUser := &webAuthnUser{user: user, creds: dbCredsToWebAuthn(dbCreds)}

	cred, err := s.webauthn.FinishRegistration(wanUser, *session, c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "finish registration: "+err.Error())
	}

	// Convert transports to string slice for DB storage.
	var transports []string
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}

	dbCred := &db.WebAuthnCredential{
		UserID:          user.ID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		Transport:       transports,
		SignCount:       int(cred.Authenticator.SignCount),
		Name:            "Passkey",
	}

	if err := s.db.CreateWebAuthnCredential(ctx, dbCred); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "saving credential: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "registered",
		"id":     dbCred.ID,
		"name":   dbCred.Name,
	})
}

// ---------------------------------------------------------------------------
// Login (unauthenticated)
// ---------------------------------------------------------------------------

type passkeyLoginBeginRequest struct {
	Email string `json:"email"`
}

type passkeyLoginCompleteRequest struct {
	Email        string `json:"email"`
	Organization string `json:"organization,omitempty"`
}

// handlePasskeyLoginBegin starts the WebAuthn login ceremony.
// POST /auth/passkey/login/begin
func (s *Server) handlePasskeyLoginBegin(c echo.Context) error {
	if s.webauthn == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "WebAuthn not configured (set ISMS_BASE_URL)")
	}

	var req passkeyLoginBeginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}

	ctx := c.Request().Context()
	clientIP := c.RealIP()

	// Per-account brute-force protection (DB-backed, shared with password login;
	// skipped when ISMS_RATE_LIMIT=0)
	if !rateLimitDisabled() {
		count, _ := s.db.CountRecentLoginAttempts(ctx, req.Email)
		if count >= maxLoginAttempts {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many attempts, try again later")
		}
	}

	user, err := s.db.GetUserByEmail(ctx, req.Email)
	if err != nil || user == nil {
		s.db.RecordLoginAttempt(ctx, req.Email, clientIP)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}
	if !user.Active {
		s.db.RecordLoginAttempt(ctx, req.Email, clientIP)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	dbCreds, err := s.db.ListWebAuthnCredentials(ctx, user.ID)
	if err != nil || len(dbCreds) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no passkeys registered for this account")
	}

	wanUser := &webAuthnUser{user: user, creds: dbCredsToWebAuthn(dbCreds)}

	assertion, session, err := s.webauthn.BeginLogin(wanUser)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "begin login: "+err.Error())
	}

	s.passkeyLogins.Store(req.Email, session)

	return c.JSON(http.StatusOK, assertion)
}

// handlePasskeyLoginComplete finishes the WebAuthn login ceremony and returns a session token.
// POST /auth/passkey/login/complete
func (s *Server) handlePasskeyLoginComplete(c echo.Context) error {
	if s.webauthn == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "WebAuthn not configured")
	}

	// We need the email to look up the session. The browser sends the credential
	// response in the request body but we also need to know which user it belongs to.
	// The email is passed as a query parameter or parsed from a wrapper JSON.
	email := c.QueryParam("email")
	orgSlug := c.QueryParam("organization")

	if email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email query parameter is required")
	}

	ctx := c.Request().Context()

	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	raw, ok := s.passkeyLogins.LoadAndDelete(email)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "no login in progress for this email")
	}
	session := raw.(*webauthn.SessionData)

	dbCreds, err := s.db.ListWebAuthnCredentials(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing credentials: "+err.Error())
	}

	wanUser := &webAuthnUser{user: user, creds: dbCredsToWebAuthn(dbCreds)}

	cred, err := s.webauthn.FinishLogin(wanUser, *session, c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "login failed: "+err.Error())
	}

	// Update the sign count in the DB for the matched credential.
	for _, dc := range dbCreds {
		if byteSliceEqual(dc.CredentialID, cred.ID) {
			s.db.UpdateWebAuthnSignCount(ctx, dc.ID, int(cred.Authenticator.SignCount))
			break
		}
	}

	// Mark email as verified on successful passkey login.
	if !user.EmailVerified {
		s.db.SetEmailVerified(ctx, user.ID)
	}

	// Resolve organization (same logic as password login).
	var orgID int
	var orgName string
	var resolvedOrgSlug string

	if orgSlug != "" {
		org, err := s.db.GetOrganizationBySlug(ctx, orgSlug)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "organization not found")
		}
		_, err = s.db.GetOrgMember(ctx, org.ID, user.ID)
		if err != nil {
			return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
		}
		orgID = org.ID
		orgName = org.Name
		resolvedOrgSlug = org.Slug
	} else {
		orgs, err := s.db.ListUserOrgs(ctx, user.ID)
		if err == nil && len(orgs) == 1 {
			orgID = orgs[0].ID
			orgName = orgs[0].Name
			resolvedOrgSlug = orgs[0].Slug
		}
	}

	var role string
	if orgID > 0 {
		if orgRole, err := s.db.GetUserRole(ctx, orgID, user.ID); err == nil {
			role = orgRole
		}
	}
	if role == "" {
		role = "reader"
	}

	// Create signed JWT session token (stateless — no DB row)
	token, err := s.createSessionJWT(user, orgID, role, resolvedOrgSlug, orgName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating session token: "+err.Error())
	}

	s.db.ClearLoginAttempts(ctx, email)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user.Email,
		Action: "login",
		Detail: "Passkey login",
	})

	return c.JSON(http.StatusOK, loginResponse{
		Token:            token,
		Email:            user.Email,
		Name:             user.Name,
		Role:             role,
		OrganizationID:   orgID,
		OrganizationName: orgName,
	})
}

// ---------------------------------------------------------------------------
// Management (authenticated)
// ---------------------------------------------------------------------------

type passkeyListItem struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  db.Epoch  `json:"created_at"`
	LastUsedAt *db.Epoch `json:"last_used_at,omitempty"`
}

// handleListPasskeys returns the current user's registered passkeys.
// GET /auth/passkeys
func (s *Server) handleListPasskeys(c echo.Context) error {
	ctx := c.Request().Context()
	email := getUserEmail(c)

	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	creds, err := s.db.ListWebAuthnCredentials(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing passkeys: "+err.Error())
	}

	items := make([]passkeyListItem, len(creds))
	for i, c := range creds {
		items[i] = passkeyListItem{
			ID:         c.ID,
			Name:       c.Name,
			CreatedAt:  c.CreatedAt,
			LastUsedAt: c.LastUsedAt,
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": items})
}

type renamePasskeyRequest struct {
	Name string `json:"name"`
}

// handleRenamePasskey renames a passkey.
// PUT /auth/passkeys/:id
func (s *Server) handleRenamePasskey(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	var req renamePasskeyRequest
	if err := c.Bind(&req); err != nil || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)

	// Verify ownership.
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	cred, err := s.db.GetWebAuthnCredentialByID(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "passkey not found")
	}
	if cred.UserID != user.ID {
		return echo.NewHTTPError(http.StatusForbidden, "not your passkey")
	}

	if err := s.db.RenameWebAuthnCredential(ctx, id, user.ID, req.Name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "renaming passkey: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "renamed", "name": req.Name})
}

// handleDeletePasskey removes a passkey.
// DELETE /auth/passkeys/:id
func (s *Server) handleDeletePasskey(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)

	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	cred, err := s.db.GetWebAuthnCredentialByID(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "passkey not found")
	}
	if cred.UserID != user.ID {
		return echo.NewHTTPError(http.StatusForbidden, "not your passkey")
	}

	if err := s.db.DeleteWebAuthnCredential(ctx, id, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "deleting passkey: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// byteSliceEqual is a helper to compare two byte slices.
func byteSliceEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
