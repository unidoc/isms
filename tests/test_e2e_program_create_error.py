"""E2E (#367): a failed Add Program stays in its dialog.

Creating a program whose key is retired (a deleted program that had objectives)
or already held by a live program is refused with a 409. The Programs view used
to answer by replacing the whole page with the red load-error panel, which lost
the dialog, the programs list and what the user had typed. These tests drive the
real page and check the message lands inside the open dialog.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_program_create_error.py -v
"""
import time
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def unique_key(prefix):
    # Keys are upper-case letters and digits only, and the e2e org is reused.
    return prefix + uuid.uuid4().hex[:6].upper()


def seed(token):
    """A live program plus a retired key (deleted program that had an objective)."""
    live = unique_key("LIVE")
    retired = unique_key("RET")
    api("post", "/programs", token, json={"key": live, "title": f"Live {live}"}, expect_status=[200, 201])
    r = api("post", "/programs", token, json={"key": retired, "title": f"Retired {retired}"}, expect_status=[200, 201])
    pid = r.json()["id"]
    api("post", "/objectives", token, json={"program_id": pid, "title": "keeps the key retired"}, expect_status=[200, 201])
    api("delete", f"/programs/{pid}", token, expect_status=200)
    return live, retired


def live_keys(token):
    rows = api("get", "/programs", token, expect_status=200).json()
    rows = rows.get("data", rows) if isinstance(rows, dict) else rows
    return {p["key"] for p in rows}


def open_dialog(page):
    page.goto(f"{BASE}/{ORG}/programs")
    page.get_by_role("button", name="Add Program").click()
    dialog = page.locator("div.max-w-2xl").filter(has=page.get_by_role("heading", name="Add Program"))
    expect(dialog).to_be_visible(timeout=10000)
    return dialog


def fill_and_submit(dialog, key, title):
    dialog.get_by_placeholder("AWARE", exact=True).fill(key)
    dialog.get_by_placeholder("Security Awareness", exact=True).fill(title)
    dialog.get_by_role("button", name="Add", exact=True).click()


class TestProgramCreateError:

    def test_retired_key_error_stays_in_dialog(self, page, tokens):
        live, retired = seed(tokens["admin"])
        dialog = open_dialog(page)
        fill_and_submit(dialog, retired, "Reusing a retired key")

        expect(dialog.get_by_text("can't be reused")).to_be_visible(timeout=8000)
        expect(dialog.get_by_text(retired).first).to_be_visible()
        expect(dialog.get_by_placeholder("AWARE", exact=True)).to_have_value(retired)
        expect(dialog.get_by_placeholder("Security Awareness", exact=True)).to_have_value("Reusing a retired key")
        # The list behind the dialog is still there.
        expect(page.get_by_text(live).first).to_be_visible()

    def test_live_key_error_stays_in_dialog(self, page, tokens):
        live, _ = seed(tokens["admin"])
        dialog = open_dialog(page)
        fill_and_submit(dialog, live, "Duplicate of a live key")

        expect(dialog.get_by_text("already exists")).to_be_visible(timeout=8000)
        expect(dialog.get_by_placeholder("AWARE", exact=True)).to_have_value(live)
        expect(dialog.get_by_placeholder("Security Awareness", exact=True)).to_have_value("Duplicate of a live key")
        expect(page.get_by_text(live).first).to_be_visible()

    def test_fixing_the_key_after_an_error_creates_the_program(self, page, tokens):
        t = tokens["admin"]
        live, _ = seed(t)
        dialog = open_dialog(page)
        fill_and_submit(dialog, live, "Second try")
        expect(dialog.get_by_text("already exists")).to_be_visible(timeout=8000)

        fresh = unique_key("NEW")
        dialog.get_by_placeholder("AWARE", exact=True).fill(fresh)
        # Editing the input clears the stale message.
        expect(dialog.get_by_text("already exists")).to_have_count(0)
        dialog.get_by_role("button", name="Add", exact=True).click()

        expect(dialog).to_have_count(0, timeout=8000)
        deadline = time.time() + 10
        while time.time() < deadline and fresh not in live_keys(t):
            time.sleep(0.25)
        assert fresh in live_keys(t)
