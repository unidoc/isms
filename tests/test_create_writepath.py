"""#26 slice A: create consistency — an entity created via suggestion-apply must
land in the same default state as one created over HTTP (shared applyXDefaults).

The headline bug: assets created via apply started 'active' while HTTP created
them 'open'. These tests assert the apply path now matches the HTTP defaults.

Search by the unique name (?q=) rather than scanning page 1 — the test stack is
persistent, so a freshly-created record can fall outside the first page.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL


def _apply_create(api_url, headers, entity_type, name_key, payload):
    payload = dict(payload)
    payload[name_key] = payload.get(name_key) or f"wp-{uuid.uuid4().hex[:8]}"
    sg = requests.post(f"{api_url}/suggestions", headers=headers, json={
        "entity_type": entity_type,
        "suggestion_type": "create",
        "title": payload[name_key],
        "rationale": "writepath consistency",
        "payload": payload,
    })
    assert sg.status_code in (200, 201), sg.text
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply", headers=headers, json={})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
    return ap.json().get("applied_entity_id"), payload[name_key]


def _find(api_url, headers, path, q, ident):
    r = requests.get(f"{api_url}{path}", headers=headers, params={"q": q, "limit": 100})
    assert r.status_code == 200, r.text
    data = r.json()
    items = data.get("data", data) if isinstance(data, dict) else data
    return next((x for x in (items or []) if x.get("identifier") == ident), None)


def test_asset_apply_matches_http_status(api_url, admin_headers):
    """Apply-created asset defaults to 'open' (HTTP default), not the old 'active'."""
    ident, name = _apply_create(api_url, admin_headers, "asset", "name", {})
    a = _find(api_url, admin_headers, "/assets", name, ident)
    assert a is not None, f"asset {ident} not found"
    assert a["status"] == "open", f"apply asset should default to 'open', got {a['status']!r}"
    assert a.get("owner"), "apply asset should default owner to the actor"


def test_legal_apply_sets_status_and_owner(api_url, admin_headers):
    ident, name = _apply_create(api_url, admin_headers, "legal_requirement", "title", {})
    lr = _find(api_url, admin_headers, "/legal", name, ident)
    assert lr is not None, f"legal {ident} not found"
    assert lr["status"] == "open", f"apply legal should default status 'open', got {lr['status']!r}"
    assert lr.get("owner"), "apply legal should default owner to the actor"


def test_system_apply_sets_status(api_url, admin_headers):
    ident, name = _apply_create(api_url, admin_headers, "system", "name", {})
    sysrec = _find(api_url, admin_headers, "/systems", name, ident)
    assert sysrec is not None, f"system {ident} not found"
    assert sysrec["status"] == "active", f"apply system should default status 'active', got {sysrec['status']!r}"
    assert sysrec.get("owner"), "apply system should default owner to the actor"


def test_supplier_apply_sets_owner(api_url, admin_headers):
    ident, name = _apply_create(api_url, admin_headers, "supplier", "name", {})
    sup = _find(api_url, admin_headers, "/suppliers", name, ident)
    assert sup is not None, f"supplier {ident} not found"
    assert sup["status"] == "active", f"apply supplier status should be 'active', got {sup['status']!r}"
    assert sup.get("owner") == ADMIN_EMAIL, f"apply supplier should default owner to actor, got {sup.get('owner')!r}"
