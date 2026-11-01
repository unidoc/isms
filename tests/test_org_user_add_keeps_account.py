"""Adding a person to an org must only touch that org's membership.

Users are global (one account per email). Inviting or adding an existing
account from an org-scoped endpoint used to rewrite the account itself: invite
deactivated it (locking the person out of every org) and POST /users reset its
name. The test stack has no SMTP, so an invite of an existing active account
must succeed as a plain membership add.

Every test creates FRESH accounts: the shared fixture users must never be
touched here, or a regression would lock them out for the rest of the run.
"""
import uuid

import requests
from conftest import API

PASSWORD = "TestPass123!"


def _fresh(label):
    email = f"{label}-{uuid.uuid4().hex[:10]}@isms-test.local"
    name = f"{label} person"
    r = requests.post(f"{API}/auth/signup", json={"email": email, "password": PASSWORD, "name": name})
    assert r.status_code in (200, 201), f"signup failed: {r.status_code} {r.text}"
    return email, name, r.json()["token"]


def _headers(token):
    return {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}


def _org_admin(label):
    """A fresh user who owns a fresh org; returns (email, slug, org-scoped headers)."""
    email, _, token = _fresh(label)
    slug = f"ouak-{uuid.uuid4().hex[:10]}"
    r = requests.post(f"{API}/organizations", headers=_headers(token), json={"name": slug, "slug": slug})
    assert r.status_code in (200, 201), f"org create failed: {r.status_code} {r.text}"
    r = requests.post(f"{API}/auth/login", json={"email": email, "password": PASSWORD, "organization": slug})
    assert r.status_code == 200, f"org admin login failed: {r.status_code} {r.text}"
    return email, slug, _headers(r.json()["token"])


def _login(email, org=None):
    body = {"email": email, "password": PASSWORD}
    if org:
        body["organization"] = org
    return requests.post(f"{API}/auth/login", json=body)


def _assert_still_themselves(email, name, org=None):
    r = _login(email, org)
    assert r.status_code == 200, f"account was locked out: {r.status_code} {r.text}"
    me = requests.get(f"{API}/me", headers=_headers(r.json()["token"]))
    assert me.status_code == 200, f"/me failed: {me.status_code} {me.text}"
    assert me.json().get("name") == name, f"name was overwritten: {me.text}"


def test_same_org_reinvite_keeps_active_member_working():
    admin_email, slug, admin = _org_admin("ouak-admin")
    email, name, _ = _fresh("ouak-member")
    r = requests.post(f"{API}/users", headers=admin, json={"email": email, "name": name, "role": "reader"})
    assert r.status_code == 200, f"add member: {r.status_code} {r.text}"

    r = requests.post(f"{API}/auth/invite", headers=admin,
                      json={"email": email, "name": "Other Name", "role": "contributor"})
    assert r.status_code == 200, f"re-invite of an active member: {r.status_code} {r.text}"
    assert r.json().get("status") == "added", r.text
    _assert_still_themselves(email, name, slug)


def test_cross_org_invite_keeps_account():
    _, _, admin_b = _org_admin("ouak-adminb")
    email, name, _ = _fresh("ouak-victim")

    r = requests.post(f"{API}/auth/invite", headers=admin_b,
                      json={"email": email, "name": "Hijacked", "role": "reader"})
    assert r.status_code == 200, f"cross-org invite: {r.status_code} {r.text}"
    assert r.json().get("status") == "added", r.text
    _assert_still_themselves(email, name)


def test_cross_org_add_user_keeps_account():
    _, slug_b, admin_b = _org_admin("ouak-adminc")
    email, name, _ = _fresh("ouak-victim2")

    r = requests.post(f"{API}/users", headers=admin_b,
                      json={"email": email, "name": "Hijacked", "role": "reader"})
    assert r.status_code == 200, f"cross-org add: {r.status_code} {r.text}"
    assert r.json().get("name") == name, f"response should show the real name: {r.text}"
    _assert_still_themselves(email, name)
    # The new membership works too.
    assert _login(email, slug_b).status_code == 200


def test_signup_with_different_case_email_is_refused():
    email, name, _ = _fresh("ouak-case")
    r = requests.post(f"{API}/auth/signup", json={
        "email": email.upper(), "password": "AttackerPass9!", "name": "Attacker"})
    assert r.status_code == 409, f"signup over an existing account (upper-cased email): {r.status_code} {r.text}"
    _assert_still_themselves(email, name)
    r = requests.post(f"{API}/auth/login", json={"email": email, "password": "AttackerPass9!"})
    assert r.status_code == 401, f"the second signup's password must not work: {r.status_code} {r.text}"
