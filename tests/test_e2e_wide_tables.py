"""E2E: a document table wider than its box scrolls as one unit (#395).

Read-only document tables render as one CSS grid per row. When the column
floors add up to more than the available width, the header grid used to clip its
own cells (columns with data but no visible header) and body cells painted past
the table's edge. A table must instead scroll sideways with header and body
together, while each row's "+" button and review checkbox stay pinned at the
visible edges and the page itself never scrolls sideways.

Own file (not test_e2e_browser.py) to avoid per-branch EOF merge conflicts.
"""
import uuid

import pytest
from test_e2e_browser import api, do_login, ORG, ADMIN, R1, pw_browser, tokens  # noqa: F401

PURPOSE = (
    "Hosting of the production platform and customer data, including backups, "
    "disaster recovery, monitoring, and the operational support needed to keep "
    "the service available, secure and recoverable for all customers."
)
BODY = f"""# Wide Tables

| Company | Purpose | Service | Location | Transfer |
|---|---|---|---|---|
| Acme Cloud | {PURPOSE} | Hosting | Frankfurt | SCCs |
| Beta Mail | Transactional email delivery | Email | Dublin | None |

Between the tables.

| Processing activity | Categories of data subjects | Categories of personal data | Recipients | Retention period | Lawful basis |
|---|---|---|---|---|---|
| Payroll administration | Employees and contractors of the organisation | Name, bank details, tax identifiers | Payroll provider | Seven years | Contract |
| Customer support | Customers and prospects of the organisation | Name, email, ticket history | Support tooling | Two years | Legitimate interest |
"""

# Reads the layout of the document's table blocks. Tables are found by their
# `.tbl-grid` rows, not by any renderer-specific attribute, so the same probe
# runs against any build.
PROBE = """
async (scrollRight) => {
  const first = document.querySelector('.comment-block');
  const blocks = [...first.parentElement.querySelectorAll(':scope > .comment-block')];
  const groups = [];
  let cur = null;
  for (const b of blocks) {
    if (b.querySelector('.tbl-grid')) {
      if (!cur) { cur = []; groups.push(cur); }
      cur.push(b);
    } else cur = null;
  }
  const out = [];
  for (const g of groups) {
    const scrollers = g.map(b => b.querySelector('.doc-prose'));
    if (scrollRight) {
      scrollers[0].scrollLeft = 99999; // the header row: its scrollbar is hidden
      await new Promise(r => setTimeout(r, 250));
    }
    const grid = b => b.querySelector('.tbl-grid');
    const hdr = grid(g[0]);
    const hr = hdr.getBoundingClientRect();
    const clipped = [...hdr.querySelectorAll('.tbl-hdr-cell')].filter(c => {
      const r = c.getBoundingClientRect();
      return r.left < hr.left - 1 || r.right > hr.right + 1;
    }).length;
    const row = g[1];
    const hcells = [...hdr.querySelectorAll('.tbl-hdr-cell')];
    const bcells = [...grid(row).children];
    const misalign = Math.max(...hcells.map((c, i) =>
      Math.abs(c.getBoundingClientRect().left - bcells[i].getBoundingClientRect().left)));
    row.scrollIntoView({block: 'center'});
    const hit = sel => {
      const el = row.querySelector(sel);
      if (!el) return false;
      const r = el.getBoundingClientRect();
      const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      return !!top && (top === el || el.contains(top));
    };
    out.push({
      scrollLefts: scrollers.map(s => Math.round(s.scrollLeft)),
      scrollable: scrollers.some(s => s.scrollWidth > s.clientWidth + 1),
      clipped, misalign,
      headerCells: hcells.length,
      purposeWidth: hcells[1].getBoundingClientRect().width,
      plusHit: hit('button[title*="omment"]'),
      checkHit: hit('.review-checkbox'),
      rowOverflow: scrollers[1].scrollWidth - scrollers[1].clientWidth,
    });
  }
  // Neither the page nor the element holding the blocks may be pushed wider
  // by a table (the pane's other content, e.g. its tab bar, is not ours).
  const root = document.documentElement;
  const pageOverflow = Math.max(root.scrollWidth - root.clientWidth,
    first.parentElement.scrollWidth - first.parentElement.clientWidth);
  return { tables: out, pageOverflow };
}
"""


@pytest.fixture(scope="module")
def wide_doc(tokens):
    t = tokens["admin"]
    doc_id = f"e2e-wide-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Wide Tables", "content": BODY,
    }, expect_status=[200, 201])
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Wide tables"},
              expect_status=[200, 201]).json()["review_id"]
    yield {"doc": doc_id, "review": rid}
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


def _check(page, label, expect_scroll=True):
    page.locator(".tbl-grid").first.wait_for(state="visible", timeout=10000)
    # Collect every failure so one run reports all broken invariants.
    fails = []

    def ck(ok, msg):
        if not ok:
            fails.append(msg)

    before = page.evaluate(PROBE, False)
    after = page.evaluate(PROBE, True)
    assert len(before["tables"]) == 2, f"{label}: expected 2 tables, got {len(before['tables'])}"
    ck(before["pageOverflow"] <= 1, f"{label}: page/pane scrolls sideways by {before['pageOverflow']}px")
    ck(after["pageOverflow"] <= 1, f"{label}: page/pane scrolls sideways after table scroll by {after['pageOverflow']}px")
    for name, b, a in zip(("sub-processor", "ropa"), before["tables"], after["tables"]):
        where = f"{label} {name}"
        ck(b["clipped"] == 0, f"{where}: {b['clipped']} of {b['headerCells']} header cells are clipped by the header grid")
        ck(b["misalign"] <= 1, f"{where}: header/body misaligned by {b['misalign']}px")
        ck(b["plusHit"], f"{where}: '+' button not hit-testable before scrolling")
        ck(b["checkHit"], f"{where}: review checkbox not hit-testable before scrolling")
        if name == "sub-processor":
            ck(b["purposeWidth"] >= 160, f"{where}: Purpose column is only {b['purposeWidth']}px")
        if expect_scroll:
            ck(a["scrollable"], f"{where}: table does not scroll sideways (header clipped / body overflow instead)")
            ck(a["scrollLefts"][-1] > 0, f"{where}: scrolling the table moved nothing: {a['scrollLefts']}")
            ck(len(set(a["scrollLefts"])) == 1, f"{where}: rows are not scrolled together: {a['scrollLefts']}")
        ck(a["clipped"] == 0, f"{where}: header cells clipped after scrolling")
        ck(a["misalign"] <= 1, f"{where}: header/body misaligned by {a['misalign']}px after scrolling")
        ck(a["plusHit"], f"{where}: '+' button not hit-testable after scrolling right")
        ck(a["checkHit"], f"{where}: review checkbox not hit-testable after scrolling right")
    assert not fails, "\n" + "\n".join(fails)


def test_documents_page_wide_tables_scroll_as_one_unit(pw_browser, wide_doc):
    ctx = pw_browser.new_context(viewport={"width": 390, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"documents/{wide_doc['doc']}")
        _check(page, "documents@390")
    finally:
        ctx.close()


def test_review_document_tab_wide_tables_scroll_as_one_unit(pw_browser, wide_doc):
    ctx = pw_browser.new_context(viewport={"width": 1024, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{wide_doc['review']}")
        page.get_by_role("button", name="Document", exact=True).first.click()
        _check(page, "review-document@1024")
    finally:
        ctx.close()


def test_desktop_sub_processor_table_fits_without_scroll(pw_browser, wide_doc):
    ctx = pw_browser.new_context(viewport={"width": 1323, "height": 900})
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1], then_goto=f"documents/{wide_doc['doc']}")
        page.locator(".tbl-grid").first.wait_for(state="visible", timeout=10000)
        r = page.evaluate(PROBE, False)
        sub = r["tables"][0]
        assert sub["rowOverflow"] <= 1, f"sub-processor table scrolls at 1323px (overflow {sub['rowOverflow']}px)"
        assert sub["clipped"] == 0
        assert sub["purposeWidth"] >= 160, f"Purpose column is only {sub['purposeWidth']}px"
    finally:
        ctx.close()
