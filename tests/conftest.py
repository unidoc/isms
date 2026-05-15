"""ISMS API test configuration.

Fully self-contained — creates its own org, users, and template.
No pre-existing users or CLI setup needed.

Server requirements:
    ISMS_USER_SIGNUP=1          Enable self-registration
    ISMS_SKIP_EMAIL_VERIFY=1    Skip email verification (users active immediately)
    ISMS_RATE_LIMIT=0           Disable rate limiting for tests

Usage:
    ISMS_TEST_URL=http://localhost:9090 pytest tests/ -v
"""
import os
import pytest
import requests

BASE_URL = os.environ.get("ISMS_TEST_URL", "http://localhost:9090")
API = f"{BASE_URL}/api/v1"

# Test credentials — all created by conftest, no pre-existing users needed
ORG_NAME = "Test Organization"
ORG_SLUG = "test-org"
ADMIN_EMAIL = "testadmin@isms-test.local"
ADMIN_PASSWORD = "TestPass123!"
ADMIN_NAME = "Test Admin"
READER_EMAIL = "testreviewer@isms-test.local"
READER_PASSWORD = "TestPass123!"
READER_NAME = "Test Reviewer"
CONTRIBUTOR_EMAIL = "testcontributor@isms-test.local"
CONTRIBUTOR_PASSWORD = "TestPass123!"
CONTRIBUTOR_NAME = "Test Contributor"


def _signup(email, password, name):
    """Sign up a new user. Returns token or None."""
    r = requests.post(f"{API}/auth/signup", json={
        "email": email, "password": password, "name": name,
    })
    if r.status_code in [200, 201]:
        data = r.json()
        return data.get("token")
    return None


def _login(email, password, org=ORG_SLUG):
    """Login and return token."""
    body = {"email": email, "password": password}
    if org:
        body["organization"] = org
    r = requests.post(f"{API}/auth/login", json=body)
    if r.status_code == 200:
        return r.json().get("token")
    return None


@pytest.fixture(scope="session")
def api_url():
    return API


@pytest.fixture(scope="session")
def base_url():
    return BASE_URL


@pytest.fixture(scope="session")
def test_setup():
    """One-time setup: signup users, create org, add template.

    Fully self-contained — no pre-existing users or CLI setup needed.
    Idempotent — safe to run against a DB that already has these users.
    """
    # 1. Sign up admin (returns token directly with ISMS_SKIP_EMAIL_VERIFY=1)
    token = _signup(ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_NAME)
    if token is None:
        # Already exists — login without org (no org yet on first run)
        token = _login(ADMIN_EMAIL, ADMIN_PASSWORD, ORG_SLUG)
    if token is None:
        token = _login(ADMIN_EMAIL, ADMIN_PASSWORD, "")
    assert token is not None, \
        f"Cannot signup or login as {ADMIN_EMAIL}. Check ISMS_USER_SIGNUP=1 and ISMS_SKIP_EMAIL_VERIFY=1"

    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}

    # 2. Create test org with ISO 27001 template (idempotent — 409 if exists)
    requests.post(f"{API}/organizations", headers=headers, json={
        "name": ORG_NAME, "slug": ORG_SLUG, "template": "iso27001",
    })

    # 3. Re-login scoped to test org to get org-scoped token
    org_token = _login(ADMIN_EMAIL, ADMIN_PASSWORD, ORG_SLUG)
    if org_token:
        token = org_token
        headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}

    # 4. Sign up reader and contributor
    _signup(READER_EMAIL, READER_PASSWORD, READER_NAME)
    _signup(CONTRIBUTOR_EMAIL, CONTRIBUTOR_PASSWORD, CONTRIBUTOR_NAME)

    # 5. Invite to org with roles (upsert — safe if already member)
    requests.post(f"{API}/users", headers=headers, json={
        "email": READER_EMAIL, "name": READER_NAME, "role": "reader",
    })
    requests.post(f"{API}/users", headers=headers, json={
        "email": CONTRIBUTOR_EMAIL, "name": CONTRIBUTOR_NAME, "role": "contributor",
    })

    # 6. Add ISO 27001 template (idempotent)
    requests.post(f"{API}/templates", headers=headers, json={"template": "iso27001"})

    # 7. Close any stale reviews from previous test runs (idempotent cleanup)
    r = requests.get(f"{API}/reviews", headers=headers)
    if r.status_code == 200:
        revs = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        for rv in (revs or []):
            if rv.get("status") in ("open", "approved", "changes_requested"):
                requests.put(f"{API}/reviews/{rv['id']}/status",
                             headers=headers, json={"status": "closed"})

    return {
        "admin_token": token,
        "org_slug": ORG_SLUG,
    }


@pytest.fixture(scope="session")
def admin_token(test_setup):
    return test_setup["admin_token"]


@pytest.fixture(scope="session")
def admin_headers(admin_token):
    return {"Authorization": f"Bearer {admin_token}", "Content-Type": "application/json"}


@pytest.fixture(scope="session")
def reader_token(test_setup):
    token = _login(READER_EMAIL, READER_PASSWORD, ORG_SLUG)
    assert token is not None, f"Reader login failed for {READER_EMAIL}"
    return token


@pytest.fixture(scope="session")
def reader_headers(reader_token):
    return {"Authorization": f"Bearer {reader_token}", "Content-Type": "application/json"}


@pytest.fixture(scope="session")
def contributor_token(test_setup):
    token = _login(CONTRIBUTOR_EMAIL, CONTRIBUTOR_PASSWORD, ORG_SLUG)
    assert token is not None, f"Contributor login failed for {CONTRIBUTOR_EMAIL}"
    return token


@pytest.fixture(scope="session")
def contributor_headers(contributor_token):
    return {"Authorization": f"Bearer {contributor_token}", "Content-Type": "application/json"}
