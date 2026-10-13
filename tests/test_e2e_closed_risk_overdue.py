"""E2E (#448): a closed risk is never flagged as overdue for review.

Closing a risk keeps its next_review, so a closed risk with a past date used to
show the red Overdue badge in the list and the "Review overdue since" warning in
the detail panel. The server's overdue summary already excludes closed risks;
the UI now agrees. An open risk with the same past date is the control.

Seeding trap: the API recomputes next_review on every create/update unless one
is sent explicitly, so status and next_review are always sent together and the
stored date is asserted to be in the past before the UI is checked.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_closed_risk_overdue.py -v
"""
import uuid
from datetime import date

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, BASE, ORG, pw_browser, tokens  # noqa: F401

PAST = "2026-04-24"
# The reopen PUT sends a different past day: an update that echoes the stored
# next_review is treated as "not chosen" and the date is recomputed (#202).
PAST_REOPEN = "2026-04-25"
BADGE = "OVERDUE"  # common.state.overdue


@pytest.fixture
def page(pw_browser, tokens):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    p = ctx.new_page()
    do_login(p, ADMIN[0], ADMIN[1])
    yield p
    ctx.close()


def risk_body(title, status, next_review=PAST):
    return {
        "title": title, "description": "e2e overdue", "risk_type": "threat",
        "origin": "internal", "status": status, "treatment": "mitigate",
        "current_likelihood": 2, "current_impact": 2, "next_review": next_review,
    }


def assert_stored_past(token, rid, status):
    r = api("get", f"/risks/{rid}", token, expect_status=200).json()
    assert r["status"] == status
    nr = r["next_review"]
    assert nr is not None, "next_review was not stored"
    # Epoch seconds or a date string, depending on the API's encoding.
    stored = date.fromtimestamp(nr) if isinstance(nr, (int, float)) else date.fromisoformat(nr[:10])
    assert stored < date.today(), f"next_review {nr!r} is not in the past, so the test proves nothing"


def seed(token, status):
    title = f"E2E overdue {status} {uuid.uuid4().hex[:8]}"
    rid = api("post", "/risks", token, json=risk_body(title, status), expect_status=[200, 201]).json()["id"]
    assert_stored_past(token, rid, status)
    return rid, title


def goto_list(page, title):
    page.goto(f"{BASE}/{ORG}/risks")
    box = page.get_by_placeholder("Search...").first
    box.wait_for(state="visible", timeout=10000)
    box.fill(title)
    row = page.locator("tr", has_text=title)
    expect(row).to_have_count(1, timeout=10000)
    return row


def open_assessment(page, rid, title):
    page.goto(f"{BASE}/{ORG}/risks/{rid}")
    # The drawer is a plain modal div, found by its width class plus the title.
    drawer = page.locator("div.max-w-4xl").filter(has=page.get_by_role("heading", name=title))
    expect(drawer).to_be_visible(timeout=15000)
    drawer.get_by_role("button", name="Assessment", exact=True).click()
    return drawer


class TestClosedRiskOverdue:

    def test_closed_risk_is_not_flagged_but_open_risk_is(self, page, tokens):
        t = tokens["admin"]
        closed_id, closed_title = seed(t, "closed")
        open_id, open_title = seed(t, "open")

        # List badge.
        expect(goto_list(page, closed_title).get_by_text(BADGE, exact=True)).to_have_count(0)
        expect(goto_list(page, open_title).get_by_text(BADGE, exact=True)).to_have_count(1)

        # Assessment tab: closed shows neither the warning nor a "Next review" line.
        drawer = open_assessment(page, closed_id, closed_title)
        expect(drawer.get_by_text("Inherent")).to_be_visible(timeout=8000)
        expect(drawer.get_by_text("Review overdue since")).to_have_count(0)
        expect(drawer.get_by_text("Next review")).to_have_count(0)

        # The open control keeps the warning.
        drawer = open_assessment(page, open_id, open_title)
        expect(drawer.get_by_text("Review overdue since")).to_be_visible(timeout=8000)

    def test_reopening_a_closed_risk_flags_it_again(self, page, tokens):
        t = tokens["admin"]
        rid, title = seed(t, "closed")
        expect(goto_list(page, title).get_by_text(BADGE, exact=True)).to_have_count(0)

        api("put", f"/risks/{rid}", t, json=risk_body(title, "open", PAST_REOPEN), expect_status=200)
        assert_stored_past(t, rid, "open")
        expect(goto_list(page, title).get_by_text(BADGE, exact=True)).to_have_count(1)
