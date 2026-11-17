"""E2E: the document Export dialog.

CI has no UniDoc licence key, so the server's answers are controlled in the
browser with page.route, registered before navigation: /config reports the
licensed formats, the versions list is fake (one version WITHOUT a number and
one labelled), and the export endpoint records the request and answers with a
small fake file or a 403. Everything else is the real app against the real
server.

What this pins: the button shows only when a format is licensed and sits before
Print; the dialog offers the licensed formats and the versions; a chosen
version is sent by its id (`version_id`, never the label, because a version can
have an empty label and "" means the current copy); the file is saved; a refusal
is shown as a toast and keeps the dialog open; Cancel and Esc close it without a
request; a reader can export.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_export_dialog.py -v
"""
import json
import re
import time
from urllib.parse import parse_qs, urlparse

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, R1, BASE, ORG, pw_browser, tokens  # noqa: F401

DOC = "iso27001-4-1"
DOC_URL = f"{BASE}/{ORG}/documents/{DOC}"

PDF_TYPE = "application/pdf"
DOCX_TYPE = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
DENIED = {
    "message": "PDF export is not available on this server",
    "code": "export_not_licensed",
    "params": {"value": "PDF"},
}

DAY = 86400


def versions_payload():
    """The shape of GET /documents/:id/versions: {"data": [...]}, newest first."""
    now = int(time.time())
    base = {
        "organization_id": 1, "document_id": DOC, "commit_hash": "0" * 40,
        "file_path": "documents/x.md", "content_hash": "", "message": "",
        "owner": "", "review_cycle_months": 0, "created_by": "e2e-admin@test.local",
    }
    return {"data": [
        dict(base, id=41, version="", created_at=now - DAY),
        dict(base, id=42, version="1.0", created_at=now - 2 * DAY),
    ]}


class Server:
    """What the page.route handlers saw and what they answer."""

    def __init__(self, formats, deny=False, filename="iso27001-4-1.pdf", content_type=PDF_TYPE):
        self.formats = formats
        self.deny = deny
        self.filename = filename
        self.content_type = content_type
        self.requests = []  # export request URLs

    def query(self, i=-1):
        return parse_qs(urlparse(self.requests[i]).query)

    def install(self, page):
        def config(route):
            resp = route.fetch()
            body = resp.json()
            body["export_formats"] = self.formats
            route.fulfill(response=resp, json=body)

        def versions(route):
            route.fulfill(status=200, content_type="application/json", body=json.dumps(versions_payload()))

        def export(route):
            self.requests.append(route.request.url)
            if self.deny:
                route.fulfill(status=403, content_type="application/json", body=json.dumps(DENIED))
                return
            route.fulfill(
                status=200, content_type=self.content_type,
                headers={"Content-Disposition": f'attachment; filename="{self.filename}"'},
                body=b"%PDF-1.4 fake" if self.content_type == PDF_TYPE else b"PK\x03\x04fake",
            )

        page.route("**/api/v1/config*", config)
        page.route(re.compile(r".*/api/v1/documents/[^/]+/versions(\?.*)?$"), versions)
        page.route(re.compile(r".*/api/v1/documents/[^/]+/export(\?.*)?$"), export)


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900}, accept_downloads=True)
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


@pytest.fixture
def reader_page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900}, accept_downloads=True)
    p = ctx.new_page()
    do_login(p, R1[0], R1[1])
    yield p
    ctx.close()


def open_doc(page, srv):
    srv.install(page)
    page.goto(DOC_URL)
    expect(page.get_by_role("button", name="Print document")).to_be_visible(timeout=20000)


def export_button(page):
    return page.get_by_role("button", name="Export", exact=True)


def open_dialog(page):
    export_button(page).click()
    dialog = page.locator("div.fixed.inset-0.z-50").filter(has=page.get_by_role("heading", name="Export document"))
    expect(dialog).to_be_visible(timeout=10000)
    return dialog


def download_button(dialog):
    return dialog.get_by_role("button", name="Download", exact=True)


def version_options(dialog):
    sel = dialog.locator("#export-version")
    return sel.locator("option").evaluate_all("els => els.map(e => [e.value, e.textContent.trim()])")


class TestExportDialog:

    # 1
    def test_hidden_when_nothing_is_licensed(self, page):
        open_doc(page, Server(formats=[]))
        expect(export_button(page)).to_have_count(0)
        expect(page.get_by_role("button", name="Print document")).to_be_visible()

    # 2
    def test_visible_with_formats_and_versions(self, page):
        open_doc(page, Server(formats=["pdf", "docx"]))
        expect(export_button(page)).to_be_visible(timeout=10000)
        # Export is the button immediately before Print in the toolbar.
        names = page.locator("div.sticky button").evaluate_all(
            "els => els.map(e => (e.textContent || '').trim() || e.getAttribute('title') || '')")
        assert "Export" in names and "Print document" in names, names
        assert names.index("Export") + 1 == names.index("Print document"), names

        dialog = open_dialog(page)
        radios = dialog.get_by_role("radio")
        expect(radios).to_have_count(2)
        expect(dialog.get_by_role("radio", name="PDF")).to_be_checked()
        expect(dialog.get_by_role("radio", name="DOCX")).not_to_be_checked()

        opts = version_options(dialog)
        assert [o[0] for o in opts] == ["", "41", "42"], opts
        assert opts[0][1] == "Current version"
        assert opts[1][1].startswith("No version number —"), opts[1]
        assert opts[2][1].startswith("1.0 —"), opts[2]
        assert len({o[0] for o in opts}) == len(opts), "two options share a value"

    # 3
    def test_download_current_pdf(self, page):
        srv = Server(formats=["pdf", "docx"], filename="iso27001-4-1.pdf")
        open_doc(page, srv)
        dialog = open_dialog(page)
        with page.expect_download(timeout=10000) as dl:
            download_button(dialog).click()
        assert dl.value.suggested_filename == "iso27001-4-1.pdf"
        assert len(srv.requests) == 1
        q = srv.query()
        assert q.get("format") == ["pdf"], q
        assert "version" not in q and "version_id" not in q, q
        expect(dialog).to_have_count(0)

    # 4: the regression: a version with an empty label
    def test_download_version_without_a_number_as_docx(self, page):
        srv = Server(formats=["pdf", "docx"], filename="iso27001-4-1-2026-11-17.docx", content_type=DOCX_TYPE)
        open_doc(page, srv)
        dialog = open_dialog(page)
        dialog.get_by_role("radio", name="DOCX").check()
        dialog.locator("#export-version").select_option(value="41")
        with page.expect_download(timeout=10000) as dl:
            download_button(dialog).click()
        assert dl.value.suggested_filename == "iso27001-4-1-2026-11-17.docx"
        q = srv.query()
        assert q.get("format") == ["docx"] and q.get("version_id") == ["41"], q
        assert "version" not in q, f"the label parameter must not be sent: {q}"
        expect(dialog).to_have_count(0)

    # 5
    def test_download_labelled_version(self, page):
        srv = Server(formats=["pdf"], filename="iso27001-4-1-v1.0.pdf")
        open_doc(page, srv)
        dialog = open_dialog(page)
        dialog.locator("#export-version").select_option(value="42")
        with page.expect_download(timeout=10000) as dl:
            download_button(dialog).click()
        assert dl.value.suggested_filename == "iso27001-4-1-v1.0.pdf"
        q = srv.query()
        assert q.get("version_id") == ["42"] and "version" not in q, q
        # With one licensed format only that format is offered.
        expect(dialog).to_have_count(0)

    # 6
    def test_refusal_shows_a_toast_and_keeps_the_dialog_open(self, page):
        srv = Server(formats=["pdf", "docx"], deny=True)
        open_doc(page, srv)
        downloads = []
        page.on("download", lambda d: downloads.append(d))
        dialog = open_dialog(page)
        download_button(dialog).click()
        toast = page.get_by_text("Export failed")
        expect(toast.first).to_be_visible(timeout=10000)
        expect(page.get_by_text("PDF export is not available on this server").first).to_be_visible()
        expect(dialog).to_be_visible()
        expect(download_button(dialog)).to_be_enabled()
        page.wait_for_timeout(1500)  # a download would have fired by now
        assert downloads == [], downloads
        assert len(srv.requests) == 1

    # 7
    def test_cancel_and_escape_close_without_a_request(self, page):
        srv = Server(formats=["pdf", "docx"])
        open_doc(page, srv)
        dialog = open_dialog(page)
        dialog.get_by_role("button", name="Cancel", exact=True).click()
        expect(dialog).to_have_count(0)
        dialog = open_dialog(page)
        page.keyboard.press("Escape")
        expect(dialog).to_have_count(0)
        assert srv.requests == []


# 8
class TestExportAsReader:

    def test_reader_can_export_the_current_pdf(self, reader_page):
        page = reader_page
        srv = Server(formats=["pdf"], filename="iso27001-4-1.pdf")
        open_doc(page, srv)
        expect(export_button(page)).to_be_visible(timeout=10000)
        dialog = open_dialog(page)
        with page.expect_download(timeout=10000) as dl:
            download_button(dialog).click()
        assert dl.value.suggested_filename == "iso27001-4-1.pdf"
        q = srv.query()
        assert q.get("format") == ["pdf"] and "version_id" not in q, q
