"""Regression (#177): applying an *update* suggestion whose entity_id is the
per-org IDENTIFIER form (CR-n / SYSTEM-n / ASSET-n / objective display_id) must
resolve to the correct row via lookup — not by stripping the prefix to an int.

The identifier suffix is a per-org sequence, which on a shared/persistent stack
differs from the global primary key. applyChange/Objective/System/AssetUpdate
used to do `parseEntityID` (strip → int) and then GetX(thatInt), so an
identifier-form suggestion silently hit the wrong row (or 'not found'). They now
resolve via GetXByIdentifier / GetObjectiveByDisplayID (mirrors #174 for
task/incident/CA).

force=true on apply bypasses stale-detection (a fresh entity's own create
changelog would otherwise flag it), so the resolve/apply path is what's exercised.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL


def _apply_update_by(api_url, headers, entity_type, entity_ref, fields):
    """Create + apply an update suggestion addressing the entity by entity_ref
    (here: its identifier form). Asserts the apply itself succeeds."""
    sg = requests.post(f"{api_url}/suggestions", headers=headers, json={
        "entity_type": entity_type,
        "suggestion_type": "update",
        "entity_id": str(entity_ref),
        "title": "identifier-resolve regression",
        "rationale": "#177",
        "payload": {"fields": fields},
    })
    assert sg.status_code in (200, 201), f"create suggestion ({entity_type}): {sg.text}"
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                       headers=headers, json={"force": True})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", \
        f"apply by identifier must resolve+succeed ({entity_type}, ref={entity_ref}): {ap.status_code} {ap.text}"


def test_change_update_by_identifier(api_url, admin_headers):
    r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
        "title": f"chg-{uuid.uuid4().hex[:8]}", "description": "d", "justification": "j",
        "priority": "high", "category": "technology", "risk_level": "medium",
        "rollback_plan": "revert",
    })
    assert r.status_code in (200, 201), r.text
    cr = r.json()
    assert cr["identifier"].startswith("CR-"), cr

    _apply_update_by(api_url, admin_headers, "change_request", cr["identifier"], {"priority": "low"})

    got = requests.get(f"{api_url}/changes/{cr['id']}", headers=admin_headers).json()
    assert got.get("priority") == "low", f"identifier-form apply hit the wrong change: {got}"


def test_system_update_by_identifier(api_url, admin_headers):
    r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
        "name": f"sys-{uuid.uuid4().hex[:8]}", "classification": "internal", "criticality": "low",
    })
    assert r.status_code in (200, 201), r.text
    sys = r.json()
    assert sys["identifier"].startswith("SYSTEM-"), sys
    new_name = f"sys-renamed-{uuid.uuid4().hex[:8]}"

    _apply_update_by(api_url, admin_headers, "system", sys["identifier"], {"name": new_name})

    got = requests.get(f"{api_url}/systems/{sys['id']}", headers=admin_headers).json()
    assert got.get("name") == new_name, f"identifier-form apply hit the wrong system: {got}"


def test_asset_update_by_identifier(api_url, admin_headers):
    r = requests.post(f"{api_url}/assets", headers=admin_headers, json={
        "name": f"ast-{uuid.uuid4().hex[:8]}", "asset_type": "system", "status": "open",
        "confidentiality": 3, "integrity": 3, "availability": 3,
    })
    assert r.status_code in (200, 201), r.text
    asset = r.json()
    assert asset["identifier"].startswith("ASSET-"), asset
    new_name = f"ast-renamed-{uuid.uuid4().hex[:8]}"

    _apply_update_by(api_url, admin_headers, "asset", asset["identifier"], {"name": new_name})

    got = requests.get(f"{api_url}/assets/{asset['id']}", headers=admin_headers).json()
    assert got.get("name") == new_name, f"identifier-form apply hit the wrong asset: {got}"


def test_objective_update_by_display_id(api_url, admin_headers):
    # Objectives need a programme; its display_id (e.g. SEC2026-1) is the identifier form.
    requests.post(f"{api_url}/programs", headers=admin_headers, json={
        "title": "Sec Programme", "owner": ADMIN_EMAIL, "key": "SEC2026",
    })
    progs = requests.get(f"{api_url}/programs", headers=admin_headers).json().get("data") or []
    assert progs, "need at least one programme to create an objective"

    r = requests.post(f"{api_url}/objectives", headers=admin_headers, json={
        "program_id": progs[0]["id"], "title": f"obj-{uuid.uuid4().hex[:8]}",
        "owner": ADMIN_EMAIL, "target_value": 5.0, "target_operator": "lte", "unit": "%",
    })
    assert r.status_code in (200, 201), r.text
    obj = r.json()
    display_id = obj.get("display_id")
    assert display_id, f"objective response missing display_id: {obj}"
    new_title = f"obj-renamed-{uuid.uuid4().hex[:8]}"

    _apply_update_by(api_url, admin_headers, "objective", display_id, {"title": new_title})

    got = requests.get(f"{api_url}/objectives/{obj['id']}", headers=admin_headers).json()
    assert got.get("title") == new_title, f"display_id apply hit the wrong objective: {got}"
