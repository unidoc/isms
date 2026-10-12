"""E2E: wide tables in Reviews -> Changes scroll inside their own box (#393).

Tables in the diff views are real <table>s in narrow panes. Auto table layout
used to squeeze the long-text column (Purpose) to its longest word and spill the
table past its pane, which then scrolled sideways as a whole, cutting off the
last columns. A table must instead scroll by itself, keep its long-text column
readable, and never make the pane or page scroll sideways. Covers a changed
table (diffTables path) and an unchanged one (plain render path) in both Split
panes and in Unified.

Own file (not test_e2e_review_diff.py) to avoid per-branch EOF merge conflicts.
"""
import uuid

import pytest
from test_e2e_browser import api, do_login, ORG, ADMIN, R1, pw_browser, tokens  # noqa: F401

PURPOSE = (
    "Hosting of the production platform and customer data, including backups, "
    "disaster recovery, monitoring, and the operational support needed to keep "
    "the service available, secure and recoverable for all customers."
)
SUB_MD = f"""| Company | Purpose | Service | Location | Transfer |
|---|---|---|---|---|
| Acme Cloud | {PURPOSE} | Hosting | Frankfurt | SCCs |
| Beta Mail | Transactional email delivery | Email | Dublin | None |"""
ROPA_MD = """| Processing activity | Categories of data subjects | Categories of personal data | Recipients | Retention period | Lawful basis |
|---|---|---|---|---|---|
| Payroll administration | Employees and contractors of the organisation | Name, bank details, tax identifiers | Payroll provider | Seven years | Contract |
| Customer support | Customers and prospects of the organisation | Name, email, ticket history | Support tooling | Two years | Legitimate interest |"""
# Stays identical in both versions: exercises the plain (non-diff) render path.
UNCHANGED_MD = """| Control | Description | Owner | Status |
|---|---|---|---|
| A.5.1 | Policies for information security are defined, approved and published to staff | CISO | Done |
| A.5.2 | Information security roles and responsibilities are defined and allocated | CISO | Open |"""

OLD_BODY = f"# Wide\n\n{SUB_MD}\n\nBetween.\n\n{ROPA_MD}\n\nAlso.\n\n{UNCHANGED_MD}\n"


def _html_table(md):
    """The shape the WYSIWYG editor stores: one HTML table."""
    lines = [ln for ln in md.split("\n") if not ln.startswith("|---")]
    rows = []
    for i, ln in enumerate(lines):
        cells = [c.strip() for c in ln.strip("|").split("|")]
        tag = "th" if i == 0 else "td"
        rows.append("<tr>" + "".join(f"<{tag}><p>{c}</p></{tag}>" for c in cells) + "</tr>")
    return "<table><tbody>" + "".join(rows) + "</tbody></table>"


NEW_BODY = (
    "# Wide\n\n"
    + _html_table(SUB_MD.replace("| SCCs |", "| DPF |"))
    + "\n\nBetween.\n\n"
    + _html_table(ROPA_MD.replace("Seven years", "Ten years"))
    + f"\n\nAlso.\n\n{UNCHANGED_MD}\n"
)

PROBE = """
async (rootSel) => {
  const root = document.querySelector(rootSel);
  const tables = [...root.querySelectorAll('.doc-prose table')];
  const out = [];
  for (const t of tables) {
    const sc = t.parentElement;
    const cs = getComputedStyle(sc);
    const holdsOnlyTable = sc.children.length === 1 && !sc.querySelector('h1,h2,h3,h4,h5,h6');
    const scrolls = ['auto', 'scroll'].includes(cs.overflowX) && sc !== root && !sc.classList.contains('doc-prose');
    const ths = [...t.querySelectorAll('tr')[0].children];
    const bodyRows = [...t.querySelectorAll('tr')].slice(1);
    const longCols = ths.map((_, i) => Math.max(...[...t.querySelectorAll('tr')].map(r =>
      (r.children[i]?.textContent || '').trim().length))).map((n, i) => n >= 40 ? i : -1).filter(i => i >= 0);
    const pr = document.createElement('div');
    pr.style.cssText = 'position:absolute;visibility:hidden;width:1ch;height:0';
    t.querySelector('td').appendChild(pr);
    const ch = pr.getBoundingClientRect().width;
    pr.remove();
    const longWidths = longCols.map(i => ths[i].getBoundingClientRect().width / ch);
    const before = sc.scrollWidth - sc.clientWidth;
    sc.scrollLeft = 99999;
    await new Promise(r => setTimeout(r, 150));
    const last = ths[ths.length - 1].getBoundingClientRect();
    const box = sc.getBoundingClientRect();
    out.push({
      holdsOnlyTable, scrolls, longWidths, rows: bodyRows.length,
      needsScroll: before > 1,
      lastVisible: last.left >= box.left - 1 && last.right <= box.right + 1,
      first: ths[0].textContent.trim(),
    });
  }
  const pane = root.matches('[data-pane]') ? root : null;
  const de = document.documentElement;
  return {
    tables: out,
    paneOverflow: pane ? pane.scrollWidth - pane.clientWidth : 0,
    pageOverflow: de.scrollWidth - de.clientWidth,
  };
}
"""


@pytest.fixture(scope="module")
def wide_review(tokens):
    t = tokens["admin"]
    r1t = api("post", "/auth/login",
              json={"email": R1[0], "password": R1[1], "organization": ORG},
              expect_status=200).json()["token"]
    doc_id = f"e2e-chwide-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", t, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "E2E Changes Wide", "content": OLD_BODY,
    }, expect_status=[200, 201])
    rid0 = api("post", f"/documents/{doc_id}/reviews", t,
               json={"reviewers": [R1[0]], "message": "baseline"},
               expect_status=[200, 201]).json()["review_id"]
    api("post", f"/reviews/{rid0}/approve", r1t,
        json={"decision": "approved", "comment": "ok"}, expect_status=200)
    api("post", f"/reviews/{rid0}/merge", t, json={}, expect_status=200)
    rid = api("post", f"/documents/{doc_id}/reviews", t,
              json={"reviewers": [R1[0]], "message": "Wide tables diff"},
              expect_status=[200, 201]).json()["review_id"]
    api("put", f"/reviews/{rid}/content", r1t, json={"content": NEW_BODY}, expect_status=200)
    yield rid
    api("put", f"/reviews/{rid}/status", t, json={"status": "closed"})


def _open_changes(page, rid):
    do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{rid}")
    page.locator("button:has-text('Changes')").first.click()
    page.wait_for_load_state("networkidle")


def _check(page, root_sel, label, expect_scroll=True, expect_tables=3):
    page.locator(f"{root_sel} .doc-prose table").first.wait_for(state="visible", timeout=10000)
    r = page.evaluate(PROBE, root_sel)
    fails = []
    if len(r["tables"]) != expect_tables:
        fails.append(f"{label}: expected {expect_tables} tables, got {len(r['tables'])}")
    if r["paneOverflow"] > 1:
        fails.append(f"{label}: pane scrolls sideways by {r['paneOverflow']}px")
    if r["pageOverflow"] > 1:
        fails.append(f"{label}: page scrolls sideways by {r['pageOverflow']}px")
    for i, t in enumerate(r["tables"]):
        w = f"{label} table {i} ({t['first']})"
        if not t["holdsOnlyTable"]:
            fails.append(f"{w}: its container holds more than the table")
        if not t["scrolls"]:
            fails.append(f"{w}: not inside its own horizontal scroller")
        if not t["lastVisible"]:
            fails.append(f"{w}: last header cell not fully visible after scrolling right")
        for wd in t["longWidths"]:
            if wd < 18:  # in 1ch of the cell font; the floor is 18ch plus padding
                fails.append(f"{w}: long-text column is only {wd:.1f}ch")
    assert not fails, "\n" + "\n".join(fails)


def test_split_view_tables_scroll_in_both_panes(pw_browser, wide_review):
    ctx = pw_browser.new_context(viewport={"width": 1323, "height": 900})
    page = ctx.new_page()
    try:
        _open_changes(page, wide_review)
        failures = []
        for pane in ("previous", "current"):
            try:
                _check(page, f'[data-pane="{pane}"]', f"split@1323 {pane}")
            except AssertionError as e:  # report both panes in one run
                failures.append(str(e))
        assert not failures, "".join(failures)
    finally:
        ctx.close()


def _unified(page):
    page.locator("button:has-text('Unified')").first.click()
    # The unified view's content column is the only place `.tc-*` blocks render.
    page.locator(".doc-prose table").first.wait_for(state="visible", timeout=10000)


def test_unified_view_tables_scroll_on_a_narrow_screen(pw_browser, wide_review):
    ctx = pw_browser.new_context(viewport={"width": 768, "height": 900})
    page = ctx.new_page()
    try:
        _open_changes(page, wide_review)
        _unified(page)
        # No [data-pane] here: the whole page is the root.
        _check(page, "body", "unified@768")
    finally:
        ctx.close()


def test_unified_desktop_sub_processor_table_still_fits(pw_browser, wide_review):
    ctx = pw_browser.new_context(viewport={"width": 1323, "height": 900})
    page = ctx.new_page()
    try:
        _open_changes(page, wide_review)
        _unified(page)
        r = page.evaluate(PROBE, "body")
        sub = next(t for t in r["tables"] if t["first"] == "Company")
        # Whether the table fits depends on the platform's font metrics, so only
        # the font-independent part is asserted: it never pushes the page wider,
        # and Purpose stays readable (it was ~181px on base on macOS).
        assert r["pageOverflow"] <= 1, "page scrolls sideways at 1323px in Unified"
        assert min(sub["longWidths"]) >= 18, f"Purpose is only {sub['longWidths']}ch"
    finally:
        ctx.close()
