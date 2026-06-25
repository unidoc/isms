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

        # 2. Unified view does too.
        page.locator("button:has-text('Unified')").first.click()
        _assert_per_cell_table_diff(page)
        page.screenshot(path="docs/screenshots/test_review_table_diff_unified.png")
    finally:
        ctx.close()
