"""E2E: Reviews -> Changes -> Unified keeps markdown pipe tables whole (#441).

The unified view diffs line by line, but a pipe table spans many lines, so a
table touched by the review fell apart into a short table plus raw `| a | b |`
text, and an untouched table whose rows came from two commits did too. Split
mode was always correct. Each table must be exactly one <table> in Unified,
with the same marks Split shows, the stats must count cells (not raw lines), and
the Advanced (raw) diff must keep listing only the changed lines.

Own file to avoid per-branch EOF merge conflicts.
"""
import time
import uuid

import pytest
from test_e2e_browser import api, do_login, ORG, ADMIN, R1, pw_browser, tokens  # noqa: F401


def table(headers, rows):
    out = ["| " + " | ".join(headers) + " |", "|" + "|".join("---" for _ in headers) + "|"]
    return "\n".join(out + ["| " + " | ".join(r) + " |" for r in rows])


def body(v):
    a = [["Acme Cloud", "Hosting", "Frankfurt"], ["Postbox Ltd", "Email", "Dublin"], ["Statly", "Monitoring", "Oregon"]]
    b = [["Laptop", "IT"], ["Badge reader", "Facilities"]]
    c = [["R-1", "Phishing"], ["R-2", "Ransomware"], ["R-3", "Insider misuse"]]
    d = [["Backups", "Daily"], ["Access review", "Quarterly"], ["Pen test", "Yearly"]]
    e_h = ["Control", "Owner"]
    prose = "This paragraph explains the registers below."
    if v in ("v2", "v3"):
        d[1][1] = "Monthly"          # D's rows now come from two commits
    if v == "v3":
        a[1][2] = "Dublin and Cork"   # A: one cell changed
        b.append(["Server rack", "IT"])  # B: row added
        del c[1]                      # C: row removed
        e_h = ["Control", "Accountable owner"]  # E: header renamed
        prose = "This paragraph explains the registers below, as reviewed."
    e = [["MFA", "Security lead"], ["Logging", "Platform team"]]
    return "\n\n".join([
        "# Pipe tables", prose,
        "## A: cell changed", table(["Company", "Service", "Location"], a),
        "## B: row added", table(["Asset", "Department"], b),
        "## C: row removed", table(["ID", "Risk"], c),
        "## D: unchanged in review", table(["Activity", "Frequency"], d),
        "## E: header renamed", table(e_h, e),
        "Closing paragraph.",
    ]) + "\n"


@pytest.fixture(scope="module")
def pipe_review(tokens):
    t = tokens["admin"]
    r1t = api("post", "/auth/login",
              json={"email": R1[0], "password": R1[1], "organization": ORG},
              expect_status=200).json()["token"]
    doc_id = f"e2e-pipes-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Pipe Tables", "content": body("v1"),
    }, expect_status=[200, 201])
    time.sleep(1.2)  # distinct commit timestamps: D's rows get two blame hashes
    api("put", f"/documents/{doc_id}/content", t, json={"content": body("v2")}, expect_status=200)
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Pipe tables"},
              expect_status=[200, 201]).json()["review_id"]
    api("put", f"/reviews/{rid}/content", r1t, json={"content": body("v3")}, expect_status=200)
    yield rid
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


# Walks the rendered blocks of `root` in document order and groups what it finds
# under the preceding h2 ("A: ...", "B: ...").
SECTIONS = """
(rootSel) => {
  const root = document.querySelector(rootSel);
  const sections = {};
  let cur = null;
  for (const prose of root.querySelectorAll('.doc-prose')) {
    for (const el of prose.children) {
      if (el.tagName === 'H2') {
        cur = el.textContent.trim()[0];
        sections[cur] = { tables: 0, rows: 0, rawPipe: [], addRows: 0, delRows: 0, cellChanges: 0, thChanges: 0 };
        continue;
      }
      if (!cur) continue;
      const tables = el.tagName === 'TABLE' ? [el] : [...el.querySelectorAll('table')];
      const s = sections[cur];
      if (tables.length) {
        for (const t of tables) {
          s.tables += 1;
          s.rows += t.querySelectorAll('tr').length;
          s.addRows += t.querySelectorAll('tr.tc-row-add').length;
          s.delRows += t.querySelectorAll('tr.tc-row-del').length;
          s.cellChanges += t.querySelectorAll('td.tc-cell-change').length;
          s.thChanges += t.querySelectorAll('th.tc-cell-change').length;
        }
      } else if (el.textContent.includes('|')) {
        s.rawPipe.push(el.textContent.trim().slice(0, 60));
      }
    }
  }
  return sections;
}
"""

# (rows incl. header, added rows, removed rows, changed td cells, changed th cells)
EXPECTED = {
    "A": (4, 0, 0, 1, 0),
    "B": (4, 1, 0, 0, 0),
    "C": (4, 0, 1, 0, 0),  # two kept rows + the removed one shown at the end
    "D": (4, 0, 0, 0, 0),
    "E": (3, 0, 0, 0, 1),
}


def _open_changes(page, rid):
    do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{rid}")
    page.locator("button:has-text('Changes')").first.click()
    page.wait_for_load_state("networkidle")


def _check_sections(page, root_sel, label):
    page.locator(f"{root_sel} .doc-prose table").first.wait_for(state="visible", timeout=10000)
    got = page.evaluate(SECTIONS, root_sel)
    fails = []
    for key, (rows, add, dele, cells, ths) in EXPECTED.items():
        s = got.get(key)
        if not s:
            fails.append(f"{label} {key}: section missing")
            continue
        w = f"{label} {key}"
        if s["tables"] != 1:
            fails.append(f"{w}: expected exactly 1 table, got {s['tables']}")
        if s["rows"] != rows:
            fails.append(f"{w}: expected {rows} table rows, got {s['rows']}")
        if s["rawPipe"]:
            fails.append(f"{w}: raw pipe text outside the table: {s['rawPipe']}")
        if (s["addRows"], s["delRows"], s["cellChanges"], s["thChanges"]) != (add, dele, cells, ths):
            fails.append(f"{w}: marks (add, del, td changed, th changed) = "
                         f"{(s['addRows'], s['delRows'], s['cellChanges'], s['thChanges'])}, expected {(add, dele, cells, ths)}")
    assert not fails, "\n" + "\n".join(fails)


def test_split_view_pipe_tables_unchanged_behaviour(pw_browser, pipe_review):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        _open_changes(page, pipe_review)
        _check_sections(page, '[data-pane="current"]', "split current")
    finally:
        ctx.close()


def test_unified_view_pipe_tables_stay_whole(pw_browser, pipe_review):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        _open_changes(page, pipe_review)
        page.locator("button:has-text('Unified')").first.click()
        _check_sections(page, "body", "unified")
        # Stats count changed cells for the tables (not raw lines): 1 (A) + 2
        # (B's added row) + 2 (C's removed row) cells, plus 1 prose line, and the
        # one removed prose line. The old raw-line stats read "+4 added -4 removed".
        stats = page.locator("text=/\\+\\d+ added/").first.inner_text()
        assert "+6 added" in stats or page.locator("text=+6 added").count() > 0, f"stats: {stats}"
        assert page.locator("text=+4 added").count() == 0, "stats still count raw lines"
        assert page.locator("text=-1 removed").count() > 0, "expected '-1 removed'"
    finally:
        ctx.close()


def test_advanced_diff_still_lists_only_changed_lines(pw_browser, pipe_review):
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        _open_changes(page, pipe_review)
        page.locator("button:has-text('Unified')").first.click()
        page.get_by_role("button", name="Advanced", exact=True).click()
        view = page.locator(".diff-view")
        view.wait_for(state="visible", timeout=10000)
        view.get_by_role("button", name="Unified", exact=True).click()
        removed = view.locator("tr.bg-red-950\\/30").all_inner_texts()
        added = view.locator("tr.bg-emerald-950\\/30").all_inner_texts()
        assert any("Postbox Ltd | Email | Dublin |" in r for r in removed), f"removed lines: {removed}"
        assert any("Dublin and Cork" in r for r in added), f"added lines: {added}"
        assert not any("Acme Cloud" in r for r in removed + added), "an unchanged row is listed as changed"
        assert len(removed) == 4 and len(added) == 4, f"{len(removed)} removed / {len(added)} added lines"
    finally:
        ctx.close()


def test_comment_badges_land_on_the_right_paragraphs(pw_browser, pipe_review, tokens):
    """Regrouping a table into one block must not shift paragraph indexes: a
    comment on the paragraph before a table, on a table, and after the last
    table still shows on that paragraph."""
    t = tokens["admin"]
    # v3 paragraphs: 3 = table A, 4 = "## B" heading, 11 = table E, 12 = closing
    for idx, quote in ((3, "table A"), (4, "## B: row added"), (11, "table E"), (12, "Closing paragraph.")):
        api("post", f"/reviews/{pipe_review}/comment", t,
            json={"body": f"on paragraph {idx}", "paragraph_index": idx, "quote": quote},
            expect_status=[200, 201])
    ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    try:
        _open_changes(page, pipe_review)
        page.locator("button:has-text('Unified')").first.click()
        page.locator(".doc-prose table").first.wait_for(state="visible", timeout=10000)
        landed = page.evaluate("""() => [...document.querySelectorAll('.group\\\\/para')]
          .filter(p => p.querySelector('div.bg-blue-600'))
          .map(p => [...p.querySelectorAll('.doc-prose')].map(d => d.textContent).join(' ').replace(/\\s+/g, ' ').trim().slice(0, 30))""")
        assert len(landed) == 4, f"expected 4 paragraphs with a badge, got {landed}"
        assert landed[0].upper().startswith("COMPANY"), landed
        assert landed[1].startswith("B: row added"), landed
        assert landed[2].upper().startswith("CONTROL"), landed
        assert landed[3].startswith("Closing paragraph."), landed
    finally:
        ctx.close()
