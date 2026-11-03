"""#322: invite people with a copyable link when email is not available.

The test stack has no SMTP, so delivery=link is the only way to invite here and
delivery=email must fail cleanly (503) without leaving a half-created member.
"""
import uuid
from urllib.parse import parse_qs, urlparse

import requests
from conftest import ADMIN_EMAIL, ORG_SLUG

PASSWORD = "InvitedPass123!"


def _fresh_email():
    return f"invitee-{uuid.uuid4().hex[:10]}@isms-test.local"


def _member_emails(api_url, headers):
    r = requests.get(f"{api_url}/admin/members", headers=headers)
    assert r.status_code == 200, r.text
    return [m["email"].lower() for m in r.json()["data"]]


def _invite_link(api_url, headers, email):
    r = requests.post(f"{api_url}/auth/invite", headers=headers,
                      json={"email": email, "name": "Invited Auditor", "role": "reader", "delivery": "link"})
    assert r.status_code == 200, f"link invite failed: {r.status_code} {r.text}"
    return r.json()


def test_link_invite_lets_the_person_set_a_password_and_log_in(api_url, admin_headers):
    email = _fresh_email()
    body = _invite_link(api_url, admin_headers, email)
    assert body["status"] == "invited" and body["delivery"] == "link" and body["email"] == email
    assert isinstance(body["expires_at"], int) and body["expires_at"] > 0

    parsed = urlparse(body["invite_url"])
    assert parsed.path == "/verify-email", body["invite_url"]
    token = parse_qs(parsed.query)["token"][0]

    # Pending until the link is used: cannot log in yet.
    r = requests.post(f"{api_url}/auth/login", json={"email": email, "password": PASSWORD, "organization": ORG_SLUG})
    assert r.status_code != 200

    r = requests.post(f"{api_url}/auth/verify-email", json={"token": token, "password": PASSWORD})
    assert r.status_code == 200, f"verify failed: {r.status_code} {r.text}"

    r = requests.post(f"{api_url}/auth/login", json={"email": email, "password": PASSWORD, "organization": ORG_SLUG})
    assert r.status_code == 200, f"invited person cannot log in: {r.status_code} {r.text}"

    # The token is single-use.
    r = requests.post(f"{api_url}/auth/verify-email", json={"token": token, "password": "Another-Pass-1!"})
    assert r.status_code == 400


def test_email_invite_without_smtp_creates_nothing(api_url, admin_headers):
    email = _fresh_email()
    r = requests.post(f"{api_url}/auth/invite", headers=admin_headers,
                      json={"email": email, "role": "reader", "delivery": "email"})
    assert r.status_code == 503, f"{r.status_code} {r.text}"
    assert email not in _member_emails(api_url, admin_headers)


def test_link_for_an_existing_member_is_a_conflict(api_url, admin_headers):
    # ADMIN_EMAIL is an active account that already has a password.
    r = requests.post(f"{api_url}/auth/invite", headers=admin_headers,
                      json={"email": ADMIN_EMAIL, "delivery": "link"})
    assert r.status_code == 409, f"{r.status_code} {r.text}"
    assert "invite_url" not in r.text


def test_resend_link_replaces_the_old_one(api_url, admin_headers):
    email = _fresh_email()
    old = parse_qs(urlparse(_invite_link(api_url, admin_headers, email)["invite_url"]).query)["token"][0]
    r = requests.post(f"{api_url}/auth/resend-invite", headers=admin_headers,
                      json={"email": email, "delivery": "link"})
    assert r.status_code == 200, f"{r.status_code} {r.text}"
    fresh = parse_qs(urlparse(r.json()["invite_url"]).query)["token"][0]
    assert fresh != old

    r = requests.post(f"{api_url}/auth/verify-email", json={"token": old, "password": PASSWORD})
    assert r.status_code == 400, "the replaced link must stop working"
    r = requests.post(f"{api_url}/auth/verify-email", json={"token": fresh, "password": PASSWORD})
    assert r.status_code == 200


def test_invalid_delivery_is_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/auth/invite", headers=admin_headers,
                      json={"email": _fresh_email(), "delivery": "pigeon"})
    assert r.status_code == 400


def test_invite_options(api_url, admin_headers, reader_headers):
    r = requests.get(f"{api_url}/admin/invite-options", headers=admin_headers)
    assert r.status_code == 200, r.text
    assert r.json() == {"email_enabled": False, "link_valid_hours": 72}
    r = requests.get(f"{api_url}/admin/invite-options", headers=reader_headers)
    assert r.status_code == 403
