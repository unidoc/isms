"""E2E: the Split review view scrolls both panes together (synced scroll).

Reviewing a side-by-side diff means lining changes up across both columns; before
this the two panes scrolled independently and you had to scroll each one. This
pins that scrolling the Previous pane moves the Current pane with it.

Own file (not test_e2e_review_diff.py) to avoid per-branch EOF merge conflicts.
"""
import uuid

import pytest
from test_e2e_browser import api, do_login, ORG, ADMIN, R1, pw_browser, tokens  # noqa: F401

# A long document so both panes overflow and actually scroll.
_PARAS = "\n\n".join(f"Paragraph {i}: control and risk text for the ISMS." for i in range(1, 61))
OLD_BODY = f"# Long Review Doc\n\n{_PARAS}"
# Reviewer proposes a small edit to one paragraph (so both panes have content).
NEW_BODY = OLD_BODY.replace("Paragraph 30:", "Paragraph 30 (edited):")


@pytest.fixture(scope="module")
def long_review(tokens):
    t = tokens["admin"]
    r1t = api("post", "/auth/login",
              json={"email": R1[0], "password": R1[1], "organization": ORG},
              expect_status=200).json()["token"]
    doc_id = f"e2e-scroll-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Scroll", "content": OLD_BODY,
    }, expect_status=[200, 201])
    rid0 = api("post", f"/documents/{doc_id}/reviews", t,
               json={"reviewers": [R1[0]], "message": "baseline"},
               expect_status=[200, 201]).json()["review_id"]
    api("post", f"/reviews/{rid0}/approve", r1t,
        json={"decision": "approved", "comment": "ok"}, expect_status=200)
    api("post", f"/reviews/{rid0}/merge", t, json={}, expect_status=200)
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Long edit"},
              expect_status=[200, 201]).json()["review_id"]
    api("put", f"/reviews/{rid}/content", r1t, json={"content": NEW_BODY}, expect_status=200)
    yield rid
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


def test_split_view_scrolls_both_panes_together(pw_browser, long_review):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{long_review}")
        page.locator("button:has-text('Changes')").first.click()
        page.wait_for_load_state("networkidle")
        # Split is the default sub-view.
        prev = page.locator('[data-pane="previous"]')
        curr = page.locator('[data-pane="current"]')
        prev.wait_for(state="visible", timeout=10000)

        # Both panes must actually overflow (otherwise the test proves nothing).
        assert prev.evaluate("el => el.scrollHeight > el.clientHeight + 50"), "previous pane should overflow"
        assert curr.evaluate("el => el.scrollHeight > el.clientHeight + 50"), "current pane should overflow"

        assert curr.evaluate("el => el.scrollTop") == 0
        # Scroll the Previous pane — the Current pane must follow.
        prev.evaluate("el => { el.scrollTop = 400 }")
        page.wait_for_timeout(200)
        assert curr.evaluate("el => el.scrollTop") > 0, "current pane should follow when previous scrolls"

        # And the reverse direction too.
        curr.evaluate("el => { el.scrollTop = 0 }")
        page.wait_for_timeout(200)
        curr.evaluate("el => { el.scrollTop = 600 }")
        page.wait_for_timeout(200)
        assert prev.evaluate("el => el.scrollTop") > 0, "previous pane should follow when current scrolls"
    finally:
        ctx.close()
