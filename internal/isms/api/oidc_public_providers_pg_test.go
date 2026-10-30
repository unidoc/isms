package api

// The unauthenticated provider list feeds the login page. It must expose only
// what the page needs and never an OIDC client secret. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run OIDCPublic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

func TestOIDCPublicProviders_NoSecretAndMinimalKeys(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "oidcpub")
	slug := fmt.Sprintf("oidcpub-%d", os.Getpid())

	const secret = "s3cr3t-value-that-must-not-leak-9f8e7d"
	enabled := &db.OIDCProvider{
		OrganizationID: orgID, ProviderName: "acme", DisplayName: "Acme SSO",
		ClientID: "client-id-xyz", ClientSecret: secret,
		DiscoveryURL: "https://idp.example.com/.well-known/openid-configuration",
		Scopes:       "openid email", DefaultRole: "reader", Enabled: true,
	}
	disabled := &db.OIDCProvider{
		OrganizationID: orgID, ProviderName: "off", DisplayName: "Disabled SSO",
		ClientID: "client-id-off", ClientSecret: secret,
		DiscoveryURL: "https://idp2.example.com/.well-known/openid-configuration",
		Scopes:       "openid email", DefaultRole: "reader", Enabled: false,
	}
	for _, p := range []*db.OIDCProvider{enabled, disabled} {
		if err := s.db.CreateOIDCProvider(ctx, p); err != nil {
			t.Fatalf("creating provider %s: %v", p.ProviderName, err)
		}
	}

	// No user or org set on the context: this is the public route.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/providers?org="+url.QueryEscape(slug), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := s.handleOIDCProviders(c); err != nil {
		t.Fatalf("handleOIDCProviders: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, secret) {
		t.Errorf("public response contains the client secret: %s", body)
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v: %s", err, body)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("got %d providers, want 1 (disabled one must not be listed): %s", len(resp.Data), body)
	}
	var keys []string
	for k := range resp.Data[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if got, want := strings.Join(keys, ","), "display_name,id,provider_name"; got != want {
		t.Errorf("provider keys = %s, want %s", got, want)
	}
	if resp.Data[0]["provider_name"] != "acme" || resp.Data[0]["display_name"] != "Acme SSO" {
		t.Errorf("unexpected provider: %v", resp.Data[0])
	}
}

func TestOIDCPublicProviders_EmptyIsArray(t *testing.T) {
	s := testServer(t)
	newTestOrg(t, s, "oidcpubempty")
	slug := fmt.Sprintf("oidcpubempty-%d", os.Getpid())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/providers?org="+url.QueryEscape(slug), nil)
	rec := httptest.NewRecorder()
	if err := s.handleOIDCProviders(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("handleOIDCProviders: %v", err)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"data":[]}` {
		t.Errorf("body = %s, want {\"data\":[]}", got)
	}
}
