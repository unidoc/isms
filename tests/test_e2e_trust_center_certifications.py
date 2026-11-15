"""E2E (#521): trust center certifications and reports, in a browser.

The API (validation, sniffing, expiry boundary, headers, limits) is covered by
the Go and pytest API tests; this file covers what only a browser shows: adding
a file and a link through the form, the badges, publishing and ordering from the
manager screen, what a visitor's page offers (Download, contact fallback), a
certification disappearing from the visitor page once it expires, the overdue
list and the not-allowed screen.

It runs in the shared e2e organization. Every certification a test creates has a
label starting "E2E Cert"; a module fixture deletes all of them (and the risk
the overdue test needs) even when a test fails, restores the security contact and
turns the trust center back off.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_trust_center_certifications.py -v
"""
import uuid
from datetime import date, timedelta

import pytest
import requests
from playwright.sync_api import expect

from test_e2e_browser import api, do_login, ADMIN, R1, BASE, ORG, API, pw_browser, tokens  # noqa: F401

PREFIX = "E2E Cert"
NOT_ALLOWED = "Only admins and managers can manage the trust center."
CONTACT = "security@e2e-trust.test"
PDF = b"%PDF-1.4\n" + b"e2e certification report\n" * 40


def _page(pw_browser, creds, width=1440):
    ctx = pw_browser.new_context(viewport={"width": width, "height": 1000})
    p = ctx.new_page()
    do_login(p, creds[0], creds[1])
    return ctx, p


@pytest.fixture
def admin_page(pw_browser, tokens):
    ctx, p = _page(pw_browser, ADMIN)
    yield p
    ctx.close()


_RISKS = []


@pytest.fixture(autouse=True, scope="module")
def restore_org(tokens):
    """Whatever a test failed to clean up, undo once the module is done."""
    t = tokens["admin"]
    before = api("get", "/trust-center/settings", t, expect_status=200).json()
    api("put", "/trust-center/settings", t, expect_status=200, json={"security_contact": CONTACT})
    yield
    for c in api("get", "/trust-center/certifications", t, expect_status=200).json():
        if c["label"].startswith(PREFIX):
            api("delete", f"/trust-center/certifications/{c['id']}", t)
    for rid in _RISKS:
        api("delete", f"/risks/{rid}", t)
    api("put", "/trust-center/settings", t, expect_status=200,
        json={"enabled": False, "security_contact": before["security_contact"]})


def set_enabled(t, enabled):
    api("put", "/trust-center/settings", t, expect_status=200, json={"enabled": enabled})


def label(name):
    return f"{PREFIX} {name} {uuid.uuid4().hex[:6]}"


def day(offset):
    return (date.today() + timedelta(days=offset)).isoformat()


def mk_file(t, name, level=None, valid_until=None, publish=True):
    fields = {"label": name, "link_type": "certificate", "issuer": "E2E Issuer"}
    if valid_until:
        fields["valid_until"] = valid_until
    if publish:
        fields.update({"publish": "true", "access_level": level or "public"})
    r = requests.post(f"{API}/trust-center/certifications", headers={"Authorization": f"Bearer {t}"},
                      data=fields, files={"file": (f"{name}.pdf", PDF, "application/pdf")}, timeout=30)
    assert r.status_code == 201, r.text
    return r.json()


def mk_link(t, name, level="gated", valid_until=None, publish=True):
    body = {"label": name, "link_type": "attestation", "url": "https://example.com/attestation"}
    if valid_until:
        body["valid_until"] = valid_until
    if publish:
        body.update({"publish": True, "access_level": level})
    return api("post", "/trust-center/certifications", t, expect_status=201, json=body).json()


def open_manager(page):
    page.goto(f"{BASE}/{ORG}/trust-center")
    expect(page.locator('[data-trust="certifications-section"]')).to_be_visible(timeout=15000)


def cert_row(page, c_id):
    return page.locator(f'[data-trust-cert-row="{c_id}"]')


def cert_rows(page, prefix_text):
    return page.locator('[data-trust-cert-row]').filter(has_text=prefix_text)


class TestCertifications:

    def test_manager_adds_a_pdf_and_a_link_publishes_and_reorders(self, admin_page, tokens):
        t, page = tokens["admin"], admin_page
        tag = uuid.uuid4().hex[:6]
        pdf_label, link_label = f"{PREFIX} PDF {tag}", f"{PREFIX} Link {tag}"
        open_manager(page)

        # Add a PDF with a date within 30 days. Publish now stays OFF by default.
        page.locator('[data-trust="cert-add-open"]').click()
        form = page.locator('[data-trust="cert-add-form"]')
        expect(form.locator('[data-trust="cert-f-publish"]')).not_to_be_checked()
        form.locator('[data-trust="cert-f-file"]').set_input_files({"name": "e2e-iso.pdf", "mimeType": "application/pdf", "buffer": PDF})
        form.locator('[data-trust="cert-f-label"]').fill(pdf_label)
        form.locator('[data-trust="cert-f-type"]').select_option("certificate")
        form.locator('[data-trust="cert-f-issuer"]').fill("E2E Issuer")
        form.locator('[data-trust="cert-f-validuntil"]').fill(day(20))
        form.locator('[data-trust="cert-add-submit"]').click()
        pdf_row = cert_rows(page, pdf_label)
        expect(pdf_row).to_have_count(1, timeout=15000)
        expect(pdf_row.locator('[data-trust="cert-state"]')).to_have_text("Expiring soon")
        expect(pdf_row.locator('[data-trust="cert-status"]')).to_have_text("Not published")
        expect(pdf_row).to_contain_text("e2e-iso.pdf")

        # Add a link, no dates.
        page.locator('[data-trust="cert-add-open"]').click()
        form = page.locator('[data-trust="cert-add-form"]')
        form.locator('[data-trust="cert-source-link"]').check()
        form.locator('[data-trust="cert-f-url"]').fill("https://example.com/attestation")
        form.locator('[data-trust="cert-f-label"]').fill(link_label)
        form.locator('[data-trust="cert-f-type"]').select_option("attestation")
        form.locator('[data-trust="cert-add-submit"]').click()
        link_row = cert_rows(page, link_label)
        expect(link_row).to_have_count(1, timeout=15000)
        expect(link_row.locator('[data-trust="cert-link"]')).to_have_attribute("href", "https://example.com/attestation")
        expect(link_row.locator('[data-trust="cert-link"]')).to_have_attribute("rel", "noopener noreferrer")
        expect(link_row.locator('[data-trust="cert-state"]')).to_have_count(0)

        # Publish the PDF as public and the link as gated.
        pdf_row.locator('[data-trust="cert-level"]').select_option("public")
        pdf_row.locator('[data-trust="cert-publish"]').click()
        expect(pdf_row.locator('[data-trust="cert-status"]')).to_have_text("Published: Public", timeout=10000)
        link_row.locator('[data-trust="cert-level"]').select_option("gated")
        link_row.locator('[data-trust="cert-publish"]').click()
        expect(link_row.locator('[data-trust="cert-status"]')).to_have_text("Published: Gated", timeout=10000)

        certs = {c["label"]: c for c in api("get", "/trust-center/certifications", t, expect_status=200).json()}
        assert certs[pdf_label]["item"]["access_level"] == "public"
        assert certs[link_label]["item"]["access_level"] == "gated"
        assert certs[pdf_label]["item"]["sort_order"] < certs[link_label]["item"]["sort_order"]

        # Reorder: move the link above the PDF.
        link_row.locator('[data-trust="cert-up"]').click()
        page.wait_for_function(
            "([a,b]) => { const t=[...document.querySelectorAll('[data-trust-cert-row]')].map(e=>e.dataset.trustCertRow); return t.indexOf(b) >= 0 && t.indexOf(b) < t.indexOf(a) }",
            arg=[str(certs[pdf_label]["id"]), str(certs[link_label]["id"])], timeout=10000)
        after = {c["label"]: c for c in api("get", "/trust-center/certifications", t, expect_status=200).json()}
        assert after[link_label]["item"]["sort_order"] < after[pdf_label]["item"]["sort_order"]

        # Mark verified, and download through the session.
        pdf_row.locator('[data-trust="cert-verify"]').click()
        expect(pdf_row.locator('[data-trust="cert-verified"]')).to_be_visible(timeout=10000)
        with page.expect_download() as dl:
            pdf_row.locator('[data-trust="cert-download"]').click()
        assert dl.value.suggested_filename.endswith(".pdf")

    def test_visitor_page_offers_download_and_the_contact_fallback(self, pw_browser, tokens):
        t = tokens["admin"]
        tag = uuid.uuid4().hex[:6]
        pub, gated = f"{PREFIX} Public {tag}", f"{PREFIX} Gated {tag}"
        mk_file(t, pub, level="public", valid_until=day(200))
        mk_link(t, gated, level="gated")
        set_enabled(t, True)
        ctx = pw_browser.new_context(viewport={"width": 1280, "height": 900})
        page = ctx.new_page()
        try:
            page.goto(f"{BASE}/{ORG}/trust")
            section = page.locator('[data-trust="certifications"]')
            expect(section).to_be_visible(timeout=15000)
            expect(section).to_contain_text("Certifications and reports")
            pub_row = section.locator('[data-trust="cert-row"]').filter(has_text=pub)
            dl = pub_row.locator('[data-trust="cert-download"]')
            expect(dl).to_have_text("Download")
            href = dl.get_attribute("href")
            assert href.startswith(f"/api/v1/trust/{ORG}/certifications/") and href.endswith("/file"), href
            got = page.evaluate("async (h) => { const r = await fetch(h); return {status: r.status, type: r.headers.get('content-type'), disp: r.headers.get('content-disposition'), n: (await r.arrayBuffer()).byteLength} }", href)
            assert got["status"] == 200 and got["type"] == "application/pdf", got
            assert got["disp"].startswith("attachment") and got["n"] == len(PDF), got
            expect(pub_row).to_contain_text("E2E Issuer")
            expect(pub_row.locator('[data-trust="cert-state"]')).to_have_text("Current")

            gated_row = section.locator('[data-trust="cert-row"]').filter(has_text=gated)
            expect(gated_row.locator('[data-trust="contact"]')).to_have_text(f"Contact {CONTACT}")
            expect(gated_row.locator('[data-trust="contact"]')).to_have_attribute("href", f"mailto:{CONTACT}")
            expect(gated_row.locator('a[href^="http"]')).to_have_count(0)
            # Certifications come before the documents section.
            order = page.evaluate("() => [...document.querySelectorAll('[data-trust=\"certifications\"], [data-trust=\"documents\"]')].map(e => e.dataset.trust)")
            assert order[0] == "certifications"
        finally:
            ctx.close()
            set_enabled(t, False)

    def test_expiring_the_certification_in_the_edit_form_removes_it_from_the_visitor_page(self, admin_page, pw_browser, tokens):
        t, page = tokens["admin"], admin_page
        name = f"{PREFIX} Expires {uuid.uuid4().hex[:6]}"
        c = mk_file(t, name, level="public", valid_until=day(60))
        set_enabled(t, True)
        ctx = pw_browser.new_context()
        visitor = ctx.new_page()
        try:
            visitor.goto(f"{BASE}/{ORG}/trust")
            expect(visitor.get_by_text(name)).to_be_visible(timeout=15000)

            open_manager(page)
            row = cert_row(page, c["id"])
            row.locator('[data-trust="cert-edit"]').click()
            form = page.locator('[data-trust="cert-edit-form"]')
            form.locator('[data-trust="cert-e-validuntil"]').fill(day(-2))
            form.locator('[data-trust="cert-edit-save"]').click()
            expect(row.locator('[data-trust="cert-state"]')).to_have_text("Expired", timeout=10000)
            expect(row.locator('[data-trust="cert-expired-hint"]')).to_have_text("Expired: hidden from visitors")

            visitor.reload()
            expect(visitor.locator('[data-trust="page"]')).to_be_visible(timeout=15000)
            expect(visitor.get_by_text(name)).to_have_count(0)
            # The file route is gone too, with nothing having run in between.
            item = api("get", "/trust-center/certifications", t, expect_status=200).json()
            item_id = next(x for x in item if x["id"] == c["id"])["item"]["id"]
            assert requests.get(f"{API}/trust/{ORG}/certifications/{item_id}/file", timeout=15).status_code == 404

            # An emptied date is sent as null and cleared.
            row.locator('[data-trust="cert-edit"]').click()
            form = page.locator('[data-trust="cert-edit-form"]')
            form.locator('[data-trust="cert-e-validuntil"]').fill("")
            form.locator('[data-trust="cert-edit-save"]').click()
            expect(row.locator('[data-trust="cert-state"]')).to_have_count(0, timeout=10000)
            visitor.reload()
            expect(visitor.get_by_text(name)).to_be_visible(timeout=15000)
        finally:
            ctx.close()
            set_enabled(t, False)

    def test_the_form_refuses_a_wrong_file_without_sending_it(self, admin_page, tokens):
        t, page = tokens["admin"], admin_page
        open_manager(page)
        before = len(api("get", "/trust-center/certifications", t, expect_status=200).json())
        page.locator('[data-trust="cert-add-open"]').click()
        form = page.locator('[data-trust="cert-add-form"]')
        form.locator('[data-trust="cert-f-file"]').set_input_files({"name": "x.html", "mimeType": "text/html", "buffer": b"<html></html>"})
        form.locator('[data-trust="cert-f-label"]').fill(f"{PREFIX} Wrong")
        form.locator('[data-trust="cert-f-type"]').select_option("other")
        form.locator('[data-trust="cert-add-submit"]').click()
        expect(form.locator('[data-trust="cert-error-file"]')).to_have_text("Only PDF, PNG or JPEG files are accepted")
        # A .pdf name over HTML bytes passes the browser check and the server says no.
        form.locator('[data-trust="cert-f-file"]').set_input_files({"name": "x.pdf", "mimeType": "application/pdf", "buffer": b"<html>not a pdf</html>"})
        form.locator('[data-trust="cert-add-submit"]').click()
        # The server's own message (its catalogue entry is lower case).
        expect(form.locator('[data-trust="cert-add-error"]')).to_have_text("only PDF, PNG or JPEG files are accepted", timeout=10000)
        assert len(api("get", "/trust-center/certifications", t, expect_status=200).json()) == before

    def test_overview_overdue_list_shows_the_certification(self, admin_page, tokens):
        t, page = tokens["admin"], admin_page
        name = f"{PREFIX} Soon {uuid.uuid4().hex[:6]}"
        mk_link(t, name, valid_until=day(8), publish=False)
        # The Overview lists overdue items next to the heat map, which needs a risk.
        rid = api("post", "/risks", t, expect_status=[200, 201], json={
            "title": f"E2E overdue cert risk {uuid.uuid4().hex[:6]}", "description": "e2e", "risk_type": "threat",
            "origin": "internal", "status": "open", "treatment": "mitigate", "current_likelihood": 2, "current_impact": 2,
        }).json()["id"]
        _RISKS.append(rid)
        page.goto(f"{BASE}/{ORG}/overview")
        row = page.locator("div.px-5.py-3").filter(has_text=name)
        expect(row).to_have_count(1, timeout=15000)
        expect(row).to_contain_text("Trust center certification")
        expect(row).to_contain_text("in 8d")
        expect(row).to_contain_text("CERT-")

    def test_a_reader_gets_the_not_allowed_message(self, pw_browser, tokens):
        ctx, page = _page(pw_browser, R1)
        try:
            requested = []
            page.on("request", lambda r: requested.append(r.url) if "/api/v1/trust-center" in r.url else None)
            page.goto(f"{BASE}/{ORG}/trust-center")
            expect(page.get_by_text(NOT_ALLOWED)).to_be_visible(timeout=15000)
            page.wait_for_timeout(500)
            assert requested == [], requested
        finally:
            ctx.close()
