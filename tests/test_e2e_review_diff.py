"""E2E: the review diff renders table edits per-cell, not as a wholesale replace (#6).

Reproduces the real reported case (María on clause 4.2): a reviewer proposes a
revision that the WYSIWYG editor stores as an HTML table (a markdown pipe table
becomes HTML on the first edit), changing only two cells. The diff must highlight
just those two cells — in BOTH the default Split view and the Unified view — and
must NOT show the markdown→HTML conversion as a change.

Kept in its own file (not the test_e2e_browser monolith) to avoid per-branch
EOF merge conflicts as review-UI work continues.
"""
import uuid

import pytest
from test_e2e_browser import api, do_login, ORG, ADMIN, R1, pw_browser, tokens  # noqa: F401

# Baseline sent for review: a clean markdown pipe table.
OLD_BODY = """# Interested Parties

| Party | Requirement | How addressed |
|---|---|---|
| Customers | Confidentiality and integrity | Certification programme |
| Employees | Clear responsibilities and training | Awareness programme |
| Suppliers | Defined security expectations | Supplier register |"""

# The reviewer's proposal: the WYSIWYG editor stores the table as HTML, and only
# two requirement cells gained a prepended phrase (Customers row is untouched).
NEW_BODY = (
    "# Interested Parties\n\n"
    "<table><tbody>"
    "<tr><th><p>Party</p></th><th><p>Requirement</p></th><th><p>How addressed</p></th></tr>"
    "<tr><td><p>Customers</p></td><td><p>Confidentiality and integrity</p></td><td><p>Certification programme</p></td></tr>"
    "<tr><td><p>Employees</p></td><td><p>Background check. Clear responsibilities and training</p></td><td><p>Awareness programme</p></td></tr>"
    "<tr><td><p>Suppliers</p></td><td><p>Review supplier security docs. Defined security expectations</p></td><td><p>Supplier register</p></td></tr>"
    "</tbody></table>"
)


@pytest.fixture(scope="module")
def table_review(tokens):
    """Mirror the real case: an APPROVED doc (so there's a baseline to diff
    against), then a new review where the reviewer proposes an HTML-table
    revision that changes two cells. Returns the review id."""
    t = tokens["admin"]
    r1t = api("post", "/auth/login",
              json={"email": R1[0], "password": R1[1], "organization": ORG},
              expect_status=200).json()["token"]
    doc_id = f"e2e-tablediff-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Table Diff", "content": OLD_BODY,
    }, expect_status=[200, 201])

    # Establish an approved baseline (the markdown table) — a real, live doc.
    rid0 = api("post", f"/documents/{doc_id}/reviews", t,
               json={"reviewers": [R1[0]], "message": "baseline"},
               expect_status=[200, 201]).json()["review_id"]
    api("post", f"/reviews/{rid0}/approve", r1t,
        json={"decision": "approved", "comment": "ok"}, expect_status=200)
    api("post", f"/reviews/{rid0}/merge", t, json={}, expect_status=200)

    # New review: the assigned reviewer proposes the HTML-table revision.
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Table diff E2E"},
              expect_status=[200, 201]).json()["review_id"]
    api("put", f"/reviews/{rid}/content", r1t, json={"content": NEW_BODY}, expect_status=200)

    # Sanity: the diff endpoint exposes the old (markdown) + new (HTML) bodies.
    d = api("get", f"/reviews/{rid}/diff", t, expect_status=200).json()
    assert d["has_branch"] is True, "review branch should exist after a proposal"
    assert "Customers" in d["old_body"], f"old_body should hold the approved markdown table, got: {d['old_body'][:120]!r}"
    assert "<table" in d["new_body"], "new_body should hold the proposed HTML table"
    yield rid
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


def _assert_per_cell_table_diff(page):
    # Renders as a real table (not escaped tag soup).
    page.locator(".doc-prose table").first.wait_for(state="visible", timeout=10000)
    # Exactly the two edited cells are highlighted — NOT the whole table.
    page.locator("td.tc-cell-change").first.wait_for(state="visible", timeout=8000)
    n = page.locator("td.tc-cell-change").count()
    assert n == 2, f"expected exactly 2 changed cells, got {n}"
    # The added phrases show as word-level insertions inside those cells.
    assert page.locator(".tc-cell-change .tc-word-ins", has_text="Background check.").count() >= 1, \
        "the 'Background check.' insertion should be highlighted"
    assert page.locator(".tc-cell-change .tc-word-ins", has_text="Review supplier security docs.").count() >= 1, \
        "the supplier-docs insertion should be highlighted"
    # An unchanged cell (Customers row) must not be flagged as changed.
    assert page.locator("td.tc-cell-change", has_text="Certification programme").count() == 0, \
        "unchanged cells must not be highlighted"


def test_review_table_diff_renders_per_cell(pw_browser, table_review):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{table_review}")
        # Open the Changes tab (default), Split view is the default sub-view.
        page.locator("button:has-text('Changes')").first.click()
        page.wait_for_load_state("networkidle")

        # 1. Default (Split) view renders the edit per-cell.
        _assert_per_cell_table_diff(page)
        page.screenshot(path="docs/screenshots/test_review_table_diff_split.png")

        # 2. Unified view does too — and its stats count CELLS, not raw lines
        #    (a md→HTML table must not read "+1 added -5 removed" for a 2-cell edit).
        page.locator("button:has-text('Unified')").first.click()
        _assert_per_cell_table_diff(page)
        assert page.locator("text=2 added").count() >= 1, "unified stats should count 2 changed cells"
        assert page.locator("text=5 removed").count() == 0, "stats must not use raw line counts for a table edit"
        page.screenshot(path="docs/screenshots/test_review_table_diff_unified.png")
    finally:
        ctx.close()


# A second table that gets deleted outright in the same revision — this is what
# broke the old sequential-counter pairing (the deletion shifted the index so the
# changed table diffed against the wrong old table). #99 review must-fix.
OLD_BODY_2 = """# Team Roster

| Name | Role |
|---|---|
| Alice | Lead |
| Bob | Dev |

# Interested Parties

| Party | Requirement |
|---|---|
| Customers | Confidentiality |
| Suppliers | Security expectations |"""

# Reviewer deletes the Team Roster table entirely and changes ONE cell in the
# Interested Parties table (Customers requirement).
NEW_BODY_2 = (
    "# Interested Parties\n\n"
    "<table><tbody>"
    "<tr><th><p>Party</p></th><th><p>Requirement</p></th></tr>"
    "<tr><td><p>Customers</p></td><td><p>Confidentiality and integrity</p></td></tr>"
    "<tr><td><p>Suppliers</p></td><td><p>Security expectations</p></td></tr>"
    "</tbody></table>"
)


@pytest.fixture(scope="module")
def table_review_with_deletion(tokens):
    """Approved baseline with TWO tables; reviewer deletes the first and changes
    one cell in the second."""
    t = tokens["admin"]
    r1t = api("post", "/auth/login",
              json={"email": R1[0], "password": R1[1], "organization": ORG},
              expect_status=200).json()["token"]
    doc_id = f"e2e-tabledel-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Table Delete", "content": OLD_BODY_2,
    }, expect_status=[200, 201])
    rid0 = api("post", f"/documents/{doc_id}/reviews", t,
               json={"reviewers": [R1[0]], "message": "baseline"},
               expect_status=[200, 201]).json()["review_id"]
    api("post", f"/reviews/{rid0}/approve", r1t,
        json={"decision": "approved", "comment": "ok"}, expect_status=200)
    api("post", f"/reviews/{rid0}/merge", t, json={}, expect_status=200)
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Delete + edit"},
              expect_status=[200, 201]).json()["review_id"]
    api("put", f"/reviews/{rid}/content", r1t, json={"content": NEW_BODY_2}, expect_status=200)
    yield rid
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


def test_review_table_diff_pairs_correctly_after_deletion(pw_browser, table_review_with_deletion):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{table_review_with_deletion}")
        page.locator("button:has-text('Changes')").first.click()
        page.wait_for_load_state("networkidle")
        page.locator(".doc-prose table").first.wait_for(state="visible", timeout=10000)

        # The Interested Parties change must be paired against the RIGHT old table
        # despite the Team Roster deletion: exactly one changed cell, the real edit.
        # (The old bug paired it against Team Roster → 0 cell-changes, Alice/Bob
        # rows wrongly appended as deletions.)
        page.locator("td.tc-cell-change").first.wait_for(state="visible", timeout=8000)
        assert page.locator("td.tc-cell-change").count() == 1, \
            f"expected exactly 1 changed cell, got {page.locator('td.tc-cell-change').count()}"
        assert page.locator(".tc-cell-change .tc-word-ins", has_text="and integrity").count() >= 1, \
            "the real word-level edit must be shown"
        assert page.locator("td.tc-cell-change", has_text="Alice").count() == 0, \
            "deleted-table rows must not be mis-paired into the changed table"
    finally:
        ctx.close()


# A document that was NEVER approved before being sent for review (no baseline
# commit). The diff must still show the reviewer's proposal against what was
# *sent* — not an empty "Previous" pane with the whole doc as "new". (#6: the
# review-diff baseline bug — old_body was empty when commit_hash was empty.)
@pytest.fixture(scope="module")
def table_review_unapproved(tokens):
    t = tokens["admin"]
    r1t = api("post", "/auth/login",
              json={"email": R1[0], "password": R1[1], "organization": ORG},
              expect_status=200).json()["token"]
    doc_id = f"e2e-tablenoappr-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Table No-Approval", "content": OLD_BODY,
    }, expect_status=[200, 201])
    # NOTE: no approve/merge — the doc has no approved baseline (commit_hash == "").
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Review"},
              expect_status=[200, 201]).json()["review_id"]
    api("put", f"/reviews/{rid}/content", r1t, json={"content": NEW_BODY}, expect_status=200)
    # The diff must expose the SENT version as old_body, not an empty baseline.
    d = api("get", f"/reviews/{rid}/diff", t, expect_status=200).json()
    assert "Customers" in d["old_body"], \
        f"old_body must be the sent version even without an approved baseline, got: {d['old_body'][:80]!r}"
    yield rid
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


def test_review_diff_shows_sent_version_without_approved_baseline(pw_browser, table_review_unapproved):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{table_review_unapproved}")
        page.locator("button:has-text('Changes')").first.click()
        page.wait_for_load_state("networkidle")
        # The reviewer's edit must render as a per-cell diff — which only happens
        # when the sent version is present as the baseline. (The bug showed an
        # empty Previous pane → whole table as "new" → 0 cell-changes.)
        page.locator("td.tc-cell-change").first.wait_for(state="visible", timeout=10000)
        assert page.locator("td.tc-cell-change").count() == 2, \
            f"expected 2 changed cells against the sent baseline, got {page.locator('td.tc-cell-change').count()}"
    finally:
        ctx.close()


# A reviewer who submits a proposed_revision *decision* (not just edits the
# branch) — so the Conversation timeline has an expandable per-event entry. #6.
@pytest.fixture(scope="module")
def table_review_proposed(tokens):
    t = tokens["admin"]
    r1t = api("post", "/auth/login",
              json={"email": R1[0], "password": R1[1], "organization": ORG},
              expect_status=200).json()["token"]
    doc_id = f"e2e-perevent-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Per-event", "content": OLD_BODY,
    }, expect_status=[200, 201])
    rid0 = api("post", f"/documents/{doc_id}/reviews", t,
               json={"reviewers": [R1[0]], "message": "baseline"},
               expect_status=[200, 201]).json()["review_id"]
    api("post", f"/reviews/{rid0}/approve", r1t,
        json={"decision": "approved", "comment": "ok"}, expect_status=200)
    api("post", f"/reviews/{rid0}/merge", t, json={}, expect_status=200)
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Review"},
              expect_status=[200, 201]).json()["review_id"]
    api("put", f"/reviews/{rid}/content", r1t, json={"content": NEW_BODY}, expect_status=200)
    # The decision record anchors the proposal commit (commit_ref) for per-event diff.
    api("post", f"/reviews/{rid}/approve", r1t,
        json={"decision": "proposed_revision", "comment": "tweaked two cells"}, expect_status=200)
    # Sanity: the per-event diff (to=proposal commit) shows the markdown→HTML edit.
    tl = api("get", f"/reviews/{rid}/timeline", t, expect_status=200).json()
    entries = tl.get("data", tl) if isinstance(tl, dict) else tl
    cref = next((e.get("data", {}).get("commit_ref") for e in entries
                 if isinstance(e, dict) and e.get("type") == "decision"
                 and e.get("decision") == "proposed_revision"
                 and e.get("data", {}).get("commit_ref")), None)
    assert cref, "proposed_revision decision must anchor a commit_ref"
    d = api("get", f"/reviews/{rid}/diff?to={cref}", t, expect_status=200).json()
    assert "Customers" in d["old_body"] and "<table" in d["new_body"], "per-event diff should be base→proposal"
    yield rid
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


def test_per_event_diff_expands_in_conversation(pw_browser, table_review_proposed):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{table_review_proposed}")
        page.locator("button:has-text('Conversation')").first.click()
        page.wait_for_load_state("networkidle")
        # Expand the reviewer's proposed-revision entry.
        page.locator("button:has-text('View changes')").first.wait_for(state="visible", timeout=10000)
        page.locator("button:has-text('View changes')").first.click()
        # The per-event diff renders the two edited cells, scoped to that revision.
        page.locator("td.tc-cell-change").first.wait_for(state="visible", timeout=8000)
        assert page.locator("td.tc-cell-change").count() == 2, \
            f"per-event diff should show exactly the 2 edited cells, got {page.locator('td.tc-cell-change').count()}"
        assert page.locator(".tc-cell-change .tc-word-ins", has_text="Background check.").count() >= 1
    finally:
        ctx.close()
