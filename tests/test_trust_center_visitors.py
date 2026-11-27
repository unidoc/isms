"""Trust center visitor sign-in (#524) over HTTP, as far as CI's server can go.

The CI server has no SMTP, so no sign-in mail can be sent and the full round
trip (link -> mail -> session) is covered by the database tests in
internal/isms/api/trust_visitor_pg_test.go against a fake SMTP sink. What is
checked here is the endpoint shape of a server WITHOUT mail: sign-in is
advertised as unavailable, the link route refuses, a made-up token is refused
with one code, sign-out is harmless, and wrong methods and bodies get the
documented answers.

Everything runs in a DEDICATED organization with its trust center enabled; the
shared conftest organization must keep its trust center off.
"""
import uuid

import pytest
import requests

PASSWORD = "TestPass123!"


def _tag():
    return uuid.uuid4().hex[:8]


def _login(api_url, email, org=None):
    body = {"email": email, "password": PASSWORD}
    if org:
        body["organization"] = org
    r = requests.post(f"{api_url}/auth/login", json=body)
    assert r.status_code == 200, r.text
    return {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}


@pytest.fixture(scope="module")
def tv_org(api_url):
    """A fresh organization whose trust center is on: its slug and admin headers."""
    t = _tag()
    slug = f"tcv-{t}"
    email = f"tcv-admin-{t}@isms-test.local"
    r = requests.post(f"{api_url}/auth/signup", json={"email": email, "password": PASSWORD, "name": "tcv"})
    assert r.status_code in (200, 201), r.text
    admin = _login(api_url, email)
    r = requests.post(f"{api_url}/organizations", headers=admin, json={"name": f"TC Visitors {t}", "slug": slug})
    assert r.status_code in (200, 201), r.text
    admin = _login(api_url, email, slug)
    r = requests.put(f"{api_url}/trust-center/settings", headers=admin, json={"enabled": True, "title": "Visitors"})
    assert r.status_code == 200, r.text
    return {"slug": slug, "admin": admin}


@pytest.fixture(scope="module")
def tv(tv_org):
    return tv_org["slug"]


def _no_smtp_only(api_url, tv_org):
    """These tests describe a server without mail; skip, with the reason, on one that has it."""
    s = requests.get(f"{api_url}/trust-center/settings", headers=tv_org["admin"]).json()
    if s["email_configured"]:
        pytest.skip("this server can send mail; the assertions describe a server without it")


def _sign_in_enabled(api_url, slug):
    r = requests.get(f"{api_url}/trust/{slug}")
    assert r.status_code == 200, r.text
    return r.json()["visitor_sign_in"]


def test_overview_advertises_the_new_fields(api_url, tv):
    r = requests.get(f"{api_url}/trust/{tv}")
    assert r.status_code == 200, r.text
    body = r.json()
    assert isinstance(body["visitor_sign_in"], bool)
    assert body["privacy_notice_url"] == ""


def test_without_mail_sign_in_is_unavailable(api_url, tv):
    if _sign_in_enabled(api_url, tv):
        pytest.skip("this server can send mail")
    r = requests.post(f"{api_url}/trust/{tv}/visitor/link", json={"email": "someone@acme-corp.test"})
    assert r.status_code == 503, r.text
    assert r.json()["code"] == "trust_sign_in_unavailable"


def test_made_up_token_is_refused_with_one_code(api_url, tv):
    token = uuid.uuid4().hex + uuid.uuid4().hex
    for route in ("/visitor/link/inspect", "/visitor/session"):
        r = requests.post(f"{api_url}/trust/{tv}{route}", json={"token": token, "name": "A", "company": "B"})
        assert r.status_code == 400, f"{route}: {r.text}"
        assert r.json()["code"] == "trust_link_invalid"
        assert "set-cookie" not in {k.lower() for k in r.headers}


def test_visitor_is_null_when_nobody_is_signed_in(api_url, tv):
    for kwargs in ({}, {"cookies": {"isms_trust_visitor": "0" * 64}}, {"headers": {"Authorization": "Bearer " + "0" * 64}}):
        r = requests.get(f"{api_url}/trust/{tv}/visitor", **kwargs)
        assert r.status_code == 200, r.text
        assert r.json() == {"visitor": None, "nda": None}
        assert r.headers.get("Cache-Control") == "no-store"


def test_sign_out_without_a_session_is_ok(api_url, tv):
    r = requests.post(f"{api_url}/trust/{tv}/visitor/sign-out", json={})
    assert r.status_code == 200, r.text
    assert r.json() == {"visitor": None, "nda": None}


def test_nda_routes_are_the_single_404_when_not_signed_in(api_url, tv):
    want = requests.get(f"{api_url}/trust/no-such-org-{_tag()}").content
    for kwargs in ({}, {"cookies": {"isms_trust_visitor": "0" * 64}}):
        r = requests.get(f"{api_url}/trust/{tv}/nda", **kwargs)
        assert r.status_code == 404 and r.content == want, r.text
        r = requests.post(f"{api_url}/trust/{tv}/nda/accept", json={"content_hash": "x"}, **kwargs)
        assert r.status_code == 404 and r.content == want, r.text
        assert r.headers.get("Cache-Control") == "no-store"


def test_nda_accept_must_be_json(api_url, tv):
    for headers in ({"Content-Type": "text/plain"}, {"Content-Type": "application/x-www-form-urlencoded"}):
        r = requests.post(f"{api_url}/trust/{tv}/nda/accept", data="content_hash=x", headers=headers)
        assert r.status_code == 400, f"{headers}: {r.status_code} {r.text}"


def test_overview_says_whether_an_nda_is_required(api_url, tv):
    r = requests.get(f"{api_url}/trust/{tv}")
    assert r.status_code == 200, r.text
    assert r.json()["nda_required"] is False


def test_post_must_be_json(api_url, tv):
    for route in ("/visitor/link", "/visitor/link/inspect", "/visitor/session", "/visitor/sign-out"):
        for headers in ({"Content-Type": "text/plain"}, {"Content-Type": "application/x-www-form-urlencoded"}):
            r = requests.post(f"{api_url}/trust/{tv}{route}", data="email=a%40b.test", headers=headers)
            assert r.status_code == 400, f"{route} {headers}: {r.status_code} {r.text}"


def test_wrong_methods_and_paths_are_the_single_404(api_url, tv):
    want = requests.get(f"{api_url}/trust/no-such-org-{_tag()}").content
    probes = [
        ("PUT", "/visitor"), ("DELETE", "/visitor"), ("POST", "/visitor"),
        ("GET", "/visitor/link"), ("GET", "/visitor/session"), ("GET", "/visitor/sign-out"),
        ("PUT", "/visitor/link"), ("DELETE", "/visitor/session"),
        ("POST", ""), ("POST", "/items"), ("POST", "/visitor/other"), ("POST", "/visitor/link/extra"),
    ]
    for method, rest in probes:
        r = requests.request(method, f"{api_url}/trust/{tv}{rest}", json={})
        assert r.status_code == 404, f"{method} {rest}: {r.status_code}"
        assert r.content == want, f"{method} {rest}: {r.text}"


def test_disabled_org_visitor_routes_are_the_single_404(api_url, test_setup):
    from conftest import ORG_SLUG

    want = requests.get(f"{api_url}/trust/no-such-org-{_tag()}").content
    for method, rest in [("GET", "/visitor"), ("POST", "/visitor/link"), ("POST", "/visitor/link/inspect"),
                         ("POST", "/visitor/session"), ("POST", "/visitor/sign-out")]:
        r = requests.request(method, f"{api_url}/trust/{ORG_SLUG}{rest}", json={"email": "a@b.test", "token": "x"})
        assert r.status_code == 404, f"{method} {rest}: {r.status_code}"
        assert r.content == want


# ---- manager API (admin or manager): sign-in settings and the visitor list ----

def test_settings_carry_the_sign_in_fields(api_url, tv_org):
    h = tv_org["admin"]
    s = requests.get(f"{api_url}/trust-center/settings", headers=h).json()
    assert s["blocked_email_domains_default"] is True
    assert "gmail.com" in s["blocked_email_domains"]
    assert s["blocked_email_domains"] == s["default_blocked_email_domains"]
    assert s["privacy_notice_url"] == ""
    assert isinstance(s["email_configured"], bool)

    r = requests.put(f"{api_url}/trust-center/settings", headers=h, json={
        "blocked_email_domains": ["Rival.TEST", "@example.com", "example.com"],
        "privacy_notice_url": "https://acme.test/privacy",
    })
    assert r.status_code == 200, r.text
    s = r.json()
    assert s["blocked_email_domains"] == ["example.com", "rival.test"]
    assert s["blocked_email_domains_default"] is False
    assert s["privacy_notice_url"] == "https://acme.test/privacy"
    assert s["default_blocked_email_domains"] != s["blocked_email_domains"]
    assert requests.get(f"{api_url}/trust/{tv_org['slug']}").json()["privacy_notice_url"] == "https://acme.test/privacy"

    r = requests.put(f"{api_url}/trust-center/settings", headers=h, json={"blocked_email_domains": []})
    assert r.json()["blocked_email_domains"] == [] and r.json()["blocked_email_domains_default"] is False
    r = requests.put(f"{api_url}/trust-center/settings", headers=h, json={"blocked_email_domains": None, "privacy_notice_url": ""})
    assert r.json()["blocked_email_domains_default"] is True and r.json()["privacy_notice_url"] == ""


def test_settings_report_that_email_is_not_configured(api_url, tv_org):
    _no_smtp_only(api_url, tv_org)
    s = requests.get(f"{api_url}/trust-center/settings", headers=tv_org["admin"]).json()
    assert s["email_configured"] is False


@pytest.mark.parametrize("body", [
    {"blocked_email_domains": ["a@b.com"]}, {"blocked_email_domains": ["nodot"]},
    {"blocked_email_domains": [" "]}, {"blocked_email_domains": ["x/y.com"]},
    {"blocked_email_domains": ["d%d.test" % i for i in range(501)]},
    {"privacy_notice_url": "javascript:alert(1)"}, {"privacy_notice_url": "ftp://x.test"},
    {"privacy_notice_url": "/relative"}, {"privacy_notice_url": "https://" + "a" * 2000},
])
def test_settings_validation(api_url, tv_org, body):
    r = requests.put(f"{api_url}/trust-center/settings", headers=tv_org["admin"], json=body)
    assert r.status_code == 400, r.text


def test_visitor_list_is_empty_for_a_fresh_org(api_url, tv_org):
    r = requests.get(f"{api_url}/trust-center/visitors", headers=tv_org["admin"])
    assert r.status_code == 200, r.text
    assert r.json() == []
    for path, method, body in [("/visitors/1/revoke-sessions", "POST", None), ("/visitors/abc/blocked", "PUT", {"blocked": True})]:
        r = requests.request(method, f"{api_url}/trust-center{path}", headers=tv_org["admin"], json=body)
        assert r.status_code == 404, r.text


def test_visitor_routes_need_manager_or_admin(api_url, test_setup, reader_headers, contributor_headers):
    for headers in (reader_headers, contributor_headers):
        for method, path, body in [("GET", "/visitors", None), ("POST", "/visitors/1/revoke-sessions", None),
                                   ("PUT", "/visitors/1/blocked", {"blocked": True})]:
            r = requests.request(method, f"{api_url}/trust-center{path}", headers=headers, json=body)
            assert r.status_code == 403, f"{method} {path}: {r.status_code} {r.text}"
    assert requests.get(f"{api_url}/trust-center/visitors").status_code == 401
