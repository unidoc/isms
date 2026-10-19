"""E2E (#459): a Readings section whose request fails says so instead of "No readings yet.".

The panel used to swallow every failure (4xx, 5xx, network) into an empty list,
so a failed load looked exactly like a record that was never assessed. The
readings request is failed in the browser with page.route, registered before
navigation; the controls leave it alone. POST /risks creates no reading by
itself, so the populated cases seed one through POST /risks/<id>/readings.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_readings_load_error.py -v
"""
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401

ERROR_TEXT = "Failed to load readings"
EMPTY_TEXT = "No readings yet."
READINGS = "**/api/v1/risks/*/readings"


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def seed_risk(token, with_reading=True):
    title = f"E2E readings {uuid.uuid4().hex[:8]}"
    body = {
        "title": title, "description": "e2e readings", "risk_type": "threat",
        "origin": "internal", "status": "open", "treatment": "mitigate",
        "current_likelihood": 2, "current_impact": 2,
    }
    rid = api("post", "/risks", token, json=body, expect_status=[200, 201]).json()["id"]
    if with_reading:
        api("post", f"/risks/{rid}/readings", token, json={
            "confidentiality": 3, "integrity": 2, "availability": 1,
            "current_likelihood": 2, "current_impact": 3,
        }, expect_status=[200, 201])
    return rid, title


def open_assessment(page, rid, title):
    page.goto(f"{BASE}/{ORG}/risks/{rid}")
    # The drawer is a plain modal div, found by its width class plus the title.
    drawer = page.locator("div.max-w-4xl").filter(has=page.get_by_role("heading", name=title))
    expect(drawer).to_be_visible(timeout=15000)
    drawer.get_by_role("button", name="Assessment", exact=True).click()
    return drawer


class TestReadingsLoadError:

    def test_server_error_shows_error_not_empty(self, page, tokens):
        rid, title = seed_risk(tokens["admin"])
        page.route(READINGS, lambda route: route.fulfill(
            status=500, content_type="application/json", body='{"message": "boom"}'))
        drawer = open_assessment(page, rid, title)
        expect(drawer.get_by_text(ERROR_TEXT)).to_be_visible(timeout=10000)
        expect(drawer.get_by_text(EMPTY_TEXT)).to_have_count(0)

    def test_network_failure_shows_error_not_empty(self, page, tokens):
        rid, title = seed_risk(tokens["admin"])
        page.route(READINGS, lambda route: route.abort())
        drawer = open_assessment(page, rid, title)
        expect(drawer.get_by_text(ERROR_TEXT)).to_be_visible(timeout=10000)
        expect(drawer.get_by_text(EMPTY_TEXT)).to_have_count(0)

    def test_retry_reloads_the_readings(self, page, tokens):
        rid, title = seed_risk(tokens["admin"])
        calls = []

        def handler(route):
            calls.append(1)
            if len(calls) == 1:
                route.fulfill(status=500, content_type="application/json", body='{"message": "boom"}')
            else:
                route.continue_()

        page.route(READINGS, handler)
        drawer = open_assessment(page, rid, title)
        expect(drawer.get_by_text(ERROR_TEXT)).to_be_visible(timeout=10000)
        drawer.get_by_role("button", name="Retry", exact=True).click()
        expect(drawer.locator("table tbody tr").first).to_be_visible(timeout=10000)
        expect(drawer.get_by_text(ERROR_TEXT)).to_have_count(0)
        expect(drawer.get_by_text(EMPTY_TEXT)).to_have_count(0)

    def test_control_real_reading_renders(self, page, tokens):
        rid, title = seed_risk(tokens["admin"])
        drawer = open_assessment(page, rid, title)
        expect(drawer.locator("table tbody tr").first).to_be_visible(timeout=10000)
        expect(drawer.get_by_text(ERROR_TEXT)).to_have_count(0)
        expect(drawer.get_by_text(EMPTY_TEXT)).to_have_count(0)

    def test_control_no_readings_shows_empty_text(self, page, tokens):
        rid, title = seed_risk(tokens["admin"], with_reading=False)
        drawer = open_assessment(page, rid, title)
        expect(drawer.get_by_text(EMPTY_TEXT)).to_be_visible(timeout=10000)
        expect(drawer.get_by_text(ERROR_TEXT)).to_have_count(0)
