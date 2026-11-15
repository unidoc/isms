"""E2E (#524): trust center visitors on the manager screen and the public page.

CI's server has NO SMTP, so no sign-in mail can be sent and a visitor can never
sign in here; that round trip is covered by the Go tests against a fake SMTP
sink and by node tests of the page. What only a browser shows, and what this
file checks against a server with no mail:

  - the manager screen warns that email is not configured;
  - the Visitors tab shows its empty state;
  - the blocked-domains editor saves a custom list, restores the defaults, and
    the "built-in list" note comes back;
  - the public page offers no sign-in link and keeps the contact fallback on a
    gated document.

Each test puts back what it touched: the trust center is switched off, the
domain list is back on the built-in default, and created documents are deleted.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_trust_center_visitors.py -v
"""
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, ADMIN, R1, BASE, ORG, pw_browser, tokens  # noqa: F401
from test_e2e_trust_center_manager import _page, cleanup, new_doc, open_manager, reviewer, set_enabled  # noqa: F401


@pytest.fixture
def admin_page(pw_browser, tokens):
    ctx, p = _page(pw_browser, ADMIN)
    yield p
    ctx.close()


def settings(token):
    return api("get", "/trust-center/settings", token, expect_status=200).json()


@pytest.fixture(autouse=True)
def needs_a_server_without_mail(tokens):
    if settings(tokens["admin"])["email_configured"]:
        pytest.skip("this server can send mail; these tests describe a server without it")


def test_manager_screen_warns_that_email_is_not_configured(admin_page):
    open_manager(admin_page)
    warning = admin_page.locator('[data-trust="warn-email"]')
    expect(warning).to_be_visible(timeout=15000)
    expect(warning).to_contain_text("Email is not configured: visitors cannot sign in.")
    expect(warning).to_contain_text("Gated documents show the security contact instead.")
    # nothing else is wrong
    expect(admin_page.locator('[data-trust="warn-paused"]')).to_have_count(0)
    expect(admin_page.locator('[data-trust="warn-withheld"]')).to_have_count(0)


def test_visitors_tab_shows_the_empty_state(admin_page):
    open_manager(admin_page)
    admin_page.locator('[data-trust-tab="visitors"]').click()
    assert "tab=visitors" in admin_page.url
    expect(admin_page.locator('[data-trust="visitors-empty"]')).to_have_text("No visitors have signed in yet.", timeout=15000)


def test_blocked_domains_editor_saves_restores_and_the_note_returns(admin_page, tokens):
    t = tokens["admin"]
    try:
        open_manager(admin_page, "settings")
        area = admin_page.locator('[data-trust="domains"]')
        note = admin_page.locator('[data-trust="domains-default-note"]')
        expect(note).to_be_visible(timeout=15000)
        expect(note).to_have_text("Using the built-in list of personal email providers.")
        assert "gmail.com" in area.input_value()

        # A custom list: saved, normalised by the server, and the note goes away.
        area.fill("Rival.test\n@example.com\n")
        admin_page.locator('[data-trust="save"]').click()
        expect(admin_page.locator('[data-trust="saved"]')).to_be_visible(timeout=8000)
        expect(note).to_have_count(0)
        stored = settings(t)
        assert stored["blocked_email_domains_default"] is False
        assert stored["blocked_email_domains"] == ["example.com", "rival.test"]

        # The saved list survives a reload.
        admin_page.reload()
        expect(admin_page.locator('[data-trust="domains"]')).to_have_value("example.com\nrival.test", timeout=15000)

        # Restore the defaults: the built-in list and the note are back.
        admin_page.locator('[data-trust="domains-restore"]').click()
        expect(note).to_be_visible()
        admin_page.locator('[data-trust="save"]').click()
        expect(admin_page.locator('[data-trust="saved"]')).to_be_visible(timeout=8000)
        stored = settings(t)
        assert stored["blocked_email_domains_default"] is True
        assert "gmail.com" in stored["blocked_email_domains"]
        expect(note).to_be_visible()

        # A bad entry is refused with an inline error and nothing is saved.
        admin_page.locator('[data-trust="domains"]').fill("nodot")
        admin_page.locator('[data-trust="save"]').click()
        expect(admin_page.locator('[data-trust="domains-error"]')).to_be_visible(timeout=8000)
        assert settings(t)["blocked_email_domains_default"] is True
    finally:
        api("put", "/trust-center/settings", t, json={"blocked_email_domains": None})


def test_public_page_has_no_sign_in_and_keeps_the_contact_fallback(pw_browser, tokens, reviewer):
    t = tokens["admin"]
    doc = new_doc(t, reviewer, f"E2E Gated {uuid.uuid4().hex[:4]}")
    try:
        api("put", "/trust-center/settings", t, expect_status=200,
            json={"enabled": True, "security_contact": "security@e2e.test"})
        api("post", f"/documents/{doc}/trust-center", t, expect_status=[200, 201], json={"access_level": "gated"})
        ctx = pw_browser.new_context(viewport={"width": 1280, "height": 900})
        try:
            page = ctx.new_page()
            page.goto(f"{BASE}/{ORG}/trust")
            expect(page.locator('[data-trust="documents"]')).to_be_visible(timeout=15000)
            expect(page.locator('[data-trust="contact"]')).to_contain_text("security@e2e.test")
            expect(page.locator('[data-trust="sign-in-link"]')).to_have_count(0)
            expect(page.locator('[data-trust="sign-in-to-view"]')).to_have_count(0)
            expect(page.locator('[data-trust="view"]')).to_have_count(0)
        finally:
            ctx.close()
    finally:
        cleanup(t, [doc])
        api("put", "/trust-center/settings", t, json={"security_contact": ""})
