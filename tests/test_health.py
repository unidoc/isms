"""Basic health and connectivity tests."""
import requests


def test_health(base_url):
    r = requests.get(f"{base_url}/healthz")
    assert r.status_code == 200
    body = r.json()
    assert body.get("status") == "ok"


def test_api_docs(base_url):
    r = requests.get(f"{base_url}/docs")
    assert r.status_code == 200
    assert "api-reference" in r.text.lower() or "scalar" in r.text.lower()


def test_openapi_spec(base_url):
    r = requests.get(f"{base_url}/api/openapi.yaml")
    assert r.status_code == 200
    assert "openapi" in r.text
    assert "isms" in r.text.lower()
