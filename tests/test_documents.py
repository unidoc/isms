"""Document management tests."""
import requests


def test_list_documents(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    assert isinstance(data["data"], list)


def test_search_documents(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/search?q=security", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    results = data["data"]
    assert isinstance(results, list)
    assert len(results) > 0
    assert "document_id" in results[0]


def test_search_min_length(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/search?q=a", headers=admin_headers)
    assert r.status_code == 200
    assert len(r.json()["data"]) == 0  # too short, no results


def test_validate_documents(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/validate", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert data["valid"] is True


def test_needs_review(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/needs-review", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    assert isinstance(data["data"], list)


def test_recently_changed(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/changed", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
