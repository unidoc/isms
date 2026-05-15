"""Audit programme and audit tests — happy path + validation + isolation."""
import requests
from conftest import ADMIN_EMAIL


def test_create_programme(api_url, admin_headers):
    r = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
        "title": "2026 Annual Audit Programme",
        "year": 2026,
    })
    assert r.status_code in [200, 201], f"Failed: {r.text}"


def test_list_programmes(api_url, admin_headers):
    r = requests.get(f"{api_url}/audit/programmes", headers=admin_headers)
    assert r.status_code == 200
    assert isinstance(r.json()["data"], list)


def test_create_audit(api_url, admin_headers):
    progs = requests.get(f"{api_url}/audit/programmes", headers=admin_headers).json()["data"]
    if len(progs) > 0:
        r = requests.post(f"{api_url}/audits", headers=admin_headers, json={
            "programme_id": progs[0]["id"],
            "title": "Full scope internal audit",
            "scope": "iso27001-*",
            "auditor": ADMIN_EMAIL,
            "audit_type": "internal",
            "planned_date": 1781740800,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"


def test_reader_cannot_create_programme(api_url, reader_headers):
    r = requests.post(f"{api_url}/audit/programmes", headers=reader_headers, json={
        "title": "test", "year": 2026,
    })
    assert r.status_code == 403


# ---------------------------------------------------------------------------
# Validation: server should reject bad enums with 400, not 500.
# ---------------------------------------------------------------------------

def test_invalid_programme_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
        "title": "Bad status programme",
        "year": 2026,
        "status": "totally-made-up",
    })
    assert r.status_code == 400, r.text


def test_invalid_year_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
        "title": "Bad year programme",
        "year": 0,
    })
    assert r.status_code == 400, r.text


def test_invalid_audit_type_rejected(api_url, admin_headers):
    progs = requests.get(f"{api_url}/audit/programmes", headers=admin_headers).json()["data"]
    if not progs:
        return
    r = requests.post(f"{api_url}/audits", headers=admin_headers, json={
        "programme_id": progs[0]["id"],
        "title": "Bad type", "scope": "*",
        "auditor": ADMIN_EMAIL,
        "audit_type": "imaginary",
    })
    assert r.status_code == 400, r.text


def test_unknown_auditor_rejected(api_url, admin_headers):
    progs = requests.get(f"{api_url}/audit/programmes", headers=admin_headers).json()["data"]
    if not progs:
        return
    r = requests.post(f"{api_url}/audits", headers=admin_headers, json={
        "programme_id": progs[0]["id"],
        "title": "Ghost auditor", "scope": "*",
        "auditor": "ghost@nowhere.invalid",
        "audit_type": "internal",
    })
    assert r.status_code == 400, r.text


def test_unknown_programme_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/audits", headers=admin_headers, json={
        "programme_id": 999999,
        "title": "Orphan", "scope": "*",
        "auditor": ADMIN_EMAIL,
        "audit_type": "internal",
    })
    assert r.status_code == 400, r.text


def test_invalid_audit_status_rejected(api_url, admin_headers):
    progs = requests.get(f"{api_url}/audit/programmes", headers=admin_headers).json()["data"]
    if not progs:
        return
    create = requests.post(f"{api_url}/audits", headers=admin_headers, json={
        "programme_id": progs[0]["id"],
        "title": "Status guard test", "scope": "*",
        "auditor": ADMIN_EMAIL,
        "audit_type": "internal",
    })
    if create.status_code not in (200, 201):
        return
    audit_id = create.json()["id"]
    r = requests.put(f"{api_url}/audits/{audit_id}/status", headers=admin_headers, json={"status": "abandoned"})
    assert r.status_code == 400, r.text


# ---------------------------------------------------------------------------
# Programme delete is gated by audit count.
# ---------------------------------------------------------------------------

def test_delete_programme_with_audits_blocked(api_url, admin_headers):
    create = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
        "title": "Programme with audit", "year": 2026,
    })
    if create.status_code not in (200, 201):
        return
    pid = create.json()["id"]
    requests.post(f"{api_url}/audits", headers=admin_headers, json={
        "programme_id": pid,
        "title": "Linked audit", "scope": "*",
        "auditor": ADMIN_EMAIL, "audit_type": "internal",
    })
    r = requests.delete(f"{api_url}/audit/programmes/{pid}", headers=admin_headers)
    assert r.status_code == 409, r.text


def test_delete_empty_programme_succeeds(api_url, admin_headers):
    create = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
        "title": "Empty programme", "year": 2026,
    })
    if create.status_code not in (200, 201):
        return
    pid = create.json()["id"]
    r = requests.delete(f"{api_url}/audit/programmes/{pid}", headers=admin_headers)
    assert r.status_code == 200, r.text
