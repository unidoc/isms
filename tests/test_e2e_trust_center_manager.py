"""E2E (#520): the trust center manager screen, one test per acceptance criterion.

The API itself (validation, roles, order, guard) is covered by the Go and pytest
API tests; this file covers what only a browser shows: the sidebar entry, the
not-allowed screen, the confirmation dialogs, the order on the public page and
the status of each row.

The e2e organization is shared with other e2e files. Each test restores the trust
center state it touched in a finally block: the trust center is switched off and
the items it published are unpublished. The documents it creates (unique ids per
run) and the contributor user stay in the organization; they are not removed.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_trust_center_manager.py -v
"""
import uuid

import pytest
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, R1, BASE, ORG, pw_browser, tokens  # noqa: F401

CONTRIB = ("e2e-tc-contrib@test.local", "E2eTest2026!", "TC Contributor")
NOT_ALLOWED = "Only admins and managers can manage the trust center."


def _page(pw_browser, creds, width=1440):
    ctx = pw_browser.new_context(viewport={"width": width, "height": 900})
    p = ctx.new_page()
    do_login(p, creds[0], creds[1])
    return ctx, p


@pytest.fixture
def admin_page(pw_browser, tokens):
    ctx, p = _page(pw_browser, ADMIN)
    yield p
    ctx.close()


@pytest.fixture(scope="module")
def reviewer(tokens):
    """R1 (a reader) approves the documents: an assigned reviewer may approve in any role."""
    t = api("post", "/auth/login", json={"email": R1[0], "password": R1[1], "organization": ORG}, expect_status=200).json()["token"]
    return {"email": R1[0], "token": t}


def new_doc(admin_token, reviewer, title, classification=None, approve=True):
    doc_id = f"e2e-tc-{uuid.uuid4().hex[:8]}"
    api("post", "/documents", admin_token, expect_status=201, json={
        "folder": "e2e-trust", "filename": f"{doc_id}.md", "document_id": doc_id, "title": title,
        "content": f"# {title}\n\nText.\n",
    })
    if classification:
        api("put", f"/documents/{doc_id}/metadata", admin_token, expect_status=200,
            json={"fields": {"classification": classification}})
    if approve:
        r = api("post", f"/documents/{doc_id}/reviews", admin_token, expect_status=201,
                json={"reviewers": [reviewer["email"]], "message": "e2e trust center"})
        rid = r.json()["review_id"]
        api("post", f"/reviews/{rid}/approve", reviewer["token"], expect_status=200, json={"decision": "approved"})
        api("post", f"/reviews/{rid}/merge", admin_token, expect_status=200, json={})
    return doc_id


def set_enabled(admin_token, enabled):
    api("put", "/trust-center/settings", admin_token, expect_status=200, json={"enabled": enabled})


def cleanup(admin_token, doc_ids):
    set_enabled(admin_token, False)
    for d in doc_ids:
        api("delete", f"/documents/{d}/trust-center", admin_token)  # 404 when never published


def item_for(admin_token, doc_id):
    for it in api("get", "/trust-center/items", admin_token, expect_status=200).json():
        if it["document"] and it["document"]["document_id"] == doc_id:
            return it
    return None


def open_manager(page, tab=None):
    page.goto(f"{BASE}/{ORG}/trust-center" + (f"?tab={tab}" if tab else ""))
    expect(page.locator('[data-trust-tab="content"]')).to_be_visible(timeout=15000)


class TestTrustCenterManager:

    def test_reader_and_contributor_have_no_entry_and_a_not_allowed_screen(self, pw_browser, tokens):
        admin_token = tokens["admin"]
        api("post", "/auth/signup", json={"email": CONTRIB[0], "password": CONTRIB[1], "name": CONTRIB[2]}, expect_status=[200, 201, 409])
        api("post", "/users", admin_token, json={"email": CONTRIB[0], "name": CONTRIB[2], "role": "contributor"}, expect_status=[200, 201, 409])
        for creds in (R1, CONTRIB):
            ctx, page = _page(pw_browser, creds)
            try:
                requested = []
                page.on("request", lambda r: requested.append(r.url) if "/api/v1/trust-center" in r.url else None)
                # The org slug under the org name comes from the loaded user record; wait
                # for it, so "no entry" is not checked before the role is known.
                expect(page.locator("aside").get_by_text(ORG, exact=True)).to_be_visible(timeout=15000)
                expect(page.locator('aside a[data-nav="trust-center"]')).to_have_count(0)
                page.goto(f"{BASE}/{ORG}/trust-center")
                expect(page.get_by_text(NOT_ALLOWED)).to_be_visible(timeout=15000)
                page.wait_for_timeout(500)
                assert requested == [], f"{creds[0]} made trust center requests: {requested}"
            finally:
                ctx.close()

    def test_admin_sees_the_entry_and_the_header(self, admin_page, tokens):
        page = admin_page
        expect(page.locator('aside a[data-nav="trust-center"]')).to_be_visible()
        set_enabled(tokens["admin"], False)
        try:
            page.locator('aside a[data-nav="trust-center"]').click()
            expect(page.locator('[data-trust="address"]')).to_have_text(f"{BASE}/{ORG}/trust", timeout=15000)
            # D2: off means no preview.
            expect(page.locator('[data-trust="preview"]')).to_be_disabled()
            expect(page.get_by_text("Turn the trust center on to preview it").first).to_be_visible()
            page.locator('[data-trust-tab="settings"]').click()
            assert "tab=settings" in page.url
            page.reload()
            expect(page.locator('[data-trust="settings-section"]')).to_be_visible(timeout=15000)
        finally:
            set_enabled(tokens["admin"], False)

    def test_turning_on_requires_the_confirmation_and_cancel_leaves_it_off(self, admin_page, tokens):
        t = tokens["admin"]
        page = admin_page
        set_enabled(t, False)
        try:
            open_manager(page, "settings")
            toggle = page.locator('[data-trust="toggle-enabled"]')
            toggle.click()
            dialog = page.locator('[data-trust="enable-dialog"]')
            expect(dialog).to_contain_text("Anything published will be visible to anyone on the internet.")
            dialog.locator('[data-trust="enable-cancel"]').click()
            expect(dialog).to_have_count(0)
            assert api("get", "/trust-center/settings", t, expect_status=200).json()["enabled"] is False

            toggle.click()
            page.locator('[data-trust="enable-confirm"]').click()
            expect(toggle).to_have_attribute("aria-checked", "true", timeout=8000)
            assert api("get", "/trust-center/settings", t, expect_status=200).json()["enabled"] is True
            # Preview is now a link to the public address.
            expect(page.locator('a[data-trust="preview"]')).to_have_attribute("href", f"{BASE}/{ORG}/trust")
        finally:
            set_enabled(t, False)

    def test_confidential_as_public_asks_and_as_gated_does_not(self, admin_page, tokens, reviewer):
        t = tokens["admin"]
        page = admin_page
        secret = new_doc(t, reviewer, f"E2E Secret {uuid.uuid4().hex[:4]}", classification="Confidential")
        gated = new_doc(t, reviewer, f"E2E Quiet {uuid.uuid4().hex[:4]}", classification="Restricted")
        try:
            open_manager(page)

            def publish_via_picker(doc_id, level_value):
                page.locator('[data-trust="open-picker"]').click()
                page.locator('[data-trust="doc-search"] input').fill(doc_id)
                page.locator(f'[data-trust-doc="{doc_id}"]').click()
                expect(page.locator('[data-trust="picker-publish"]')).to_be_enabled(timeout=8000)
                page.locator(f'input[name="trust-picker-level"][value="{level_value}"]').check()
                page.locator('[data-trust="picker-publish"]').click()

            # Public: the confirmation, Cancel keeps it unpublished.
            publish_via_picker(secret, "public")
            dialog = page.locator('[data-trust="classification-dialog"]')
            expect(dialog).to_contain_text("This document is classified confidential. Publishing it as Public makes it readable by anyone on the internet.")
            dialog.locator('[data-trust="classification-cancel"]').click()
            assert item_for(t, secret) is None
            # Publish anyway publishes it.
            page.locator('[data-trust="picker-publish"]').click()
            expect(dialog).to_be_visible()
            dialog.locator('[data-trust="classification-confirm"]').click()
            expect(page.locator(f'[data-trust-row="{secret}"]')).to_be_visible(timeout=8000)
            assert item_for(t, secret)["access_level"] == "public"

            # Gated: no dialog at all.
            publish_via_picker(gated, "gated")
            expect(page.locator(f'[data-trust-row="{gated}"]')).to_be_visible(timeout=8000)
            assert page.locator('[data-trust="classification-dialog"]').count() == 0
            assert item_for(t, gated)["access_level"] == "gated"

            # Changing a stored level to public asks too; Cancel puts the select back.
            row = page.locator(f'[data-trust-row="{gated}"]')
            row.locator('[data-trust="level"]').select_option("public")
            expect(dialog).to_be_visible()
            dialog.locator('[data-trust="classification-cancel"]').click()
            expect(row.locator('[data-trust="level"]')).to_have_value("gated")
            assert item_for(t, gated)["access_level"] == "gated"
        finally:
            cleanup(t, [secret, gated])

    def test_reordering_changes_the_order_on_the_public_page(self, admin_page, tokens, reviewer):
        t = tokens["admin"]
        page = admin_page
        tag = uuid.uuid4().hex[:4]
        first = new_doc(t, reviewer, f"E2E Order Alpha {tag}")
        second = new_doc(t, reviewer, f"E2E Order Bravo {tag}")
        try:
            set_enabled(t, True)
            for d in (first, second):
                api("post", f"/documents/{d}/trust-center", t, expect_status=201, json={"access_level": "public"})

            def public_order():
                page.goto(f"{BASE}/{ORG}/trust")
                expect(page.get_by_text(f"E2E Order Alpha {tag}").first).to_be_visible(timeout=15000)
                text = page.locator("body").inner_text()
                return text.index(f"E2E Order Alpha {tag}") < text.index(f"E2E Order Bravo {tag}")

            assert public_order() is True, "published in this order: Alpha first"
            open_manager(page)
            row = page.locator(f'[data-trust-row="{first}"]')
            expect(row.locator('[data-trust="up"]')).to_be_disabled()  # first row: nowhere to go up
            expect(row.locator('[data-trust="down"]')).to_be_enabled()
            row.locator('[data-trust="down"]').click()
            expect(page.locator('[data-trust-row] [data-trust="title"]').filter(has_text=f"Bravo {tag}")).to_be_visible()
            page.wait_for_function(
                "([a,b]) => { const t=[...document.querySelectorAll('[data-trust-row]')].map(e=>e.dataset.trustRow); return t.indexOf(b) < t.indexOf(a) && t.indexOf(a) >= 0 }",
                arg=[first, second], timeout=8000)
            assert public_order() is False, "after Alpha moved down, Bravo must come first"
        finally:
            cleanup(t, [first, second])

    def test_each_status_is_shown(self, admin_page, tokens, reviewer):
        t = tokens["admin"]
        page = admin_page
        tag = uuid.uuid4().hex[:4]
        pub = new_doc(t, reviewer, f"E2E Status Published {tag}")
        draft = new_doc(t, reviewer, f"E2E Status Draft {tag}")
        retired = new_doc(t, reviewer, f"E2E Status Retired {tag}")
        never = new_doc(t, reviewer, f"E2E Status Never {tag}", approve=False)
        try:
            for d in (pub, draft, retired):
                api("post", f"/documents/{d}/trust-center", t, expect_status=201, json={"access_level": "gated"})
            api("put", f"/documents/{draft}/content", t, expect_status=200, json={"content": "# Draft\n\nEdited, not approved.\n"})
            api("put", f"/documents/{retired}/metadata", t, expect_status=200, json={"fields": {"status": "retired"}})

            open_manager(page)
            status = lambda d: page.locator(f'[data-trust-row="{d}"] [data-trust="status"]')
            expect(status(pub)).to_have_text("Published", timeout=10000)
            expect(status(draft)).to_contain_text("in progress, visitors see v")
            expect(status(retired)).to_have_text("Retired, hidden")
            # Retired and still retired: Publish again is offered, disabled, with the reason.
            row = page.locator(f'[data-trust-row="{retired}"]')
            expect(row.locator('[data-trust="publish-again"]')).to_be_disabled()
            expect(row).to_contain_text("Retired; restore the document first")

            # A document with no approved version cannot be published: the picker says so.
            page.locator('[data-trust="open-picker"]').click()
            page.locator('[data-trust="doc-search"] input').fill(never)
            page.locator(f'[data-trust-doc="{never}"]').click()
            expect(page.locator('[data-trust="picker-reason"]')).to_have_text(
                "No approved version yet. Get the document approved before publishing.", timeout=8000)
            expect(page.locator('[data-trust="picker-publish"]')).to_be_disabled()
            page.locator('[data-trust="picker"] button:text-is("Cancel")').click()

            # A retired, hidden row can still be removed.
            row.locator('[data-trust="remove"]').click()
            page.locator('[data-trust="confirm-yes"]').click()
            expect(row).to_have_count(0, timeout=8000)
            assert item_for(t, retired) is None
        finally:
            cleanup(t, [pub, draft, retired])

    def test_unsaved_settings_survive_the_switch_and_a_tab_change(self, admin_page, tokens):
        t = tokens["admin"]
        page = admin_page
        set_enabled(t, False)
        try:
            open_manager(page, "settings")
            contact = page.locator('[data-trust="contact"]')
            contact.fill("unsaved@e2e.example")
            page.locator('[data-trust="toggle-enabled"]').click()
            page.locator('[data-trust="enable-confirm"]').click()
            expect(page.locator('[data-trust="toggle-enabled"]')).to_have_attribute("aria-checked", "true", timeout=8000)
            expect(contact).to_have_value("unsaved@e2e.example")
            expect(page.locator('[data-trust="saved"]')).to_have_count(0)
            page.locator('[data-trust-tab="content"]').click()
            page.locator('[data-trust-tab="settings"]').click()
            expect(contact).to_have_value("unsaved@e2e.example")
            assert api("get", "/trust-center/settings", t, expect_status=200).json()["security_contact"] != "unsaved@e2e.example"
        finally:
            api("put", "/trust-center/settings", t, json={"enabled": False, "security_contact": ""})
