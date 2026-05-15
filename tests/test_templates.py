"""Template management tests."""
import requests


def test_list_available_templates(api_url, admin_headers):
    """GET /templates/available returns templates from disk registry."""
    r = requests.get(f"{api_url}/templates/available", headers=admin_headers)
    assert r.status_code == 200
    data = r.json().get("data") or r.json()
    assert isinstance(data, list)
    assert len(data) >= 1
    # Each template should have id, name, description
    for t in data:
        assert "id" in t, f"Template missing id: {t}"
        assert "name" in t, f"Template missing name: {t}"
        assert "description" in t, f"Template missing description: {t}"


def test_available_includes_iso27001(api_url, admin_headers):
    """ISO 27001 template should be in available list."""
    r = requests.get(f"{api_url}/templates/available", headers=admin_headers)
    data = r.json().get("data") or r.json()
    ids = [t["id"] for t in data]
    assert "iso27001" in ids


def test_available_no_nis2_lite(api_url, admin_headers):
    """nis2-lite should NOT be in available list (removed)."""
    r = requests.get(f"{api_url}/templates/available", headers=admin_headers)
    data = r.json().get("data") or r.json()
    ids = [t["id"] for t in data]
    assert "nis2-lite" not in ids


def test_add_template(api_url, admin_headers):
    r = requests.post(f"{api_url}/templates", headers=admin_headers,
                      json={"template": "soc2"})
    assert r.status_code == 201, f"Expected 201, got {r.status_code}: {r.text}"
    assert r.json()["status"] == "scaffolded"


def test_add_invalid_template(api_url, admin_headers):
    r = requests.post(f"{api_url}/templates", headers=admin_headers,
                      json={"template": "invalid"})
    assert r.status_code == 400


def test_documents_after_add(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
    assert r.status_code == 200
    folders = [f["name"] for f in r.json()["data"]]
    assert "iso27001" in folders


def test_remove_template(api_url, admin_headers):
    # Add nis2 first, then remove it
    requests.post(f"{api_url}/templates", headers=admin_headers,
                  json={"template": "nis2"})

    r = requests.delete(f"{api_url}/templates/nis2", headers=admin_headers)
    assert r.status_code == 200, f"Remove failed: {r.text}"


def test_remove_nonexistent_template(api_url, admin_headers):
    r = requests.delete(f"{api_url}/templates/invalid", headers=admin_headers)
    assert r.status_code == 400
