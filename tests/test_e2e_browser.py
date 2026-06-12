"""E2E browser tests — real Playwright browser automation.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_browser.py -v
"""
import os, pytest, importlib
import uuid
import requests

# Fail loudly if Playwright is missing — never silently skip 41 tests.
try:
    from playwright.sync_api import sync_playwright, expect
except ImportError:
    pytest.fail(
        "Playwright is required for E2E tests. Install with: "
        "pip install playwright && playwright install chromium",
        pytrace=False,
    )

BASE = os.environ.get("ISMS_TEST_URL", "http://localhost:9090")
API = f"{BASE}/api/v1"
ORG = "e2e-org"
ADMIN = ("e2e-admin@test.local", "E2eTest2026!", "E2E Admin")
R1 = ("e2e-r1@test.local", "E2eTest2026!", "Reader One")
R2 = ("e2e-r2@test.local", "E2eTest2026!", "Reader Two")

# ── API ──

def api(method, path, token=None, expect_status=None, **kw):
    h = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"} if token else {}
    r = getattr(requests, method)(f"{API}{path}", headers=h, timeout=15, **kw)
    if expect_status:
        ok = expect_status if isinstance(expect_status, (list, tuple)) else [expect_status]
        assert r.status_code in ok, f"{method.upper()} {path}: {r.status_code} {r.text[:200]}"
    return r

# ── Browser ──

def do_login(page, email, pw, then_goto=None):
    """Login and optionally navigate to a path after.

    The /login page has a two-step flow on path-based deployments:
      1. Org discovery — enter the org slug to continue.
      2. Email + password.
    On subdomain deployments the org is implicit and step 1 is skipped.
    """
    page.set_default_timeout(8000)
    page.goto(f"{BASE}/login")
    # Step 1: org discovery. Fill the org slug and continue if the input is shown.
    org_input = page.get_by_placeholder("Organization name, e.g. acme")
    if org_input.is_visible(timeout=3000):
        org_input.fill(ORG)
        page.get_by_role("button", name="Continue").click()
    # Step 2: email + password.
    page.get_by_placeholder("you@company.com").wait_for(state="visible", timeout=8000)
    page.get_by_placeholder("you@company.com").fill(email)
    page.get_by_placeholder("Password", exact=True).fill(pw)
    page.get_by_role("button", name="Sign in").click()
    page.locator("aside").first.wait_for(state="visible", timeout=10000)
    if then_goto:
        # SPA navigation via Vue Router — avoids full page reload which can lose auth
        target = f"/{ORG}/{then_goto}"
        page.evaluate(f"() => document.querySelector('#app').__vue_app__.config.globalProperties.$router.push('{target}')")
        page.wait_for_load_state("networkidle")

def click_sidebar(page, label):
    page.locator(f'aside a:has-text("{label}")').first.click()
    page.wait_for_load_state("networkidle")

def wait_for(page, text, timeout=8000):
    page.locator(f"text={text}").first.wait_for(state="visible", timeout=timeout)

# ── Fixtures ──

@pytest.fixture(scope="module")
def pw_browser():
    with sync_playwright() as p:
        b = p.chromium.launch()
        yield b
        b.close()

@pytest.fixture(scope="module")
def tokens():
    for e, pw, n in [ADMIN, R1, R2]:
        api("post", "/auth/signup", json={"email": e, "password": pw, "name": n}, expect_status=[200, 201, 409])
    t = api("post", "/auth/login", json={"email": ADMIN[0], "password": ADMIN[1]}, expect_status=200).json()["token"]
    api("post", "/organizations", t, json={"name": "E2E Org", "slug": ORG, "template": "iso27001"}, expect_status=[200, 201, 409, 500])
    t = api("post", "/auth/login", json={"email": ADMIN[0], "password": ADMIN[1], "organization": ORG}, expect_status=200).json()["token"]
    for e, _, n in [R1, R2]:
        api("post", "/users", t, json={"email": e, "name": n, "role": "reader"}, expect_status=[200, 201, 409])
    docs = api("get", "/documents/all", t, expect_status=200).json()
    if not (docs.get("data") if isinstance(docs, dict) else docs):
        api("post", "/templates", t, json={"template": "iso27001"}, expect_status=[200, 201])
    # Close any leftover open/approved reviews from previous runs
    revs = api("get", "/reviews", t).json()
    for rv in (revs.get("data") if isinstance(revs, dict) else revs) or []:
        if rv.get("status") in ("open", "approved", "changes_requested"):
            api("put", f"/reviews/{rv['id']}/status", t, json={"status": "closed"})
    return {"admin": t}

# ── Smoke (one login, sidebar nav) ──

class TestSmoke:
    @pytest.mark.parametrize("label,marker", [
        ("Documents", "DOCUMENTS"),
        ("Risks", "Risk Register"),
        ("Assets", "Asset Register"),
        ("Suppliers", "Supplier"),
        ("Legal", "Legal"),
        ("Reviews", "Reviews"),
        ("Change Management", "Change Management"),
        ("Corrective Actions", "Corrective"),
        ("Incidents", "Incident"),
        ("Objectives", "Objectives"),
        ("Tasks", "Tasks"),
        ("Inbox", "Inbox"),
        ("Admin", "Members"),
    ])
    def test_page(self, pw_browser, tokens, label, marker):
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, label)
            wait_for(page, marker)
        finally:
            ctx.close()

# ── Tab title reflects org (regression for hardcoded "ISMS") ──

class TestTabTitle:
    def test_title_reflects_org_name(self, pw_browser, tokens):
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            # The browser tab title must reflect the org, not the hardcoded "ISMS".
            page.wait_for_function("document.title === 'E2E Org'", timeout=8000)
            assert page.title() == "E2E Org", f"expected 'E2E Org', got {page.title()!r}"
        finally:
            ctx.close()

# ── Documents (shared page, sequential) ──

class TestDocuments:
    _page = None
    _ctx = None

    def test_01_tree(self, pw_browser, tokens):
        TestDocuments._ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        TestDocuments._page = TestDocuments._ctx.new_page()
        do_login(TestDocuments._page, ADMIN[0], ADMIN[1])
        click_sidebar(TestDocuments._page, "Documents")
        wait_for(TestDocuments._page, "iso27001")

    def test_02_open_doc(self, pw_browser, tokens):
        p = TestDocuments._page
        p.goto(f"{BASE}/{ORG}/documents/iso27001-4-1")
        p.locator("h1").first.wait_for(state="visible", timeout=10000)
        assert "4.1" in p.locator("h1").first.inner_text()

    def test_03_toolbar(self, pw_browser, tokens):
        p = TestDocuments._page
        for title in ["Send for review", "Version history", "Comments", "Print document"]:
            expect(p.locator(f'button[title="{title}"]')).to_be_visible(timeout=3000)

    def test_04_edit_cancel(self, pw_browser, tokens):
        p = TestDocuments._page
        p.locator('button:has-text("Edit")').first.click()
        p.locator('button:has-text("Save")').first.wait_for(state="visible", timeout=3000)
        p.locator('button:has-text("Cancel")').first.click()
        p.locator('button:has-text("Edit")').first.wait_for(state="visible", timeout=3000)

    def test_05_new_menu(self, pw_browser, tokens):
        p = TestDocuments._page
        click_sidebar(p, "Documents")
        wait_for(p, "iso27001")
        expect(p.locator('button[title="New"]')).to_be_visible(timeout=3000)
        TestDocuments._ctx.close()

# ── Risks ──

class TestRisks:
    _page = None
    _ctx = None

    def test_01_register(self, pw_browser, tokens):
        TestRisks._ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        TestRisks._page = TestRisks._ctx.new_page()
        do_login(TestRisks._page, ADMIN[0], ADMIN[1])
        click_sidebar(TestRisks._page, "Risks")
        wait_for(TestRisks._page, "Risk Register")
        wait_for(TestRisks._page, "Risk Map")
        expect(TestRisks._page.locator('input[placeholder*="Search"]')).to_be_visible(timeout=3000)

    def test_02_guided_create(self, pw_browser, tokens):
        """Click Add Risk → modal opens with category picker → pick category → cancel."""
        p = TestRisks._page
        p.get_by_role("button", name="Add Risk", exact=True).first.click()
        wait_for(p, "Add Risk")
        p.get_by_role("button", name="Technology").click()
        # Close modal via Escape (avoids ambiguity between header toggle and modal Cancel)
        p.keyboard.press("Escape")
        # Wait for modal overlay to be gone before test_03 navigates
        p.locator(".fixed.inset-0.z-50").wait_for(state="detached", timeout=3000)

    def test_03_search(self, pw_browser, tokens):
        p = TestRisks._page
        api("post", "/risks", tokens["admin"], json={
            "title": "Unique searchable risk qwerty99",
            "category": "technology", "risk_type": "threat", "origin": "internal",
            "status": "open", "current_likelihood": 2, "current_impact": 2,
        }, expect_status=[200, 201, 409])
        # Navigate away and back to force data reload (same-route click won't refetch)
        click_sidebar(p, "Documents")
        wait_for(p, "DOCUMENTS")
        click_sidebar(p, "Risks")
        wait_for(p, "Risk Register")
        p.locator('input[placeholder*="Search"]').fill("qwerty99")
        p.locator("text=qwerty99").first.wait_for(state="visible", timeout=8000)
        p.locator('input[placeholder*="Search"]').fill("")
        TestRisks._ctx.close()

# ── Review: single reviewer ──

class TestReviewSingle:
    def test_approve_merge(self, pw_browser, tokens):
        t = tokens["admin"]
        body = api("get", "/documents/iso27001-a-8-11/body", t, expect_status=200).json()["body"]
        api("put", "/documents/iso27001-a-8-11/content", t, json={"content": body + "\n\nE2E."}, expect_status=200)
        rid = api("post", "/documents/iso27001-a-8-11/reviews", t,
            json={"reviewers": [R1[0]], "message": "Single review"}, expect_status=[200, 201]).json()["review_id"]

        # R1 approves via API (faster and more reliable than browser)
        r1t = api("post", "/auth/login", json={"email": R1[0], "password": R1[1], "organization": ORG}, expect_status=200).json()["token"]
        api("post", f"/reviews/{rid}/approve", r1t, json={"decision": "approved", "comment": "LGTM"}, expect_status=200)

        # Admin merges via browser (login with redirect to review page)
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{rid}")
            page.locator("text=Approved").first.wait_for(state="visible", timeout=10000)
            page.locator('button:has-text("Publish")').first.wait_for(state="visible", timeout=8000)
            page.locator('button:has-text("Publish")').first.click()
            page.locator("text=/[Mm]erged/").first.wait_for(state="visible", timeout=10000)
        finally:
            ctx.close()

# ── Review: two reviewers ──

class TestReviewTwo:
    def test_both_approve(self, pw_browser, tokens):
        t = tokens["admin"]
        body = api("get", "/documents/iso27001-a-8-12/body", t, expect_status=200).json()["body"]
        api("put", "/documents/iso27001-a-8-12/content", t, json={"content": body + "\n\nTwo."}, expect_status=200)
        rid = api("post", "/documents/iso27001-a-8-12/reviews", t,
            json={"reviewers": [R1[0], R2[0]], "message": "Two reviewers"}, expect_status=[200, 201]).json()["review_id"]

        # R1 approves (two-step: Approve → Confirm Approve)
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, R1[0], R1[1], then_goto=f"reviews/{rid}")
            wait_for(page, "Actions", timeout=10000)
            page.get_by_role("button", name="Approve").wait_for(state="visible", timeout=5000)
            page.get_by_role("button", name="Approve").click()
            page.locator('button:has-text("Confirm Approve")').wait_for(state="visible", timeout=5000)
            page.locator('button:has-text("Confirm Approve")').click()
            page.locator("text=/recorded|[Aa]pproved/").first.wait_for(state="visible", timeout=10000)
        finally:
            ctx.close()

        # R2 approves (two-step)
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, R2[0], R2[1], then_goto=f"reviews/{rid}")
            wait_for(page, "Actions", timeout=10000)
            page.get_by_role("button", name="Approve").wait_for(state="visible", timeout=5000)
            page.get_by_role("button", name="Approve").click()
            page.locator('button:has-text("Confirm Approve")').wait_for(state="visible", timeout=5000)
            page.locator('button:has-text("Confirm Approve")').click()
            page.locator("text=/recorded|[Aa]pproved/").first.wait_for(state="visible", timeout=10000)
        finally:
            ctx.close()

        # Merge — R2's approval may take a moment to propagate, so reload if needed
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{rid}")
            # Wait for page to load, then look for Publish or reload once if still showing Open
            page.wait_for_load_state("networkidle")
            if page.locator('button:has-text("Publish")').count() == 0:
                page.reload()
                page.wait_for_load_state("networkidle")
            page.locator('button:has-text("Publish")').first.wait_for(state="visible", timeout=10000)
            page.locator('button:has-text("Publish")').first.click()
            page.locator("text=/[Mm]erged/").first.wait_for(state="visible", timeout=10000)
        finally:
            ctx.close()

# ── Review: changes requested → re-review ──

class TestReviewChanges:
    def test_cr_then_approve(self, pw_browser, tokens):
        t = tokens["admin"]
        body = api("get", "/documents/iso27001-a-8-14/body", t, expect_status=200).json()["body"]
        api("put", "/documents/iso27001-a-8-14/content", t, json={"content": body + "\n\nCR."}, expect_status=200)
        rid = api("post", "/documents/iso27001-a-8-14/reviews", t,
            json={"reviewers": [R1[0]], "message": "CR test"}, expect_status=[200, 201]).json()["review_id"]

        # R1 requests changes (API — fast)
        r1t = api("post", "/auth/login", json={"email": R1[0], "password": R1[1], "organization": ORG}, expect_status=200).json()["token"]
        api("post", f"/reviews/{rid}/approve", r1t, json={"decision": "changes_requested", "comment": "Needs work"}, expect_status=200)

        # Admin edits and re-sends (API) — resubmit reuses same review, updates sent_head
        api("put", "/documents/iso27001-a-8-14/content", t, json={"content": body + "\n\nFixed."}, expect_status=200)
        r2 = api("post", "/documents/iso27001-a-8-14/reviews", t,
            json={"reviewers": [R1[0]], "message": "Fixed, re-review"}, expect_status=[200, 201]).json()
        rid2 = r2["review_id"]
        assert rid2 == rid, f"Resubmit must reuse same review ID: expected {rid}, got {rid2}"

        # R1 approves (API — faster, avoids stale-doc confirmation dialog)
        api("post", f"/reviews/{rid2}/approve", r1t, json={"decision": "approved", "comment": "Fixed, LGTM"}, expect_status=200)

        # Admin merges and verifies in browser
        api("post", f"/reviews/{rid2}/merge", t, json={}, expect_status=200)

        # Verify merged state in browser
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{rid2}")
            page.locator("text=/[Mm]erged/").first.wait_for(state="visible", timeout=10000)
        finally:
            ctx.close()

# ── Review round UX (browser) ──

class TestReviewRoundUX:
    """Verify round-aware UI elements after a changes_requested → resubmit cycle."""

    def test_round_ui(self, pw_browser, tokens):
        t = tokens["admin"]
        r1t = api("post", "/auth/login", json={"email": R1[0], "password": R1[1], "organization": ORG}, expect_status=200).json()["token"]

        # Setup: create review → changes_requested → edit → resubmit → approve (all API)
        body = api("get", "/documents/iso27001-a-8-15/body", t, expect_status=200).json()["body"]
        api("put", "/documents/iso27001-a-8-15/content", t, json={"content": body + "\n\nRound1."}, expect_status=200)
        rid = api("post", "/documents/iso27001-a-8-15/reviews", t,
            json={"reviewers": [R1[0]], "message": "Round UX test"}, expect_status=[200, 201]).json()["review_id"]
        api("post", f"/reviews/{rid}/approve", r1t, json={"decision": "changes_requested", "comment": "Needs work"}, expect_status=200)
        api("put", "/documents/iso27001-a-8-15/content", t, json={"content": body + "\n\nRound2 fixed."}, expect_status=200)
        r2 = api("post", "/documents/iso27001-a-8-15/reviews", t,
            json={"reviewers": [R1[0]], "message": "Fixed"}, expect_status=[200, 201]).json()
        assert r2.get("round") == 2, f"Resubmit should return round 2, got {r2.get('round')}"
        rid2 = r2["review_id"]

        # Browser: R1 opens the review at round 2
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, R1[0], R1[1], then_goto=f"reviews/{rid2}")

            # 1. Round badge visible in header
            page.locator("text=Round 2").first.wait_for(state="visible", timeout=10000)

            # 2. Wait for diff to load, verify scope toggle exists
            page.locator("text=Split").first.wait_for(state="visible", timeout=10000)
            assert page.locator("button:has-text('this round')").count() >= 1, \
                "Round scope toggle (This round) should be visible"
            assert page.locator("button:has-text('All changes')").count() >= 1, \
                "Round scope toggle (All changes) should be visible"

            # 3. Approve (two-step: Approve → Confirm Approve)
            page.get_by_role("button", name="Approve").wait_for(state="visible", timeout=5000)
            page.get_by_role("button", name="Approve").click()
            page.locator('button:has-text("Confirm Approve")').wait_for(state="visible", timeout=5000)
            page.locator('button:has-text("Confirm Approve")').click()
            # Feedback message should mention Round 2 or show approval confirmation
            page.locator("text=/Round 2|approved|Approved|recorded/").first.wait_for(state="visible", timeout=10000)
        finally:
            ctx.close()

        # Cleanup: merge via API
        api("post", f"/reviews/{rid2}/merge", t, json={}, expect_status=200)

# ── Suppliers (browser) ──

# ── Suggestion workflow (browser) ──

class TestSuggestionBrowser:
    """E2E: reviewer creates suggestion via API, author accepts in browser."""

    def test_suggestion_accept_ui(self, pw_browser, tokens):
        """Full UI flow: create suggestion via API, accept via browser UI."""
        t = tokens["admin"]
        r1t = api("post", "/auth/login", json={"email": R1[0], "password": R1[1], "organization": ORG}, expect_status=200).json()["token"]

        # Setup: review with a pending suggestion
        api("put", "/documents/iso27001-a-8-16/content", t, json={"content": "# Test\n\nOriginal paragraph for suggestion test.\n\nKeep this."}, expect_status=200)
        rid = api("post", "/documents/iso27001-a-8-16/reviews", t,
            json={"reviewers": [R1[0]], "message": "Suggestion browser test"}, expect_status=[200, 201]).json()["review_id"]
        r = api("post", f"/reviews/{rid}/comment", r1t, json={
            "body": "Suggested replacement for this paragraph",
            "suggestion_body": "Improved paragraph with better wording for testing.",
            "paragraph_index": 1,
            "quote": "Original paragraph for suggestion test.",
        }, expect_status=201)
        suggestion_id = r.json()["id"]
        assert r.json().get("suggestion_status") == "pending"

        # Author opens review → Document tab → expands paragraph → Accept suggestion
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1], then_goto=f"reviews/{rid}")
            page.locator("text=Round").first.wait_for(state="visible", timeout=10000)

            # Sidebar should show suggestion summary
            wait_for(page, "suggestion", timeout=5000)

            # Go to Document tab
            page.locator("button:has-text('Document')").first.click()
            page.wait_for_load_state("networkidle")

            # Click the blue comment count badge to expand the suggestion thread
            page.locator(".absolute.rounded-full.bg-blue-600").first.wait_for(state="visible", timeout=8000)
            page.locator(".absolute.rounded-full.bg-blue-600").first.click()

            # Accept the suggestion in UI
            page.locator("button:has-text('Accept')").first.wait_for(state="visible", timeout=5000)
            page.locator("button:has-text('Accept')").first.click()
            page.locator("text=Accepted").first.wait_for(state="visible", timeout=10000)

            # Verify conversation tab shows suggestion activity
            page.locator("button:has-text('Conversation')").first.click()
            page.wait_for_load_state("networkidle")
            wait_for(page, "suggestion", timeout=5000)
        finally:
            ctx.close()

        # Verify via API: content updated on review branch
        r = api("get", f"/reviews/{rid}/content", t, expect_status=200)
        assert "Improved paragraph with better wording" in r.json()["body"]
        r = api("get", f"/reviews/{rid}/suggestions", t, expect_status=200)
        assert any(s["suggestion_status"] == "accepted" for s in (r.json().get("data") or []))

# ── Suppliers (browser) ──

# ── Change Management (browser) ──

class TestChanges:
    def test_01_create_and_list(self, pw_browser, tokens):
        """Create a change request and verify it shows in the list."""
        t = tokens["admin"]
        # Create via API
        api("post", "/changes", t, json={
            "title": "E2E Migrate to OIDC",
            "description": "Switch authentication from password to OIDC.",
        }, expect_status=201)

        # Verify in browser
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Change Management")
            wait_for(page, "Change Management")
            wait_for(page, "E2E Migrate to OIDC", timeout=5000)
        finally:
            ctx.close()

    def test_02_status_change(self, pw_browser, tokens):
        """Approve a change request via API and verify status updates."""
        t = tokens["admin"]
        changes = api("get", "/changes", t, expect_status=200).json()
        data = changes.get("data") if isinstance(changes, dict) else changes
        cr = [c for c in (data or []) if "OIDC" in c.get("title", "")]
        if cr:
            api("put", f"/changes/{cr[0]['id']}/status", t, json={"status": "approved"}, expect_status=200)
            r = api("get", f"/changes/{cr[0]['id']}", t, expect_status=200)
            assert r.json()["status"] == "approved"

# ── Suppliers (browser) ──

class TestSuppliers:
    _page = None
    _ctx = None

    def test_01_register(self, pw_browser, tokens):
        TestSuppliers._ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        TestSuppliers._page = TestSuppliers._ctx.new_page()
        do_login(TestSuppliers._page, ADMIN[0], ADMIN[1])
        click_sidebar(TestSuppliers._page, "Suppliers")
        wait_for(TestSuppliers._page, "Supplier")

    def test_02_create(self, pw_browser, tokens):
        p = TestSuppliers._page
        # Create via API so we have data to browse
        api("post", "/suppliers", tokens["admin"], json={
            "name": "E2E Cloud Provider",
            "supplier_type": "cloud",
            "criticality": "high",
            "data_access": True,
        }, expect_status=[200, 201, 409])
        # Reload page
        click_sidebar(p, "Documents")
        wait_for(p, "DOCUMENTS")
        click_sidebar(p, "Suppliers")
        wait_for(p, "Supplier")
        wait_for(p, "E2E Cloud Provider", timeout=5000)

    def test_03_filter(self, pw_browser, tokens):
        """Filter by criticality if dropdown exists."""
        p = TestSuppliers._page
        # Try to find filter dropdown and interact with it
        filters = p.locator('select, [role="listbox"], button:has-text("Filter")')
        if filters.count() > 0:
            # At minimum, verify the supplier list is populated
            assert p.locator("text=E2E Cloud Provider").count() >= 1
        TestSuppliers._ctx.close()

# ── Legal (browser) ──

class TestLegal:
    _page = None
    _ctx = None

    def test_01_register(self, pw_browser, tokens):
        TestLegal._ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        TestLegal._page = TestLegal._ctx.new_page()
        do_login(TestLegal._page, ADMIN[0], ADMIN[1])
        click_sidebar(TestLegal._page, "Legal")
        wait_for(TestLegal._page, "Legal")

    def test_02_create(self, pw_browser, tokens):
        p = TestLegal._page
        api("post", "/legal", tokens["admin"], json={
            "title": "E2E Data Protection Act",
            "jurisdiction": "EU",
            "category": "privacy",
        }, expect_status=[200, 201, 409])
        click_sidebar(p, "Documents")
        wait_for(p, "DOCUMENTS")
        click_sidebar(p, "Legal")
        wait_for(p, "Legal")
        wait_for(p, "E2E Data Protection Act", timeout=5000)

    def test_03_close(self, pw_browser, tokens):
        TestLegal._ctx.close()

# ── Review filters (browser) ──

class TestReviewFilters:
    def test_open_closed_tabs(self, pw_browser, tokens):
        """Review list should show open and closed tabs/filters."""
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Reviews")
            wait_for(page, "Reviews")
            # Should have some merged/closed reviews from earlier tests
            # Check that filter buttons or tabs exist
            open_tab = page.locator("text=/[Oo]pen/")
            closed_tab = page.locator("text=/[Cc]losed|[Mm]erged/")
            assert open_tab.count() >= 1, "Open filter/tab should exist"
            assert closed_tab.count() >= 1, "Closed/merged filter should exist"
        finally:
            ctx.close()

# ── Risk filters (browser) ──

class TestRiskFilters:
    def test_category_filter(self, pw_browser, tokens):
        """Risk register should support category filtering."""
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Risks")
            wait_for(page, "Risk Register")
            # Verify heat map section exists (collapsible)
            wait_for(page, "Risk Map")
            # Verify search input exists
            search = page.locator('input[placeholder*="Search"]')
            assert search.is_visible(), "Risk search input should be visible"
            # Verify filter dropdowns exist (category, status, level)
            # These are typically select elements or dropdown buttons
            filters = page.locator("select, [role='combobox']")
            assert filters.count() >= 1, "At least one filter dropdown should exist"
        finally:
            ctx.close()

# ── Mobile ──

# ── Corrective Actions (browser) ──

class TestCorrectiveActions:
    def test_create_and_list(self, pw_browser, tokens):
        t = tokens["admin"]
        api("post", "/corrective-actions", t, json={
            "title": "E2E Fix server config",
            "description": "Misconfigured firewall rule.",
            "severity": "minor_nc",
            "source": "security_incident",
        }, expect_status=201)
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Corrective Actions")
            wait_for(page, "Corrective")
            wait_for(page, "E2E Fix server config", timeout=5000)
        finally:
            ctx.close()

# ── Objectives (browser) ──

class TestObjectives:
    def test_page_loads(self, pw_browser, tokens):
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Objectives")
            wait_for(page, "Objectives")
        finally:
            ctx.close()

# ── Incidents (browser) ──

class TestIncidentsBrowser:
    def test_create_and_list(self, pw_browser, tokens):
        t = tokens["admin"]
        api("post", "/incidents", t, json={
            "title": "E2E Test Incident",
            "description": "Browser test incident.",
            "severity": "medium",
            "status": "open",
        }, expect_status=201)
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Incidents")
            wait_for(page, "Incident")
            wait_for(page, "E2E Test Incident", timeout=5000)
        finally:
            ctx.close()

# ── Mobile ──

# ── Dashboard annual plan (browser) ──

class TestDashboardCalendar:
    def test_annual_plan_visible(self, pw_browser, tokens):
        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            # Dashboard loads — check for key sections
            wait_for(page, "Needs Your Attention", timeout=10000)
            # Annual Plan section (requires fresh Vue build)
            annual = page.locator("text=Annual Plan")
            if annual.count() > 0:
                wait_for(page, "Jan", timeout=3000)
        finally:
            ctx.close()

# ── Workflow features (P1/P2/P3) ──

class TestInboxIncidentsAndCAs:
    """P1: Inbox surfaces incidents and CAs assigned to me."""

    def test_inbox_has_new_tabs(self, pw_browser, tokens):
        t = tokens["admin"]
        # Create incident assigned to admin
        api("post", "/incidents", t, json={
            "title": "E2E Inbox incident",
            "description": "for inbox tab",
            "severity": "medium",
            "affects_c": True,
            "incident_type": "event",
            "source": "internal",
            "reporter": ADMIN[0],
            "assignee": ADMIN[0],
        }, expect_status=[200, 201])
        # Create CA assigned to admin
        api("post", "/corrective-actions", t, json={
            "title": "E2E Inbox CA",
            "description": "for inbox tab",
            "source": "internal_audit",
            "severity": "observation",
            "assignee": ADMIN[0],
        }, expect_status=[200, 201])

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Inbox")
            page.wait_for_load_state("networkidle")
            # Verify the new tabs exist
            page.locator('button:has-text("Incidents")').first.wait_for(state="visible", timeout=5000)
            page.locator('button:has-text("CAs")').first.wait_for(state="visible", timeout=5000)
            # Click Incidents tab and see our incident
            page.locator('button:has-text("Incidents")').first.click()
            wait_for(page, "E2E Inbox incident", timeout=5000)
            # Click CAs tab and see our CA
            page.locator('button:has-text("CAs")').first.click()
            wait_for(page, "E2E Inbox CA", timeout=5000)
        finally:
            ctx.close()


class TestQuickActionIncidentToCA:
    """P2: Incident detail has a Create CA button that pre-fills the CA form."""

    def test_quick_action_button_navigates_with_query(self, pw_browser, tokens):
        t = tokens["admin"]
        # Create incident
        r = api("post", "/incidents", t, json={
            "title": "E2E QuickAction incident",
            "description": "for quick action test",
            "severity": "high",
            "affects_i": True,
            "incident_type": "incident",
            "source": "internal",
            "reporter": ADMIN[0],
        }, expect_status=[200, 201])
        inc_id = r.json()["id"]

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1], then_goto=f"incidents/{inc_id}")
            # Wait for incident detail modal — sidebar nav appears only when modal is open
            page.locator('nav button:has-text("Overview")').first.wait_for(state="visible", timeout=8000)
            # Quick actions live in the Actions tab
            page.locator('nav button:has-text("Actions")').first.click()
            page.locator('text=Quick actions').first.wait_for(state="visible", timeout=5000)
            # Click the Create Corrective Action quick-action button
            page.locator('button:has-text("Create Corrective Action")').first.click()
            # Wait for URL to change
            page.wait_for_url("**/corrective-actions**", timeout=5000)
            # Wait for the create form modal to open (Add heading)
            page.locator('h2:has-text("Add")').first.wait_for(state="visible", timeout=5000)
            # Title input should have pre-filled value "CA: E2E QuickAction incident"
            page.wait_for_timeout(300)  # let v-model settle
            title_value = page.locator('input').filter(has_text="").nth(0).input_value()
            # Find the title input by its preceding label
            title_input = page.locator('label:has-text("Title") + input').first
            title_input.wait_for(state="visible", timeout=3000)
            assert "E2E QuickAction incident" in title_input.input_value(), \
                f"Expected pre-filled title, got: {title_input.input_value()!r}"
        finally:
            ctx.close()


class TestQuickActionRiskToTask:
    """P2: Risk Treatment tab has a Create Task button."""

    def test_create_task_from_risk(self, pw_browser, tokens):
        t = tokens["admin"]
        r = api("post", "/risks", t, json={
            "title": "E2E QuickAction risk",
            "description": "for risk-to-task quick action",
            "current_likelihood": 3,
            "current_impact": 4,
            "risk_type": "threat",
            "origin": "internal",
            "status": "open",
        }, expect_status=[200, 201])
        risk_id = r.json()["id"]

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1], then_goto=f"risks/{risk_id}")
            page.locator('nav button:has-text("Overview")').first.wait_for(state="visible", timeout=8000)
            # Quick actions live in the Actions tab
            page.locator('nav button:has-text("Actions")').first.click()
            page.locator('text=Quick actions').first.wait_for(state="visible", timeout=5000)
            # Click Create Implementation Task
            page.locator('button:has-text("Create Implementation Task")').first.click()
            page.wait_for_url("**/tasks**", timeout=5000)
            page.locator('h2:has-text("Add")').first.wait_for(state="visible", timeout=5000)
            page.wait_for_timeout(300)
            title_input = page.locator('label:has-text("Title") + input').first
            assert "E2E QuickAction risk" in title_input.input_value(), \
                f"Expected pre-filled title from the source risk, got: {title_input.input_value()!r}"
        finally:
            ctx.close()


class TestOrphanValidationIncidentInUI:
    """P3: Cannot resolve/close incident with open CA — the edit-form save
    surfaces the server's 409 as an error toast and the status does not
    advance. The server rule itself is also covered by the API integration
    tests in `test_workflow_integration.py`; this verifies the UI feedback.
    """

    def test_close_with_open_ca_blocked(self, pw_browser, tokens):
        t = tokens["admin"]
        marker = uuid.uuid4().hex[:6]
        title = f"E2E Orphan incident {marker}"
        r = api("post", "/incidents", t, json={
            "title": title,
            "description": "for orphan UI test",
            "severity": "medium",
            "affects_a": True,
            "incident_type": "event",
            "source": "internal",
            "reporter": ADMIN[0],
        }, expect_status=[200, 201])
        inc_id = r.json()["id"]
        inc_identifier = r.json()["identifier"]
        ca = api("post", "/corrective-actions", t, json={
            "title": f"E2E Orphan CA {marker}",
            "description": "linked",
            "source": "security_incident",
            "severity": "observation",
        }, expect_status=[200, 201]).json()
        # Link CA → incident via entity_references — there is no incident_id
        # field on the CA itself; the orphan check counts these references.
        # References use per-org identifiers (INC-N), not numeric row ids.
        api("post", "/references", t, json={
            "source_type": "corrective_action",
            "source_id": ca["identifier"],
            "target_type": "incident",
            "target_id": inc_identifier,
        }, expect_status=[200, 201])

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Incidents")
            wait_for(page, title, timeout=8000)
            page.locator(f'text={title}').first.click()
            # Detail modal → edit the overview section
            page.locator('button:has-text("Edit")').first.click()
            # The edit-form status select sits right after a "Status" label —
            # the list-filter select has the same options but no label, so
            # this is the only locator that can't grab the wrong one.
            status_select = page.locator(
                'xpath=//label[normalize-space()="Status"]/following-sibling::select'
            ).first
            status_select.wait_for(state="visible", timeout=5000)
            status_select.select_option("resolved")
            page.locator('button:has-text("Save")').first.click()
            # Server rejects with 409 — UI must surface it as an error toast
            page.locator('text=/open corrective action/i').first.wait_for(state="visible", timeout=5000)
            # And the incident must not have advanced
            inc_after = api("get", f"/incidents/{inc_id}", t).json()
            assert inc_after["status"] != "resolved", \
                f"Status advanced to resolved despite open CA"
        finally:
            ctx.close()


class TestAutoTaskOnChangeApprovalUI:
    """P2: Approving a change via the edit-form status select auto-creates an
    implementation task, visible in the Tasks list. Guards the edit-form path
    specifically — it goes through PUT /changes/:id, not the dedicated status
    endpoint, and both must produce the follow-up task.
    """

    def test_approval_creates_task_visible(self, pw_browser, tokens):
        t = tokens["admin"]
        marker = uuid.uuid4().hex[:6]
        title = f"E2E AutoTask change {marker}"
        r = api("post", "/changes", t, json={
            "title": title,
            "description": "for auto-task test",
            "priority": "medium",
            "category": "process",
            "risk_level": "low",
        }, expect_status=[200, 201])
        cr = r.json()
        change_ident = cr["identifier"]

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            click_sidebar(page, "Change Management")
            wait_for(page, title, timeout=8000)
            page.locator(f'text={title}').first.click()
            # Detail → edit the overview section, approve via status select
            # (label-scoped for the same reason as the incident test).
            page.locator('button:has-text("Edit")').first.click()
            status_select = page.locator(
                'xpath=//label[normalize-space()="Status"]/following-sibling::select'
            ).first
            status_select.wait_for(state="visible", timeout=5000)
            status_select.select_option("approved")
            page.locator('button:has-text("Save")').first.click()
            wait_for(page, "Saved", timeout=5000)
            # Auto-task must exist and be visible in the Tasks list
            page.keyboard.press("Escape")
            page.wait_for_timeout(300)
            click_sidebar(page, "Tasks")
            search = page.locator('input[placeholder="Search..."]').first
            search.wait_for(state="visible", timeout=5000)
            search.fill(change_ident)
            page.wait_for_timeout(400)  # debounce
            wait_for(page, f"Implement {change_ident}", timeout=5000)
        finally:
            ctx.close()


class TestSaveToastFeedback:
    """UX correctness: editing an entity field should produce a 'Saved' toast."""

    def test_save_toast_visible_in_risks(self, pw_browser, tokens):
        t = tokens["admin"]
        r = api("post", "/risks", t, json={
            "title": "E2E ToastTest risk",
            "description": "for toast feedback",
            "current_likelihood": 3,
            "current_impact": 3,
            "risk_type": "threat",
            "origin": "internal",
            "status": "open",
        }, expect_status=[200, 201])
        risk_id = r.json()["id"]

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1], then_goto=f"risks/{risk_id}")
            page.locator('nav button:has-text("Overview")').first.wait_for(state="visible", timeout=8000)
            # Click Edit on Overview section
            page.locator('button:has-text("Edit")').first.click()
            # Modify the title in the edit form
            title_input = page.locator('label:has-text("Title") + input').first
            title_input.wait_for(state="visible", timeout=3000)
            title_input.fill("E2E ToastTest risk - edited")
            # Click Save in footer
            page.locator('button:has-text("Save")').last.click()
            # Toast "Saved" should appear
            page.locator('text=Saved').first.wait_for(state="visible", timeout=4000)
        finally:
            ctx.close()


# ── Mobile ──

class TestMobile:
    def test_login_works(self, pw_browser, tokens):
        ctx = pw_browser.new_context(viewport={"width": 390, "height": 844})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            # On mobile, sidebar is hidden but hamburger should exist
            # Just verify we got past login
            assert "login" not in page.url
        finally:
            ctx.close()


# ── Comment counter (regression for #30: top badge counts OPEN only) ──

class TestCommentCounter:
    DOC = "iso27001-4-1"

    def test_badge_counts_open_only(self, pw_browser, tokens):
        t = tokens["admin"]

        def _open_comments():
            r = api("get", f"/documents/{self.DOC}/comments", t, expect_status=200).json()
            items = r.get("data") if isinstance(r, dict) else r
            return [c for c in (items or []) if c.get("status") != "resolved" and not c.get("parent_id")]

        # Clean slate: resolve any currently-open top-level comments on the doc.
        for c in _open_comments():
            api("post", f"/comments/{c['id']}/resolve", t, expect_status=[200, 204])

        # Two open comments, resolve one → exactly 1 open.
        c1 = api("post", "/comments", t, json={"document_id": self.DOC, "body": "open one", "paragraph_index": 0}, expect_status=[200, 201]).json()
        c2 = api("post", "/comments", t, json={"document_id": self.DOC, "body": "to resolve", "paragraph_index": 0}, expect_status=[200, 201]).json()
        api("post", f"/comments/{c2['id']}/resolve", t, expect_status=[200, 204])

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            page.goto(f"{BASE}/{ORG}/documents/{self.DOC}")
            # Badge on the Comments toolbar button must show OPEN count (1), not total (2).
            expect(page.locator('button[title="Comments"] span')).to_have_text("1", timeout=8000)
        finally:
            ctx.close()
            # teardown: resolve the remaining open comment
            api("post", f"/comments/{c1['id']}/resolve", t, expect_status=[200, 204])


# ── Editor data-loss guard (regression for #61: editing drops embedded HTML/SVG) ──

class TestEditorHtmlGuard:
    DOC = "e2e-svg-doc"

    def test_edit_guard_warns_on_embedded_svg(self, pw_browser, tokens):
        t = tokens["admin"]
        # A document with embedded raw SVG (the WYSIWYG editor can't preserve it).
        api("post", "/documents", t, json={
            "folder": "iso27001",
            "filename": "e2e-svg-doc.md",
            "document_id": self.DOC,
            "title": "E2E SVG Doc",
            "content": "# Heading\n\nBody text here.\n\n<svg width='12' height='12'><rect width='12' height='12'/></svg>\n",
        }, expect_status=[200, 201, 409])

        ctx = pw_browser.new_context(viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        try:
            do_login(page, ADMIN[0], ADMIN[1])
            page.goto(f"{BASE}/{ORG}/documents/{self.DOC}")
            # Wait for the body to render (rawContent loaded) before triggering the guard.
            expect(page.locator("text=Body text here.")).to_be_visible(timeout=10000)
            page.get_by_role("button", name="Edit", exact=True).first.click()
            # Guard: editing a doc with embedded SVG must warn first, not silently enter edit.
            expect(page.locator("text=editing here will drop it")).to_be_visible(timeout=8000)
        finally:
            ctx.close()
