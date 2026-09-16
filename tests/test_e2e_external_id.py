"""E2E (#39): the optional external_id is wired up in all seven register views.

The API round-trip is covered by each register's own pytest suite; what this
file covers is the UI: the input in the create form, the input in the overview
editor, the inline display beside the canonical identifier (rendered only when
non-empty), and the folding of the value into the register's ?q= search.

All seven registers share the same markup for those four pieces, so the suite is
parametrized over a register table rather than repeated seven times.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_external_id.py -v
"""
import uuid
import time

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, ORG, pw_browser, tokens  # noqa: F401

# Detail-modal header. The canonical identifier renders font-mono text-slate-600
# and the external id — when set — font-mono text-slate-500, in this one header.
HEADER = "div.border-b.border-slate-800.px-6.py-3"
EXT_IN_HEADER = f"{HEADER} span.font-mono.text-slate-500"
EXT_INPUT = 'label:has-text("External ID") + input'

# ── Register table ──
#
# key, SPA route, API path, "add" button label, the title/name placeholder in the
# create form, the title field's name, and the extra fields the API demands when
# a record is seeded directly (mirrors each register's own pytest suite).
REGISTERS = [
    ("incidents", "incidents", "/incidents", "Add Incident",
     "Brief incident title", "title",
     {"description": "e2e external id", "severity": "low", "affects_c": True,
      "incident_type": "event", "source": "internal", "reporter": ADMIN[0]}),
    ("risks", "risks", "/risks", "Add Risk",
     "e.g. Ransomware attack on production systems", "title", {}),
    ("assets", "assets", "/assets", "Add Asset",
     "e.g. Production Database, Customer Portal", "name",
     {"asset_type": "other", "status": "open"}),
    ("suppliers", "suppliers", "/suppliers", "Add Supplier",
     "e.g. AWS, Office 365", "name",
     {"supplier_type": "saas", "criticality": "medium"}),
    ("systems", "systems", "/systems", "Add System",
     "e.g. Production ERP", "name",
     {"classification": "internal", "criticality": "low"}),
    ("legal", "legal", "/legal", "Add Legal Requirement",
     "e.g. GDPR, NIS2 Directive", "title",
     {"jurisdiction": "EU", "category": "privacy"}),
    ("corrective_actions", "corrective-actions", "/corrective-actions", "Add Corrective Action",
     "Brief title of the nonconformity or improvement", "title",
     {"source": "feedback", "severity": "observation"}),
]
REG_IDS = [r[0] for r in REGISTERS]


def tag(prefix):
    """A value unique to this run — the e2e org is reused across runs, so a
    fixed string would also match records left behind by an earlier one.

    Titles and external ids get *independent* tags on purpose: a title that
    contained the external id would match the ?q= search through `title ILIKE`
    and the search assertions would pass with the external_id folding removed.
    """
    return f"{prefix}-{uuid.uuid4().hex[:8].upper()}"


def goto(page, path):
    """SPA navigation via Vue Router — a full reload can lose auth."""
    page.evaluate(
        "() => document.querySelector('#app').__vue_app__.config.globalProperties"
        f".$router.push('/{ORG}/{path}')")
    page.wait_for_load_state("networkidle")


def search_list(page, term):
    """Type into the register's list search box. Every register debounces the
    ?q= refetch by 250ms, so callers must assert with a retrying expect()."""
    box = page.get_by_placeholder("Search...").first
    box.wait_for(state="visible", timeout=8000)
    box.fill(term)
    page.wait_for_load_state("networkidle")


def wait_external_id(reg_path, rid, token, want, timeout=15.0):
    """Poll the API until the record's external_id settles on `want`.

    A save clicked in the browser fires an async PUT; asserting on rendered copy
    alone can pass while the request is still in flight (and closing the browser
    context then cancels it server-side). Gate on the real state first.
    """
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        last = api("get", f"{reg_path}/{rid}", token, expect_status=200).json().get("external_id") or ""
        if last == want:
            return
        time.sleep(0.25)
    raise AssertionError(f"{reg_path}/{rid} external_id never became {want!r} (last: {last!r})")


def wait_for_record(reg_path, term, token, timeout=15.0):
    """Poll ?q= until a record matches, and return the matching rows.

    The page is already at networkidle when the create button is clicked, so
    there is no load state to wait on and an immediate API read can beat the
    POST to the server.
    """
    deadline = time.time() + timeout
    while time.time() < deadline:
        body = api("get", f"{reg_path}?q={term}", token, expect_status=200).json()
        rows = body.get("data") if isinstance(body, dict) else body
        if rows:
            return rows
        time.sleep(0.25)
    raise AssertionError(f"no record matched {reg_path}?q={term} within {timeout}s")


def seed(reg_path, title_field, extra, token, title, external_id=None):
    body = dict(extra)
    body[title_field] = title
    if external_id is not None:
        body["external_id"] = external_id
    return api("post", reg_path, token, json=body, expect_status=[200, 201]).json()


def open_detail(page, route, rid):
    """Deep-link to the record — every register view opens its detail modal on
    the /:id route."""
    goto(page, f"{route}/{rid}")
    page.locator(HEADER).first.wait_for(state="visible", timeout=10000)


def edit_external_id(page, value):
    """Open the overview editor, set External ID, save."""
    page.get_by_role("button", name="Edit", exact=True).first.click()
    field = page.locator(EXT_INPUT).first
    field.wait_for(state="visible", timeout=8000)
    field.fill(value)
    page.get_by_role("button", name="Save", exact=True).first.click()


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


@pytest.mark.parametrize("key,route,reg_path,add_label,placeholder,title_field,extra",
                         REGISTERS, ids=REG_IDS)
class TestRegisterExternalID:
    """UI coverage of the optional external_id on all seven registers (#39)."""

    def test_create_through_the_form_persists_and_is_searchable(
            self, page, tokens, key, route, reg_path, add_label, placeholder, title_field, extra):
        """Create a record with an external id through the UI, then find it
        again by searching the list for that value.

        The title deliberately shares nothing with the external id, so the
        search can only match through the external_id clause.
        """
        token = tokens["admin"]
        ext = tag("EXT")
        title = f"E2E create {uuid.uuid4().hex[:8]}"
        goto(page, route)
        page.get_by_role("button", name=add_label, exact=True).first.click()
        page.get_by_placeholder(placeholder).fill(title)
        page.locator(EXT_INPUT).first.fill(ext)
        page.get_by_role("button", name="Add", exact=True).click()
        page.wait_for_load_state("networkidle")

        # Persisted server-side with exactly what was typed.
        rows = wait_for_record(reg_path, ext, token)
        assert rows[0].get("external_id") == ext, f"{key}: {rows[0]!r}"

        # Searching the list for the external id surfaces the row, with the
        # value rendered inline beside the canonical identifier.
        search_list(page, ext)
        expect(page.get_by_text(title, exact=True).first).to_be_visible(timeout=10000)
        expect(page.get_by_text(ext, exact=True).first).to_be_visible(timeout=10000)

    def test_edit_then_clear_through_the_overview_form(
            self, page, tokens, key, route, reg_path, add_label, placeholder, title_field, extra):
        """Change the external id from the overview editor, then clear it to
        empty and confirm the inline display disappears."""
        token = tokens["admin"]
        ext = tag("EXT")
        rec = seed(reg_path, title_field, extra, token, f"E2E edit {uuid.uuid4().hex[:8]}", ext)
        rid = rec["id"]

        # Edit it to a new value.
        changed = tag("EXT")
        open_detail(page, route, rid)
        expect(page.locator(EXT_IN_HEADER)).to_have_text(ext)
        edit_external_id(page, changed)
        wait_external_id(reg_path, rid, token, changed)
        expect(page.locator(EXT_IN_HEADER)).to_have_text(changed, timeout=10000)

        # Clear it. This is the case a bug was already caught on at the API
        # layer; prove the UI path all the way through.
        edit_external_id(page, "")
        wait_external_id(reg_path, rid, token, "")
        expect(page.locator(EXT_IN_HEADER)).to_have_count(0, timeout=10000)

    def test_record_without_external_id_renders_nothing(
            self, page, tokens, key, route, reg_path, add_label, placeholder, title_field, extra):
        """A record that never had an external id renders no stray label or
        empty element — neither in the list row nor in the detail header."""
        token = tokens["admin"]
        title = f"E2E bare {uuid.uuid4().hex[:8]}"
        rec = seed(reg_path, title_field, extra, token, title)
        assert not rec.get("external_id")

        # List: filter down to this one record, then count the inline elements.
        goto(page, route)
        search_list(page, title)
        expect(page.get_by_text(title, exact=True).first).to_be_visible(timeout=10000)
        expect(page.locator(".font-mono.text-slate-500")).to_have_count(0)

        # Detail header.
        open_detail(page, route, rec["id"])
        expect(page.locator(EXT_IN_HEADER)).to_have_count(0)
