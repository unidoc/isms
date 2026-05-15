"""Authentication and authorization tests."""
import requests
from conftest import API, ADMIN_EMAIL, ADMIN_PASSWORD, READER_EMAIL, READER_PASSWORD, ORG_SLUG


def test_login_admin(api_url):
    r = requests.post(f"{api_url}/auth/login", json={
        "email": ADMIN_EMAIL,
        "password": ADMIN_PASSWORD,
        "organization": ORG_SLUG,
    })
    assert r.status_code == 200
    data = r.json()
    assert "token" in data
    assert data["email"] == ADMIN_EMAIL
    assert data["role"] == "admin"


def test_login_reader(api_url):
    r = requests.post(f"{api_url}/auth/login", json={
        "email": READER_EMAIL,
        "password": READER_PASSWORD,
        "organization": ORG_SLUG,
    })
    assert r.status_code == 200
    assert r.json()["role"] == "reader"


def test_login_wrong_password(api_url):
    r = requests.post(f"{api_url}/auth/login", json={
        "email": ADMIN_EMAIL,
        "password": "wrongpassword",
        "organization": ORG_SLUG,
    })
    assert r.status_code == 401


def test_me(api_url, admin_headers):
    r = requests.get(f"{api_url}/me", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert data["email"] == ADMIN_EMAIL
    assert data["role"] == "admin"
    assert data["authenticated"] is True


def test_unauthenticated_access(api_url):
    r = requests.get(f"{api_url}/me")
    assert r.status_code == 401
