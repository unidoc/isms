"""E2E (#433): nothing is created until "Create Document" is clicked.

The New Document dialog is a <form>, and a typeless <button> inside a form
submits it. The "Show advanced options" toggle was typeless, so clicking it after
typing a title created the document and navigated away. The folder "change" link
and the member picker's option and clear buttons had the same missing type, but
they unmount inside their own click handler, so the submit never fired: they were
latent, not live. The author and folder cases here are therefore regression
guards; the node test (web/test/newDocumentDialogButtons.test.js) is what pins
those buttons' types. These cases open the dialog the way a user does and check,
through the API, that nothing was created. Only a click (or Enter on the
focused button) on Create creates a document: the form has no submit button, so
Enter in a field does nothing.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_new_document_dialog.py -v
"""
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, R1, BASE, ORG, pw_browser, tokens  # noqa: F401

AUTHOR = R1  # a member who is not the logged-in admin, so the pick is visible


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def open_dialog(page):
    """Documents page, the + next to the header, then New Document."""
    page.goto(f"{BASE}/{ORG}/documents")
    page.locator('button[title="New"]').click()
    page.get_by_role("button", name="New Document", exact=True).click()
    dialog = page.locator("form").filter(has=page.get_by_role("heading", name="New Document"))
    expect(dialog).to_be_visible(timeout=8000)
    return dialog


def new_title():
    # Unique per test: the slug is the document id and the e2e org is reused.
    title = f"E2E dialog {uuid.uuid4().hex[:10]}"
    return title, title.lower().replace(" ", "-")


def body_status(token, slug):
    return api("get", f"/documents/{slug}/body", token).status_code


def assert_still_open(page, dialog, url_before, token, slug):
    # Give a (wrongly) triggered create time to finish: it closes the dialog
    # and navigates, so wait on the absence of both rather than sleeping.
    page.wait_for_timeout(1000)
    assert page.url == url_before, f"navigated away to {page.url}"
    expect(dialog).to_be_visible()
    assert body_status(token, slug) == 404, "document was created early"


def pick_author(dialog):
    dialog.get_by_text("Select author...").or_(dialog.get_by_text(ADMIN[2])).first.click()
    dialog.get_by_role("button", name=AUTHOR[2]).click()
    expect(dialog.get_by_text(AUTHOR[2])).to_be_visible()


class TestNewDocumentDialog:

    def test_advanced_toggle_does_not_create(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        url = page.url
        dialog.get_by_role("button", name="Show advanced options").click()
        expect(dialog.get_by_role("button", name="Hide advanced options")).to_be_visible()
        assert_still_open(page, dialog, url, t, slug)

    def test_author_pick_does_not_create(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        url = page.url
        dialog.get_by_role("button", name="Show advanced options").click()
        pick_author(dialog)
        assert_still_open(page, dialog, url, t, slug)

    def test_enter_in_author_search_does_not_create(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        dialog.get_by_role("button", name="Show advanced options").click()
        url = page.url
        dialog.get_by_text(ADMIN[2]).first.click()
        search = dialog.get_by_placeholder("Search...")
        search.fill("Reader")
        search.press("Enter")
        assert_still_open(page, dialog, url, t, slug)
        # Enter does not pick either: the author is still the logged-in admin.
        expect(dialog.get_by_text(ADMIN[2])).to_be_visible()

    def test_folder_change_does_not_create(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        url = page.url
        dialog.get_by_role("button", name="change").click()
        expect(dialog.get_by_placeholder("Type or select folder")).to_be_visible()
        assert_still_open(page, dialog, url, t, slug)

    def test_create_button_creates_with_picked_author(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        dialog.get_by_role("button", name="Show advanced options").click()
        pick_author(dialog)
        dialog.get_by_role("button", name="Create Document").click()
        try:
            expect(dialog).to_be_hidden(timeout=10000)
            r = api("get", f"/documents/{slug}/body", t, expect_status=200)
            assert AUTHOR[0] in r.text, f"author is not the picked member: {r.text[:300]}"
        finally:
            api("delete", f"/documents/{slug}", t)

    def test_cancel_creates_nothing(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        dialog.get_by_role("button", name="Cancel").click()
        expect(dialog).to_be_hidden()
        assert body_status(t, slug) == 404

    def test_enter_in_title_does_not_create(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        field = dialog.get_by_placeholder("e.g. Data Classification Policy")
        field.fill(title)
        url = page.url
        field.press("Enter")
        assert_still_open(page, dialog, url, t, slug)

    def test_enter_in_folder_input_does_not_create(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        url = page.url
        dialog.get_by_role("button", name="change").click()
        folder = dialog.get_by_placeholder("Type or select folder")
        folder.focus()
        folder.press("Enter")
        assert_still_open(page, dialog, url, t, slug)

    def test_enter_on_focused_create_creates(self, page, tokens):
        t = tokens["admin"]
        title, slug = new_title()
        dialog = open_dialog(page)
        dialog.get_by_placeholder("e.g. Data Classification Policy").fill(title)
        create = dialog.get_by_role("button", name="Create Document")
        create.focus()
        create.press("Enter")
        try:
            expect(dialog).to_be_hidden(timeout=10000)
            api("get", f"/documents/{slug}/body", t, expect_status=200)
        finally:
            api("delete", f"/documents/{slug}", t)
