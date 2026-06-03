"""E2E routing tests — Playwright against a tenant subdomain host.

Verifies the subdomain-bound routing model in a real browser:

  - /organizations picker is never reachable from a tenant subdomain
  - Org switcher dropdown hides "Switch organization" + otherOrgs
  - Stale-token refresh on a subdomain goes to /login, not the picker
  - "Read the docs" link on Landing navigates natively to /docs (Scalar),
    not into the SPA org-discovery flow

The devenv container is configured for these tests:
  - ISMS_DOMAIN=localhost
  - ISMS_SUBDOMAIN_ROUTING=1
  - extra_hosts: acme-logistics.localhost → 127.0.0.1

Run from inside the devenv container:
    pytest tests/test_e2e_routing.py -v
"""
import os
from urllib.parse import urlparse

import pytest
import requests

try:
    from playwright.sync_api import sync_playwright, expect
except ImportError:
    pytest.fail(
        "Playwright is required for E2E routing tests. Install with: "
        "pip install playwright && playwright install chromium",
        pytrace=False,
    )

APEX = os.environ.get("ISMS_TEST_URL", "http://localhost:9090")
ORG_SLUG = "acme-logistics"
# Subdomain host: swap the apex hostname for <slug>.localhost, keeping the
# port. Works from the host (browsers resolve *.localhost to 127.0.0.1) and
# from the dev container (compose network alias on the isms-test service).
_apex = urlparse(APEX)
SUBDOMAIN = f"{_apex.scheme}://{ORG_SLUG}.localhost" + (f":{_apex.port}" if _apex.port else "")

ADMIN_EMAIL = "routing-admin@test.local"
ADMIN_PASSWORD = "RoutingTest2026!"
ADMIN_NAME = "Routing Admin"


def _api(method, base, path, token=None, **kw):
    h = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"} if token else {}
    return getattr(requests, method)(f"{base}/api/v1{path}", headers=h, timeout=15, **kw)


@pytest.fixture(scope="module")
def setup_org():
    """Create the routing test org + admin user with membership."""
    _api("post", APEX, "/auth/signup",
         json={"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD, "name": ADMIN_NAME})
    token = _api("post", APEX, "/auth/login",
                 json={"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD}).json().get("token")
    assert token, "Could not signup or login routing admin"
    # Create the org (409 if exists — both fine for our purposes)
    _api("post", APEX, "/organizations", token,
         json={"name": "Acme Logistics", "slug": ORG_SLUG, "template": "iso27001"})
    # Re-login scoped to the org so subsequent /me returns the right slug
    scoped = _api("post", APEX, "/auth/login",
                  json={"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD,
                        "organization": ORG_SLUG}).json().get("token")
    return {"token": scoped or token}


@pytest.fixture(scope="module")
def browser():
    with sync_playwright() as p:
        # Chromium hard-maps *.localhost to 127.0.0.1 (RFC 6761), bypassing
        # DNS and /etc/hosts. Remap the tenant subdomain to the apex host so
        # the browser reaches the actual test server (e.g. the isms-test
        # container when run from the devenv work container).
        b = p.chromium.launch(args=[
            f"--host-resolver-rules=MAP {ORG_SLUG}.localhost {_apex.hostname}",
        ])
        yield b
        b.close()


def _login_on_subdomain(page, email, password):
    """Login at <slug>.localhost. The org is implicit (Host header) so the
    org-discovery step is skipped — straight to email + password."""
    page.set_default_timeout(8000)
    page.goto(f"{SUBDOMAIN}/login")
    page.get_by_placeholder("you@company.com").wait_for(state="visible", timeout=8000)
    page.get_by_placeholder("you@company.com").fill(email)
    page.get_by_placeholder("Password", exact=True).fill(password)
    page.get_by_role("button", name="Sign in").click()
    page.locator("aside").first.wait_for(state="visible", timeout=10000)


class TestSubdomainBound:
    """On a tenant subdomain, the org is bound — no picker, no switcher targets."""

    def test_organizations_route_redirects(self, browser, setup_org):
        """Direct navigation to /organizations on subdomain → bounces to /overview."""
        ctx = browser.new_context()
        page = ctx.new_page()
        try:
            _login_on_subdomain(page, ADMIN_EMAIL, ADMIN_PASSWORD)
            page.goto(f"{SUBDOMAIN}/organizations")
            # Router guard should redirect immediately.
            page.wait_for_url(f"{SUBDOMAIN}/overview", timeout=5000)
            assert "/organizations" not in page.url
        finally:
            ctx.close()

    def test_switcher_hides_switch_organization_link(self, browser, setup_org):
        """Header org switcher must not surface a "Switch organization" link."""
        ctx = browser.new_context()
        page = ctx.new_page()
        try:
            _login_on_subdomain(page, ADMIN_EMAIL, ADMIN_PASSWORD)
            # Open the header org-switcher dropdown.
            page.locator("button:has-text('" + ORG_SLUG + "')").first.click()
            # The dropdown shows current org + (admin-only) Settings, but the
            # "Switch organization" link must be absent on subdomain.
            expect(page.locator("a:has-text('Switch organization')")).to_have_count(0)
            expect(page.locator("text=Create new organization")).to_have_count(0)
        finally:
            ctx.close()

    def test_stale_token_lands_on_login_not_picker(self, browser, setup_org):
        """Stale-token refresh on subdomain → /login (not /organizations)."""
        ctx = browser.new_context()
        page = ctx.new_page()
        try:
            # Plant a deliberately invalid token so /me returns 401 on first call.
            page.goto(SUBDOMAIN)
            page.evaluate("localStorage.setItem('isms_api_token', 'invalid-jwt-for-test')")
            # Navigate to an org-scoped route — the auth guard should redirect.
            page.goto(f"{SUBDOMAIN}/overview")
            page.wait_for_url(f"{SUBDOMAIN}/login**", timeout=5000)
            # Must NOT have landed on the picker — that would leak other orgs.
            assert "/organizations" not in page.url
        finally:
            ctx.close()


class TestDocsLinkServedNatively:
    """Regression for the global anchor interceptor fix.

    Clicking "Read the docs" on Landing.vue must trigger a real browser
    navigation to the server-served /docs Scalar UI — not a Vue Router push
    that gets caught by the auth guard.
    """

    def test_docs_link_navigates_to_scalar(self, browser):
        """Click → full nav → Scalar HTML loads (no SPA shell, no login redirect)."""
        ctx = browser.new_context()
        page = ctx.new_page()
        try:
            page.goto(APEX)  # Landing on apex (path-based mode)
            # Wait for the Landing CTAs to render.
            page.locator("a[href='/docs']").first.wait_for(state="visible", timeout=5000)
            with page.expect_navigation(timeout=8000) as nav_info:
                page.locator("a[href='/docs']").first.click()
            response = nav_info.value
            # Scalar UI loads its bundle from CDN. The response body must
            # include "scalar" — otherwise the SPA shell came through and the
            # interceptor bug is back.
            content = page.content().lower()
            assert "scalar" in content, (
                "Expected Scalar API reference HTML at /docs; the SPA shell "
                "appears to have intercepted the click (regression)."
            )
            # And the URL stays on /docs — not bounced to /login.
            assert page.url.rstrip("/").endswith("/docs"), f"unexpected URL: {page.url}"
        finally:
            ctx.close()
