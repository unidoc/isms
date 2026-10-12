"""E2E (#394): an audit programme can be linked, and its drawer works end to end.

The reference API and the entity-comments API are covered by test_references.py;
what this file covers is the UI that API tests cannot see: the Links tab on the
programme drawer, the AP-<id> display id (instead of PROG-<id>), the reverse
chip on a document's Links tab and its navigation back to the programme drawer,
and the drawer's Discussion tab saving a comment.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_audit_programme_links.py -v
"""
import time
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401

# A clause the iso27001 template scaffolds into the e2e org.
DOC_ID = "iso27001-4-1"
DOC_TITLE = "4.1 External and Internal Issues"


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def seed_programme(token):
    """A programme with a title unique to this run; the e2e org is reused."""
    title = f"E2E programme {uuid.uuid4().hex[:8]}"
    r = api("post", "/audit/programmes", token, json={"title": title, "year": 2026}, expect_status=[200, 201])
    return r.json()["id"], title


def drawer_for(page, title):
    # The drawer is a plain modal div with no role or test id, so it is found by
    # its width class plus the programme's title heading.
    return page.locator("div.max-w-4xl").filter(has=page.get_by_role("heading", name=title))


def open_programme(page, pid, title):
    page.goto(f"{BASE}/{ORG}/audit/programmes/{pid}")
    drawer = drawer_for(page, title)
    # The drawer animates in; wait for something inside it before using its tabs.
    expect(drawer).to_be_visible(timeout=15000)
    return drawer


def wait_links(token, typ, ident, want, timeout=15.0):
    """Poll GET /references until `want` rows come back (the UI's POST is async)."""
    deadline = time.time() + timeout
    rows = []
    while time.time() < deadline:
        rows = api("get", f"/references?type={typ}&id={ident}", token, expect_status=200).json()["data"] or []
        if len(rows) >= want:
            return rows
        time.sleep(0.25)
    raise AssertionError(f"{typ} {ident} never reached {want} links (last: {rows!r})")


class TestAuditProgrammeLinks:

    def test_link_document_from_programme_links_tab(self, page, tokens):
        t = tokens["admin"]
        pid, title = seed_programme(t)
        ap = f"AP-{pid}"

        drawer = open_programme(page, pid, title)
        # The display id is AP-<id>; the Programs register's PROG- label is gone.
        expect(drawer.get_by_text(ap, exact=True)).to_be_visible(timeout=8000)
        expect(page.get_by_text(f"PROG-{pid}", exact=True)).to_have_count(0)

        drawer.get_by_role("button", name="Links", exact=True).click()
        drawer.get_by_role("button", name="+ Add link").click()
        drawer.get_by_placeholder("Search documents, risks, incidents...").fill(DOC_TITLE)
        page.get_by_role("button", name=DOC_TITLE).first.click()

        expect(drawer.get_by_role("link", name=DOC_TITLE)).to_be_visible(timeout=8000)
        rows = wait_links(t, "audit_programme", ap, 1)
        mine = [r for r in rows
                if DOC_ID in (r.get("target_id"), r.get("source_id"))]
        assert len(mine) == 1, rows
        r = mine[0]
        assert ap in (r["target_id"], r["source_id"]), r
        assert r["title"] == DOC_TITLE, r

    def test_document_links_tab_navigates_to_programme(self, page, tokens):
        t = tokens["admin"]
        pid, title = seed_programme(t)
        ap = f"AP-{pid}"
        # The bare row id on purpose: it must be stored, and shown, as AP-<id>.
        api("post", "/references", t, json={
            "source_type": "document", "source_id": DOC_ID,
            "target_type": "audit_programme", "target_id": str(pid),
        }, expect_status=201)

        page.goto(f"{BASE}/{ORG}/documents/{DOC_ID}")
        page.get_by_role("button", name="Links", exact=True).click()
        chip = page.get_by_role("link", name=title)
        expect(chip).to_be_visible(timeout=15000)
        expect(chip).to_contain_text("PROGRAMME")
        chip.click()

        expect(page).to_have_url(f"{BASE}/{ORG}/audit/programmes/{pid}", timeout=8000)
        drawer = drawer_for(page, title)
        expect(drawer).to_be_visible(timeout=15000)
        expect(drawer.get_by_text(ap, exact=True)).to_be_visible(timeout=8000)

    def test_programme_discussion_saves_comment(self, page, tokens):
        t = tokens["admin"]
        pid, title = seed_programme(t)
        text = f"programme comment {uuid.uuid4().hex[:8]}"

        drawer = open_programme(page, pid, title)
        drawer.get_by_role("button", name="Discussion").click()
        drawer.get_by_placeholder("Add a comment...").fill(text)
        drawer.get_by_role("button", name="Post", exact=True).click()

        expect(drawer.get_by_text(text)).to_be_visible(timeout=8000)
        stored = api("get", f"/entity-comments/audit_programme/{pid}", t, expect_status=200).json()
        rows = stored.get("data") if isinstance(stored, dict) else stored
        assert any(c["body"] == text for c in rows or []), stored
