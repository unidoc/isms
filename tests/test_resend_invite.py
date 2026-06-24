"""Regression for #42: resend-invite for pending invited users.

The happy path (resend succeeds, email sent) needs SMTP, which the test stack
doesn't configure — so this covers the auth + eligibility guards that run before
the mailer: only admins/managers may resend, only pending users qualify, and an
unknown email 404s. (Those guards are the safety-relevant part of the endpoint.)
"""
import uuid

import requests
from conftest import ADMIN_EMAIL, CONTRIBUTOR_EMAIL


def test_resend_requires_manager_or_admin(api_url, contributor_headers):
    r = requests.post(f"{api_url}/auth/resend-invite", headers=contributor_headers,
                      json={"email": ADMIN_EMAIL})
    assert r.status_code == 403, f"contributors must not resend invites: {r.status_code} {r.text}"


def test_resend_to_active_user_is_rejected(api_url, admin_headers):
    # CONTRIBUTOR_EMAIL is an active, accepted member — nothing to resend.
    r = requests.post(f"{api_url}/auth/resend-invite", headers=admin_headers,
                      json={"email": CONTRIBUTOR_EMAIL})
    assert r.status_code == 409, f"resend to an active user should be 409: {r.status_code} {r.text}"


def test_resend_to_unknown_user_404(api_url, admin_headers):
    r = requests.post(f"{api_url}/auth/resend-invite", headers=admin_headers,
                      json={"email": f"nobody-{uuid.uuid4().hex[:8]}@isms-test.local"})
    assert r.status_code == 404, f"resend to an unknown user should be 404: {r.status_code} {r.text}"


def test_resend_requires_email(api_url, admin_headers):
    r = requests.post(f"{api_url}/auth/resend-invite", headers=admin_headers, json={})
    assert r.status_code == 400, f"missing email should be 400: {r.status_code} {r.text}"
