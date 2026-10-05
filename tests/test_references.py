"""Cross-reference tests."""
import uuid

import pytest
import requests


def _links(refs, typ, ident):
    """Entries whose either end is the given (type, identifier)."""
    return [x for x in refs
            if (x.get("target_type"), x.get("target_id")) == (typ, ident)
            or (x.get("source_type"), x.get("source_id")) == (typ, ident)]


@pytest.fixture(scope="module")
def risk_id(api_url, admin_headers):
    """A risk of this module's own, so the tests don't depend on a risk having
    been created by another test file (#405)."""
    r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
        "title": f"ref-test-risk-{uuid.uuid4().hex[:8]}",
        "current_likelihood": 2, "current_impact": 3, "risk_type": "threat",
        "origin": "external", "status": "open", "treatment": "mitigate",
    })
    assert r.status_code in (200, 201), f"Create risk failed: {r.text}"
    return r.json()["identifier"]


def test_create_reference(api_url, admin_headers, risk_id):
    r = requests.post(f"{api_url}/references", headers=admin_headers, json={
        "source_type": "risk",
        "source_id": risk_id,
        "target_type": "document",
        "target_id": "iso27001-4-1",
    })
    assert r.status_code in [200, 201], f"Create ref failed: {r.text}"


def test_list_references(api_url, admin_headers, risk_id):
    r = requests.get(f"{api_url}/references?type=risk&id={risk_id}", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    assert len(_links(data["data"], "document", "iso27001-4-1")) >= 1


def test_bidirectional(api_url, admin_headers, risk_id):
    """Reference should be findable from both sides."""
    r1 = requests.get(f"{api_url}/references?type=risk&id={risk_id}", headers=admin_headers)
    r2 = requests.get(f"{api_url}/references?type=document&id=iso27001-4-1", headers=admin_headers)
    assert r1.status_code == 200
    assert r2.status_code == 200
    assert len(_links(r1.json()["data"], "document", "iso27001-4-1")) >= 1
    assert len(_links(r2.json()["data"], "risk", risk_id)) >= 1
