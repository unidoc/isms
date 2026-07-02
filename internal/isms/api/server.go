// Package api provides the ISMS JSON API and serves the Vue SPA.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gopkg.in/yaml.v3"
	"isms.sh/internal/isms/blob"
	"isms.sh/internal/isms/db"
	"isms.sh/internal/isms/mail"
	"isms.sh/internal/isms/notify"
	"isms.sh/internal/isms/store"
)

// Server is the ISMS API server.
type Server struct {
	db       *db.DB
	notifier *notify.Notifier
	mailer   *mail.Mailer
	echo     *echo.Echo
	addr     string
	webDir   string
	webFS    fs.FS    // embedded web assets (optional, used if webDir is empty)
	stores   sync.Map // orgID -> *store.Store cache

	// Unified storage for org files (branding, evidence, etc.).
	// Local filesystem by default, S3 with ISMS_STORAGE_BACKEND=s3.
	blobs blob.Store

	// JWT session signing secret
	secret string

	// WebAuthn / passkey support
	webauthn             *gowebauthn.WebAuthn
	passkeyRegistrations sync.Map // email -> *webauthn.SessionData
	passkeyLogins        sync.Map // email -> *webauthn.SessionData

	// Platform-level legal docs (markdown files on disk, rendered and served)
	termsFile        string // ISMS_TERMS_FILE
	privacyFile      string // ISMS_PRIVACY_FILE
	hidePoweredBy    bool   // ISMS_HIDE_POWERED_BY=1
	subdomainRouting bool   // ISMS_SUBDOMAIN_ROUTING=1 enables (default off)
	apexHost         string // derived from ISMS_BASE_URL — the deployment's canonical apex

	// In-memory search index per org — avoids 10+ DB queries per keystroke.
	searchIndex *SearchIndex

	// Cloudflare Access: shared JWKS cache + JIT provisioning config, used by
	// both the auth middleware and the cf-session web-login handler.
	cfKeyCache  *cfKeyCache
	cfProvision cfProvisionConfig
}

// storeForOrg returns (or creates and caches) a store for the given organization.
// Uses LoadOrStore to prevent duplicate Store creation under concurrent requests.
// orgMailCtx carries everything an email needs to address a tenant correctly:
// the brand (sender name + color) and the org's own base URLs for links.
type orgMailCtx struct {
	Branding mail.Branding
	// AppURL is the base for in-app, org-scoped pages (e.g. /reviews/123): a
	// tenant subdomain/custom domain, or <base>/<slug> on path-based hosts.
	AppURL string
	// PublicURL is the base for public pages (e.g. /verify-email), which are
	// never mounted under /:org: a tenant subdomain/custom domain, or the bare
	// <base> on path-based hosts.
	PublicURL string
}

// orgMail resolves the per-org email context (brand + link bases). It falls back
// to neutral defaults — the bare ISMS_BASE_URL and an "ISMS" brand — so a
// tenant's mail never carries another org's identity, the operator's SMTP_FROM
// display name (#16 sender leak), or a link into the wrong org.
func (s *Server) orgMail(ctx context.Context, orgID int) orgMailCtx {
	raw := strings.TrimRight(os.Getenv("ISMS_BASE_URL"), "/")
	m := orgMailCtx{AppURL: raw, PublicURL: raw}
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil || org == nil {
		return m
	}
	m.Branding.Name = org.Name
	if color, err := s.db.GetOrgSetting(ctx, orgID, "branding_color"); err == nil {
		m.Branding.Color = color
	}
	m.AppURL, m.PublicURL = orgURLs(raw, org, s.subdomainRouting)
	return m
}

// orgURLs returns the per-org base URLs for in-app (org-scoped) pages and for
// public pages, mirroring the SPA router's mounting rules:
//   - custom domain:  https://<domain> for both
//   - subdomain host: https://<slug>.<apex> for both (the org IS the host)
//   - path-based:     <base>/<slug> for app pages, <base> for public pages
//
// Subdomain URLs are used only when subdomainRouting is enabled (the deployment
// actually serves tenants on wildcard subdomains); otherwise links stay
// path-based, so the generated URL always matches how requests are routed —
// never inferred from the host shape alone.
func orgURLs(baseURL string, org *db.Organization, subdomainRouting bool) (app, public string) {
	if org.Domain != nil && *org.Domain != "" {
		d := *org.Domain
		if !strings.Contains(d, "://") {
			scheme := "https"
			if strings.HasPrefix(baseURL, "http://") {
				scheme = "http"
			}
			d = scheme + "://" + d
		}
		d = strings.TrimRight(d, "/")
		return d, d
	}
	const sep = "://"
	i := strings.Index(baseURL, sep)
	if i < 0 {
		return baseURL, baseURL
	}
	scheme := baseURL[:i]
	hostAndPort := baseURL[i+len(sep):]
	host := hostAndPort
	port := ""
	if c := strings.LastIndex(hostAndPort, ":"); c > 0 {
		host = hostAndPort[:c]
		port = hostAndPort[c:] // includes ':'
	}
	// Only emit subdomain URLs when the deployment actually routes by subdomain.
	// Otherwise (path-based, incl. a single-tenant box on a real domain) links
	// must stay path-based — never guess subdomain from the host shape alone.
	canSubdomain := subdomainRouting &&
		strings.Contains(host, ".") &&
		!strings.HasPrefix(host, "localhost") &&
		!isIPLiteral(host)
	if canSubdomain {
		apex := strings.TrimPrefix(host, "www.")
		sub := fmt.Sprintf("%s://%s.%s%s", scheme, org.Slug, apex, port)
		return sub, sub
	}
	return baseURL + "/" + org.Slug, baseURL
}

func (s *Server) storeForOrg(ctx context.Context, orgID int) (*store.Store, error) {
	if v, ok := s.stores.Load(orgID); ok {
		return v.(*store.Store), nil
	}
	// Create new store
	org, err := s.db.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}
	st, err := store.NewBare(org.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("opening repo for org %d: %w", orgID, err)
	}
	// Configure SSH commit signing if key is set
	if keyPath := os.Getenv("ISMS_SIGNING_KEY"); keyPath != "" {
		committerName := os.Getenv("ISMS_SIGNING_NAME")
		if committerName == "" {
			committerName = "isms.sh"
		}
		committerEmail := os.Getenv("ISMS_SIGNING_EMAIL")
		if committerEmail == "" {
			committerEmail = "git@isms.sh"
		}
		st.SetSigning(&store.SigningConfig{
			KeyPath:        keyPath,
			CommitterName:  committerName,
			CommitterEmail: committerEmail,
		})
	}
	// Use LoadOrStore to avoid duplicates — another goroutine may have raced us
	actual, _ := s.stores.LoadOrStore(orgID, st)
	return actual.(*store.Store), nil
}

// New creates a new API server.
func New(addr, webDir string, database *db.DB) *Server {
	return NewWithFS(addr, webDir, database, nil)
}

// NewWithFS creates a new API server with an optional embedded web filesystem.
func NewWithFS(addr, webDir string, database *db.DB, embeddedFS fs.FS) *Server {
	srv := &Server{db: database, addr: addr, webDir: webDir, webFS: embeddedFS, searchIndex: NewSearchIndex()}

	// Server secret — master key, derived into purpose-specific keys via HKDF.
	secret := os.Getenv("ISMS_SECRET")
	if secret == "" {
		log.Fatal("ISMS_SECRET is required. Generate one with: openssl rand -hex 32")
	} else if len(secret) < 32 {
		log.Fatal("ISMS_SECRET must be at least 32 characters")
	}
	// Derive separate keys for JWT signing and encryption (domain separation).
	jwtKey := deriveKey([]byte(secret), "isms-jwt-hs256-v1", 32)
	aesKey := deriveKey([]byte(secret), "isms-aes-gcm-secrets-v1", 32)
	srv.secret = string(jwtKey)
	database.SetEncryptionKey(string(aesKey))

	// Platform-level legal docs and branding
	srv.termsFile = os.Getenv("ISMS_TERMS_FILE")
	srv.privacyFile = os.Getenv("ISMS_PRIVACY_FILE")
	srv.hidePoweredBy = os.Getenv("ISMS_HIDE_POWERED_BY") == "1"
	// Default OFF — subdomain routing requires wildcard DNS + wildcard TLS,
	// which is a production-only setup. Self-hosted, dev, and demo
	// deployments use path-based routing. Set ISMS_SUBDOMAIN_ROUTING=1 on
	// the public prod deployment that has `*.isms.sh` wildcards.
	srv.subdomainRouting = os.Getenv("ISMS_SUBDOMAIN_ROUTING") == "1"

	// SMTP mailer from env vars
	srv.mailer = mail.New(mail.Config{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     os.Getenv("SMTP_PORT"),
		User:     os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("SMTP_FROM"),
	})
	// Notifier — reads per-org settings from Postgres at send time.
	// BaseURL is the only platform-level config (used for links in messages).
	srv.notifier = notify.New(notify.Config{
		BaseURL: os.Getenv("ISMS_BASE_URL"),
	})

	// Storage backend for org assets (logos, favicons, evidence, etc.).
	// blob.NewFromEnv reads ISMS_DATA_DIR + ISMS_STORAGE_BACKEND + ISMS_S3_*
	// — the same env vars the demo seeder and any other code path use, so
	// configuration lives in exactly one place (internal/isms/blob/blob.go).
	store, err := blob.NewFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	srv.blobs = store
	if os.Getenv("ISMS_STORAGE_BACKEND") == "s3" {
		log.Println("Storage backend: S3 (bucket: " + os.Getenv("ISMS_S3_BUCKET") + ")")
	}

	// WebAuthn / passkey configuration
	if baseURL := os.Getenv("ISMS_BASE_URL"); baseURL != "" {
		rpID := extractHostname(baseURL)
		wconfig := &gowebauthn.Config{
			RPDisplayName: "ISMS",
			RPID:          rpID,
			RPOrigins:     []string{baseURL},
		}
		if wan, err := gowebauthn.New(wconfig); err == nil {
			srv.webauthn = wan
		}
	}

	srv.echo = echo.New()
	srv.echo.HideBanner = true

	// IP extractor — honor Cloudflare's CF-Connecting-IP first, then X-Real-IP
	// (set by our nginx), then standard X-Forwarded-For. This makes c.RealIP()
	// return the actual client IP through the CF → nginx → Go chain, which is
	// what auth rate limiting and login_attempts tracking depend on.
	srv.echo.IPExtractor = func(req *http.Request) string {
		if ip := req.Header.Get("Cf-Connecting-Ip"); ip != "" {
			return ip
		}
		if ip := req.Header.Get("X-Real-Ip"); ip != "" {
			return ip
		}
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			// First entry is the original client; rest are proxies.
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
		if req.RemoteAddr != "" {
			// Strip port. net.SplitHostPort handles bracketed IPv6
			// ("[::1]:54321" → "::1") — a naive LastIndex(':') cut would
			// return "[::1]", which downstream inet casts reject.
			if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
				return host
			}
			return req.RemoteAddr
		}
		return ""
	}

	// Security headers — must be before other middleware
	srv.echo.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Frame-Options", "DENY")
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			c.Response().Header().Set("X-XSS-Protection", "1; mode=block")
			c.Response().Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			c.Response().Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: blob:; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net")
			c.Response().Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			return next(c)
		}
	})

	// API documentation (before auth middleware so /docs and /api/openapi.yaml are public)
	srv.registerDocs()

	srv.echo.Use(middleware.Logger())
	srv.echo.Use(middleware.Recover())
	// Global request body cap (defends against memory exhaustion).
	// Per-route overrides apply for endpoints that need higher limits (e.g. evidence uploads).
	srv.echo.Use(middleware.BodyLimit("10M"))
	corsOrigins := []string{}
	if origin := os.Getenv("ISMS_CORS_ORIGIN"); origin != "" {
		corsOrigins = strings.Split(origin, ",")
	} else if baseURL := os.Getenv("ISMS_BASE_URL"); baseURL != "" {
		corsOrigins = []string{baseURL}
	}
	if len(corsOrigins) == 0 {
		// Refuse to fall back to localhost in production — that would let any
		// localhost browser drive the API. Require explicit dev opt-in.
		if os.Getenv("ISMS_DEV_MODE") == "1" {
			corsOrigins = []string{"http://localhost:*"}
		} else {
			log.Fatal("ISMS_BASE_URL or ISMS_CORS_ORIGIN must be set in production (set ISMS_DEV_MODE=1 to allow localhost)")
		}
	}
	srv.echo.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: corsOrigins,
	}))

	// Rate limiting is intentionally NOT applied here — it's handled at the
	// edge (nginx / Cloudflare) where real client IPs are visible. Doing it
	// in-process would either share one bucket across all users (when behind
	// a proxy) or duplicate edge limits and make 429s harder to attribute.

	// Org resolution from subdomain / custom domain / path prefix.
	// Must run BEFORE auth so org_id is available for token verification.
	//
	// The base domain (host part of ISMS_BASE_URL) is the apex for this
	// deployment. Subdomains of it (e.g. <slug>.isms.sh) are treated as
	// tenant subdomains; the apex itself is path-based. ISMS_DOMAIN is an
	// explicit override for unusual setups.
	baseDomain := os.Getenv("ISMS_DOMAIN")
	if baseDomain == "" {
		if baseURL := os.Getenv("ISMS_BASE_URL"); baseURL != "" {
			baseDomain = extractHostname(baseURL)
		}
	}
	srv.apexHost = baseDomain
	if baseDomain != "" {
		srv.echo.Use(OrgResolverMiddleware(database, baseDomain, srv.subdomainRouting))
	}

	// Authentication + role enforcement
	teamDomain := os.Getenv("CLOUDFLARE_TEAM_DOMAIN") // e.g. mycompany.cloudflareaccess.com
	cfAudience := os.Getenv("ISMS_CF_AUDIENCE")       // CF Access Application Audience (AUD) tag
	if teamDomain != "" {
		srv.cfKeyCache = newCFKeyCache(teamDomain, cfAudience)
	}
	// JIT provisioning on first CF Access login (opt-in, default off). See #98.
	// Creates the user row only — org membership/role is an explicit admin/CLI step.
	srv.cfProvision = cfProvisionConfig{
		Enabled: os.Getenv("ISMS_CF_AUTO_PROVISION") == "true" || os.Getenv("ISMS_CF_AUTO_PROVISION") == "1",
	}
	srv.echo.Use(AuthMiddleware(AuthConfig{
		CloudflareTeamDomain: teamDomain,
		CloudflareAudience:   cfAudience,
		DB:                   database,
		Secret:               srv.secret,
		KeyCache:             srv.cfKeyCache,
		Provision:            srv.cfProvision,
	}))
	srv.echo.Use(srv.RoleMiddleware())
	// Tenant isolation model (defense in depth):
	//
	// 1. Application-layer: every org-scoped query includes WHERE organization_id = $1.
	//    This is the primary enforcement and is comprehensive across all DB methods.
	//
	// 2. Postgres RLS (defense-in-depth): RLS policies are enabled in the schema.
	//    Enforced on transactional operations via WithOrgTx(), which does
	//    SET LOCAL app.current_org_id = X inside each transaction.
	//
	// 3. RLSMiddleware() is defined but NOT registered globally because wrapping
	//    every request in a single transaction causes connection pool exhaustion
	//    under concurrent load (especially git operations that hold locks).
	//    SET LOCAL only works within a transaction, so a per-request middleware
	//    cannot protect non-transactional pool queries anyway.
	//
	// Handlers that need guaranteed RLS isolation should use WithOrgTx() directly.
	// The combination of app-layer WHERE clauses + RLS on transactions provides
	// two independent layers of tenant isolation.

	// Auto-detect Vue dist directory if not specified.
	if srv.webDir == "" {
		for _, candidate := range []string{"web/dist", "../web/dist"} {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				srv.webDir = candidate
				break
			}
		}
	}

	srv.routes()
	return srv
}

// Start runs the server until ctx is cancelled, then performs a graceful shutdown.
// On ctx.Done, in-flight requests are given up to 30s to complete via echo.Shutdown.
// The caller is expected to plumb an os/signal-cancelled context (SIGINT/SIGTERM).
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := s.echo.Start(s.addr)
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Println("shutdown signal received — draining (30s grace)...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.echo.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		log.Println("server stopped cleanly")
		return nil
	}
}

func (s *Server) routes() {
	api := s.echo.Group("/api/v1")

	// Unauthenticated endpoints — per-IP rate limit applied here, NOT on the
	// authenticated paths below (refresh/logout/etc.) which are legitimate
	// session traffic. Per-email brute-force protection still lives inside
	// the individual handlers (handleLogin, handlePasskeyLoginComplete).
	authLimit := AuthRateLimitMiddleware(s.db)
	api.POST("/auth/login", s.handleLogin, authLimit)
	api.GET("/auth/cf-session", s.handleCFSession, authLimit) // Cloudflare Access → web session (#98)
	api.POST("/auth/signup", s.handleSignup, authLimit)
	api.POST("/auth/verify-email", s.handleVerifyEmail, authLimit)
	api.POST("/auth/forgot-password", s.handleForgotPassword, authLimit)
	api.POST("/auth/passkey/login/begin", s.handlePasskeyLoginBegin, authLimit)
	api.POST("/auth/passkey/login/complete", s.handlePasskeyLoginComplete, authLimit)

	// Token refresh and logout (authenticated)
	api.POST("/auth/refresh", s.handleRefresh)
	api.POST("/auth/logout", s.handleLogout)
	api.POST("/auth/switch-org", s.handleSwitchOrg)

	// User management (authenticated)
	api.POST("/auth/invite", s.handleInviteUser)
	api.POST("/auth/resend-invite", s.handleResendInvite)

	// Self-service (authenticated)
	api.PUT("/auth/password", s.handleChangePassword)
	api.PUT("/auth/profile", s.handleUpdateProfile)
	api.POST("/auth/otp/setup", s.handleOTPSetup)
	api.POST("/auth/otp/verify", s.handleOTPVerify)
	api.DELETE("/auth/otp", s.handleOTPDisable)

	// Personal Access Tokens (authenticated, self-service)
	api.GET("/auth/api-keys", s.handleListMyAPIKeys)
	api.POST("/auth/api-keys", s.handleCreateMyAPIKey)
	api.DELETE("/auth/api-keys/:id", s.handleRevokeMyAPIKey)

	// Passkeys (authenticated)
	api.POST("/auth/passkey/register/begin", s.handlePasskeyRegisterBegin)
	api.POST("/auth/passkey/register/complete", s.handlePasskeyRegisterComplete)
	api.GET("/auth/passkeys", s.handleListPasskeys)
	api.PUT("/auth/passkeys/:id", s.handleRenamePasskey)
	api.DELETE("/auth/passkeys/:id", s.handleDeletePasskey)

	// Auth / user
	api.GET("/me", s.handleMe)
	api.GET("/me/organizations", s.handleMyOrganizations)
	api.GET("/users", s.handleListUsers)
	api.POST("/users", s.handleUpsertUser)
	api.POST("/organizations", s.handleCreateOrganization)
	api.GET("/templates/available", s.handleListAvailableTemplates)
	api.POST("/templates", s.handleAddTemplate)
	api.DELETE("/templates/:name", s.handleRemoveTemplate)

	// Config
	api.GET("/config", s.handleGetConfig)

	// Dynamic documents — scans git repo, no hardcoded structure
	api.GET("/documents/needs-review", s.handleNeedsReview)
	api.GET("/documents/all", s.handleListAllDocuments)
	api.GET("/documents/search", s.handleSearchDocuments)
	api.GET("/search", s.handleUniversalSearch)
	api.GET("/documents/changed", s.handleChangedDocuments)
	api.GET("/documents/file/:folder/:id", s.handleGetDocument)
	api.GET("/documents/:docId/body", s.handleGetDocumentBody)
	api.GET("/documents/:docId/blame", s.handleDocumentBlame)
	api.PUT("/documents/:docId/metadata", s.handleUpdateDocumentMetadata)
	api.PUT("/documents/:docId/content", s.handleUpdateDocumentContent)
	api.POST("/documents", s.handleCreateDocument)
	api.POST("/documents/folders", s.handleCreateFolder)
	api.DELETE("/documents/:docId", s.handleDeleteDocument)
	api.GET("/documents/validate", s.handleValidateDocuments)

	// Assets (Postgres)
	api.GET("/assets", s.handleListAssets)
	api.GET("/assets/stats", s.handleAssetStats)
	api.GET("/assets/:id", s.handleGetAsset)
	api.POST("/assets", s.handleAddAsset)
	api.PUT("/assets/:id", s.handleUpdateAsset)
	api.DELETE("/assets/:id", s.handleDeleteAsset)
	api.GET("/assets/:id/reviews", s.handleListAssetReviews)
	api.POST("/assets/:id/reviews", s.handleCreateAssetReview)
	api.GET("/assets/:id/readings", s.handleListAssetReadings)
	api.POST("/assets/:id/readings", s.handleCreateAssetReading)

	// Systems
	api.GET("/systems", s.handleListSystems)
	api.GET("/systems/stats", s.handleSystemStats)
	api.GET("/systems/:id", s.handleGetSystem)
	api.POST("/systems", s.handleCreateSystem)
	api.PUT("/systems/:id", s.handleUpdateSystem)
	api.DELETE("/systems/:id", s.handleDeleteSystem)
	api.GET("/systems/:id/access-reviews", s.handleListAccessReviews)
	api.POST("/systems/:id/access-reviews", s.handleCreateAccessReview)
	api.GET("/systems/:id/readings", s.handleListSystemReadings)
	api.POST("/systems/:id/readings", s.handleCreateSystemReading)
	api.DELETE("/access-reviews/:id", s.handleDeleteAccessReview)

	// Risks (Postgres)
	api.GET("/risks", s.handleListRisks)
	api.GET("/risks/stats", s.handleRiskStats)
	api.GET("/risks/:id", s.handleGetRisk)
	api.POST("/risks", s.handleAddRisk)
	api.PUT("/risks/:id", s.handleUpdateRisk)
	api.DELETE("/risks/:id", s.handleDeleteRisk)
	api.GET("/risks/matrix", s.handleRiskMatrix)
	api.GET("/risks/:id/advisories", s.handleRiskAdvisories)
	api.GET("/risks/:id/readings", s.handleListRiskReadings)
	api.POST("/risks/:id/readings", s.handleCreateRiskReading)

	// Suppliers (Postgres)
	api.GET("/suppliers", s.handleListSuppliers)
	api.GET("/suppliers/stats", s.handleSupplierStats)
	api.GET("/suppliers/:id", s.handleGetSupplier)
	api.POST("/suppliers", s.handleAddSupplier)
	api.PUT("/suppliers/:id", s.handleUpdateSupplier)
	api.DELETE("/suppliers/:id", s.handleDeleteSupplier)
	api.GET("/suppliers/:id/reviews", s.handleListSupplierReviews)
	api.POST("/suppliers/:id/reviews", s.handleCreateSupplierReview)
	api.GET("/suppliers/:id/readings", s.handleListSupplierReadings)
	api.POST("/suppliers/:id/readings", s.handleCreateSupplierReading)

	// Inbox (Postgres + git)
	api.GET("/inbox", s.handleInbox)
	api.GET("/inbox/dump", s.handleInboxDump)

	// Reviews (Postgres)
	api.GET("/reviews", s.handleListReviews)
	api.GET("/reviews/stats", s.handleReviewStats)
	api.POST("/reviews", s.handleCreateReview)
	api.GET("/reviews/:id", s.handleGetReview)
	api.GET("/reviews/:id/assignments", s.handleListReviewAssignments)
	api.GET("/reviews/:id/suggestions", s.handleListSuggestions)
	api.PUT("/reviews/:id/status", s.handleUpdateReviewStatus)
	api.POST("/reviews/:id/forward", s.handleForwardReview)
	api.GET("/reviews/:id/timeline", s.handleReviewTimeline)
	api.GET("/reviews/:id/diff", s.handleReviewDiff)
	api.POST("/reviews/:id/comment", s.handleAddReviewComment)
	api.POST("/reviews/:id/approve", s.handleReviewApprove)
	api.POST("/reviews/:id/merge", s.handleMergeReview)
	api.POST("/reviews/:id/accept-and-merge", s.handleAcceptAndMerge)
	api.GET("/reviews/:id/policy-status", s.handleReviewPolicyStatus)
	api.PUT("/reviews/:id/content", s.handleUpdateReviewContent)
	api.GET("/reviews/:id/content", s.handleGetReviewContent)
	api.POST("/documents/:docId/reviews", s.handleReviewSend)
	api.POST("/documents/:docId/confirm-review", s.handleConfirmDocumentReview)
	api.GET("/agent/pending-actions", s.handleAgentPendingActions)

	// Comments (Postgres)
	api.GET("/comments/open", s.handleAllOpenComments)
	api.GET("/documents/:docId/comments", s.handleListCommentsDB)
	api.POST("/comments", s.handleAddCommentDB)
	api.POST("/comments/:id/resolve", s.handleResolveCommentDB)
	api.POST("/comments/:id/accept", s.handleAcceptSuggestion)
	api.POST("/comments/:id/reject", s.handleRejectSuggestion)

	// Approvals (Postgres)
	api.GET("/documents/:docId/approvals", s.handleListApprovals)

	// Decision log (Postgres — immutable audit trail)
	api.GET("/documents/:docId/decisions", s.handleListDocumentDecisions)
	api.GET("/reviews/:id/decisions", s.handleListReviewDecisions)

	// Tasks (Postgres)
	api.GET("/tasks", s.handleListTasks)
	api.GET("/tasks/stats", s.handleTaskStats)
	api.GET("/tasks/:id", s.handleGetTask)
	api.POST("/tasks", s.handleCreateTask)
	api.PUT("/tasks/:id", s.handleUpdateTask)
	api.PUT("/tasks/:id/status", s.handleUpdateTaskStatus)
	api.DELETE("/tasks/:id", s.handleDeleteTask)

	// Change requests (Postgres)
	api.GET("/changes", s.handleListChanges)
	api.GET("/changes/stats", s.handleChangeStats)
	api.GET("/changes/:id", s.handleGetChange)
	api.POST("/changes", s.handleCreateChange)
	api.PUT("/changes/:id", s.handleUpdateChange)
	api.PUT("/changes/:id/status", s.handleUpdateChangeStatus)
	api.DELETE("/changes/:id", s.handleDeleteChange)

	// Implementation status (Postgres + git)
	api.GET("/implementation", s.handleListImplementation)
	api.PUT("/implementation/:itemId", s.handleUpdateImplementation)
	api.GET("/implementation/progress", s.handleImplementationProgress)

	// Notifications (Postgres)
	api.GET("/notifications", s.handleListNotifications)
	api.POST("/notifications/:id/read", s.handleMarkRead)
	api.POST("/notifications/read-all", s.handleMarkAllRead)
	api.GET("/notifications/count", s.handleUnreadCount)

	// Entity Suggestions
	api.POST("/suggestions", s.handleCreateEntitySuggestion)
	api.GET("/suggestions", s.handleListEntitySuggestions)
	api.GET("/suggestions/:id", s.handleGetEntitySuggestion)
	api.PUT("/suggestions/:id", s.handleUpdateEntitySuggestion)
	api.DELETE("/suggestions/:id", s.handleDeleteEntitySuggestion)
	api.POST("/suggestions/:id/claim", s.handleClaimEntitySuggestion)
	api.POST("/suggestions/:id/apply", s.handleApplyEntitySuggestion)
	api.POST("/suggestions/:id/reject", s.handleRejectEntitySuggestion)
	api.POST("/suggestions/:id/withdraw", s.handleWithdrawEntitySuggestion)

	// Overdue dashboard
	api.GET("/overdue", s.handleOverdueSummary)
	api.POST("/overdue/tasks", s.handleCreateOverdueTasks)

	// Audits (Postgres)
	api.GET("/audit/programmes", s.handleListAuditProgrammes)
	api.POST("/audit/programmes", s.handleCreateAuditProgramme)
	api.GET("/audit/programmes/:id", s.handleGetAuditProgramme)
	api.PUT("/audit/programmes/:id", s.handleUpdateAuditProgramme)
	api.DELETE("/audit/programmes/:id", s.handleDeleteAuditProgramme)
	api.GET("/audit/calendar", s.handleAuditCalendar)
	api.GET("/audit/findings", s.handlePaginatedAuditFindings)
	api.GET("/audits", s.handleListAudits)
	api.POST("/audits", s.handleCreateAudit)
	api.GET("/audits/:id", s.handleGetAudit)
	api.PUT("/audits/:id", s.handleUpdateAudit)
	api.PUT("/audits/:id/status", s.handleUpdateAuditStatus)
	api.GET("/audits/:id/items", s.handleListAuditItems)
	api.POST("/audits/:id/items", s.handleCreateAuditItem)
	api.PUT("/audit-items/:id", s.handleUpdateAuditItem)
	api.DELETE("/audit-items/:id", s.handleDeleteAuditItem)
	api.GET("/audits/:id/findings", s.handleListAuditFindingsForAudit)
	api.POST("/audit-findings", s.handleAddAuditFinding)
	api.GET("/audit-findings/:id", s.handleGetAuditFinding)
	api.PUT("/audit-findings/:id", s.handleUpdateAuditFinding)
	api.PUT("/audit-findings/:id/status", s.handleUpdateAuditFindingStatus)
	api.DELETE("/audit-findings/:id", s.handleDeleteAuditFinding)

	// Legal Register (Postgres)
	api.GET("/legal", s.handleListLegal)
	api.GET("/legal/stats", s.handleLegalStats)
	api.POST("/legal", s.handleCreateLegal)
	api.GET("/legal/:id", s.handleGetLegal)
	api.PUT("/legal/:id", s.handleUpdateLegal)
	api.DELETE("/legal/:id", s.handleDeleteLegal)
	api.GET("/legal/:id/readings", s.handleListLegalReadings)
	api.POST("/legal/:id/readings", s.handleCreateLegalReading)

	// Incidents (Postgres)
	api.GET("/incidents", s.handleListIncidents)
	api.POST("/incidents", s.handleCreateIncident)
	api.GET("/incidents/stats", s.handleIncidentStats)
	api.GET("/incidents/:id", s.handleGetIncident)
	api.PUT("/incidents/:id", s.handleUpdateIncident)
	api.PUT("/incidents/:id/status", s.handleUpdateIncidentStatus)
	api.DELETE("/incidents/:id", s.handleDeleteIncident)

	// Corrective Actions (Postgres)
	api.GET("/corrective-actions", s.handleListCorrectiveActions)
	api.POST("/corrective-actions", s.handleCreateCorrectiveAction)
	api.GET("/corrective-actions/stats", s.handleCorrectiveActionStats)
	api.GET("/corrective-actions/:id", s.handleGetCorrectiveAction)
	api.PUT("/corrective-actions/:id", s.handleUpdateCorrectiveAction)
	api.PUT("/corrective-actions/:id/status", s.handleUpdateCorrectiveActionStatus)
	api.DELETE("/corrective-actions/:id", s.handleDeleteCorrectiveAction)

	// Programs (Postgres)
	api.GET("/programs", s.handleListPrograms)
	api.POST("/programs", s.handleCreateProgram)
	api.GET("/programs/:id", s.handleGetProgram)
	api.PUT("/programs/:id", s.handleUpdateProgram)
	api.DELETE("/programs/:id", s.handleDeleteProgram)

	// Objectives (Postgres)
	api.GET("/objectives", s.handleListObjectives)
	api.GET("/objectives/stats", s.handleObjectiveStats)
	api.POST("/objectives", s.handleCreateObjective)
	api.GET("/objectives/:id", s.handleGetObjective)
	api.PUT("/objectives/:id", s.handleUpdateObjective)
	api.DELETE("/objectives/:id", s.handleDeleteObjective)
	api.POST("/objectives/:id/archive", s.handleArchiveObjective)
	api.POST("/objectives/:id/unarchive", s.handleUnarchiveObjective)

	// Checkins (Postgres)
	api.GET("/objectives/:id/checkins", s.handleListCheckins)
	api.POST("/objectives/:id/checkins", s.handleCreateCheckin)
	api.PUT("/checkins/:id", s.handleUpdateCheckin)
	api.DELETE("/checkins/:id", s.handleDeleteCheckin)

	// Evidence (S3-backed) — allow larger uploads than the global 10M cap.
	api.POST("/checkins/:id/evidence", s.handleUploadEvidence, middleware.BodyLimit("50M"))
	api.GET("/checkins/:id/evidence", s.handleListEvidence)
	api.GET("/evidence/:id/download", s.handleDownloadEvidence)
	api.DELETE("/evidence/:id", s.handleDeleteEvidence)

	// OIDC auth (unauthenticated — skipped in middleware)
	// Rate-limit the authorize endpoint (initiates the OIDC handshake);
	// providers/callback are read-only or driven by the IdP, no need to limit.
	api.GET("/auth/oidc/providers", s.handleOIDCProviders)
	api.GET("/auth/oidc/authorize", s.handleOIDCAuthorize, authLimit)
	api.GET("/auth/oidc/callback", s.handleOIDCCallback)

	// Admin (role-gated)
	admin := api.Group("/admin")
	admin.Use(s.AdminOnly())
	admin.GET("/members", s.handleAdminListMembers)
	admin.PUT("/members/:userId/role", s.handleAdminUpdateRole)
	admin.DELETE("/members/:userId", s.handleAdminRemoveMember)
	admin.GET("/api-keys", s.handleAdminListAPIKeys)
	admin.GET("/oidc", s.handleAdminListOIDC)
	admin.POST("/oidc", s.handleAdminCreateOIDC)
	admin.PUT("/oidc/:id", s.handleAdminUpdateOIDC)
	admin.DELETE("/oidc/:id", s.handleAdminDeleteOIDC)
	admin.POST("/oidc/:id/test", s.handleAdminTestOIDC)
	admin.GET("/settings", s.handleAdminListSettings)
	admin.PUT("/settings", s.handleAdminUpdateSetting)
	admin.POST("/branding/upload", s.handleBrandingUpload)
	admin.DELETE("/branding/:name", s.handleBrandingDelete)
	admin.GET("/policies", s.handleAdminListPolicies)
	admin.POST("/policies", s.handleAdminCreatePolicy)
	admin.PUT("/policies/:id", s.handleAdminUpdatePolicy)
	admin.DELETE("/policies/:id", s.handleAdminDeletePolicy)

	// Entity cross-references (bidirectional links)
	api.GET("/references", s.handleListReferences)
	api.POST("/references", s.handleCreateReference)
	api.DELETE("/references/:id", s.handleDeleteReference)

	// Entity Comments (generic comments on any entity)
	api.GET("/entity-comments/:type/:id", s.handleListEntityComments)
	api.POST("/entity-comments", s.handleCreateEntityComment)
	api.POST("/entity-comments/:id/resolve", s.handleResolveEntityComment)
	api.DELETE("/entity-comments/:id", s.handleDeleteEntityComment)

	// Entity Reactions (emoji reactions)
	api.POST("/reactions", s.handleToggleReaction)
	api.GET("/reactions/:targetType/:targetId", s.handleListReactions)

	// Entity Changelog (audit trail)
	api.GET("/changelog", s.handleListChangelog)
	api.GET("/changelog/:type/:id", s.handleEntityChangelog)

	// Document versions & diff (Postgres + git)
	api.GET("/documents/:docId/versions", s.handleListVersions)
	api.GET("/documents/:docId/diff", s.handleDocumentDiff)

	// Activity (Postgres)
	api.GET("/activity", s.handleListActivity)
	api.GET("/documents/:docId/activity", s.handleDocumentActivity)

	// Git smart HTTP protocol (authenticated via Bearer token or Basic auth)
	s.echo.GET("/git/:uuid/info/refs", s.handleGitInfoRefs)
	s.echo.POST("/git/:uuid/git-upload-pack", s.handleGitUploadPack)
	s.echo.POST("/git/:uuid/git-receive-pack", s.handleGitReceivePack)

	// Branding & legal
	s.echo.GET("/branding/logo", s.handleLogo)
	s.echo.GET("/branding/favicon.ico", s.handleFavicon)
	s.echo.GET("/favicon.ico", s.handleFavicon) // standard browser path
	s.echo.GET("/terms", s.handleTerms)
	s.echo.GET("/privacy", s.handlePrivacy)

	// Health — runs a real SELECT against the organizations table so we catch
	// "TCP up, statement engine wedged" and "schema not migrated" cases that a
	// pure pgx.Ping would silently pass. Tight 2s budget so a slow probe call
	// doesn't itself become a liveness flap.
	s.echo.GET("/healthz", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := s.db.Healthcheck(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "unhealthy",
				"error":  err.Error(),
			})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Vue SPA — serve static files with index.html fallback
	// Priority: webDir (disk) > webFS (embedded) > nothing
	var spaFS fs.FS
	if s.webDir != "" {
		spaFS = os.DirFS(s.webDir)
	} else if s.webFS != nil {
		spaFS = s.webFS
	}
	if spaFS != nil {
		fileServer := http.FileServerFS(spaFS)
		s.echo.GET("/*", echo.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Go vanity import: go get isms.sh@latest
			if r.URL.Query().Get("go-get") == "1" {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<!DOCTYPE html><html><head>
<meta name="go-import" content="isms.sh git https://github.com/unidoc/isms">
<meta name="go-source" content="isms.sh https://github.com/unidoc/isms https://github.com/unidoc/isms/tree/main{/dir} https://github.com/unidoc/isms/blob/main{/dir}/{file}#L{line}">
</head><body>go get isms.sh</body></html>`))
				return
			}
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			if f, err := fs.Stat(spaFS, path); err == nil && !f.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})))
	}
}

// --- Git-backed JSON handlers ---

func (s *Server) handleGetConfig(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	// Org context comes strictly from URL (subdomain, custom domain, or path)
	// via OrgResolverMiddleware which sets it on the request. No query-param
	// fallback, no single-org auto-detect. Apex domain = no org = neutral config.

	// All config comes from PostgreSQL — isms.yaml is not read at runtime.
	type configResponse struct {
		Branding         map[string]string `json:"branding,omitempty"`
		OrganizationName string            `json:"organization_name"`
		OrganizationSlug string            `json:"organization_slug"`
		HasTerms         bool              `json:"has_terms"`
		HasPrivacy       bool              `json:"has_privacy"`
		ShowPoweredBy    bool              `json:"show_powered_by"`
		TermsURL         string            `json:"terms_url,omitempty"`
		PrivacyURL       string            `json:"privacy_url,omitempty"`

		// Routing: does this deployment serve tenant orgs on wildcard
		// subdomains (<slug>.<apex>), or only path-based (<apex>/<slug>)?
		// Controlled by ISMS_SUBDOMAIN_ROUTING — default true for prod, false
		// for demo / dev where wildcard DNS + TLS isn't set up.
		SubdomainRouting bool   `json:"subdomain_routing"`
		ApexHost         string `json:"apex_host,omitempty"`
	}
	resp := configResponse{}
	resp.SubdomainRouting = s.subdomainRouting
	resp.ApexHost = s.apexHost

	// Branding from org settings (Postgres)
	branding := map[string]string{}
	for _, key := range []string{"branding_name", "branding_color", "branding_footer"} {
		if val, err := s.db.GetOrgSetting(ctx, orgID, key); err == nil && val != "" {
			branding[key] = val
		}
	}
	// Branding files from blob store
	orgUUID := s.resolveOrgUUID(c)
	if orgUUID != "" {
		if ok, _ := s.blobs.Exists(ctx, orgUUID, "branding/logo.svg"); ok {
			branding["branding_logo"] = "/branding/logo"
		} else if ok, _ := s.blobs.Exists(ctx, orgUUID, "branding/logo.png"); ok {
			branding["branding_logo"] = "/branding/logo"
		}
		if ok, _ := s.blobs.Exists(ctx, orgUUID, "branding/favicon.ico"); ok {
			branding["branding_favicon"] = "/branding/favicon.ico"
		} else if ok, _ := s.blobs.Exists(ctx, orgUUID, "branding/favicon.png"); ok {
			branding["branding_favicon"] = "/branding/favicon.ico"
		}
	}
	if len(branding) > 0 {
		resp.Branding = branding
	}

	// Add org info
	if org, err := s.db.GetOrganization(ctx, orgID); err == nil {
		resp.OrganizationName = org.Name
		resp.OrganizationSlug = org.Slug
	}

	// Platform-level defaults
	resp.HasTerms = s.termsFile != ""
	resp.HasPrivacy = s.privacyFile != ""
	resp.ShowPoweredBy = !s.hidePoweredBy

	// Org-level overrides (Postgres org_settings take precedence)
	if val, err := s.db.GetOrgSetting(ctx, orgID, "show_powered_by"); err == nil && val != "" {
		resp.ShowPoweredBy = val == "true" || val == "1"
	}
	if val, err := s.db.GetOrgSetting(ctx, orgID, "terms_url"); err == nil && val != "" {
		resp.TermsURL = val
		resp.HasTerms = true
	}
	if val, err := s.db.GetOrgSetting(ctx, orgID, "privacy_url"); err == nil && val != "" {
		resp.PrivacyURL = val
		resp.HasPrivacy = true
	}

	return c.JSON(http.StatusOK, resp)
}

// handleListAllDocuments scans the git repo root for directories containing .md
// files and returns them grouped by folder. No hardcoded structure — whatever
// directories exist in the repo become document folders.
func (s *Server) handleListAllDocuments(c echo.Context) error {
	orgID := getOrgID(c)
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}

	type DocFile struct {
		DocumentID string `json:"document_id"`
		Title      string `json:"title"`
		Version    string `json:"version"`
		Status     string `json:"status"`
		Author     string `json:"author"`
		Path       string `json:"path"`
		Folder     string `json:"folder"`
	}
	type DocFolder struct {
		Name       string      `json:"name"`
		Title      string      `json:"title,omitempty"` // from .title file
		Files      []DocFile   `json:"files"`
		SubFolders []DocFolder `json:"subfolders,omitempty"`
	}

	docsRoot := st.DocsRoot()
	entries, err := st.ReadDir(docsRoot)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}

	// readDirTitle reads .title file from a directory if it exists
	readDirTitle := func(dirPath string) string {
		data, err := st.ReadFile(filepath.Join(dirPath, ".title"))
		if err != nil || len(data) == 0 {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	// buildFolder recursively builds a folder with subfolders and files
	var buildFolder func(dirPath, topFolder string) DocFolder
	buildFolder = func(dirPath, topFolder string) DocFolder {
		name := filepath.Base(dirPath)
		folder := DocFolder{
			Name:  name,
			Title: readDirTitle(dirPath),
		}

		dirEntries, err := st.ReadDir(dirPath)
		if err != nil {
			return folder
		}

		for _, de := range dirEntries {
			if strings.HasPrefix(de.Name(), ".") {
				continue
			}
			childPath := filepath.Join(dirPath, de.Name())

			if de.IsDir() {
				sub := buildFolder(childPath, topFolder)
				if len(sub.Files) > 0 || len(sub.SubFolders) > 0 || sub.Title != "" {
					folder.SubFolders = append(folder.SubFolders, sub)
				}
			} else if strings.HasSuffix(de.Name(), ".md") {
				pf, loadErr := st.LoadDocument(childPath)
				if loadErr != nil {
					continue
				}
				relPath, _ := filepath.Rel(docsRoot, childPath)
				folder.Files = append(folder.Files, DocFile{
					DocumentID: pf.Frontmatter.DocumentID,
					Title:      pf.Frontmatter.Title,
					Version:    pf.Frontmatter.Version,
					Status:     pf.Frontmatter.Status,
					Author:     pf.Frontmatter.Author,
					Path:       relPath,
					Folder:     topFolder,
				})
			}
		}
		return folder
	}

	var folders []DocFolder
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		folder := buildFolder(filepath.Join(docsRoot, name), name)
		if len(folder.Files) > 0 || len(folder.SubFolders) > 0 || folder.Title != "" {
			folders = append(folders, folder)
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": folders})
}

// handleGetDocument loads a single document by folder and document ID.
func (s *Server) handleGetDocument(c echo.Context) error {
	orgID := getOrgID(c)
	folder := c.Param("folder")
	docID := c.Param("id")

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "repository not found")
	}

	docsRoot := filepath.Clean(st.DocsRoot())
	folderPath := filepath.Clean(filepath.Join(docsRoot, folder))

	// Ensure the folder path is within documents root (prevent traversal)
	if !strings.HasPrefix(folderPath, docsRoot) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid folder")
	}

	var found *store.DocumentFile
	st.WalkDir(folderPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		pf, loadErr := st.LoadDocument(path)
		if loadErr != nil {
			return nil
		}
		if pf.Frontmatter.DocumentID == docID {
			found = pf
			return filepath.SkipAll
		}
		return nil
	})

	if found == nil {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	raw, _ := st.ReadFile(found.Path)
	content := stripFrontmatter(string(raw))

	resp := map[string]interface{}{
		"document_id":  found.Frontmatter.DocumentID,
		"title":        found.Frontmatter.Title,
		"type":         found.Frontmatter.Type,
		"version":      found.Frontmatter.Version,
		"status":       found.Frontmatter.Status,
		"author":       found.Frontmatter.Author,
		"owner":        found.Frontmatter.Owner,
		"review_cycle": found.Frontmatter.ReviewCycle,
		"next_review":  found.Frontmatter.NextReview,
		"content":      content,
		"folder":       folder,
	}
	if found.Frontmatter.Status == "in_review" {
		ctx := c.Request().Context()
		if openReview, _ := s.db.GetOpenReviewForDocument(ctx, orgID, docID); openReview != nil {
			resp["active_review_id"] = openReview.ID
			resp["active_review_status"] = openReview.Status
			resp["active_review_round"] = openReview.Round
			// Pending suggestions count helps the author know what to address next.
			if pending, err := s.db.CountPendingSuggestionsForReview(ctx, orgID, openReview.ID); err == nil {
				resp["active_review_pending_suggestions"] = pending
			}
		}
	}
	return c.JSON(http.StatusOK, resp)
}

// handleSearchDocuments searches all documents for a query string.
// GET /api/v1/documents/search?q=encryption&limit=20
func (s *Server) handleSearchDocuments(c echo.Context) error {
	query := c.QueryParam("q")
	if strings.TrimSpace(query) == "" || len(strings.TrimSpace(query)) < 2 {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}

	orgID := getOrgID(c)
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}

	limit := 20
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	results := st.Search(query, limit)
	if results == nil {
		results = []store.SearchResult{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": results})
}

// handleChangedDocuments returns documents changed in recent commits.
// GET /api/v1/documents/changed?commits=10
func (s *Server) handleChangedDocuments(c echo.Context) error {
	orgID := getOrgID(c)
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}

	maxCommits := 10
	if n := c.QueryParam("commits"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 && v <= 100 {
			maxCommits = v
		}
	}

	changed := st.RecentlyChanged(maxCommits)
	if changed == nil {
		changed = []store.ChangedFile{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": changed})
}

// handleUpdateDocumentMetadata updates a frontmatter field on a document.
// GET /api/v1/documents/:docId/body — returns document content by document_id.
// Resolves any document regardless of folder structure.
func (s *Server) handleGetDocumentBody(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	path := st.FindDocumentByID(docID)
	if path == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found: "+docID)
	}

	pf, err := st.LoadDocument(path)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "loading document: "+err.Error())
	}

	resp := map[string]interface{}{
		"document_id":  pf.Frontmatter.DocumentID,
		"title":        pf.Frontmatter.Title,
		"version":      pf.Frontmatter.Version,
		"status":       pf.Frontmatter.Status,
		"author":       pf.Frontmatter.Author,
		"owner":        pf.Frontmatter.Owner,
		"review_cycle": pf.Frontmatter.ReviewCycle,
		"next_review":  pf.Frontmatter.NextReview,
		"body":         pf.Body,
		"path":         path,
	}
	// Include active review ID so the frontend can link directly to it
	if pf.Frontmatter.Status == "in_review" {
		ctx := c.Request().Context()
		if openReview, _ := s.db.GetOpenReviewForDocument(ctx, orgID, docID); openReview != nil {
			resp["active_review_id"] = openReview.ID
			resp["active_review_status"] = openReview.Status
			resp["active_review_round"] = openReview.Round
			if pending, err := s.db.CountPendingSuggestionsForReview(ctx, orgID, openReview.ID); err == nil {
				resp["active_review_pending_suggestions"] = pending
			}
		}
	}
	return c.JSON(http.StatusOK, resp)
}

// GET /api/v1/documents/:docId/blame
func (s *Server) handleDocumentBlame(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	path := st.FindDocumentByID(docID)
	if path == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found: "+docID)
	}

	atRef := c.QueryParam("ref")
	lines, err := st.BlameFile(path, atRef)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "blame: "+err.Error())
	}
	if lines == nil {
		lines = []store.BlameLine{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"lines": lines})
}

// PUT /api/v1/documents/:docId/metadata
// Requires manager or admin role.
func (s *Server) handleUpdateDocumentMetadata(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")

	// Only manager/admin can update document metadata
	role, _ := c.Get("user_role").(string)
	if role != "manager" && role != "admin" {
		return echo.NewHTTPError(http.StatusForbidden, "requires manager or admin role")
	}

	var req struct {
		Fields map[string]string `json:"fields"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req.Fields) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no fields to update")
	}

	// Whitelist allowed fields
	allowed := map[string]bool{"author": true, "owner": true, "version": true, "status": true, "classification": true, "review_cycle": true, "type": true}
	for k := range req.Fields {
		if !allowed[k] {
			return echo.NewHTTPError(http.StatusBadRequest, "field not allowed: "+k)
		}
	}

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	docPath := st.FindDocumentByID(docID)
	if docPath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(c.Request().Context(), email)
	authorName := email
	if err == nil && user.Name != "" {
		authorName = user.Name
	}

	commitHash, err := st.UpdateDocumentMetadataMulti(docPath, req.Fields, authorName, email)
	if err != nil {
		if err == store.ErrConflict {
			return echo.NewHTTPError(http.StatusConflict, "the document was modified by another user — please refresh and try again")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "updating document: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"commit": commitHash, "status": "updated"})
}

// handleValidateDocuments checks for duplicate document IDs and other issues.
// GET /api/v1/documents/validate
// POST /api/v1/documents — create a new document
// POST /api/v1/documents/folders — create a folder with .title file
func (s *Server) handleCreateFolder(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	var req struct {
		Path  string `json:"path"`  // e.g. "iso27001/policies/new-folder"
		Title string `json:"title"` // display name for .title file
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Path == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	titleContent := req.Title
	if titleContent == "" {
		// Use last segment as title
		parts := strings.Split(req.Path, "/")
		titleContent = parts[len(parts)-1]
	}

	gitPath := filepath.Join("documents", req.Path, ".title")
	email := getUserEmail(c)
	user, _ := s.db.GetUserByEmail(c.Request().Context(), email)
	authorName := email
	if user != nil && user.Name != "" {
		authorName = user.Name
	}

	commitHash, err := st.CommitFile(gitPath, []byte(titleContent+"\n"), authorName, email,
		fmt.Sprintf("chore: create folder %s", req.Path))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("creating folder: %v", err))
	}

	return c.JSON(http.StatusCreated, map[string]string{"path": req.Path, "commit": commitHash})
}

func (s *Server) handleCreateDocument(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)

	var req struct {
		Folder     string `json:"folder"`      // e.g. "iso27001/policies"
		Filename   string `json:"filename"`    // e.g. "data-classification.md"
		DocumentID string `json:"document_id"` // e.g. "data-classification"
		Title      string `json:"title"`       // e.g. "Data Classification Policy"
		Type       string `json:"type"`        // optional: control, policy, procedure, etc.
		Content    string `json:"content"`     // optional initial body
		Author     string `json:"author"`      // optional
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Folder == "" || req.Filename == "" || req.DocumentID == "" || req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "folder, filename, document_id, and title are required")
	}
	// Ensure filename ends with .md
	if !strings.HasSuffix(req.Filename, ".md") {
		req.Filename += ".md"
	}

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Check for duplicate document_id
	if existing := st.FindDocumentByID(req.DocumentID); existing != "" {
		return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("document_id %q already exists", req.DocumentID))
	}

	// Build frontmatter + body
	actor := getUserEmail(c)
	author := req.Author
	if author == "" {
		author = actor
	}
	fm := fmt.Sprintf("---\ndocument_id: %q\ntitle: %q", req.DocumentID, req.Title)
	if req.Type != "" {
		fm += fmt.Sprintf("\ntype: %q", req.Type)
	}
	fm += fmt.Sprintf("\nstatus: \"draft\"\nversion: \"0.1\"\nauthor: %q\nowner: %q\nreview_cycle: 12", author, author)
	fm += "\n---\n"

	body := req.Content
	if body == "" {
		body = fmt.Sprintf("# %s\n\n> TODO: Write content for this document.\n", req.Title)
	}

	content := fm + body
	gitPath := filepath.Join("documents", req.Folder, req.Filename)

	email := getUserEmail(c)
	user, _ := s.db.GetUserByEmail(c.Request().Context(), email)
	authorName := email
	if user != nil && user.Name != "" {
		authorName = user.Name
	}

	commitHash, err := st.CommitFile(gitPath, []byte(content), authorName, email,
		fmt.Sprintf("docs(%s): create document", req.DocumentID))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("creating document: %v", err))
	}

	s.searchIndex.Invalidate(orgID)

	s.logAndNotify(c.Request().Context(), orgID, &db.Activity{
		DocumentID: req.DocumentID,
		Actor:      email,
		Action:     "document_created",
		Detail:     fmt.Sprintf("Created %s in %s", req.Title, req.Folder),
	})

	return c.JSON(http.StatusCreated, map[string]string{
		"document_id": req.DocumentID,
		"path":        gitPath,
		"commit":      commitHash,
	})
}

// DELETE /api/v1/documents/:docId — archive/delete a document
func (s *Server) handleDeleteDocument(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	docID := c.Param("docId")

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	docPath := st.FindDocumentByID(docID)
	if docPath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	email := getUserEmail(c)
	user, _ := s.db.GetUserByEmail(c.Request().Context(), email)
	authorName := email
	if user != nil && user.Name != "" {
		authorName = user.Name
	}

	commitHash, err := st.DeleteFile(docPath, authorName, email,
		fmt.Sprintf("docs(%s): delete document", docID))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("deleting document: %v", err))
	}

	s.searchIndex.Invalidate(orgID)

	s.logAndNotify(c.Request().Context(), orgID, &db.Activity{
		DocumentID: docID,
		Actor:      email,
		Action:     "document_deleted",
		Detail:     fmt.Sprintf("Deleted document %s", docID),
	})

	return c.JSON(http.StatusOK, map[string]string{"commit": commitHash, "status": "deleted"})
}

func (s *Server) handleValidateDocuments(c echo.Context) error {
	orgID := getOrgID(c)
	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"valid": true, "errors": []string{}})
	}

	var errors []string
	dupes := st.ValidateUniqueDocumentIDs()
	for id, paths := range dupes {
		errors = append(errors, fmt.Sprintf("duplicate document_id %q found in: %s", id, strings.Join(paths, ", ")))
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"valid":      len(errors) == 0,
		"errors":     errors,
		"duplicates": dupes,
	})
}

func (s *Server) handleUpdateDocumentContent(c echo.Context) error {
	orgID := getOrgID(c)
	docID := c.Param("docId")

	// Only manager/admin can update document content
	role, _ := c.Get("user_role").(string)
	if role != "manager" && role != "admin" {
		return echo.NewHTTPError(http.StatusForbidden, "requires manager or admin role")
	}

	var req struct {
		Content string  `json:"content"`
		Version string  `json:"version,omitempty"`
		Author  string  `json:"author,omitempty"`
		Owner   *string `json:"owner,omitempty"` // pointer to distinguish "not sent" from "set to empty"
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	st, err := s.storeForOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	docPath := st.FindDocumentByID(docID)
	if docPath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	// Load current document to get frontmatter
	pf, err := st.LoadDocument(docPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "loading document: "+err.Error())
	}

	// Auto-increment version when editing an approved document
	if pf.Frontmatter.Status == "approved" && req.Version == "" {
		pf.Frontmatter.Version = incrementVersion(pf.Frontmatter.Version)
		pf.Frontmatter.Status = "draft"
	}

	// Update metadata fields if provided
	if req.Version != "" {
		// Prevent version downgrade
		if compareVersions(req.Version, pf.Frontmatter.Version) < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("cannot lower version from %s to %s", pf.Frontmatter.Version, req.Version))
		}
		pf.Frontmatter.Version = req.Version
	}
	if req.Author != "" {
		pf.Frontmatter.Author = req.Author
	}
	if req.Owner != nil {
		pf.Frontmatter.Owner = *req.Owner
	}

	// Auto-set author and owner on first edit if not already set
	if pf.Frontmatter.Author == "" {
		pf.Frontmatter.Author = getUserEmail(c)
	}
	if pf.Frontmatter.Owner == "" {
		pf.Frontmatter.Owner = getUserEmail(c)
	}

	// Replace body
	pf.Body = req.Content

	// Serialize frontmatter + body
	fmBytes, _ := yaml.Marshal(pf.Frontmatter)
	newContent := "---\n" + string(fmBytes) + "---\n" + pf.Body

	email := getUserEmail(c)
	user, err := s.db.GetUserByEmail(c.Request().Context(), email)
	authorName := email
	if err == nil && user.Name != "" {
		authorName = user.Name
	}

	message := fmt.Sprintf("docs(%s): update document", docID)
	expectedHead := c.Request().Header.Get("If-Match")
	commitHash, err := st.CommitFile(docPath, []byte(newContent), authorName, email, message, expectedHead)
	if err != nil {
		if err == store.ErrConflict {
			return echo.NewHTTPError(http.StatusConflict, "the document was modified by another user — please refresh and try again")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "saving document: "+err.Error())
	}

	// After successful save, check for open review and log activity
	ctx := c.Request().Context()
	openReview, _ := s.db.GetOpenReviewForDocument(ctx, orgID, docID)
	if openReview != nil {
		s.logAndNotify(ctx, orgID, &db.Activity{
			DocumentID: docID,
			ReviewID:   &openReview.ID,
			Actor:      email,
			Action:     "document_updated",
			Detail:     "Document updated during review",
		})
		// Note: do NOT auto-transition changes_requested → open here.
		// The review stays in changes_requested until the admin explicitly
		// re-sends via POST /documents/:id/reviews, which resets assignments,
		// updates sent_head, and transitions the status properly.
	}

	// Version records are NOT created on draft edits.
	// document_versions tracks official milestones only (merge, confirm).
	// Git commit history is the raw edit log.

	return c.JSON(http.StatusOK, map[string]string{"commit": commitHash, "status": "updated"})
}

// handleNeedsReview returns documents where the git version is ahead of the
// last approved review in Postgres.
func (s *Server) handleNeedsReview(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	st, err := s.storeForOrg(ctx, orgID)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}

	type NeedsReviewDoc struct {
		DocumentID        string   `json:"document_id"`
		Title             string   `json:"title"`
		Folder            string   `json:"folder"`
		Path              string   `json:"path"`
		CurrentCommit     string   `json:"current_commit"`
		CurrentCommitTime string   `json:"current_commit_time"`
		ApprovedCommit    string   `json:"approved_commit"`
		ApprovedAt        string   `json:"approved_at"`
		ApprovedVersion   string   `json:"approved_version"`
		ChangeSummary     []string `json:"change_summary"`
		NeverApproved     bool     `json:"never_approved"`
	}

	docsRoot := st.DocsRoot()
	entries, err := st.ReadDir(docsRoot)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"data": []interface{}{}})
	}

	var results []NeedsReviewDoc

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		st.WalkDir(filepath.Join(docsRoot, name), func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			pf, loadErr := st.LoadDocument(path)
			if loadErr != nil || pf.Frontmatter.DocumentID == "" {
				return nil
			}

			relPath, _ := filepath.Rel(docsRoot, path)
			gitPath := "documents/" + filepath.ToSlash(relPath)

			// Get last commit for this file
			commitHash, commitTime, _, _, gitErr := st.FileLastCommit(gitPath)
			if gitErr != nil {
				return nil
			}

			// Check Postgres for last approved review
			review, _ := s.db.GetLastApprovedReview(ctx, orgID, pf.Frontmatter.DocumentID)

			shortCurrent := commitHash
			if len(shortCurrent) > 8 {
				shortCurrent = shortCurrent[:8]
			}

			if review == nil {
				// Never approved
				results = append(results, NeedsReviewDoc{
					DocumentID:        pf.Frontmatter.DocumentID,
					Title:             pf.Frontmatter.Title,
					Folder:            name,
					Path:              filepath.ToSlash(relPath),
					CurrentCommit:     shortCurrent,
					CurrentCommitTime: commitTime.Format("2006-01-02 15:04"),
					ApprovedCommit:    "never",
					ApprovedAt:        "",
					ChangeSummary:     []string{"Document has never been reviewed"},
					NeverApproved:     true,
				})
			} else {
				// Use merge_commit as the approved baseline (falls back to commit_hash for older reviews)
				approvedRef := review.MergeCommit
				if approvedRef == "" {
					approvedRef = review.CommitHash
				}
				if approvedRef == commitHash {
					return nil // no changes since approval
				}
				// The commit advanced, but only flag a re-review if the reviewed
				// content (the body) actually changed. A frontmatter-only edit
				// (any metadata field — not the markdown body) writes a new commit
				// without changing what's reviewed, and must not trigger "changed
				// since last approval" (#3).
				if approvedRef != "" {
					approvedBody, bodyErr := st.DocumentBodyAtRef(approvedRef, gitPath)
					if bodyErr != nil {
						// Safe fallback: flag for review. Log it so intermittent
						// git-read failures don't silently cause false positives.
						c.Logger().Warnf("needs-review: body-at-ref failed, flagging %s: ref=%s err=%v", gitPath, approvedRef, bodyErr)
					} else if strings.TrimSpace(approvedBody) == strings.TrimSpace(pf.Body) {
						return nil // only frontmatter metadata changed since approval
					}
				}
				// Changed since approval
				var changeSummary []string
				if approvedRef != "" {
					changeSummary = st.CommitsSince(gitPath, approvedRef)
				}
				if len(changeSummary) == 0 {
					changeSummary = []string{"Content changed since last approval"}
				}

				results = append(results, NeedsReviewDoc{
					DocumentID:        pf.Frontmatter.DocumentID,
					Title:             pf.Frontmatter.Title,
					Folder:            name,
					Path:              filepath.ToSlash(relPath),
					CurrentCommit:     shortCurrent,
					CurrentCommitTime: commitTime.Format("2006-01-02 15:04"),
					ApprovedCommit:    approvedRef,
					ApprovedAt:        review.UpdatedAt.Format("2006-01-02 15:04"),
					ApprovedVersion:   review.Version,
					ChangeSummary:     changeSummary,
					NeverApproved:     false,
				})
			}
			// else: approved commit matches current commit — no action needed
			return nil
		})
	}

	if results == nil {
		results = []NeedsReviewDoc{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": results})
}

func (s *Server) handleListAssets(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.AssetListParams{
		Page:   page,
		Limit:  limit,
		Sort:   c.QueryParam("sort"),
		Search: c.QueryParam("q"),
		Type:   c.QueryParam("type"),
		Status: c.QueryParam("status"),
		Owner:  c.QueryParam("owner"),
	}
	assets, total, err := s.db.PaginatedAssets(ctx, orgID, params)
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
		"data":      assets,
		"total":     total,
		"page":      params.Page,
		"page_size": params.Limit,
	})
}

func (s *Server) handleAssetStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.AssetStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleGetAsset(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}
	a, err := s.db.GetAsset(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	return c.JSON(http.StatusOK, a)
}

func (s *Server) handleAddAsset(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	var req assetCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	a := db.Asset{
		Name:            req.Name,
		Description:     req.Description,
		AssetType:       req.AssetType,
		Status:          req.Status,
		Owner:           req.Owner,
		PrimaryLocation: req.PrimaryLocation,
		Confidentiality: req.Confidentiality,
		Integrity:       req.Integrity,
		Availability:    req.Availability,
		LastReview:      req.LastReview,
		NextReview:      req.NextReview,
		Notes:           req.Notes,
	}
	if a.Owner == "" {
		a.Owner = getUserEmail(c)
	}
	if a.Status == "" {
		a.Status = "open"
	}
	if a.AssetType == "" {
		a.AssetType = "other"
	}
	if err := validateEnum("status", a.Status, db.AssetStatuses); err != nil {
		return err
	}
	if err := validateEnum("asset_type", a.AssetType, db.AssetTypes); err != nil {
		return err
	}
	if err := s.db.CreateAsset(ctx, orgID, &a); err != nil {
		return pgxHTTPError(err)
	}
	actor := getUserEmail(c)
	s.createReferencesForEntity(ctx, orgID, "asset", a.Identifier, actor, req.References)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "asset",
		EntityID:   a.ID,
		Action:     "create",
		ChangedBy:  actor,
	})
	s.searchUpsert(orgID, "asset", a.Identifier, a.Name, a.Identifier+" "+a.Name+" "+a.Description)
	if out, err := s.db.GetAsset(ctx, orgID, a.ID); err == nil {
		return c.JSON(http.StatusCreated, out)
	}
	return c.JSON(http.StatusCreated, a)
}

func (s *Server) handleDeleteAsset(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}
	old, _ := s.db.GetAsset(ctx, orgID, id)
	if err := s.db.DeleteAsset(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}
	if old != nil {
		_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
			EntityType: "asset",
			EntityID:   old.ID,
			Action:     "delete",
			ChangedBy:  getUserEmail(c),
		})
		s.searchRemove(orgID, "asset", old.Identifier)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListSystems(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	supplierID, _ := strconv.ParseInt(c.QueryParam("supplier_id"), 10, 64)
	params := db.SystemListParams{
		Page:        page,
		Limit:       limit,
		Sort:        c.QueryParam("sort"),
		Search:      c.QueryParam("q"),
		Department:  c.QueryParam("department"),
		Criticality: c.QueryParam("criticality"),
		Status:      c.QueryParam("status"),
		Owner:       c.QueryParam("owner"),
		SupplierID:  supplierID,
	}
	items, total, err := s.db.PaginatedSystems(c.Request().Context(), orgID, params)
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

func (s *Server) handleSystemStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.SystemStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleGetSystem(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system id")
	}
	sys, err := s.db.GetSystem(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "system not found")
	}
	return c.JSON(http.StatusOK, sys)
}

func (s *Server) handleCreateSystem(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	var req systemCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sys := db.System{
		Name:            req.Name,
		Description:     req.Description,
		SupplierID:      req.SupplierID,
		Department:      req.Department,
		Classification:  req.Classification,
		Criticality:     req.Criticality,
		Status:          req.Status,
		RPOHours:        req.RPOHours,
		RTOHours:        req.RTOHours,
		Confidentiality: req.Confidentiality,
		Integrity:       req.Integrity,
		Availability:    req.Availability,
		LastReview:      req.LastReview,
		NextReview:      req.NextReview,
		Owner:           req.Owner,
		Notes:           req.Notes,
	}
	if sys.Status == "" {
		sys.Status = "active"
	}
	if sys.Owner == "" {
		sys.Owner = getUserEmail(c)
	}
	// Seed description with ## Purpose heading; seed notes with ## Access control heading.
	// Only when fields are empty so we never overwrite user input.
	if sys.Description == "" {
		sys.Description = "## Purpose\n\n"
	}
	if sys.Notes == "" {
		sys.Notes = "## Access control\n\n"
	}
	if err := validateEnum("status", sys.Status, db.SystemStatuses); err != nil {
		return err
	}
	if err := validateEnum("criticality", sys.Criticality, db.SystemCriticalities); err != nil {
		return err
	}
	if err := validateEnum("classification", sys.Classification, db.SystemClassifications); err != nil {
		return err
	}
	// Verify supplier belongs to this org if referenced.
	if sys.SupplierID != nil && *sys.SupplierID > 0 {
		if _, err := s.db.GetSupplier(ctx, orgID, *sys.SupplierID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "supplier not found in this organization")
		}
	}
	if err := s.db.CreateSystem(ctx, orgID, &sys); err != nil {
		return pgxHTTPError(err)
	}
	actor := getUserEmail(c)
	s.createReferencesForEntity(ctx, orgID, "system", sys.Identifier, actor, req.References)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "system",
		EntityID:   sys.ID,
		Action:     "create",
		ChangedBy:  actor,
	})
	s.searchUpsert(orgID, "system", sys.Identifier, sys.Name, sys.Identifier+" "+sys.Name+" "+sys.Description)
	// Re-fetch to populate resolved owner email and computed fields
	if fresh, err := s.db.GetSystem(ctx, orgID, sys.ID); err == nil && fresh != nil {
		sys = *fresh
	}
	return c.JSON(http.StatusCreated, sys)
}

func (s *Server) handleGetRisk(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid risk id")
	}
	r, err := s.db.GetRisk(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "risk not found")
	}
	return c.JSON(http.StatusOK, r)
}

func (s *Server) handleRiskStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.RiskStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleListRisks(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.RiskListParams{
		Page:     page,
		Limit:    limit,
		Sort:     c.QueryParam("sort"),
		Search:   c.QueryParam("q"),
		Level:    c.QueryParam("level"),
		Category: c.QueryParam("category"),
		Status:   c.QueryParam("status"),
		Owner:    c.QueryParam("owner"),
	}
	risks, total, err := s.db.PaginatedRisks(ctx, orgID, params)
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
		"data":      risks,
		"total":     total,
		"page":      params.Page,
		"page_size": params.Limit,
	})
}

func (s *Server) handleAddRisk(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	var req riskCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	r := db.Risk{
		Title:                         req.Title,
		Description:                   req.Description,
		RiskType:                      req.RiskType,
		Origin:                        req.Origin,
		Category:                      req.Category,
		CurrentLikelihood:             req.CurrentLikelihood,
		CurrentImpact:                 req.CurrentImpact,
		ConfidentialityImpact:         req.ConfidentialityImpact,
		IntegrityImpact:               req.IntegrityImpact,
		AvailabilityImpact:            req.AvailabilityImpact,
		InherentLikelihood:            req.InherentLikelihood,
		InherentImpact:                req.InherentImpact,
		InherentConfidentialityImpact: req.InherentConfidentialityImpact,
		InherentIntegrityImpact:       req.InherentIntegrityImpact,
		InherentAvailabilityImpact:    req.InherentAvailabilityImpact,
		TargetLikelihood:              req.TargetLikelihood,
		TargetImpact:                  req.TargetImpact,
		Treatment:                     req.Treatment,
		TreatmentPlan:                 req.TreatmentPlan,
		TreatmentDueDate:              req.TreatmentDueDate,
		Owner:                         req.Owner,
		Status:                        req.Status,
		LastReview:                    req.LastReview,
		NextReview:                    req.NextReview,
		Notes:                         req.Notes,
	}
	if r.Owner == "" {
		r.Owner = getUserEmail(c)
	}
	// Sensible defaults so the light create form (title + category) just works.
	// User refines via the edit modal if these aren't right.
	if r.Status == "" {
		r.Status = "open"
	}
	if r.RiskType == "" {
		r.RiskType = "threat"
	}
	if r.Origin == "" {
		r.Origin = "internal"
	}
	// Seed description with section headings when empty, so the user has clear
	// places to fill in both the risk description and its potential consequences.
	if r.Description == "" {
		r.Description = "## Description\n\n\n\n## Potential consequences\n\n"
	}
	if err := validateEnum("status", r.Status, db.RiskStatuses); err != nil {
		return err
	}
	if err := validateEnum("risk_type", r.RiskType, db.RiskTypes); err != nil {
		return err
	}
	if err := validateEnum("origin", r.Origin, db.RiskOrigins); err != nil {
		return err
	}
	if err := validateEnum("category", r.Category, db.RiskCategories); err != nil {
		return err
	}
	if err := validateEnum("treatment", r.Treatment, db.TreatmentOptions); err != nil {
		return err
	}
	if err := s.validateOrgMember(c, r.Owner); err != nil {
		return err
	}
	if err := s.db.CreateRisk(ctx, orgID, &r); err != nil {
		return pgxHTTPError(err)
	}
	actor := getUserEmail(c)
	s.createReferencesForEntity(ctx, orgID, "risk", r.Identifier, actor, req.References)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "risk",
		EntityID:   r.ID,
		Action:     "create",
		ChangedBy:  actor,
	})
	s.searchUpsert(orgID, "risk", r.Identifier, r.Title, r.Identifier+" "+r.Title+" "+r.Description+" "+r.Category)
	if out, err := s.db.GetRisk(ctx, orgID, r.ID); err == nil {
		return c.JSON(http.StatusCreated, out)
	}
	return c.JSON(http.StatusCreated, r)
}

func (s *Server) handleRiskMatrix(c echo.Context) error {
	type Cell struct {
		Likelihood int    `json:"likelihood"`
		Impact     int    `json:"impact"`
		Score      int    `json:"score"`
		Level      string `json:"level"`
	}
	var cells []Cell
	for l := 1; l <= 5; l++ {
		for i := 1; i <= 5; i++ {
			score := l * i
			cells = append(cells, Cell{l, i, score, db.ScoreToLevel(score)})
		}
	}
	return c.JSON(http.StatusOK, cells)
}

// handleRiskAdvisories returns advisory messages about CIA consistency between
// a risk and its linked assets. This is informational only, not enforcement.
func (s *Server) handleRiskAdvisories(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()

	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid risk id")
	}

	risk, err := s.db.GetRisk(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "risk not found")
	}

	// Find linked assets via entity_references (both directions). References
	// store per-org identifiers ("RISK-12"), not numeric row ids.
	refs, err := s.db.ListAllReferencesForEntity(ctx, orgID, "risk", risk.Identifier)
	if err != nil {
		refs = nil
	}

	type Advisory struct {
		Level   string `json:"level"` // "warning" or "info"
		Message string `json:"message"`
	}

	var advisories []Advisory
	ciaNames := db.CIALevelNames

	for _, ref := range refs {
		// Determine the asset ID from the reference.
		var assetIDStr string
		if ref.SourceType == "risk" && ref.TargetType == "asset" {
			assetIDStr = ref.TargetID
		} else if ref.SourceType == "asset" && ref.TargetType == "risk" {
			assetIDStr = ref.SourceID
		} else {
			continue
		}

		asset, err := s.db.GetAssetByIdentifier(ctx, orgID, assetIDStr)
		if err != nil {
			continue
		}

		// Compare CIA ratings: advise when risk CIA is lower than asset CIA.
		// A lower risk CIA than asset CIA may mean the risk underestimates
		// the impact on a high-value asset.
		if risk.ConfidentialityImpact != nil && asset.Confidentiality != nil {
			rc, ac := *risk.ConfidentialityImpact, *asset.Confidentiality
			if ac > 0 && rc > 0 && rc < ac {
				advisories = append(advisories, Advisory{
					Level:   "warning",
					Message: fmt.Sprintf("Risk confidentiality impact (%d - %s) is lower than linked asset '%s' (%d - %s) — consider reviewing", rc, ciaNames[rc], asset.Name, ac, ciaNames[ac]),
				})
			}
		}
		if risk.IntegrityImpact != nil && asset.Integrity != nil {
			ri, ai := *risk.IntegrityImpact, *asset.Integrity
			if ai > 0 && ri > 0 && ri < ai {
				advisories = append(advisories, Advisory{
					Level:   "warning",
					Message: fmt.Sprintf("Risk integrity impact (%d - %s) is lower than linked asset '%s' (%d - %s) — consider reviewing", ri, ciaNames[ri], asset.Name, ai, ciaNames[ai]),
				})
			}
		}
		if risk.AvailabilityImpact != nil && asset.Availability != nil {
			ra, aa := *risk.AvailabilityImpact, *asset.Availability
			if aa > 0 && ra > 0 && ra < aa {
				advisories = append(advisories, Advisory{
					Level:   "warning",
					Message: fmt.Sprintf("Risk availability impact (%d - %s) is lower than linked asset '%s' (%d - %s) — consider reviewing", ra, ciaNames[ra], asset.Name, aa, ciaNames[aa]),
				})
			}
		}

		// Also advise if asset has CIA rated but risk has no CIA at all.
		assetHasCIA := (asset.Confidentiality != nil && *asset.Confidentiality > 0) ||
			(asset.Integrity != nil && *asset.Integrity > 0) ||
			(asset.Availability != nil && *asset.Availability > 0)
		riskHasCIA := (risk.ConfidentialityImpact != nil && *risk.ConfidentialityImpact > 0) ||
			(risk.IntegrityImpact != nil && *risk.IntegrityImpact > 0) ||
			(risk.AvailabilityImpact != nil && *risk.AvailabilityImpact > 0)
		if assetHasCIA && !riskHasCIA {
			advisories = append(advisories, Advisory{
				Level:   "info",
				Message: fmt.Sprintf("Linked asset '%s' has CIA ratings but this risk has none — consider adding CIA impact assessment", asset.Name),
			})
		}
	}

	if advisories == nil {
		advisories = []Advisory{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": advisories})
}

func (s *Server) handleDeleteRisk(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid risk id")
	}
	old, _ := s.db.GetRisk(ctx, orgID, id)
	if err := s.db.DeleteRisk(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}
	if old != nil {
		_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
			EntityType: "risk",
			EntityID:   old.ID,
			Action:     "delete",
			ChangedBy:  getUserEmail(c),
		})
		s.searchRemove(orgID, "risk", old.Identifier)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListSuppliers(c echo.Context) error {
	orgID := getOrgID(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	params := db.SupplierListParams{
		Page:        page,
		Limit:       limit,
		Sort:        c.QueryParam("sort"),
		Search:      c.QueryParam("q"),
		Type:        c.QueryParam("type"),
		Criticality: c.QueryParam("criticality"),
		Status:      c.QueryParam("status"),
		Owner:       c.QueryParam("owner"),
	}
	items, total, err := s.db.PaginatedSuppliers(c.Request().Context(), orgID, params)
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

func (s *Server) handleSupplierStats(c echo.Context) error {
	orgID := getOrgID(c)
	stats, err := s.db.SupplierStats(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

func (s *Server) handleGetSupplier(c echo.Context) error {
	orgID := getOrgID(c)
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid supplier id")
	}
	sup, err := s.db.GetSupplier(c.Request().Context(), orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "supplier not found")
	}
	return c.JSON(http.StatusOK, sup)
}

func (s *Server) handleAddSupplier(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	var req supplierCreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sup := db.Supplier{
		Name:            req.Name,
		SupplierType:    req.SupplierType,
		Criticality:     req.Criticality,
		DataAccess:      req.DataAccess,
		Contact:         req.Contact,
		ContractRef:     req.ContractRef,
		Status:          req.Status,
		Owner:           req.Owner,
		ContractExpiry:  req.ContractExpiry,
		Confidentiality: req.Confidentiality,
		Integrity:       req.Integrity,
		Availability:    req.Availability,
		LastReview:      req.LastReview,
		NextReview:      req.NextReview,
		Notes:           req.Notes,
	}
	if sup.Status == "" {
		sup.Status = "active"
	}
	if sup.SupplierType == "" {
		sup.SupplierType = "other"
	}
	if sup.Criticality == "" {
		sup.Criticality = "medium"
	}
	if sup.Owner == "" {
		sup.Owner = getUserEmail(c)
	}
	// Seed notes with ## Services heading when empty.
	if sup.Notes == "" {
		sup.Notes = "## Services\n\n"
	}
	if err := validateEnum("status", sup.Status, db.SupplierStatuses); err != nil {
		return err
	}
	if err := validateEnum("supplier_type", sup.SupplierType, db.SupplierTypes); err != nil {
		return err
	}
	if err := validateEnum("criticality", sup.Criticality, db.CriticalityLevels); err != nil {
		return err
	}
	if err := s.db.CreateSupplier(ctx, orgID, &sup); err != nil {
		return pgxHTTPError(err)
	}
	actor := getUserEmail(c)
	s.createReferencesForEntity(ctx, orgID, "supplier", sup.Identifier, actor, req.References)
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "supplier",
		EntityID:   sup.ID,
		Action:     "create",
		ChangedBy:  actor,
	})
	s.searchUpsert(orgID, "supplier", sup.Identifier, sup.Name, sup.Identifier+" "+sup.Name+" "+sup.Notes)
	// Re-fetch to populate resolved owner email and computed fields
	if fresh, err := s.db.GetSupplier(ctx, orgID, sup.ID); err == nil && fresh != nil {
		sup = *fresh
	}
	return c.JSON(http.StatusCreated, sup)
}

func (s *Server) handleUpdateAsset(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}
	old, err := s.db.GetAsset(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	var req assetUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.AssetStatuses); err != nil {
			return err
		}
	}
	if req.AssetType != nil {
		if err := validateEnum("asset_type", *req.AssetType, db.AssetTypes); err != nil {
			return err
		}
	}
	if req.Owner != nil && *req.Owner != "" {
		if err := s.validateOrgMember(c, *req.Owner); err != nil {
			return err
		}
	}
	updated := *old
	updated.ID = id
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.Description != nil {
		updated.Description = *req.Description
	}
	if req.AssetType != nil {
		updated.AssetType = *req.AssetType
	}
	if req.Status != nil {
		updated.Status = *req.Status
	}
	if req.Owner != nil {
		updated.Owner = *req.Owner
	}
	if req.PrimaryLocation != nil {
		updated.PrimaryLocation = *req.PrimaryLocation
	}
	if req.Confidentiality != nil {
		updated.Confidentiality = *req.Confidentiality
	}
	if req.Integrity != nil {
		updated.Integrity = *req.Integrity
	}
	if req.Availability != nil {
		updated.Availability = *req.Availability
	}
	if req.LastReview != nil {
		updated.LastReview = *req.LastReview
	}
	if req.NextReview != nil {
		updated.NextReview = *req.NextReview
	}
	if req.Notes != nil {
		updated.Notes = *req.Notes
	}
	if err := s.db.UpdateAsset(ctx, orgID, &updated); err != nil {
		return pgxHTTPError(err)
	}
	after, _ := s.db.GetAsset(ctx, orgID, id)
	if after != nil {
		actor := getUserEmail(c)
		reason := c.QueryParam("reason")
		changes := db.DiffFields("asset", id, actor, reason, old.ToChangeMap(), after.ToChangeMap())
		if len(changes) > 0 {
			_ = s.db.LogChanges(ctx, orgID, changes)
		}
		s.searchUpsert(orgID, "asset", after.Identifier, after.Name, after.Identifier+" "+after.Name+" "+after.Description)
		s.logAndNotify(ctx, orgID, &db.Activity{
			Actor:  actor,
			Action: "asset_updated",
			Detail: fmt.Sprintf("%s updated", after.Identifier),
		})
		return c.JSON(http.StatusOK, after)
	}
	return c.JSON(http.StatusOK, updated)
}

func (s *Server) handleUpdateRisk(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid risk id")
	}
	old, err := s.db.GetRisk(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "risk not found")
	}
	var req riskUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.RiskStatuses); err != nil {
			return err
		}
	}
	if req.RiskType != nil {
		if err := validateEnum("risk_type", *req.RiskType, db.RiskTypes); err != nil {
			return err
		}
	}
	if req.Origin != nil {
		if err := validateEnum("origin", *req.Origin, db.RiskOrigins); err != nil {
			return err
		}
	}
	if req.Category != nil {
		if err := validateEnum("category", *req.Category, db.RiskCategories); err != nil {
			return err
		}
	}
	if req.Treatment != nil {
		if err := validateEnum("treatment", *req.Treatment, db.TreatmentOptions); err != nil {
			return err
		}
	}
	if req.Owner != nil && *req.Owner != "" {
		if err := s.validateOrgMember(c, *req.Owner); err != nil {
			return err
		}
	}
	updated := *old
	updated.ID = id
	if req.Title != nil {
		updated.Title = *req.Title
	}
	if req.Description != nil {
		updated.Description = *req.Description
	}
	if req.RiskType != nil {
		updated.RiskType = *req.RiskType
	}
	if req.Origin != nil {
		updated.Origin = *req.Origin
	}
	if req.Category != nil {
		updated.Category = *req.Category
	}
	if req.CurrentLikelihood != nil {
		updated.CurrentLikelihood = *req.CurrentLikelihood
	}
	if req.CurrentImpact != nil {
		updated.CurrentImpact = *req.CurrentImpact
	}
	if req.ConfidentialityImpact != nil {
		updated.ConfidentialityImpact = *req.ConfidentialityImpact
	}
	if req.IntegrityImpact != nil {
		updated.IntegrityImpact = *req.IntegrityImpact
	}
	if req.AvailabilityImpact != nil {
		updated.AvailabilityImpact = *req.AvailabilityImpact
	}
	if req.InherentLikelihood != nil {
		updated.InherentLikelihood = *req.InherentLikelihood
	}
	if req.InherentImpact != nil {
		updated.InherentImpact = *req.InherentImpact
	}
	if req.InherentConfidentialityImpact != nil {
		updated.InherentConfidentialityImpact = *req.InherentConfidentialityImpact
	}
	if req.InherentIntegrityImpact != nil {
		updated.InherentIntegrityImpact = *req.InherentIntegrityImpact
	}
	if req.InherentAvailabilityImpact != nil {
		updated.InherentAvailabilityImpact = *req.InherentAvailabilityImpact
	}
	if req.TargetLikelihood != nil {
		updated.TargetLikelihood = *req.TargetLikelihood
	}
	if req.TargetImpact != nil {
		updated.TargetImpact = *req.TargetImpact
	}
	if req.Treatment != nil {
		updated.Treatment = *req.Treatment
	}
	if req.TreatmentPlan != nil {
		updated.TreatmentPlan = *req.TreatmentPlan
	}
	if req.TreatmentDueDate != nil {
		updated.TreatmentDueDate = *req.TreatmentDueDate
	}
	if req.Owner != nil {
		updated.Owner = *req.Owner
	}
	if req.Status != nil {
		updated.Status = *req.Status
	}
	if req.LastReview != nil {
		updated.LastReview = *req.LastReview
	}
	if req.NextReview != nil {
		updated.NextReview = *req.NextReview
	}
	if req.Notes != nil {
		updated.Notes = *req.Notes
	}

	if err := updated.Validate(); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	actor := getUserEmail(c)

	// Auto-set acceptance provenance when status changes to accepted
	if updated.Status == "accepted" && old.Status != "accepted" {
		now := db.NewEpoch(time.Now())
		updated.AcceptedAt = &now
		if u, _ := s.db.GetUserByEmail(ctx, actor); u != nil {
			updated.AcceptedByID = &u.ID
		}
	}
	// Reopen: clear accepted_at / accepted_by_id when transitioning AWAY from accepted.
	if old.Status == "accepted" && updated.Status != "" && updated.Status != "accepted" {
		updated.AcceptedAt = nil
		updated.AcceptedByID = nil
	}

	if err := s.db.UpdateRisk(ctx, orgID, &updated); err != nil {
		return pgxHTTPError(err)
	}
	after, _ := s.db.GetRisk(ctx, orgID, id)
	if after != nil {
		reason := c.QueryParam("reason")
		changes := db.DiffFields("risk", id, actor, reason, old.ToChangeMap(), after.ToChangeMap())
		if len(changes) > 0 {
			_ = s.db.LogChanges(ctx, orgID, changes)
		}
		s.searchUpsert(orgID, "risk", after.Identifier, after.Title, after.Identifier+" "+after.Title+" "+after.Description+" "+after.Category)
		s.logAndNotify(ctx, orgID, &db.Activity{
			Actor:  actor,
			Action: "risk_updated",
			Detail: fmt.Sprintf("%s updated", after.Identifier),
		})
		return c.JSON(http.StatusOK, after)
	}
	return c.JSON(http.StatusOK, updated)
}

func (s *Server) handleUpdateSystem(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system id")
	}
	old, err := s.db.GetSystem(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "system not found")
	}
	var req systemUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.SystemStatuses); err != nil {
			return err
		}
	}
	if req.Criticality != nil {
		if err := validateEnum("criticality", *req.Criticality, db.SystemCriticalities); err != nil {
			return err
		}
	}
	if req.Classification != nil {
		if err := validateEnum("classification", *req.Classification, db.SystemClassifications); err != nil {
			return err
		}
	}
	if req.SupplierID != nil && *req.SupplierID != nil && **req.SupplierID > 0 {
		if _, err := s.db.GetSupplier(ctx, orgID, **req.SupplierID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "supplier not found in this organization")
		}
	}
	if req.Owner != nil && *req.Owner != "" {
		if err := s.validateOrgMember(c, *req.Owner); err != nil {
			return err
		}
	}
	updated := *old
	updated.ID = id
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.Description != nil {
		updated.Description = *req.Description
	}
	if req.SupplierID != nil {
		updated.SupplierID = *req.SupplierID
	}
	if req.Department != nil {
		updated.Department = *req.Department
	}
	if req.Classification != nil {
		updated.Classification = *req.Classification
	}
	if req.Criticality != nil {
		updated.Criticality = *req.Criticality
	}
	if req.Status != nil {
		updated.Status = *req.Status
	}
	if req.RPOHours != nil {
		updated.RPOHours = *req.RPOHours
	}
	if req.RTOHours != nil {
		updated.RTOHours = *req.RTOHours
	}
	if req.Confidentiality != nil {
		updated.Confidentiality = *req.Confidentiality
	}
	if req.Integrity != nil {
		updated.Integrity = *req.Integrity
	}
	if req.Availability != nil {
		updated.Availability = *req.Availability
	}
	if req.LastReview != nil {
		updated.LastReview = *req.LastReview
	}
	if req.NextReview != nil {
		updated.NextReview = *req.NextReview
	}
	if req.Owner != nil {
		updated.Owner = *req.Owner
	}
	if req.Notes != nil {
		updated.Notes = *req.Notes
	}
	if err := s.db.UpdateSystem(ctx, orgID, &updated); err != nil {
		return pgxHTTPError(err)
	}
	after, _ := s.db.GetSystem(ctx, orgID, id)
	if after != nil {
		actor := getUserEmail(c)
		reason := c.QueryParam("reason")
		changes := db.DiffFields("system", id, actor, reason, old.ToChangeMap(), after.ToChangeMap())
		if len(changes) > 0 {
			_ = s.db.LogChanges(ctx, orgID, changes)
		}
		s.searchUpsert(orgID, "system", after.Identifier, after.Name, after.Identifier+" "+after.Name+" "+after.Description)
		s.logAndNotify(ctx, orgID, &db.Activity{
			Actor:  actor,
			Action: "system_updated",
			Detail: fmt.Sprintf("%s updated", after.Identifier),
		})
		return c.JSON(http.StatusOK, after)
	}
	return c.JSON(http.StatusOK, updated)
}

func (s *Server) handleDeleteSystem(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system id")
	}
	old, _ := s.db.GetSystem(ctx, orgID, id)
	if err := s.db.DeleteSystem(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}
	if old != nil {
		_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
			EntityType: "system",
			EntityID:   old.ID,
			Action:     "delete",
			ChangedBy:  getUserEmail(c),
		})
		s.searchRemove(orgID, "system", old.Identifier)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListAccessReviews(c echo.Context) error {
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	systemID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system id")
	}
	reviews, err := s.db.ListAccessReviews(ctx, orgID, systemID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if reviews == nil {
		reviews = []db.AccessReview{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": reviews})
}

func (s *Server) handleCreateAccessReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	systemID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system id")
	}
	var ar db.AccessReview
	if err := c.Bind(&ar); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Verify the system belongs to this org before creating an access review.
	if _, err := s.db.GetSystem(ctx, orgID, systemID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "system not found in this organization")
	}
	ar.SystemID = systemID
	if ar.ReviewedBy == "" {
		ar.ReviewedBy = getUserEmail(c)
	}
	if ar.ReviewedAt.IsZero() {
		ar.ReviewedAt = db.EpochNow()
	}
	if err := s.db.CreateAccessReview(ctx, orgID, &ar); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
		EntityType: "access_review",
		EntityID:   ar.ID,
		Action:     "create",
		ChangedBy:  getUserEmail(c),
	})
	return c.JSON(http.StatusCreated, ar)
}

func (s *Server) handleDeleteAccessReview(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid access review id")
	}
	if err := s.db.DeleteAccessReview(ctx, orgID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleUpdateSupplier(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid supplier id")
	}
	old, err := s.db.GetSupplier(ctx, orgID, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "supplier not found")
	}
	var req supplierUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Status != nil {
		if err := validateEnum("status", *req.Status, db.SupplierStatuses); err != nil {
			return err
		}
	}
	if req.SupplierType != nil {
		if err := validateEnum("supplier_type", *req.SupplierType, db.SupplierTypes); err != nil {
			return err
		}
	}
	if req.Criticality != nil {
		if err := validateEnum("criticality", *req.Criticality, db.CriticalityLevels); err != nil {
			return err
		}
	}
	if req.Owner != nil && *req.Owner != "" {
		if err := s.validateOrgMember(c, *req.Owner); err != nil {
			return err
		}
	}
	updated := *old
	updated.ID = id
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.SupplierType != nil {
		updated.SupplierType = *req.SupplierType
	}
	if req.Criticality != nil {
		updated.Criticality = *req.Criticality
	}
	if req.DataAccess != nil {
		updated.DataAccess = *req.DataAccess
	}
	if req.Contact != nil {
		updated.Contact = *req.Contact
	}
	if req.ContractRef != nil {
		updated.ContractRef = *req.ContractRef
	}
	if req.Status != nil {
		updated.Status = *req.Status
	}
	if req.Owner != nil {
		updated.Owner = *req.Owner
	}
	if req.ContractExpiry != nil {
		updated.ContractExpiry = *req.ContractExpiry
	}
	if req.Confidentiality != nil {
		updated.Confidentiality = *req.Confidentiality
	}
	if req.Integrity != nil {
		updated.Integrity = *req.Integrity
	}
	if req.Availability != nil {
		updated.Availability = *req.Availability
	}
	if req.LastReview != nil {
		updated.LastReview = *req.LastReview
	}
	if req.NextReview != nil {
		updated.NextReview = *req.NextReview
	}
	if req.Notes != nil {
		updated.Notes = *req.Notes
	}
	if err := s.db.UpdateSupplier(ctx, orgID, &updated); err != nil {
		return pgxHTTPError(err)
	}
	after, _ := s.db.GetSupplier(ctx, orgID, id)
	if after != nil {
		actor := getUserEmail(c)
		reason := c.QueryParam("reason")
		changes := db.DiffFields("supplier", id, actor, reason, old.ToChangeMap(), after.ToChangeMap())
		if len(changes) > 0 {
			_ = s.db.LogChanges(ctx, orgID, changes)
		}
		s.searchUpsert(orgID, "supplier", after.Identifier, after.Name, after.Identifier+" "+after.Name+" "+after.Notes)
		s.logAndNotify(ctx, orgID, &db.Activity{
			Actor:  actor,
			Action: "supplier_updated",
			Detail: fmt.Sprintf("%s updated", after.Identifier),
		})
		return c.JSON(http.StatusOK, after)
	}
	return c.JSON(http.StatusOK, updated)
}

func (s *Server) handleDeleteSupplier(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid supplier id")
	}
	old, _ := s.db.GetSupplier(ctx, orgID, id)
	if err := s.db.DeleteSupplier(ctx, orgID, id); err != nil {
		return pgxHTTPError(err)
	}
	if old != nil {
		_ = s.db.LogChange(ctx, orgID, &db.ChangelogEntry{
			EntityType: "supplier",
			EntityID:   old.ID,
			Action:     "delete",
			ChangedBy:  getUserEmail(c),
		})
		s.searchRemove(orgID, "supplier", old.Identifier)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleOverdueSummary(c echo.Context) error {
	orgID := getOrgID(c)
	summary, err := s.db.GetOverdueSummary(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, summary)
}

func (s *Server) handleCreateOverdueTasks(c echo.Context) error {
	if err := requireRole(c, "admin", "manager"); err != nil {
		return err
	}
	orgID := getOrgID(c)
	ctx := c.Request().Context()
	actor := getUserEmail(c)
	result, err := s.db.CreateOverdueReviewTasks(ctx, orgID, actor)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Document review tasks are now included in CreateOverdueReviewTasks via OverdueDocumentReviews (Postgres-based)

	s.logAndNotify(ctx, orgID, &db.Activity{
		Actor:  actor,
		Action: "overdue_tasks_created",
		Detail: fmt.Sprintf("Created %d review tasks (%d skipped, already existed)", len(result.Created), result.Skipped),
	})
	return c.JSON(http.StatusOK, result)
}

// stripFrontmatter removes YAML frontmatter from markdown content.
// incrementVersion bumps a version string by 0.1 (e.g. "1.0" → "1.1", "2.3" → "2.4").
// If the version is empty or unparseable, returns "0.1".
func incrementVersion(v string) string {
	if v == "" {
		return "0.1"
	}
	parts := strings.SplitN(v, ".", 2)
	if len(parts) == 2 {
		minor := 0
		fmt.Sscanf(parts[1], "%d", &minor)
		return fmt.Sprintf("%s.%d", parts[0], minor+1)
	}
	// No dot — append .1
	return v + ".1"
}

// compareVersions returns -1 if a < b, 0 if equal, 1 if a > b.
// Simple numeric comparison of major.minor.
func compareVersions(a, b string) int {
	pa := strings.SplitN(a, ".", 2)
	pb := strings.SplitN(b, ".", 2)
	aMaj, aMin := 0, 0
	bMaj, bMin := 0, 0
	fmt.Sscanf(pa[0], "%d", &aMaj)
	if len(pa) > 1 {
		fmt.Sscanf(pa[1], "%d", &aMin)
	}
	fmt.Sscanf(pb[0], "%d", &bMaj)
	if len(pb) > 1 {
		fmt.Sscanf(pb[1], "%d", &bMin)
	}
	if aMaj != bMaj {
		if aMaj < bMaj {
			return -1
		}
		return 1
	}
	if aMin != bMin {
		if aMin < bMin {
			return -1
		}
		return 1
	}
	return 0
}

func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---") {
		return s
	}
	rest := s[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return s
	}
	return strings.TrimLeft(rest[idx+4:], "\n")
}

// parseID extracts a numeric ID from a string that may have a prefix like "ASSET-5", "RISK-3", etc.
func parseID(s string) (int64, error) {
	for _, prefix := range []string{"ASSET-", "RISK-", "SUPPLIER-", "SYSTEM-"} {
		s = strings.TrimPrefix(s, prefix)
	}
	return strconv.ParseInt(s, 10, 64)
}

// extractHostname parses a URL and returns just the hostname (no port).
// deriveKey uses HKDF-SHA256 to derive a purpose-specific key from the master secret.
func deriveKey(master []byte, label string, n int) []byte {
	h := hmac.New(sha256.New, master)
	h.Write([]byte(label))
	derived := h.Sum(nil)
	return derived[:n]
}

func extractHostname(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "localhost"
	}
	return u.Hostname()
}
