"""The unauthenticated OIDC provider list must not leak provider secrets.

GET /auth/oidc/providers is public (the login page calls it before sign-in).
It may expose only id, provider_name and display_name.
"""
import requests

from conftest import ORG_SLUG

SECRET = "pytest-oidc-secret-must-not-leak-4c1d"


def test_public_provider_list_hides_secret_and_config(api_url, admin_headers):
    r = requests.post(
        f"{api_url}/admin/oidc",
        headers=admin_headers,
        json={
            "provider_name": "pytest-leak-probe",
            "display_name": "Leak Probe",
            "client_id": "pytest-client-id",
            "client_secret": SECRET,
            "discovery_url": "https://idp.example.com/.well-known/openid-configuration",
            "scopes": "openid email",
            "enabled": True,
        },
    )
    assert r.status_code == 201, r.text
    pid = r.json()["id"]
    try:
        # No Authorization header: this is the public route.
        pub = requests.get(f"{api_url}/auth/oidc/providers", params={"org": ORG_SLUG})
        assert pub.status_code == 200, pub.text
        assert SECRET not in pub.text, f"client secret leaked: {pub.text}"
        items = [p for p in pub.json()["data"] if p["provider_name"] == "pytest-leak-probe"]
        assert len(items) == 1, pub.text
        assert set(items[0].keys()) == {"id", "provider_name", "display_name"}, items[0]
        assert items[0]["display_name"] == "Leak Probe"
        for p in pub.json()["data"]:
            assert set(p.keys()) == {"id", "provider_name", "display_name"}, p
    finally:
        d = requests.delete(f"{api_url}/admin/oidc/{pid}", headers=admin_headers)
        assert d.status_code in (200, 204), d.text
