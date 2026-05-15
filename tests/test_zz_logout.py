"""Logout test — runs last (prefixed zz_ for ordering)."""
import requests
from conftest import API, ADMIN_EMAIL, ADMIN_PASSWORD, ORG_SLUG


def test_logout(api_url):
    """Test logout with a fresh token (not the session-scoped one)."""
    login = requests.post(f"{api_url}/auth/login", json={
        "email": ADMIN_EMAIL,
        "password": ADMIN_PASSWORD,
        "organization": ORG_SLUG,
    })
    assert login.status_code == 200, f"Login failed: {login.text}"
    token = login.json()["token"]
    headers = {"Authorization": f"Bearer {token}"}

    r = requests.get(f"{api_url}/me", headers=headers)
    assert r.status_code == 200

    r = requests.post(f"{api_url}/auth/logout", headers=headers, json={})
    assert r.status_code == 200

    r = requests.get(f"{api_url}/me", headers=headers)
    assert r.status_code == 401
