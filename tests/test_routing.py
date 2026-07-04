"""Multi-tenant routing tests — backend OrgResolverMiddleware.

Verifies that the org context is resolved from the request host (or path)
before auth, and that the routing model documented in
docs/multi-tenant-routing.md holds at the wire level:

  - Subdomain `<slug>.<apex>` → org bound to that slug
  - Apex (no subdomain) → no org context (landing)
  - Unknown subdomain → no org context (fall through, no leak)

Requires the devenv server to be running with:
    ISMS_DOMAIN=localhost
    ISMS_SUBDOMAIN_ROUTING=1
    extra_hosts: acme-logistics.localhost → 127.0.0.1
"""
import os
import requests
import pytest

from conftest import BASE_URL, API, ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_NAME, _signup, _login

ROUTING_ORG_SLUG = "acme-logistics"
ROUTING_ORG_NAME = "Acme Logistics"


def _ensure_routing_org():
    """Create the routing test org if it doesn't already exist."""
    token = _signup(ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_NAME)
    if token is None:
        token = _login(ADMIN_EMAIL, ADMIN_PASSWORD, "")
    assert token is not None, "Could not auth admin"
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    # 409 if already exists — idempotent
    requests.post(f"{API}/organizations", headers=headers, json={
        "name": ROUTING_ORG_NAME, "slug": ROUTING_ORG_SLUG,
    })


def _server_config():
    """Fetch /api/v1/config to learn what the server thinks its apex and
    subdomain-routing settings are. Tests adapt to whatever the server
    reports rather than hard-coding `localhost` — that way they work in
    any deployment (devenv, staging, production-clone) as long as
    subdomain routing is enabled.
    """
    return requests.get(f"{BASE_URL}/api/v1/config").json()


def _require_subdomain_routing(cfg):
    if not cfg.get("subdomain_routing"):
        pytest.skip(
            "server has ISMS_SUBDOMAIN_ROUTING off — subdomain resolution "
            "tests need it on (set in devenv/compose.yml)"
        )
    if not cfg.get("apex_host"):
        pytest.skip("server has no apex_host configured (ISMS_DOMAIN unset)")


class TestSubdomainResolution:
    """OrgResolverMiddleware resolves the org from the Host header."""

    @classmethod
    def setup_class(cls):
        _ensure_routing_org()
        cls.cfg = _server_config()

    def test_subdomain_sets_org_context(self):
        """Host header `<slug>.<apex>` → org context for that slug."""
        _require_subdomain_routing(self.cfg)
        host = f"{ROUTING_ORG_SLUG}.{self.cfg['apex_host']}"
        r = requests.get(f"{BASE_URL}/api/v1/config", headers={"Host": host})
        assert r.status_code == 200, f"got {r.status_code}: {r.text[:200]}"
        cfg = r.json()
        assert cfg.get("organization_slug") == ROUTING_ORG_SLUG, (
            f"expected org_slug={ROUTING_ORG_SLUG!r} from Host: {host}, "
            f"got {cfg.get('organization_slug')!r}"
        )

    def test_apex_has_no_org_context(self):
        """Plain apex Host → no org bound, neutral config.

        NB: the server auto-resolves the apex to the org when a deployment has
        EXACTLY ONE org (single-org self-hosted convenience). This suite always
        has multiple orgs (the admin org from conftest + acme-logistics created
        in setup_class), so the apex stays neutral here.
        """
        _require_subdomain_routing(self.cfg)
        r = requests.get(
            f"{BASE_URL}/api/v1/config",
            headers={"Host": self.cfg["apex_host"]},
        )
        assert r.status_code == 200
        cfg = r.json()
        assert not cfg.get("organization_slug"), (
            f"apex must not surface an org_slug with multiple orgs; "
            f"got {cfg.get('organization_slug')!r}"
        )

    def test_unknown_subdomain_falls_through(self):
        """Subdomain pointing at a non-existent org → no org context (no 5xx, no leak)."""
        _require_subdomain_routing(self.cfg)
        host = f"definitely-not-an-org-12345.{self.cfg['apex_host']}"
        r = requests.get(f"{BASE_URL}/api/v1/config", headers={"Host": host})
        assert r.status_code == 200
        cfg = r.json()
        assert not cfg.get("organization_slug")

    def test_subdomain_routing_advertised_in_config(self):
        """The SPA needs `subdomain_routing` + `apex_host` to build entry URLs.
        Skipped (not failed) when the deployment runs path-only — the test
        verifies the wire format when subdomain routing IS enabled.
        """
        _require_subdomain_routing(self.cfg)
        assert self.cfg.get("subdomain_routing") is True
        assert self.cfg.get("apex_host"), "apex_host must be set when subdomain routing is on"


class TestSpaApexInjection:
    """Regression: the SPA index.html must carry the deployment's apex host as a
    <meta name="isms-apex"> tag so the frontend classifies its own hostname (apex
    vs tenant subdomain) synchronously at boot — before /api/v1/config loads.

    Without it, host classification runs at module-import time (router.js,
    App.vue) with only the isms.sh seed, so a self-hosted apex on any other
    domain (e.g. isms.stsplatform.com) is misread as a tenant subdomain "isms".
    The login page then probes a phantom org and 404s on /auth/oidc/providers.

    A meta tag is used (not an inline <script>) because the CSP is script-src
    'self' — an inline script would be blocked and silently never run.
    """

    @classmethod
    def setup_class(cls):
        cls.cfg = _server_config()

    def test_index_html_injects_apex_host(self):
        apex = self.cfg.get("apex_host")
        if not apex:
            pytest.skip("server has no apex_host configured (ISMS_DOMAIN unset)")
        r = requests.get(f"{BASE_URL}/")
        assert r.status_code == 200, f"got {r.status_code}: {r.text[:200]}"
        assert "text/html" in r.headers.get("content-type", "")
        marker = f'<meta name="isms-apex" content="{apex}">'
        assert marker in r.text, (
            f"expected {marker!r} injected into index.html; "
            f"apex_host={apex!r} not found in served HTML"
        )

    def test_apex_injection_is_not_an_inline_script(self):
        """Guard against regressing to an inline <script>, which the CSP
        (script-src 'self') would block from executing."""
        r = requests.get(f"{BASE_URL}/")
        assert "window.__ISMS_APEX__" not in r.text, (
            "apex host must be injected as a <meta> tag, not an inline script "
            "(CSP script-src 'self' blocks inline scripts)"
        )

    def test_spa_fallback_route_also_injects(self):
        """A deep-link client route (/login) falls back to index.html and must
        carry the same injection — /login is exactly where the bug surfaced."""
        apex = self.cfg.get("apex_host")
        if not apex:
            pytest.skip("server has no apex_host configured (ISMS_DOMAIN unset)")
        r = requests.get(f"{BASE_URL}/login")
        assert r.status_code == 200, f"got {r.status_code}: {r.text[:200]}"
        assert f'<meta name="isms-apex" content="{apex}">' in r.text


class TestDocsServedNatively:
    """Regression guard: /docs is a server-served public path, not an SPA route.

    The Vue SPA's global anchor-click interceptor used to capture <a href="/docs">
    and push it through Vue Router, which has no /docs route, so the auth guard
    redirected users to /login. The fix is to only intercept when
    router.resolve(href).matched.length > 0 — but at the HTTP layer, /docs must
    always return the Scalar API reference HTML.
    """

    def test_docs_returns_scalar_html_unauthenticated(self):
        r = requests.get(f"{BASE_URL}/docs")
        assert r.status_code == 200
        assert "text/html" in r.headers.get("Content-Type", "")
        # Scalar UI loads its bundle from a known CDN path.
        assert "scalar" in r.text.lower(), (
            "/docs should serve the Scalar API reference HTML, not the SPA index"
        )

    def test_openapi_spec_served_unauthenticated(self):
        r = requests.get(f"{BASE_URL}/api/openapi.yaml")
        assert r.status_code == 200
        assert "yaml" in r.headers.get("Content-Type", "").lower()
        assert "openapi:" in r.text.lower()
