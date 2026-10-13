"""E2E (#436): audits, findings and audit programmes are reachable from search.

Covers the UI that the /search API tests cannot see: a document's Links tab
picker offering and storing audit records, Cmd/Ctrl+K opening the right drawer,
and the audit checklist picker NOT offering audit records as checklist items.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_audit_search.py -v
"""
import time
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401

# A clause the iso27001 template scaffolds into the e2e org.
DOC_ID = "iso27001-4-1"


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def seed(token, tag):
    """A programme, an audit in it and a finding on it, all titled with `tag`."""
    prog = api("post", "/audit/programmes", token,
               json={"title": f"Prog {tag}", "year": 2026}, expect_status=[200, 201]).json()
    audit = api("post", "/audits", token,
                json={"title": f"Audit {tag}", "status": "planned", "programme_id": prog["id"]},
                expect_status=[200, 201]).json()
    finding = api("post", "/audit-findings", token,
                  json={"audit_id": audit["id"], "title": f"Finding {tag}", "finding_type": "minor_nc"},
                  expect_status=[200, 201]).json()
    return prog, audit, finding


def drawer_for(page, title):
    # The drawer is a plain modal div with no role or test id, so it is found by
    # its width class (programme and finding max-w-4xl, audit max-w-5xl) plus the
    # record's title heading.
    return page.locator("div.max-w-4xl, div.max-w-5xl").filter(has=page.get_by_role("heading", name=title))


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


class TestAuditSearchUI:

    def test_document_links_tab_finds_and_links_audit_records(self, page, tokens):
        t = tokens["admin"]
        tag = uuid.uuid4().hex[:8]
        prog, audit, finding = seed(t, tag)

        page.goto(f"{BASE}/{ORG}/documents/{DOC_ID}")
        page.get_by_role("button", name="Links", exact=True).click()
        for typ, ident, title in [
            ("audit", f"AUDIT-{audit['id']}", f"Audit {tag}"),
            ("audit_finding", f"FIND-{finding['id']}", f"Finding {tag}"),
            ("audit_programme", f"AP-{prog['id']}", f"Prog {tag}"),
        ]:
            page.get_by_role("button", name="+ Add link").click()
            page.get_by_placeholder("Search documents, risks, incidents...").fill(title)
            row = page.get_by_role("button", name=title)
            expect(row).to_be_visible(timeout=8000)
            row.click()
            # Wait for the chip and the closed picker before the next round: the
            # picker closes only after the POST resolves, and a "+ Add link" click
            # before that toggles it shut. The dropdown also hides itself 200ms after
            # the input blurs, which would otherwise hide the next round's results.
            expect(page.get_by_role("link", name=title)).to_be_visible(timeout=8000)
            expect(page.get_by_placeholder("Search documents, risks, incidents...")).to_have_count(0)
            page.wait_for_timeout(400)
            rows = wait_links(t, typ, ident, 1)
            assert any(DOC_ID in (r.get("target_id"), r.get("source_id")) for r in rows), rows
            assert any(r["target_type"] == typ and r["target_id"] == ident for r in rows) or \
                any(r["source_type"] == typ and r["source_id"] == ident for r in rows), rows

    @pytest.mark.parametrize("kind", ["programmes", "findings", "audits"])
    def test_global_search_opens_record(self, page, tokens, kind):
        t = tokens["admin"]
        tag = uuid.uuid4().hex[:8]
        prog, audit, finding = seed(t, tag)
        rec, title, ref = {
            "programmes": (prog, f"Prog {tag}", f"AP-{prog['id']}"),
            "findings": (finding, f"Finding {tag}", f"FIND-{finding['id']}"),
            "audits": (audit, f"Audit {tag}", f"AUDIT-{audit['id']}"),
        }[kind]

        page.goto(f"{BASE}/{ORG}/documents/{DOC_ID}")
        expect(page.get_by_role("button", name="Links", exact=True)).to_be_visible(timeout=15000)
        page.keyboard.press("Control+k")
        box = page.get_by_placeholder("Search documents, risks, suppliers, incidents...")
        expect(box).to_be_visible(timeout=8000)
        box.fill(title)
        expect(page.get_by_role("button", name=title)).to_be_visible(timeout=8000)
        page.keyboard.press("Enter")

        expect(page).to_have_url(f"{BASE}/{ORG}/audit/{kind}/{rec['id']}", timeout=8000)
        drawer = drawer_for(page, title)
        expect(drawer).to_be_visible(timeout=15000)
        expect(drawer.get_by_text(ref, exact=True)).to_be_visible(timeout=8000)

    def test_checklist_picker_excludes_audit_records(self, page, tokens):
        t = tokens["admin"]
        tag = uuid.uuid4().hex[:8]
        prog, audit, finding = seed(t, tag)
        api("post", "/risks", t, json={
            "title": f"Risk {tag}", "description": "d",
            "risk_type": "threat", "origin": "external", "status": "open",
            "current_likelihood": 2, "current_impact": 2,
        }, expect_status=[200, 201])

        page.goto(f"{BASE}/{ORG}/audit/audits/{audit['id']}")
        drawer = drawer_for(page, f"Audit {tag}")
        expect(drawer).to_be_visible(timeout=15000)
        drawer.get_by_role("button", name="Items").click()
        drawer.get_by_role("button", name="+ Add Item").click()
        drawer.get_by_placeholder("Search documents, controls, clauses, risks, assets...").fill(tag)

        expect(drawer.get_by_role("button", name=f"Risk {tag}")).to_be_visible(timeout=8000)
        # Search has settled once the risk row is there; none of the audit records may be offered.
        for title in (f"Prog {tag}", f"Audit {tag}", f"Finding {tag}"):
            expect(drawer.get_by_role("button", name=title)).to_have_count(0)
        for label in ("AUDIT", "FINDING", "PROGRAMME"):
            expect(drawer.locator("div.max-h-56 span.uppercase", has_text=label)).to_have_count(0)
