"""E2E (#525): the click-through NDA on the public trust center page.

CI's server has no SMTP, so a visitor can never sign in for real. The trust API
is therefore answered in the browser with page.route, registered before
navigation: a small stand-in for the server keeps where the visitor stands with
the NDA (`required` / `accepted`), the current NDA hash, and answers the visitor,
items, NDA, accept and document routes the way the real server does. Everything
else is the real app (the built page, the i18n files) against the real server.

What this pins: a signed-in visitor who has not accepted is shown the NDA panel;
Accept stays disabled until the box is ticked; a 409 (a newer version was
approved while the text was open) replaces the text and unticks the box; the
accept request carries the hash that was shown; after accepting, the gated
file link appears on the page.

Own file, no organization or login needed: the slug is made up and every trust
call for it is mocked.

Requires: pip install playwright && playwright install chromium
Run:      pytest tests/test_e2e_trust_center_nda.py -v
"""
import json
import re

from playwright.sync_api import expect

from test_e2e_browser import BASE, pw_browser  # noqa: F401

SLUG = "nda-e2e"
GATED_CERT = "33333333-3333-4333-8333-333333333333"
GATED_DOC = "11111111-1111-4111-8111-111111111111"
VISITOR = {"email": "ada@auditco.test", "name": "Ada", "company": "Audit Co"}
H1, H2 = "a" * 64, "b" * 64


class FakeTrust:
    def __init__(self):
        self.status = "required"
        self.hash = H1
        self.text = "Keep it secret."
        self.accept_bodies = []

    def nda(self):
        return {"version": "2.0", "approved_at": 1760000000, "content_hash": self.hash,
                "title": "Mutual NDA", "html": f"<p>{self.text}</p>", "company": VISITOR["company"]}

    def items(self):
        cert = {"id": GATED_CERT, "kind": "certification", "section": "", "sort_order": 1,
                "access_level": "gated", "title": "SOC 2 report", "link_type": "audit_report",
                "issuer": "X", "source": "file", "state": "current"}
        if self.status == "accepted":
            cert.update(file_name="soc2.pdf", file_size=10)
        doc = {"id": GATED_DOC, "kind": "document", "section": "", "sort_order": 1,
               "access_level": "gated", "title": "Gated Policy", "version": "1.0",
               "approved_at": 1760000000, "content_hash": "c" * 64}
        return [doc, cert]

    def handle(self, route):
        req = route.request
        path = req.url.split("/api/v1/trust/" + SLUG, 1)[1].split("?")[0]

        def send(status, body):
            route.fulfill(status=status, content_type="application/json", body=json.dumps(body))

        not_found = {"message": "item not found", "code": "not_found"}
        if path == "":
            send(200, {"title": "", "security_contact": "sec@acme.test", "branding_name": "Acme",
                       "branding_color": "", "default_locale": "en", "show_powered_by": False,
                       "overview": None, "visitor_sign_in": True, "nda_required": True})
        elif path == "/items":
            send(200, self.items())
        elif path == "/visitor":
            send(200, {"visitor": VISITOR, "nda": {"status": self.status}})
        elif path == "/nda" and req.method == "GET":
            send(200, self.nda()) if self.status == "required" else send(404, not_found)
        elif path == "/nda/accept":
            body = json.loads(req.post_data or "{}")
            self.accept_bodies.append(body)
            if body.get("content_hash") != self.hash:
                send(409, {"message": "the NDA has changed", "code": "trust_nda_outdated", "nda": self.nda()})
            else:
                self.status = "accepted"
                send(200, {"visitor": VISITOR, "nda": {"status": "accepted", "version": "2.0", "content_hash": self.hash}})
        else:
            send(404, not_found)


def open_page(pw_browser, fake):
    ctx = pw_browser.new_context(viewport={"width": 1100, "height": 900})
    page = ctx.new_page()
    page.route(re.compile(r".*/api/v1/trust/" + SLUG + r"(/.*)?(\?.*)?$"), fake.handle)
    page.goto(f"{BASE}/{SLUG}/trust")
    return ctx, page


def test_signed_in_visitor_accepts_the_nda_and_the_gated_file_appears(pw_browser):
    fake = FakeTrust()
    ctx, page = open_page(pw_browser, fake)
    try:
        panel = page.locator('[data-trust="nda-panel"]')
        expect(panel).to_be_visible(timeout=15000)
        expect(panel.locator('[data-trust="nda-title"]')).to_have_text("Mutual NDA")
        expect(panel.locator('[data-trust="nda-body"]')).to_contain_text("Keep it secret.")
        expect(panel).to_contain_text("I accept this NDA on behalf of Audit Co")
        accept = panel.locator('[data-trust="nda-accept"]')
        expect(accept).to_be_disabled()
        # Before accepting there is no download, only the prompt.
        expect(page.locator('[data-trust="cert-download"]')).to_have_count(0)

        # A newer version is approved while the text is open: the 409 replaces
        # the text, unticks the box and asks again.
        fake.hash, fake.text = H2, "The brand new text."
        panel.locator('[data-trust="nda-check"]').check()
        expect(accept).to_be_enabled()
        accept.click()
        expect(panel.locator('[data-trust="nda-changed"]')).to_contain_text("The NDA has changed. Read the new version and accept it again.")
        expect(panel.locator('[data-trust="nda-body"]')).to_contain_text("The brand new text.")
        expect(panel.locator('[data-trust="nda-check"]')).not_to_be_checked()
        expect(accept).to_be_disabled()

        panel.locator('[data-trust="nda-check"]').check()
        accept.click()
        expect(panel).to_have_count(0)
        expect(page.locator('[data-trust="cert-download"]')).to_be_visible()
        assert [b["content_hash"] for b in fake.accept_bodies] == [H1, H2]
    finally:
        ctx.close()
