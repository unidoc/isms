"""E2E (#460): the document Edit button waits for the content to load.

Edit copies the loaded body into the editor once, at click time. Clicking it
while the content request was still in flight opened an empty editor that stayed
empty, and Save then replaced the document body with nothing. After a failed load
the error message sat where the body should be, and Edit would have saved that.

The delay is applied inside the page (a wrapped window.fetch), because a
page.route handler that sleeps blocks the whole sync Playwright driver and the
click could never happen mid-load. The failure uses page.route, which fulfils
instantly. The control edits and saves a normally loaded document.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_edit_before_load.py -v
"""
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401

PARAGRAPH = "Marker paragraph that must survive editing."
CONTENT = f"# Edit gate\n\n{PARAGRAPH}\n"
DELAY_MS = 3000

# Holds back every document-content response, as a slow server would.
DELAY_FETCH = """
(() => {
  const real = window.fetch.bind(window)
  window.fetch = (input, init) => {
    const url = typeof input === 'string' ? input : input.url
    const p = real(input, init)
    if (!url.includes('/documents/file/')) return p
    return new Promise((resolve, reject) => setTimeout(() => p.then(resolve, reject), %d))
  }
})()
""" % DELAY_MS


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def seed_doc(token):
    doc = f"e2e-edit-gate-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", token, json={
        "folder": "iso27001", "filename": f"{doc}.md", "document_id": doc,
        "title": f"E2E edit gate {doc[-8:]}", "content": CONTENT,
    }, expect_status=[200, 201])
    return doc


def edit_button(page):
    return page.get_by_role("button", name="Edit", exact=True).first


class TestEditBeforeLoad:

    def test_edit_is_disabled_until_the_content_loads(self, page, tokens):
        doc = seed_doc(tokens["admin"])
        page.add_init_script(DELAY_FETCH)
        page.goto(f"{BASE}/{ORG}/documents/{doc}")
        btn = edit_button(page)
        btn.wait_for(state="visible", timeout=15000)
        expect(btn).to_be_disabled()
        expect(page.locator(".doc-prose").get_by_text(PARAGRAPH)).to_be_visible(timeout=15000)
        expect(btn).to_be_enabled()
        btn.click()
        expect(page.locator(".ProseMirror").get_by_text(PARAGRAPH)).to_be_visible(timeout=10000)

    def test_edit_is_disabled_after_a_failed_load(self, page, tokens):
        doc = seed_doc(tokens["admin"])
        page.route("**/api/v1/documents/file/**", lambda route: route.fulfill(
            status=500, content_type="application/json", body='{"message": "boom"}'))
        page.goto(f"{BASE}/{ORG}/documents/{doc}")
        btn = edit_button(page)
        btn.wait_for(state="visible", timeout=15000)
        expect(page.get_by_text("boom").first).to_be_visible(timeout=10000)
        expect(btn).to_be_disabled()

    def test_control_edit_and_save_keeps_the_body(self, page, tokens):
        doc = seed_doc(tokens["admin"])
        page.goto(f"{BASE}/{ORG}/documents/{doc}")
        expect(page.locator(".doc-prose").get_by_text(PARAGRAPH)).to_be_visible(timeout=15000)
        expect(edit_button(page)).to_be_enabled()
        edit_button(page).click()
        editor = page.locator(".ProseMirror")
        expect(editor.get_by_text(PARAGRAPH)).to_be_visible(timeout=10000)
        editor.get_by_text(PARAGRAPH).click()
        page.keyboard.press("End")
        page.keyboard.type("!")
        page.get_by_role("button", name="Save", exact=True).first.click()
        expect(edit_button(page)).to_be_visible(timeout=15000)
        body = api("get", f"/documents/{doc}/body", tokens["admin"], expect_status=200).text
        assert PARAGRAPH in body, body
