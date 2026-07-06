package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"net/http"
	netmail "net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"isms.sh/internal/isms/db"
)

// --- Per-account brute-force protection (DB-backed) ---

const maxLoginAttempts = 5

// --- Login ---

type loginRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	OTP          string `json:"otp,omitempty"`
	Organization string `json:"organization,omitempty"` // org slug (optional)
}

type loginResponse struct {
	Token            string `json:"token"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	Role             string `json:"role"`
	OTPRequired      bool   `json:"otp_required,omitempty"`
	OrganizationID   int    `json:"organization_id,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
	OrganizationSlug string `json:"organization_slug,omitempty"`
}

// handleLogin authenticates a user with email + password and returns an API token.
func (s *Server) handleLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Email == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email and password required")
	}

	ctx := c.Request().Context()
	clientIP := c.RealIP()

	// Per-account brute-force protection (DB-backed; skipped when ISMS_RATE_LIMIT=0)
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

	if !user.Active || !user.HasPassword() {
		s.db.RecordLoginAttempt(ctx, req.Email, clientIP)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		s.db.RecordLoginAttempt(ctx, req.Email, clientIP)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	// Check OTP if enabled
	if user.HasOTP() {
		if req.OTP == "" {
			return c.JSON(http.StatusOK, loginResponse{OTPRequired: true})
		}
		if !verifyTOTP(*user.OTPSecret, req.OTP) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid OTP code")
		}
		// TOTP replay protection
		alreadyUsed, err := s.db.CheckAndSetTOTPUsed(ctx, user.ID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "checking TOTP")
		}
		if alreadyUsed {
			return echo.NewHTTPError(http.StatusUnauthorized, "OTP code already used, wait for next code")
		}
	}

	// Mark email as verified on successful password login
	if !user.EmailVerified {
		s.db.SetEmailVerified(ctx, user.ID)
	}

	// Resolve organization
	var orgID int
	var orgName string
	var orgSlug string

	// If the request came in via an org subdomain or custom domain, the
	// OrgResolverMiddleware already set org_slug on the context. Use that
	// as the implicit organization when the request body didn't specify one.
	if req.Organization == "" {
		if s, ok := c.Get("org_slug").(string); ok && s != "" {
			req.Organization = s
		}
	}

	if req.Organization != "" {
		// Explicit org slug provided — look it up
		org, err := s.db.GetOrganizationBySlug(ctx, req.Organization)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "organization not found")
		}
		// Verify user is a member
		_, err = s.db.GetOrgMember(ctx, org.ID, user.ID)
		if err != nil {
			return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
		}
		orgID = org.ID
		orgName = org.Name
		orgSlug = org.Slug
	} else {
		// No org specified — auto-select if user belongs to exactly one org
		orgs, err := s.db.ListUserOrgs(ctx, user.ID)
		if err == nil && len(orgs) == 1 {
			orgID = orgs[0].ID
			orgName = orgs[0].Name
			orgSlug = orgs[0].Slug
		}
		// If user belongs to 0 or multiple orgs, orgID stays 0
		// Frontend can prompt for org selection if needed
	}

	// Get user's role within the org
	var role string
	if orgID > 0 {
		if orgRole, err := s.db.GetUserRole(ctx, orgID, user.ID); err == nil {
			role = orgRole
		}
	}
	if role == "" {
		role = "reader" // default fallback
	}

	// Create signed JWT session token (stateless — no DB row)
	token, err := s.createSessionJWT(user, orgID, role, orgSlug, orgName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating session token: "+err.Error())
	}

	s.db.ClearLoginAttempts(ctx, req.Email)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  user.Email,
		Action: "login",
		Detail: "Password login",
	})

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

// --- Signup (self-registration) ---

type signupRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// handleSignup creates a new user account with email verification.
// POST /api/v1/auth/signup
// Gated by ISMS_USER_SIGNUP env var (must be "true" or "1" to enable).
func (s *Server) handleSignup(c echo.Context) error {
	signup := os.Getenv("ISMS_USER_SIGNUP")
	if signup != "true" && signup != "1" {
		return echo.NewHTTPError(http.StatusForbidden, "self-registration is disabled")
	}

	var req signupRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Email == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email and password required")
	}
	if req.Name == "" {
		req.Name = req.Email
	}
	if len(req.Password) < 7 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 7 characters")
	}

	ctx := c.Request().Context()

	// Check if user already exists
	existing, _ := s.db.GetUserByEmail(ctx, req.Email)
	if existing != nil {
		return echo.NewHTTPError(http.StatusConflict, "account already exists — try logging in")
	}

	skipVerify := os.Getenv("ISMS_SKIP_EMAIL_VERIFY") == "1" || os.Getenv("ISMS_SKIP_EMAIL_VERIFY") == "true"

	// Create user — active immediately if email verification is skipped
	user := &db.User{Email: req.Email, Name: req.Name, Active: skipVerify}
	if err := s.db.UpsertUser(ctx, user); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating account: "+err.Error())
	}

	// Set password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "hashing password")
	}
	if err := s.db.SetPassword(ctx, user.ID, string(hash)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "saving password")
	}

	if skipVerify {
		// Mark email as verified and return token directly
		s.db.SetEmailVerified(ctx, user.ID)
		jwtToken, err := s.createSessionJWT(user, 0, "", "", "")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "generating token")
		}
		return c.JSON(http.StatusCreated, map[string]interface{}{
			"token": jwtToken,
			"email": user.Email,
			"name":  user.Name,
		})
	}

	// Generate email verification token
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	tokenHash := sha256.Sum256([]byte(token))
	if err := s.db.CreateEmailVerification(ctx, user.ID, hex.EncodeToString(tokenHash[:])); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating verification")
	}

	// Send verification email
	if s.mailer == nil || !s.mailer.Enabled() {
		log.Printf("signup: mailer not configured (set SMTP_HOST/SMTP_FROM env) — user %s created but no verification email sent", req.Email)
		return echo.NewHTTPError(http.StatusInternalServerError,
			"email delivery is not configured on this server — contact the administrator")
	}
	baseURL := os.Getenv("ISMS_BASE_URL")
	if err := s.mailer.SendVerification(req.Email, req.Name, baseURL, token); err != nil {
		log.Printf("signup: SendVerification to %s failed: %v", req.Email, err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			"failed to send verification email: "+err.Error())
	}
	log.Printf("signup: verification email sent to %s (base=%s)", req.Email, baseURL)

	return c.JSON(http.StatusCreated, map[string]string{
		"status":  "verification_sent",
		"message": "Check your email to verify your account",
	})
}

// --- Forgot Password ---

// handleForgotPassword issues a password reset link by email. Also re-activates
// unverified accounts — clicking the reset link both sets a password and marks
// the email as verified, so this single flow covers "forgot password" AND
// "verification email never arrived".
// POST /api/v1/auth/forgot-password
func (s *Server) handleForgotPassword(c echo.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil || req.Email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}
	ctx := c.Request().Context()

	// Don't leak which emails exist — return 200 either way.
	user, _ := s.db.GetUserByEmail(ctx, req.Email)
	if user == nil {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "sent_if_exists",
			"message": "If an account exists for that email, a reset link has been sent.",
		})
	}

	if s.mailer == nil || !s.mailer.Enabled() {
		log.Printf("forgot-password: mailer not configured — user %s requested reset but no email sent", req.Email)
		return echo.NewHTTPError(http.StatusInternalServerError,
			"email delivery is not configured on this server — contact the administrator")
	}

	// Generate reset token
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	tokenHash := sha256.Sum256([]byte(token))
	if err := s.db.CreatePasswordResetToken(ctx, user.ID, hex.EncodeToString(tokenHash[:])); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating reset token")
	}

	baseURL := os.Getenv("ISMS_BASE_URL")
	if err := s.mailer.SendPasswordReset(req.Email, user.Name, baseURL, token); err != nil {
		log.Printf("forgot-password: SendPasswordReset to %s failed: %v", req.Email, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to send reset email: "+err.Error())
	}
	log.Printf("forgot-password: reset email sent to %s", req.Email)

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "sent_if_exists",
		"message": "If an account exists for that email, a reset link has been sent.",
	})
}

// --- Refresh JWT ---

// handleRefresh issues a new JWT if the current one is valid and the user is still active.
// POST /api/v1/auth/refresh
func (s *Server) handleRefresh(c echo.Context) error {
	email := getUserEmail(c)
	if email == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil || !user.Active {
		return echo.NewHTTPError(http.StatusUnauthorized, "account not active")
	}

	// Get current org from context (set by JWT or API key)
	orgID := getOrgID(c)

	// Re-verify org membership and get current role
	var role, orgSlug, orgName string
	if orgID > 0 {
		r, err := s.db.GetUserRole(ctx, orgID, user.ID)
		if err != nil {
			return echo.NewHTTPError(http.StatusForbidden, "no longer a member of this organization")
		}
		role = r
		if org, err := s.db.GetOrganization(ctx, orgID); err == nil {
			orgSlug = org.Slug
			orgName = org.Name
		}
	}

	token, err := s.createSessionJWT(user, orgID, role, orgSlug, orgName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating session")
	}

	// Revoke the old token to prevent reuse after refresh.
	authHeader := c.Request().Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		oldRaw := strings.TrimPrefix(authHeader, "Bearer ")
		// Only block JWTs (skip API keys which start with "isms_").
		if !strings.HasPrefix(oldRaw, "isms_") {
			if oldClaims, err := validateSessionJWT(oldRaw, s.secret); err == nil && oldClaims.ExpiresAt != nil {
				_ = s.db.BlockJWT(ctx, sha256Hash(oldRaw), oldClaims.ExpiresAt.Time)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"token": token})
}

// handleSwitchOrg issues a new JWT for a different organization.
func (s *Server) handleSwitchOrg(c echo.Context) error {
	var req struct {
		Slug string `json:"slug"`
	}
	if err := c.Bind(&req); err != nil || req.Slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "slug is required")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)

	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	org, err := s.db.GetOrganizationBySlug(ctx, req.Slug)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "organization not found")
	}

	// Verify membership
	role, err := s.db.GetUserRole(ctx, org.ID, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "not a member of this organization")
	}

	token, err := s.createSessionJWT(user, org.ID, role, org.Slug, org.Name)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating session")
	}

	return c.JSON(http.StatusOK, loginResponse{
		Token:            token,
		Email:            user.Email,
		Name:             user.Name,
		Role:             role,
		OrganizationID:   org.ID,
		OrganizationName: org.Name,
	})
}

// --- Self-service: change password ---

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangePassword(c echo.Context) error {
	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.NewPassword == "" || len(req.NewPassword) < 7 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 7 characters")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	// If user already has a password, verify current one
	if user.HasPassword() {
		if req.CurrentPassword == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "current password required")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "current password is incorrect")
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "hashing password")
	}

	if err := s.db.SetPassword(ctx, user.ID, string(hash)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "saving password")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "password updated"})
}

// --- Self-service: change email (verify-before-swap) ---

type changeEmailRequest struct {
	NewEmail        string `json:"new_email"`
	CurrentPassword string `json:"current_password"`
	OTP             string `json:"otp"`
}

// handleRequestEmailChange starts a self-service email change. The account's
// active email is NOT touched here: we re-authenticate the caller, record the
// new address as pending, and mail a confirmation link to that new address.
// The swap happens only when that link is verified (handleVerifyEmailChange).
// PUT /api/v1/auth/email
func (s *Server) handleRequestEmailChange(c echo.Context) error {
	var req changeEmailRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	newEmail := strings.ToLower(strings.TrimSpace(req.NewEmail))
	if newEmail == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "new_email is required")
	}
	if addr, err := netmail.ParseAddress(newEmail); err != nil || addr.Address != newEmail {
		return echo.NewHTTPError(http.StatusBadRequest, "new_email is not a valid email address")
	}

	ctx := c.Request().Context()
	user, err := s.db.GetUserByEmail(ctx, getUserEmail(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	if newEmail == strings.ToLower(user.Email) {
		return echo.NewHTTPError(http.StatusBadRequest, "that is already your email address")
	}

	// Re-authenticate: password (if set) and TOTP (if enabled). A changed email
	// is a takeover vector, so we never rely on the session alone.
	if user.HasPassword() {
		if req.CurrentPassword == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "current password required")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "current password is incorrect")
		}
	}
	if user.HasOTP() {
		if req.OTP == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "authenticator code required")
		}
		if !verifyTOTP(*user.OTPSecret, req.OTP) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid authenticator code")
		}
	}

	// Reject addresses already taken by another account (best-effort; the DB
	// unique index is the real guard, re-checked at swap time).
	if existing, _ := s.db.GetUserByEmail(ctx, newEmail); existing != nil {
		return echo.NewHTTPError(http.StatusConflict, "that email address is already in use")
	}

	if err := s.db.SetPendingEmail(ctx, user.ID, newEmail); err != nil {
		if err == db.ErrEmailTaken {
			return echo.NewHTTPError(http.StatusConflict, "that email address is already in use")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "recording email change: "+err.Error())
	}

	// One live email-change token at a time — a fresh request invalidates the old.
	if err := s.db.InvalidateEmailVerifications(ctx, user.ID, "email_change"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "preparing verification")
	}
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	if err := s.db.CreateEmailChangeToken(ctx, user.ID, hex.EncodeToString(hash[:])); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating verification")
	}

	if s.mailer == nil || !s.mailer.Enabled() {
		log.Printf("change-email: mailer not configured — user %d requested change to %s but no email sent", user.ID, newEmail)
		return echo.NewHTTPError(http.StatusInternalServerError,
			"email delivery is not configured on this server — contact the administrator")
	}
	m := s.orgMail(ctx, getOrgID(c))
	if err := s.mailer.SendEmailChangeVerificationBranded(newEmail, user.Name, m.PublicURL, token, m.Branding); err != nil {
		log.Printf("change-email: SendEmailChangeVerification to %s failed: %v", newEmail, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to send confirmation email: "+err.Error())
	}
	log.Printf("change-email: confirmation sent to %s for user %d", newEmail, user.ID)

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "verification_sent",
		"message": "Check your new inbox to confirm the change. Your current email stays active until you do.",
	})
}

// handleVerifyEmailChange completes an email change: the link mailed to the new
// address swaps it in. Unauthenticated — the token is the proof, and the caller
// may be on a device without an active session.
// POST /api/v1/auth/verify-email-change
func (s *Server) handleVerifyEmailChange(c echo.Context) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.Bind(&req); err != nil || req.Token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token is required")
	}

	ctx := c.Request().Context()
	hash := sha256.Sum256([]byte(req.Token))
	verification, err := s.db.LookupEmailVerification(ctx, hex.EncodeToString(hash[:]))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired confirmation link")
	}
	if verification.Purpose != "email_change" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired confirmation link")
	}

	newEmail, err := s.db.SwapPendingEmail(ctx, verification.UserID)
	if err != nil {
		if err == db.ErrEmailTaken {
			return echo.NewHTTPError(http.StatusConflict,
				"that email address was claimed by another account — the change was not applied")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "no pending email change for this link")
	}

	s.db.UseEmailVerification(ctx, verification.ID)
	log.Printf("change-email: user %d email changed to %s", verification.UserID, newEmail)

	// Existing sessions carry the old email as subject and will fail on refresh —
	// the user signs in again with the new address. We don't mint a token here.
	return c.JSON(http.StatusOK, map[string]string{
		"status": "email_changed",
		"email":  newEmail,
	})
}

// --- Self-service: update profile name ---

type updateProfileRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleUpdateProfile(c echo.Context) error {
	var req updateProfileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	if err := s.db.UpdateName(ctx, user.ID, req.Name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "updating name")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "profile updated", "name": req.Name})
}

// --- Self-service: OTP setup ---

type otpSetupResponse struct {
	Secret string `json:"secret"` // base32 encoded
	URI    string `json:"uri"`    // otpauth:// URI for QR code
}

func (s *Server) handleOTPSetup(c echo.Context) error {
	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	// Generate random secret
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "generating secret")
	}
	b32Secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)

	// Store secret (not yet verified)
	if err := s.db.SetOTPSecret(ctx, user.ID, b32Secret); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "saving OTP secret")
	}

	// Build otpauth URI — org name from DB
	orgID := getOrgID(c)
	issuer := "ISMS"
	if org, orgErr := s.db.GetOrganization(ctx, orgID); orgErr == nil && org.Name != "" {
		issuer = org.Name
	}
	uri := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=6&period=30",
		issuer, user.Email, b32Secret, issuer)

	return c.JSON(http.StatusOK, otpSetupResponse{
		Secret: b32Secret,
		URI:    uri,
	})
}

// --- Self-service: OTP verify (first time) ---

type otpVerifyRequest struct {
	Code string `json:"code"`
}

func (s *Server) handleOTPVerify(c echo.Context) error {
	var req otpVerifyRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	if user.OTPSecret == nil || *user.OTPSecret == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "OTP not set up — call POST /auth/otp/setup first")
	}

	if !verifyTOTP(*user.OTPSecret, req.Code) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid OTP code")
	}

	if err := s.db.VerifyOTP(ctx, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "verifying OTP")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "OTP enabled"})
}

// --- Self-service: OTP disable ---

func (s *Server) handleOTPDisable(c echo.Context) error {
	var req otpVerifyRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	// Re-authenticate with the current OTP code before removing the second
	// factor — otherwise a hijacked session could silently disable 2FA (#27).
	if user.OTPSecret == nil || *user.OTPSecret == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "OTP is not enabled")
	}
	if req.Code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "current OTP code is required to disable OTP")
	}
	if !verifyTOTP(*user.OTPSecret, req.Code) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid OTP code")
	}

	if err := s.db.ClearOTP(ctx, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "disabling OTP")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "OTP disabled"})
}

// --- Invite user (sends verification email) ---

type inviteRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func (s *Server) handleInviteUser(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	var req inviteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}
	if req.Name == "" {
		req.Name = req.Email
	}
	if req.Role == "" {
		req.Role = "reader"
	}
	validRoles := map[string]bool{"admin": true, "manager": true, "contributor": true, "reader": true}
	if !validRoles[req.Role] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
	}

	// Only admin can invite as admin or manager
	callerRole, _ := c.Get("user_role").(string)
	if (req.Role == "admin" || req.Role == "manager") && callerRole != "admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only admins can invite as admin or manager")
	}

	ctx := c.Request().Context()

	// Create user as INACTIVE until email is verified
	user := &db.User{Email: req.Email, Name: req.Name, Active: false}
	if err := s.db.UpsertUser(ctx, user); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating user: "+err.Error())
	}

	// Add user to the current organization with the specified role
	// (membership exists but user is inactive — can't authenticate until verified)
	if orgID > 0 {
		if err := s.db.AddOrgMember(ctx, orgID, user.ID, req.Role); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "adding to organization: "+err.Error())
		}
	}

	// Generate verification token
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	if err := s.db.CreateEmailVerification(ctx, user.ID, tokenHash); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating verification: "+err.Error())
	}

	// Send email
	if s.mailer == nil || !s.mailer.Enabled() {
		return echo.NewHTTPError(http.StatusInternalServerError, "email not configured (SMTP_HOST)")
	}

	m := s.orgMail(ctx, orgID)
	if err := s.mailer.SendVerificationBranded(req.Email, req.Name, m.PublicURL, token, m.Branding); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "sending email: "+err.Error())
	}

	// Log activity
	actor := getUserEmail(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "user_invited",
		Detail: fmt.Sprintf("Invited %s (%s) as %s", req.Name, req.Email, req.Role),
	})

	return c.JSON(http.StatusOK, map[string]string{
		"status": "invited",
		"email":  req.Email,
	})
}

// handleResendInvite re-sends the verification email to a pending (not yet
// verified) invited user — for invites that were lost or expired (#42).
func (s *Server) handleResendInvite(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}

	user, err := s.db.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	// Must be a member of this org — don't resend across organizations.
	if _, err := s.db.GetOrgMember(ctx, orgID, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user is not a member of this organization")
	}
	// Only pending invites can be resent; an active user has already accepted.
	if user.Active {
		return echo.NewHTTPError(http.StatusConflict, "user has already accepted the invite")
	}

	if s.mailer == nil || !s.mailer.Enabled() {
		return echo.NewHTTPError(http.StatusInternalServerError, "email not configured (SMTP_HOST)")
	}

	// Invalidate any still-live invite tokens before issuing a new one, so a
	// leaked/intercepted earlier link can't be used in parallel with this one.
	if err := s.db.InvalidateEmailVerifications(ctx, user.ID, "verify"); err != nil {
		log.Printf("resend-invite: invalidating old tokens for %s failed: %v", user.Email, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "creating verification")
	}

	// Fresh verification token (72h), same as the original invite.
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	if err := s.db.CreateEmailVerification(ctx, user.ID, hex.EncodeToString(hash[:])); err != nil {
		log.Printf("resend-invite: creating verification for %s failed: %v", user.Email, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "creating verification")
	}

	m := s.orgMail(ctx, orgID)
	if err := s.mailer.SendVerificationBranded(user.Email, user.Name, m.PublicURL, token, m.Branding); err != nil {
		log.Printf("resend-invite: sending email to %s failed: %v", user.Email, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to send invite email")
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  getUserEmail(c),
		Action: "user_invite_resent",
		Detail: fmt.Sprintf("Resent invite to %s", user.Email),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "invite_resent", "email": user.Email})
}

// --- Verify email + set password ---

type verifyEmailRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// handleVerifyEmail verifies an email token and sets the user's password.
// This endpoint is unauthenticated.
func (s *Server) handleVerifyEmail(c echo.Context) error {
	var req verifyEmailRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token is required")
	}
	if req.Password == "" || len(req.Password) < 7 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 7 characters")
	}

	ctx := c.Request().Context()

	// Look up verification token
	hash := sha256.Sum256([]byte(req.Token))
	tokenHash := hex.EncodeToString(hash[:])

	verification, err := s.db.LookupEmailVerification(ctx, tokenHash)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired verification link")
	}

	// Get the user
	user, err := s.db.GetUserByID(ctx, verification.UserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	// Hash password and save
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "hashing password")
	}

	if err := s.db.SetPassword(ctx, user.ID, string(passwordHash)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "saving password")
	}

	// Mark email as verified and activate user
	s.db.SetEmailVerified(ctx, user.ID)
	s.db.SetUserActive(ctx, user.ID, true)

	// Mark verification token as used
	s.db.UseEmailVerification(ctx, verification.ID)

	// Resolve org for the session token — auto-select if user belongs to exactly one org
	var orgID int
	var orgName string
	var orgSlug string
	var role string
	orgs, err := s.db.ListUserOrgs(ctx, user.ID)
	if err == nil && len(orgs) == 1 {
		orgID = orgs[0].ID
		orgName = orgs[0].Name
		orgSlug = orgs[0].Slug
		if orgRole, err := s.db.GetUserRole(ctx, orgID, user.ID); err == nil {
			role = orgRole
		}
	}
	if role == "" {
		role = "reader" // default fallback
	}

	// Create signed JWT session token (stateless — no DB row)
	loginToken, err := s.createSessionJWT(user, orgID, role, orgSlug, orgName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating session token: "+err.Error())
	}

	return c.JSON(http.StatusOK, loginResponse{
		Token:            loginToken,
		Email:            user.Email,
		Name:             user.Name,
		Role:             role,
		OrganizationID:   orgID,
		OrganizationName: orgName,
	})
}

// --- Personal Access Tokens (self-service) ---

// handleListMyAPIKeys returns all API keys for the authenticated user.
func (s *Server) handleListMyAPIKeys(c echo.Context) error {
	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	keys, err := s.db.ListUserAPIKeys(ctx, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "listing API keys: "+err.Error())
	}
	if keys == nil {
		keys = []db.APIKey{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": keys})
}

// handleCreateMyAPIKey creates a new personal access token for the authenticated user.
// Tokens are scoped to the current organization by default. Pass "scope": "global" to
// create an unscoped token (only useful for multi-org users).
func (s *Server) handleCreateMyAPIKey(c echo.Context) error {
	ctx := c.Request().Context()
	email := getUserEmail(c)

	var req struct {
		Name        string `json:"name"`
		Permissions string `json:"permissions"`
		Scope       string `json:"scope"` // "org" (default) or "global"
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.Permissions == "" {
		req.Permissions = "read-write"
	}

	// Default: scope token to current org. "global" creates an unscoped token.
	var orgScope *int
	orgID := getOrgID(c)
	if req.Scope != "global" && orgID > 0 {
		orgScope = &orgID
	}

	token, tokenHash := generateAPIKey()
	tok, err := s.db.CreateAPIKey(ctx, req.Name, tokenHash, email, req.Permissions, orgScope, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "creating API key: "+err.Error())
	}

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  email,
		Action: "api_key_created",
		Detail: fmt.Sprintf("Created personal access token %q (permissions: %s, org-scoped: %v)", req.Name, req.Permissions, orgScope != nil),
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"token":           token,
		"id":              tok.ID,
		"name":            tok.Name,
		"permissions":     tok.Permissions,
		"organization_id": tok.OrganizationID,
	})
}

// handleRevokeMyAPIKey revokes a personal access token owned by the authenticated user.
func (s *Server) handleRevokeMyAPIKey(c echo.Context) error {
	ctx := c.Request().Context()
	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid API key ID")
	}

	if err := s.db.RevokeAPIKey(ctx, user.ID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "revoking API key: "+err.Error())
	}

	orgID := getOrgID(c)
	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  email,
		Action: "api_key_revoked",
		Detail: fmt.Sprintf("Revoked personal access token %d", id),
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "API key revoked"})
}

// --- Logout (JWT revocation) ---

// handleLogout blocks the current JWT so it cannot be reused.
// POST /api/v1/auth/logout
func (s *Server) handleLogout(c echo.Context) error {
	authHeader := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return echo.NewHTTPError(http.StatusBadRequest, "no bearer token")
	}
	rawToken := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse to get expiry time
	claims, err := validateSessionJWT(rawToken, s.secret)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid token")
	}

	tokenHash := sha256Hash(rawToken)
	expiresAt := claims.ExpiresAt.Time

	ctx := c.Request().Context()
	if err := s.db.BlockJWT(ctx, tokenHash, expiresAt); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "blocking token")
	}

	// Cleanup expired entries
	s.db.CleanExpiredBlockedJWTs(ctx)
	s.db.CleanOldLoginAttempts(ctx)
	s.db.DeleteExpiredOIDCSessions(ctx)

	return c.JSON(http.StatusOK, map[string]string{"status": "logged out"})
}

// --- TOTP implementation (RFC 6238) ---

func generateAPIKey() (string, string) {
	raw := make([]byte, 32)
	rand.Read(raw)
	token := "isms_" + hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:])
}

// verifyTOTP checks a 6-digit TOTP code against a base32-encoded secret.
// Accepts current time step and one step before/after (±30s window).
func verifyTOTP(base32Secret, code string) bool {
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(base32Secret)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	timeStep := int64(30)

	// Check current, previous, and next time steps
	for _, offset := range []int64{0, -1, 1} {
		counter := (now / timeStep) + offset
		if generateTOTPCode(secret, counter) == code {
			return true
		}
	}
	return false
}

func generateTOTPCode(secret []byte, counter int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))

	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0F
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7FFFFFFF
	otp := code % uint32(math.Pow10(6))

	return fmt.Sprintf("%06d", otp)
}
