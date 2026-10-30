"""E2E (#308): numeric settings save from Admin > Settings.

The generic number input binds with v-model on type="number", which hands Vue a
Number, and saveSetting sent it as a JSON number. The settings PUT binds a string,
so every numeric setting (review cycles, ai_review_max_rounds, risk_appetite...)
failed with 400 "invalid request" and kept its old value. Only visible on the
real page, so this drives it.

The org is shared across the suite, so every key the test changes is restored
to its previous state (an org override is removed by saving an empty field).

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_settings_numeric_save.py -v
"""
import time

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401

SYSTEM_KEY = "system_review_cycle_critical"
SYSTEM_LABEL = "Critical System Review Cycle"
SUPPLIER_KEY = "supplier_review_cycle_high"
SUPPLIER_LABEL = "High Supplier Review Cycle"


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def stored(token, key):
    data = api("get", "/admin/settings", token, expect_status=200).json()
    rows = data if isinstance(data, list) else data.get("data", [])
    row = next(r for r in rows if r["key"] == key)
    return row


def wait_stored(token, key, want, timeout=8.0):
    """Poll: the save is an async PUT, so the UI click returns before it lands."""
    deadline = time.time() + timeout
    got = None
    while time.time() < deadline:
        got = stored(token, key)["value"]
        if got == want:
            return
        time.sleep(0.25)
    raise AssertionError(f"{key} is {got!r}, want {want!r}")


def open_settings(page):
    page.goto(f"{BASE}/{ORG}/admin/settings")
    page.get_by_text("Review Cycles").first.wait_for(state="visible", timeout=10000)


def setting_row(page, label):
    # The row is the div holding the label and the control; the Save button sits in it.
    return page.locator("div.px-5.py-4").filter(has=page.get_by_text(label, exact=True))


def save_in_ui(page, label, value):
    row = setting_row(page, label)
    expect(row).to_have_count(1)
    row.locator("input[type=number]").fill(value)
    row.get_by_role("button", name="Save", exact=True).click()
    return row


@pytest.fixture
def restore(tokens):
    """Remember each key's org override and put it back after the test."""
    t = tokens["admin"]
    touched = {}

    def watch(key):
        touched[key] = stored(t, key)["value"]
    yield watch
    for key, before in touched.items():
        # Restoring the registry default is the same as no override for these keys.
        api("put", "/admin/settings", t, json={"key": key, "value": ""}, expect_status=200)
        if stored(t, key)["value"] != before:
            api("put", "/admin/settings", t, json={"key": key, "value": str(before)}, expect_status=200)


class TestNumericSettingSave:

    def test_system_and_supplier_cycles_persist(self, page, tokens, restore):
        t = tokens["admin"]
        restore(SYSTEM_KEY)
        restore(SUPPLIER_KEY)
        open_settings(page)

        save_in_ui(page, SYSTEM_LABEL, "4")
        wait_stored(t, SYSTEM_KEY, "4")
        expect(page.get_by_text("invalid request")).to_have_count(0)

        save_in_ui(page, SUPPLIER_LABEL, "5")
        wait_stored(t, SUPPLIER_KEY, "5")
        expect(page.get_by_text("invalid request")).to_have_count(0)

    def test_clearing_reverts_to_the_registry_default(self, page, tokens, restore):
        t = tokens["admin"]
        restore(SYSTEM_KEY)
        default = "1"  # registry default for critical
        api("put", "/admin/settings", t, json={"key": SYSTEM_KEY, "value": "7"}, expect_status=200)
        assert stored(t, SYSTEM_KEY)["value"] == "7"

        open_settings(page)
        save_in_ui(page, SYSTEM_LABEL, "")
        wait_stored(t, SYSTEM_KEY, default)
