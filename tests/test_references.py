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


# --- Audit programmes as a reference type (#394) ---

@pytest.fixture(scope="module")
def programme(api_url, admin_headers):
    r = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
        "title": f"ref-test-programme-{uuid.uuid4().hex[:8]}", "year": 2026,
    })
    assert r.status_code in (200, 201), f"Create programme failed: {r.text}"
    return r.json()


def test_programme_link_stored_as_ap_id(api_url, admin_headers, programme):
    """A bare row id and AP-<id> are the same programme: one pair, stored AP-<id>."""
    pid = str(programme["id"])
    ap = f"AP-{pid}"
    r = requests.post(f"{api_url}/references", headers=admin_headers, json={
        "source_type": "document", "source_id": "iso27001-4-1",
        "target_type": "audit_programme", "target_id": pid,
    })
    assert r.status_code == 201, r.text
    assert r.json()["target_id"] == ap
    r = requests.post(f"{api_url}/references", headers=admin_headers, json={
        "source_type": "document", "source_id": "iso27001-4-1",
        "target_type": "audit_programme", "target_id": ap,
    })
    assert r.status_code in (200, 201), r.text

    for ident in (ap, pid):
        r = requests.get(f"{api_url}/references?type=audit_programme&id={ident}", headers=admin_headers)
        assert r.status_code == 200
        links = _links(r.json()["data"], "document", "iso27001-4-1")
        assert len(links) == 1, f"?id={ident}: {r.text}"
        assert links[0]["title"], r.text

    # From the document side: the programme, under its title.
    r = requests.get(f"{api_url}/references?type=document&id=iso27001-4-1", headers=admin_headers)
    links = _links(r.json()["data"], "audit_programme", ap)
    assert len(links) == 1, r.text
    assert links[0]["title"] == programme["title"]


def test_programme_as_source(api_url, admin_headers, programme, risk_id):
    ap = f"AP-{programme['id']}"
    r = requests.post(f"{api_url}/references", headers=admin_headers, json={
        "source_type": "audit_programme", "source_id": str(programme["id"]),
        "target_type": "risk", "target_id": risk_id,
    })
    assert r.status_code == 201, r.text
    assert r.json()["source_id"] == ap


def test_programme_link_rejects_unknown_programme(api_url, admin_headers, risk_id):
    r = requests.post(f"{api_url}/references", headers=admin_headers, json={
        "source_type": "risk", "source_id": risk_id,
        "target_type": "audit_programme", "target_id": "99999999",
    })
    assert r.status_code == 400, r.text


def test_programme_comment_saves(api_url, admin_headers, programme):
    """The programme Discussion tab posts entity_type audit_programme."""
    r = requests.post(f"{api_url}/entity-comments", headers=admin_headers, json={
        "entity_type": "audit_programme", "entity_id": str(programme["id"]),
        "body": "programme comment",
    })
    assert r.status_code in (200, 201), r.text
