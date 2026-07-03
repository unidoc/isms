"""E2E (#17): register pages are usable on a phone-width viewport.

The registers (risks, assets, suppliers, …) render a filter bar + a wide data
table. On a 390px viewport the filter bar used to overflow off-screen (fixed-width
selects, no wrap) and the table columns were clipped with no way to reach them.

This pins the mobile fix:
  1. No page-level horizontal overflow — the filter bar wraps instead of pushing
     the whole page wider than the viewport.
  2. The table lives in a horizontal-scroll container, so the off-screen columns
     are reachable by swiping rather than lost.

Own file to keep the monolithic test_e2e_browser.py free of per-branch conflicts.
"""
import pytest
from test_e2e_browser import do_login, ORG, ADMIN, pw_browser  # noqa: F401

MOBILE = {"width": 390, "height": 844}  # iPhone 12-ish
REGISTERS = ["risks", "assets", "suppliers", "legal", "systems", "objectives"]


def _goto(page, view):
    page.evaluate(
        "(v) => document.querySelector('#app').__vue_app__.config.globalProperties"
        ".$router.push('/' + v)",
        f"{ORG}/{view}",
    )
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(300)


def test_register_pages_usable_on_mobile(pw_browser):
    ctx = pw_browser.new_context(viewport=MOBILE)
    page = ctx.new_page()
    try:
        do_login(page, ADMIN[0], ADMIN[1])
        for view in REGISTERS:
            _goto(page, view)

            # 1. The page itself must not scroll sideways — the filter bar wraps.
            overflow = page.evaluate(
                "() => document.documentElement.scrollWidth - window.innerWidth")
            assert overflow <= 2, \
                f"{view}: page overflows horizontally by {overflow}px on mobile"

            # 2. Any data table is wrapped in a horizontal scroll container so its
            #    columns stay reachable instead of being clipped by the card.
            if page.locator("table").count() > 0:
                ox = page.evaluate(
                    "() => { const t = document.querySelector('table');"
                    " return t ? getComputedStyle(t.parentElement).overflowX : '' }")
                assert ox in ("auto", "scroll"), \
                    f"{view}: table not in a horizontal scroll container (overflowX={ox})"
    finally:
        ctx.close()
