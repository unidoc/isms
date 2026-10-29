"""E2E (#316): external links on a supplier's Links tab.

The API (validation, roles, org isolation, history) is covered by the Go tests;
this file covers what only a browser shows: the section on the supplier drawer,
the add / verify / edit / remove flow for a manager, the anchor's attributes,
and that a reader gets the list with no controls.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_supplier_external_links.py -v
"""
import datetime
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, R1, BASE, ORG, pw_browser, tokens  # noqa: F401


@pytest.fixture
def admin_page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


@pytest.fixture
def reader_page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, R1[0], R1[1])
    yield p
    ctx.close()


def seed_supplier(token):
    """A supplier with a name unique to this run; the e2e org is reused."""
    name = f"E2E supplier {uuid.uuid4().hex[:8]}"
    r = api("post", "/suppliers", token,
            json={"name": name, "supplier_type": "saas", "criticality": "medium"},
            expect_status=[200, 201])
    return r.json()["identifier"], name


def open_links_tab(page, ident, name):
    page.goto(f"{BASE}/{ORG}/suppliers/{ident}")
    # The drawer is a plain modal div with no role or test id, so it is found by
    # its width class plus the supplier's name heading.
    drawer = page.locator("div.max-w-4xl").filter(has=page.get_by_role("heading", name=name))
    expect(drawer).to_be_visible(timeout=15000)
    page.wait_for_timeout(2500)  # the drawer animates in
    drawer.get_by_role("button", name="Links", exact=True).click()
    section = drawer.locator('[data-testid="external-links"]')
    expect(section).to_be_visible(timeout=8000)
    return drawer, section


def list_links(token, ident):
    return api("get", f"/external-links?type=supplier&id={ident}", token, expect_status=200).json()["data"]


def day(epoch):
    d = datetime.datetime.fromtimestamp(epoch, datetime.timezone.utc)
    return f"{d.day} {d.strftime('%b %Y')}"


class TestSupplierExternalLinks:

    def test_manager_adds_verifies_edits_removes(self, admin_page, tokens):
        t = tokens["admin"]
        ident, name = seed_supplier(t)
        page = admin_page
        label = f"SOC 2 {uuid.uuid4().hex[:6]}"
        url = "https://trust.example.com/soc2.pdf"

        _, section = open_links_tab(page, ident, name)
        expect(section.get_by_text("No external links")).to_be_visible()

        # Client-side validation: an unusable URL never reaches the server.
        section.get_by_role("button", name="Add", exact=True).click()
        section.get_by_label("URL").fill("javascript:alert(1)")
        section.get_by_label("Label").fill(label)
        section.get_by_label("Type").select_option("audit_report")
        section.get_by_role("button", name="Save", exact=True).click()
        expect(section.get_by_text("Enter a full http:// or https:// address")).to_be_visible()
        assert list_links(t, ident) == []

        # Add, with a valid-until date in the past.
        section.get_by_label("URL").fill(url)
        section.get_by_label("Valid until").fill("2020-01-15")
        section.get_by_role("button", name="Save", exact=True).click()

        row = section.locator('[data-testid="external-link-row"]').filter(has_text=label)
        expect(row).to_be_visible(timeout=8000)
        expect(row).to_contain_text("Expired")
        expect(row).to_contain_text("Audit report")
        expect(row).to_contain_text("15 Jan 2020")
        anchor = row.get_by_role("link", name=label)
        expect(anchor).to_have_attribute("href", url)
        expect(anchor).to_have_attribute("target", "_blank")
        expect(anchor).to_have_attribute("rel", "noopener noreferrer")
        stored = list_links(t, ident)
        assert len(stored) == 1 and stored[0]["url"] == url and stored[0]["state"] == "expired", stored

        # Mark verified today: the stored date, shown as that calendar day.
        row.get_by_role("button", name="Mark verified today").click()
        expect(row).to_contain_text("Verified", timeout=8000)
        verified = list_links(t, ident)[0]
        assert verified["verified_at"] and verified["verified_by"] == ADMIN[0], verified
        expect(row).to_contain_text(day(verified["verified_at"]))

        # Edit the label; the dates survive the full-replace PUT.
        row.get_by_role("button", name="Edit", exact=True).click()
        new_label = label + " renamed"
        section.get_by_label("Label").fill(new_label)
        section.get_by_role("button", name="Save", exact=True).click()
        renamed = section.locator('[data-testid="external-link-row"]').filter(has_text=new_label)
        expect(renamed).to_be_visible(timeout=8000)
        after = list_links(t, ident)[0]
        assert after["label"] == new_label and after["valid_until"] == verified["valid_until"] \
            and after["verified_at"] == verified["verified_at"], after

        # Remove, through the app's confirm dialog.
        renamed.get_by_role("button", name="Delete", exact=True).click()
        dialog = page.locator("div.fixed.inset-0").filter(has_text="Remove this external link?")
        expect(dialog).to_be_visible()
        dialog.get_by_role("button", name="Delete", exact=True).click()
        expect(section.get_by_text("No external links")).to_be_visible(timeout=8000)
        assert list_links(t, ident) == []

    def test_reader_sees_links_without_controls(self, reader_page, tokens):
        t = tokens["admin"]
        ident, name = seed_supplier(t)
        label = f"ISO cert {uuid.uuid4().hex[:6]}"
        api("post", "/external-links", t, json={
            "entity_type": "supplier", "entity_id": ident, "url": "https://certs.example.com/iso.pdf",
            "label": label, "link_type": "certificate", "valid_until": "2020-01-15",
        }, expect_status=201)

        _, section = open_links_tab(reader_page, ident, name)
        row = section.locator('[data-testid="external-link-row"]').filter(has_text=label)
        expect(row).to_be_visible(timeout=8000)
        expect(row).to_contain_text("Expired")
        expect(row.get_by_role("link", name=label)).to_have_attribute("rel", "noopener noreferrer")
        for control in ("Add", "Edit", "Mark verified today", "Delete"):
            expect(section.get_by_role("button", name=control)).to_have_count(0)
