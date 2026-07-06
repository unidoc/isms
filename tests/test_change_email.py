"""Regression for #128: self-service email change (verify-before-swap).

The happy path (confirmation mailed to the new address, link swaps the email)
needs SMTP, which the test stack doesn't configure — and by design the swap only
happens after the token mailed to the *new* inbox is clicked. So this covers the
guards that run before the mailer: re-authentication (current password / OTP),
input validation, and rejection of already-taken or unchanged addresses. Those
guards are the safety-relevant part — an email change is an account-takeover
vector, so it must never proceed on the session alone.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL, ADMIN_PASSWORD, CONTRIBUTOR_EMAIL


def _fresh_email():
    return f"changed-{uuid.uuid4().hex[:8]}@isms-test.local"


def test_me_exposes_pending_email_field(api_url, admin_headers):
    r = requests.get(f"{api_url}/me", headers=admin_headers)
    assert r.status_code == 200
    assert "pending_email" in r.json()


def test_new_email_required(api_url, admin_headers):
    r = requests.put(f"{api_url}/auth/email", headers=admin_headers, json={
        "current_password": ADMIN_PASSWORD,
    })
    assert r.status_code == 400, r.text


def test_invalid_email_rejected(api_url, admin_headers):
    r = requests.put(f"{api_url}/auth/email", headers=admin_headers, json={
        "new_email": "not-an-email",
        "current_password": ADMIN_PASSWORD,
    })
    assert r.status_code == 400, r.text


def test_same_as_current_rejected(api_url, admin_headers):
    r = requests.put(f"{api_url}/auth/email", headers=admin_headers, json={
        "new_email": ADMIN_EMAIL,
        "current_password": ADMIN_PASSWORD,
    })
    assert r.status_code == 400, r.text


def test_wrong_password_rejected(api_url, admin_headers):
    r = requests.put(f"{api_url}/auth/email", headers=admin_headers, json={
        "new_email": _fresh_email(),
        "current_password": "definitely-wrong",
    })
    assert r.status_code == 401, r.text


def test_missing_password_rejected(api_url, admin_headers):
    r = requests.put(f"{api_url}/auth/email", headers=admin_headers, json={
        "new_email": _fresh_email(),
    })
    assert r.status_code == 400, r.text


def test_taken_email_rejected(api_url, admin_headers):
    # CONTRIBUTOR_EMAIL already belongs to another account — re-auth succeeds,
    # then the address collision is caught (before any mailer/state change).
    r = requests.put(f"{api_url}/auth/email", headers=admin_headers, json={
        "new_email": CONTRIBUTOR_EMAIL,
        "current_password": ADMIN_PASSWORD,
    })
    assert r.status_code == 409, r.text


def test_requires_authentication(api_url):
    r = requests.put(f"{api_url}/auth/email", json={
        "new_email": _fresh_email(),
        "current_password": ADMIN_PASSWORD,
    })
    assert r.status_code == 401, r.text


def test_verify_missing_token(api_url):
    r = requests.post(f"{api_url}/auth/verify-email-change", json={})
    assert r.status_code == 400, r.text


def test_verify_bogus_token(api_url):
    r = requests.post(f"{api_url}/auth/verify-email-change", json={
        "token": "deadbeef" * 8,
    })
    assert r.status_code == 400, r.text


def test_cancel_requires_authentication(api_url):
    r = requests.delete(f"{api_url}/auth/email")
    assert r.status_code == 401, r.text


def test_cancel_pending_is_idempotent(api_url, admin_headers):
    # Cancelling clears any pending change; a no-op when nothing is pending still
    # succeeds (the recovery path must always leave a clean state). Lets a user
    # escape a stuck "pending" banner without DB access.
    r = requests.delete(f"{api_url}/auth/email", headers=admin_headers)
    assert r.status_code == 200, r.text
    me = requests.get(f"{api_url}/me", headers=admin_headers).json()
    assert not me.get("pending_email"), me
