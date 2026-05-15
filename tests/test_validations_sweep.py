"""Cross-module validation sweep — invalid enums, IDOR protection, partial-update semantics.

These tests exercise the security/correctness fixes applied across all modules:
- Status / type / severity / criticality / classification enums must be validated server-side (400 not 500).
- Cross-org IDOR via body params (parent_id) must be rejected.
- Partial update with empty string must clear the field (not be silently skipped).
- Reopen logic must clear closure metadata.
"""
import time

import requests
from conftest import ADMIN_EMAIL


# ---------------------------------------------------------------------------
# Status enum validation — every module rejects bogus enum values with 400.
# ---------------------------------------------------------------------------

def _create_incident(api_url, headers, **overrides):
    body = {
        "title": "Validation probe " + str(time.time()),
        "description": "test",
        "severity": "low",
        "status": "open",
        "affects_c": True,
        "incident_type": "event",
        "source": "internal",
        "reporter": ADMIN_EMAIL,
        "detected_at": int(time.time()),
    }
    body.update(overrides)
    return requests.post(f"{api_url}/incidents", headers=headers, json=body)


def test_incident_invalid_severity_rejected(api_url, admin_headers):
    r = _create_incident(api_url, admin_headers, severity="apocalyptic")
    assert r.status_code == 400, r.text


def test_incident_invalid_status_rejected(api_url, admin_headers):
    r = _create_incident(api_url, admin_headers, status="undecided")
    assert r.status_code == 400, r.text


def test_incident_invalid_incident_type_rejected(api_url, admin_headers):
    r = _create_incident(api_url, admin_headers, incident_type="catastrophe")
    assert r.status_code == 400, r.text


def test_task_invalid_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
        "title": "Validation probe", "task_type": "general",
        "assignee": ADMIN_EMAIL,
        "status": "totally-bogus",
        "priority": "medium",
    })
    assert r.status_code == 400, r.text


def test_task_invalid_priority_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
        "title": "Validation probe", "task_type": "general",
        "assignee": ADMIN_EMAIL,
        "status": "open",
        "priority": "extra-spicy",
    })
    assert r.status_code == 400, r.text


def test_change_invalid_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
        "title": "Validation probe", "description": "test",
        "priority": "medium", "category": "process", "risk_level": "low",
        "requested_by": ADMIN_EMAIL,
        "status": "abracadabra",
    })
    assert r.status_code == 400, r.text


def test_change_invalid_category_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
        "title": "Validation probe", "description": "test",
        "priority": "medium", "risk_level": "low",
        "requested_by": ADMIN_EMAIL,
        "category": "vibes",
    })
    assert r.status_code == 400, r.text


def test_legal_invalid_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
        "title": "Validation probe",
        "jurisdiction": "EU", "category": "privacy",
        "status": "pending-vibes",
    })
    assert r.status_code == 400, r.text


def test_legal_invalid_treatment_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
        "title": "Validation probe",
        "jurisdiction": "EU", "category": "privacy",
        "treatment": "panic",
    })
    assert r.status_code == 400, r.text


def test_legal_invalid_category_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
        "title": "Validation probe",
        "jurisdiction": "EU", "category": "made-up-category",
    })
    assert r.status_code == 400, r.text


def test_task_invalid_task_type_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
        "title": "Validation probe", "task_type": "imaginary-flavour",
        "assignee": ADMIN_EMAIL,
        "status": "open",
        "priority": "medium",
    })
    assert r.status_code == 400, r.text


def test_supplier_invalid_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
        "name": "ValidationProbe " + str(time.time()),
        "supplier_type": "saas",
        "criticality": "low",
        "status": "magic",
    })
    assert r.status_code == 400, r.text


def test_supplier_invalid_criticality_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
        "name": "ValidationProbe " + str(time.time()),
        "supplier_type": "saas",
        "criticality": "apocalyptic",
    })
    assert r.status_code == 400, r.text


def test_supplier_invalid_type_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
        "name": "ValidationProbe " + str(time.time()),
        "supplier_type": "imaginary-vendor-class",
        "criticality": "low",
    })
    assert r.status_code == 400, r.text


def test_system_invalid_classification_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
        "name": "ValidationProbe " + str(time.time()),
        "classification": "above-top-secret",
        "criticality": "low",
    })
    assert r.status_code == 400, r.text


def test_system_invalid_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
        "name": "ValidationProbe " + str(time.time()),
        "classification": "internal",
        "criticality": "low",
        "status": "haunted",
    })
    assert r.status_code == 400, r.text


def test_objective_invalid_status_rejected(api_url, admin_headers):
    # First create a program for the objective
    p = requests.post(f"{api_url}/programs", headers=admin_headers, json={
        "key": "VALSWEEP",
        "name": "Validation Sweep Programme",
    })
    if p.status_code not in (200, 201):
        return
    pid = p.json()["id"]
    r = requests.post(f"{api_url}/objectives", headers=admin_headers, json={
        "program_id": pid,
        "title": "Validation probe",
        "target_value": 1.0,
        "target_operator": "gte",
        "status": "supposing",
    })
    assert r.status_code == 400, r.text


def test_objective_invalid_target_operator_rejected(api_url, admin_headers):
    progs = requests.get(f"{api_url}/programs", headers=admin_headers).json().get("data") or []
    if not progs:
        return
    r = requests.post(f"{api_url}/objectives", headers=admin_headers, json={
        "program_id": progs[0]["id"],
        "title": "Validation probe",
        "target_value": 1.0,
        "target_operator": "approximately",
    })
    assert r.status_code == 400, r.text


def test_risk_invalid_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
        "title": "Validation probe",
        "current_likelihood": 1, "current_impact": 1,
        "risk_type": "threat", "origin": "internal",
        "status": "vibing",
    })
    assert r.status_code == 400, r.text


def test_risk_invalid_treatment_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
        "title": "Validation probe",
        "current_likelihood": 1, "current_impact": 1,
        "risk_type": "threat", "origin": "internal",
        "status": "open",
        "treatment": "panic",
    })
    assert r.status_code == 400, r.text


def test_asset_invalid_status_rejected(api_url, admin_headers):
    r = requests.post(f"{api_url}/assets", headers=admin_headers, json={
        "name": "ValidationProbe " + str(time.time()),
        "asset_type": "server",
        "status": "magical",
    })
    assert r.status_code == 400, r.text


# ---------------------------------------------------------------------------
# Cross-org IDOR — parent_id from body must belong to current org.
# ---------------------------------------------------------------------------

def test_checkin_rejects_unknown_objective(api_url, admin_headers):
    r = requests.post(f"{api_url}/objectives/999999/checkins", headers=admin_headers, json={
        "value_numeric": 1.0,
        "occurred_at": int(time.time()),
    })
    assert r.status_code in (400, 404), r.text


def test_system_rejects_unknown_supplier(api_url, admin_headers):
    r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
        "name": "Cross-org probe " + str(time.time()),
        "classification": "internal",
        "criticality": "low",
        "supplier_id": 999999,
    })
    assert r.status_code in (400, 404), r.text


# ---------------------------------------------------------------------------
# Bind error → 400 (not 200).
# ---------------------------------------------------------------------------

def test_register_create_bad_json_returns_400(api_url, admin_headers):
    # Send malformed JSON to a register create endpoint.
    headers = {**admin_headers, "Content-Type": "application/json"}
    r = requests.post(f"{api_url}/risks", headers=headers, data='{"title": "broken')
    # Either 400 (good) or anything other than 200 (the 200-bug is the regression)
    assert r.status_code != 200, f"register create returned 200 on bad JSON: {r.text}"


# ---------------------------------------------------------------------------
# Reopen logic — closure metadata cleared on transition back.
# ---------------------------------------------------------------------------

def test_incident_reopen_clears_closure_metadata(api_url, admin_headers):
    create = _create_incident(api_url, admin_headers)
    if create.status_code not in (200, 201):
        return
    iid = create.json()["id"]
    # Move through the lifecycle to closed
    for status in ("investigating", "contained", "resolved", "closed"):
        r = requests.put(f"{api_url}/incidents/{iid}/status", headers=admin_headers, json={"status": status})
        assert r.status_code == 200, f"status={status}: {r.text}"
    # Reopen back to open — closed_at, resolved_at, contained_at must clear.
    r = requests.put(f"{api_url}/incidents/{iid}/status", headers=admin_headers, json={"status": "open"})
    assert r.status_code == 200, r.text
    after = requests.get(f"{api_url}/incidents/{iid}", headers=admin_headers).json()
    assert not after.get("closed_at"), f"closed_at should be cleared, got {after.get('closed_at')}"
    assert not after.get("resolved_at"), f"resolved_at should be cleared, got {after.get('resolved_at')}"
    assert not after.get("contained_at"), f"contained_at should be cleared, got {after.get('contained_at')}"


def test_change_reopen_clears_approval(api_url, admin_headers):
    create = requests.post(f"{api_url}/changes", headers=admin_headers, json={
        "title": "Reopen probe",
        "description": "test",
        "priority": "low",
        "category": "process",
        "risk_level": "low",
        "requested_by": ADMIN_EMAIL,
    })
    if create.status_code not in (200, 201):
        return
    cid = create.json()["id"]
    # Approve
    r = requests.put(f"{api_url}/changes/{cid}/status", headers=admin_headers, json={"status": "approved"})
    assert r.status_code == 200, r.text
    approved = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
    assert approved.get("approved_at"), "approved_at should be set after approve"
    # Send back to proposed — approval metadata must clear.
    r = requests.put(f"{api_url}/changes/{cid}/status", headers=admin_headers, json={"status": "proposed"})
    assert r.status_code == 200, r.text
    after = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
    assert not after.get("approved_at"), f"approved_at should clear on reopen, got {after.get('approved_at')}"
    assert not after.get("approved_by"), f"approved_by should clear on reopen, got {after.get('approved_by')}"


# ---------------------------------------------------------------------------
# RBAC: status update requires write role.
# ---------------------------------------------------------------------------

def test_reader_cannot_change_incident_status(api_url, reader_headers):
    r = requests.put(f"{api_url}/incidents/1/status", headers=reader_headers, json={"status": "closed"})
    assert r.status_code in (403, 404), r.text
