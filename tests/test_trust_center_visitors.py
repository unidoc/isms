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
def tv(api_url):
    """A fresh organization whose trust center is on. Returns its slug."""
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
    return slug


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
        assert r.json() == {"visitor": None}
        assert r.headers.get("Cache-Control") == "no-store"


def test_sign_out_without_a_session_is_ok(api_url, tv):
    r = requests.post(f"{api_url}/trust/{tv}/visitor/sign-out", json={})
    assert r.status_code == 200, r.text
    assert r.json() == {"visitor": None}


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
