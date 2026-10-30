"""E2E (#408): link text stays readable whatever brand color the admin picks.

A global rule recoloured every `a.text-blue-400` / `button.text-blue-400` to the
raw org brand color, so a dark brand such as #1a365d turned the #RISK-n links in
comments into ~1.2:1 text on the dark theme. The links now use a derived text
color (>= 4.5:1). The @mention span, which is not matched by that rule, is the
control and must keep Tailwind blue-400.

The original branding_color is restored afterwards; other tests leave a dark
color behind on their org, so nothing here depends on test order.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_brand_contrast.py -v
"""
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401

DARK_BRAND = "#1a365d"
SLATE_800 = (0x1d, 0x29, 0x3d)  # Tailwind v4 slate-800 as rendered
DOC = "iso27001-4-1"


def luminance(rgb):
    def lin(v):
        s = v / 255
        return s / 12.92 if s <= 0.03928 else ((s + 0.055) / 1.055) ** 2.4
    r, g, b = rgb
    return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)


def contrast(a, b):
    la, lb = luminance(a), luminance(b)
    return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)


def blue_400_rgb(page):
    """Tailwind v4's --color-blue-400 as sRGB (it must exist at runtime)."""
    return tuple(page.evaluate("""() => {
        const v = getComputedStyle(document.documentElement).getPropertyValue('--color-blue-400').trim()
        if (!v) throw new Error('--color-blue-400 is not defined')
        const c = document.createElement('canvas'); c.width = c.height = 1
        const x = c.getContext('2d'); x.fillStyle = v
        x.fillRect(0, 0, 1, 1); return Array.from(x.getImageData(0, 0, 1, 1).data.slice(0, 3))
    }"""))


def resolved_rgb(locator, prop="color"):
    """Computed text color as sRGB. Tailwind v4 colors compute as oklch(), so
    paint the value on a canvas and read the pixel back."""
    return tuple(locator.evaluate("""(e, prop) => {
        const c = document.createElement('canvas'); c.width = c.height = 1
        const x = c.getContext('2d'); x.fillStyle = getComputedStyle(e)[prop]
        x.fillRect(0, 0, 1, 1); return Array.from(x.getImageData(0, 0, 1, 1).data.slice(0, 3))
    }""", prop))


@pytest.fixture
def dark_brand(tokens):
    t = tokens["admin"]
    original = api("get", "/config", t, expect_status=200).json().get("branding", {}).get("branding_color", "")
    api("put", "/admin/settings", t, json={"key": "branding_color", "value": DARK_BRAND}, expect_status=[200, 204])
    try:
        yield t
    finally:
        api("put", "/admin/settings", t, json={"key": "branding_color", "value": original}, expect_status=[200, 204])


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def test_entity_link_in_comment_is_readable_on_dark_brand(page, dark_brand):
    t = dark_brand
    risk = api("post", "/risks", t, json={
        "title": f"E2E brand {uuid.uuid4().hex[:8]}", "description": "e2e brand contrast",
        "risk_type": "threat", "origin": "internal", "status": "open", "treatment": "mitigate",
        "current_likelihood": 2, "current_impact": 2,
    }, expect_status=[200, 201]).json()
    ident = risk.get("identifier") or f"RISK-{risk['id']}"
    body = f"ping @{ADMIN[0]} about #{ident} {uuid.uuid4().hex[:6]}"
    c = api("post", "/comments", t, json={"document_id": DOC, "body": body, "paragraph_index": 0},
            expect_status=[200, 201]).json()
    try:
        page.goto(f"{BASE}/{ORG}/documents/{DOC}")
        page.locator("h1").first.wait_for(state="visible", timeout=10000)
        page.locator('button[title="Comments"]').click()
        link = page.locator("a.text-blue-400", has_text=f"#{ident}").first
        expect(link).to_be_visible(timeout=10000)
        mention = page.locator("span.text-blue-400", has_text=f"@{ADMIN[0]}").first
        expect(mention).to_be_visible(timeout=10000)

        link_rgb = resolved_rgb(link)
        link_color = f"rgb{link_rgb}"
        ratio = contrast(link_rgb, SLATE_800)
        assert ratio >= 4.5, f"link color {link_color} is {ratio:.2f}:1 on slate-800"
        org_name = page.locator(".brand-org-name").first
        expect(org_name).to_be_visible(timeout=10000)
        org_rgb = resolved_rgb(org_name)
        assert contrast(org_rgb, SLATE_800) >= 4.5, f"sidebar org name rgb{org_rgb} is {contrast(org_rgb, SLATE_800):.2f}:1"
        # Control: not matched by the brand rule, stays Tailwind blue-400.
        assert resolved_rgb(mention) == blue_400_rgb(page)
        assert resolved_rgb(mention) != link_rgb
    finally:
        api("post", f"/comments/{c['id']}/resolve", t, expect_status=[200, 204])


@pytest.fixture
def brand(tokens):
    """Set branding_color for one test and restore the original afterwards."""
    t = tokens["admin"]
    original = api("get", "/config", t, expect_status=200).json().get("branding", {}).get("branding_color", "")

    def set_brand(value):
        api("put", "/admin/settings", t, json={"key": "branding_color", "value": value}, expect_status=[200, 204])

    try:
        yield set_brand
    finally:
        set_brand(original)


def test_unbranded_link_matches_mention_color(page, tokens, brand):
    """With no brand color the link falls back to blue-400, same as a mention."""
    t = tokens["admin"]
    brand("")
    risk = api("post", "/risks", t, json={
        "title": f"E2E brand {uuid.uuid4().hex[:8]}", "description": "e2e brand contrast",
        "risk_type": "threat", "origin": "internal", "status": "open", "treatment": "mitigate",
        "current_likelihood": 2, "current_impact": 2,
    }, expect_status=[200, 201]).json()
    ident = risk.get("identifier") or f"RISK-{risk['id']}"
    c = api("post", "/comments", t, json={
        "document_id": DOC, "body": f"ping @{ADMIN[0]} about #{ident} {uuid.uuid4().hex[:6]}", "paragraph_index": 0,
    }, expect_status=[200, 201]).json()
    try:
        page.goto(f"{BASE}/{ORG}/documents/{DOC}")
        page.locator("h1").first.wait_for(state="visible", timeout=10000)
        page.locator('button[title="Comments"]').click()
        link = page.locator("a.text-blue-400", has_text=f"#{ident}").first
        mention = page.locator("span.text-blue-400", has_text=f"@{ADMIN[0]}").first
        expect(link).to_be_visible(timeout=10000)
        expect(mention).to_be_visible(timeout=10000)
        assert resolved_rgb(link) == resolved_rgb(mention) == blue_400_rgb(page)
    finally:
        api("post", f"/comments/{c['id']}/resolve", t, expect_status=[200, 204])


def test_login_link_uses_default_text_color(pw_browser, tokens):
    """Logged-out auth screens use the derived brand text color, not the raw one.

    On path-mode deployments the login page shows the default (unbranded) color
    for every org, so this exercises the default: --brand-text-color resolves to
    blue-400. (The raw default --brand-color is blue-500, rgb(59, 130, 246).)
    """
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    try:
        p.goto(f"{BASE}/login")
        org_input = p.get_by_placeholder("Organization name, e.g. acme")
        if org_input.is_visible(timeout=3000):
            org_input.fill(ORG)
            p.get_by_role("button", name="Continue").click()
        p.get_by_placeholder("you@company.com").wait_for(state="visible", timeout=10000)
        link = p.locator("a.brand-text[href$='/forgot-password']").first
        expect(link).to_be_visible(timeout=10000)
        assert resolved_rgb(link) == blue_400_rgb(p)
    finally:
        ctx.close()


def add_risk_button(page):
    page.goto(f"{BASE}/{ORG}/risks")
    btn = page.locator("button.bg-blue-600.text-white:not([disabled])").first
    expect(btn).to_be_visible(timeout=15000)
    return btn


def test_primary_button_label_is_readable_on_light_brand(page, brand):
    """A light brand (#fde047) is 1.32:1 against a white label; the fill darkens."""
    brand("#fde047")
    btn = add_risk_button(page)
    bg, fg = resolved_rgb(btn, "backgroundColor"), resolved_rgb(btn)
    assert contrast(bg, fg) >= 4.5, f"button rgb{bg} with label rgb{fg} is {contrast(bg, fg):.2f}:1"
    assert bg == (132, 116, 37), f"fill rgb{bg} is not the derived #847425"


def test_primary_button_fill_unchanged_on_dark_brand(page, brand):
    """White already passes on #1a365d, so the fill keeps the raw brand color."""
    brand(DARK_BRAND)
    btn = add_risk_button(page)
    assert resolved_rgb(btn, "backgroundColor") == (26, 54, 93)


DEFAULT_FILL = (52, 114, 216)  # readableFillColor('#3b82f6') = #3472d8


def test_stored_default_brand_renders_as_unbranded(page, tokens, brand):
    """The Admin form saves #3b82f6 with every branding save; it must look unbranded."""
    t = tokens["admin"]
    brand("#3b82f6")
    risk = api("post", "/risks", t, json={
        "title": f"E2E brand {uuid.uuid4().hex[:8]}", "description": "e2e brand contrast",
        "risk_type": "threat", "origin": "internal", "status": "open", "treatment": "mitigate",
        "current_likelihood": 2, "current_impact": 2,
    }, expect_status=[200, 201]).json()
    ident = risk.get("identifier") or f"RISK-{risk['id']}"
    c = api("post", "/comments", t, json={
        "document_id": DOC, "body": f"ping @{ADMIN[0]} about #{ident} {uuid.uuid4().hex[:6]}", "paragraph_index": 0,
    }, expect_status=[200, 201]).json()
    try:
        page.goto(f"{BASE}/{ORG}/documents/{DOC}")
        page.locator("h1").first.wait_for(state="visible", timeout=10000)
        page.locator('button[title="Comments"]').click()
        link = page.locator("a.text-blue-400", has_text=f"#{ident}").first
        mention = page.locator("span.text-blue-400", has_text=f"@{ADMIN[0]}").first
        expect(link).to_be_visible(timeout=10000)
        expect(mention).to_be_visible(timeout=10000)
        assert resolved_rgb(link) == resolved_rgb(mention) == blue_400_rgb(page)
        btn = add_risk_button(page)
        assert resolved_rgb(btn, "backgroundColor") == DEFAULT_FILL
    finally:
        api("post", f"/comments/{c['id']}/resolve", t, expect_status=[200, 204])


def test_clearing_the_brand_color_in_admin_resets_without_reload(page, tokens, brand):
    """Clear + save in Admin > Branding must drop the old colors in the same session."""
    t = tokens["admin"]
    brand(DARK_BRAND)
    page.goto(f"{BASE}/{ORG}/admin/branding")
    color = page.locator('input[type="text"][placeholder="#3b82f6"]')
    expect(color).to_be_visible(timeout=15000)
    expect(color).to_have_value(DARK_BRAND, timeout=10000)
    org_name = page.locator(".brand-org-name").first
    dark_text = resolved_rgb(org_name)
    assert dark_text != blue_400_rgb(page), "dark brand text should differ from the default"

    color.fill("")
    page.get_by_role("button", name="Save branding").click()
    expect(page.get_by_text("Branding saved")).to_be_visible(timeout=10000)
    stored = api("get", "/config", t, expect_status=200).json().get("branding", {}).get("branding_color")
    assert not stored, f"the server did not store an empty color: {stored!r}"

    # No reload: the variables live on <html> and must already be reset.
    assert resolved_rgb(org_name) == blue_400_rgb(page)
    page.evaluate("() => document.querySelector('#app').__vue_app__.config.globalProperties.$router.push('/%s/risks')" % ORG)
    btn = page.locator("button.bg-blue-600.text-white:not([disabled])").first
    expect(btn).to_be_visible(timeout=15000)
    assert resolved_rgb(btn, "backgroundColor") == DEFAULT_FILL
    assert resolved_rgb(page.locator(".brand-org-name").first) == blue_400_rgb(page)


def test_admin_hint_names_what_is_adjusted_for_readability(page, brand):
    """Typing a color in Admin > Branding shows a live hint (nothing is saved)."""
    brand(DARK_BRAND)  # known stored color; restored by the fixture afterwards
    page.goto(f"{BASE}/{ORG}/admin/branding")
    color = page.locator('input[type="text"][placeholder="#3b82f6"]')
    expect(color).to_be_visible(timeout=15000)
    hint = page.locator('[data-testid="brand-adjusted-hint"]')

    color.fill("#1a365d")
    expect(hint).to_contain_text("For readability, links use", timeout=5000)
    expect(hint).to_contain_text("#8190a6")
    assert "buttons" not in hint.inner_text()

    color.fill("#fde047")
    expect(hint).to_contain_text("For readability, buttons use", timeout=5000)
    expect(hint).to_contain_text("#847425")
    assert "links" not in hint.inner_text()

    color.fill("#ff4477")
    expect(hint).to_contain_text("links use", timeout=5000)
    expect(hint).to_contain_text("and buttons use")

    color.fill("#3b82f6")
    expect(hint).to_have_count(0, timeout=5000)


def test_late_login_config_does_not_reset_the_brand(pw_browser, tokens, brand):
    """A sign-in page's /config that lands after the app loaded must be ignored.

    With several orgs, pre-login /config carries no branding in path mode, so
    applying it late would wipe the brand the app applied with the org token.
    A single-org server includes the branding in it, so the test strips it from
    the held response to stand in for the multi-org case.
    """
    brand(DARK_BRAND)
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    held = []

    def hold_unauthenticated(route):
        # Park only requests made before sign-in; the test releases them later.
        if route.request.headers.get("authorization"):
            route.continue_()
        else:
            held.append(route)

    page.route("**/api/v1/config*", hold_unauthenticated)
    try:
        page.goto(f"{BASE}/{ORG}/login")
        email = page.get_by_placeholder("you@company.com")
        email.wait_for(state="visible", timeout=10000)
        email.fill(ADMIN[0])
        page.get_by_placeholder("Password", exact=True).fill(ADMIN[1])
        page.get_by_role("button", name="Sign in").click()
        page.locator("aside").first.wait_for(state="visible", timeout=10000)
        assert held, "the sign-in page's /config was not held, so the test proves nothing"

        org_name = page.locator(".brand-org-name").first
        expect(org_name).to_be_visible(timeout=10000)
        dark_text = resolved_rgb(org_name)
        assert dark_text != blue_400_rgb(page), "the app did not apply the dark brand"

        for route in held:  # the late, branding-less response lands now
            body = route.fetch().json()
            body.pop("branding", None)
            route.fulfill(json=body)
        page.wait_for_timeout(1500)
        assert resolved_rgb(org_name) == dark_text, "late /config reset the brand"
    finally:
        ctx.close()
