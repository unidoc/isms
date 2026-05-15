"""Cross-reference tests."""
import requests


def test_create_reference(api_url, admin_headers):
    r = requests.post(f"{api_url}/references", headers=admin_headers, json={
        "source_type": "risk",
        "source_id": "RISK-1",
        "target_type": "document",
        "target_id": "iso27001-4-1",
    })
    assert r.status_code in [200, 201], f"Create ref failed: {r.text}"


def test_list_references(api_url, admin_headers):
    r = requests.get(f"{api_url}/references?type=risk&id=RISK-1", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    assert len(data["data"]) >= 1


def test_bidirectional(api_url, admin_headers):
    """Reference should be findable from both sides."""
    r1 = requests.get(f"{api_url}/references?type=risk&id=RISK-1", headers=admin_headers)
    r2 = requests.get(f"{api_url}/references?type=document&id=iso27001-4-1", headers=admin_headers)
    assert r1.status_code == 200
    assert r2.status_code == 200
    assert len(r1.json()["data"]) >= 1
    assert len(r2.json()["data"]) >= 1
