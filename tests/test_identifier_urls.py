"""Entity endpoints accept the identifier form in the URL, not just numeric id.

Regression for the API asymmetry where task/incident/corrective-action/legal
(and audit-finding) endpoints parsed the :id param with raw strconv.Atoi, so only
a numeric id worked and the identifier form (TASK-6, INC-3, …) returned 400. The
fix routed them through parseID (same helper suppliers/assets/risks/systems use)
and standardized these entities' id to int64.

Audit findings use the exact same parseID path but need a programme+audit to
exist first, so they're not set up here — the mechanism is identical.
"""
import uuid

import pytest
import requests

ENTITIES = [
    {"name": "task", "path": "tasks",
     "create": {"task_type": "general", "status": "open", "priority": "medium"}},
    {"name": "incident", "path": "incidents",
     "create": {"severity": "medium"}},
    {"name": "corrective_action", "path": "corrective-actions",
     "create": {"description": "identifier url regression"}},
    {"name": "legal", "path": "legal",
     "create": {"jurisdiction": "EU", "category": "privacy"}},
]


@pytest.mark.parametrize("cfg", ENTITIES, ids=[e["name"] for e in ENTITIES])
def test_identifier_and_numeric_urls_resolve_same_record(cfg, api_url, admin_headers):
    path = cfg["path"]
    payload = dict(cfg["create"])
    payload["title"] = f"idurl-{cfg['name']}-{uuid.uuid4().hex[:8]}"

    r = requests.post(f"{api_url}/{path}", headers=admin_headers, json=payload)
    assert r.status_code in (200, 201), f"create {cfg['name']} failed: {r.text}"
    ent = r.json()
    num_id = ent["id"]
    ident = ent["identifier"]
    assert ident, f"expected an identifier on {cfg['name']}"

    # GET by identifier and by numeric id resolve the same record.
    by_ident = requests.get(f"{api_url}/{path}/{ident}", headers=admin_headers)
    by_num = requests.get(f"{api_url}/{path}/{num_id}", headers=admin_headers)
    assert by_ident.status_code == 200, f"GET {path}/{ident} failed: {by_ident.text}"
    assert by_num.status_code == 200, f"GET {path}/{num_id} failed: {by_num.text}"
    assert by_ident.json()["id"] == num_id == by_num.json()["id"]

    # PUT accepts the identifier form (write path) — full valid object, tweaked title.
    upd = dict(payload)
    upd["title"] = payload["title"] + " (updated)"
    up = requests.put(f"{api_url}/{path}/{ident}", headers=admin_headers, json=upd)
    assert up.status_code in (200, 204), f"PUT {path}/{ident} failed: {up.text}"

    # DELETE accepts the identifier form too.
    dele = requests.delete(f"{api_url}/{path}/{ident}", headers=admin_headers)
    assert dele.status_code in (200, 204), f"DELETE {path}/{ident} failed: {dele.text}"


def test_garbage_identifier_still_400(api_url, admin_headers):
    """A non-numeric, non-identifier id is still rejected (parseID didn't loosen this)."""
    r = requests.get(f"{api_url}/tasks/not-an-id", headers=admin_headers)
    assert r.status_code == 400, r.text
