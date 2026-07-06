"""#26 slice 1 (CA): corrective actions share one enforced write path.

Closes the two CA bypasses the epic names:
  - suggestion-apply seeded a different starting state ("assessment" vs "todo")
  - suggestion-apply skipped resolved_at / resolved_by_id on →resolved
Both HTTP create/update and suggestion-apply now go through the same code.
"""
import uuid

import requests


def _ca_by_identifier(api_url, headers, identifier, q=""):
    # Search by the (unique) title rather than scanning the first page — the test
    # stack is persistent, so a plain list is paginated and a freshly-created CA
    # can fall outside it (GET /corrective-actions/:id only accepts a numeric id).
    r = requests.get(f"{api_url}/corrective-actions", headers=headers,
                     params={"q": q, "limit": 100})
    assert r.status_code == 200, r.text
    data = r.json()
    items = data.get("data", data) if isinstance(data, dict) else data
    return next((x for x in (items or []) if x.get("identifier") == identifier), None)


def test_apply_create_uses_handler_default_status(api_url, admin_headers):
    """A CA created via suggestion-apply starts in the same state as one created
    over HTTP — 'todo', not the old apply-only 'assessment'."""
    title = f"ca-writepath-{uuid.uuid4().hex[:8]}"
    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "corrective_action",
        "suggestion_type": "create",
        "title": title,
        "rationale": "from audit",
        "payload": {"title": title, "description": "fix it"},
    })
    assert sg.status_code in (200, 201), sg.text
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply", headers=admin_headers, json={})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text

    ident = ap.json().get("applied_entity_id")
    ca = _ca_by_identifier(api_url, admin_headers, ident, q=title)
    assert ca is not None, f"CA {ident} not found after apply"
    assert ca["status"] == "todo", f"apply-created CA should default to 'todo', got {ca['status']!r}"


def test_apply_resolve_sets_resolved_at(api_url, admin_headers):
    """Resolving a CA via suggestion-apply stamps resolved_at — the closure
    metadata suggestion-apply previously skipped."""
    r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
        "title": f"ca-resolve-{uuid.uuid4().hex[:8]}", "description": "x",
    })
    assert r.status_code in (200, 201), r.text
    ca_id = r.json()["id"]

    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "corrective_action",
        "suggestion_type": "update",
        "entity_id": str(ca_id),
        "title": "Resolve CA",
        "rationale": "done",
        "payload": {"fields": {"status": "resolved"}},
    })
    assert sg.status_code in (200, 201), sg.text
    # force=true bypasses stale-detection (the fresh CA's create changelog would
    # otherwise flag it) so the enforced resolve path is what's exercised.
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply", headers=admin_headers, json={"force": True})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text

    got = requests.get(f"{api_url}/corrective-actions/{ca_id}", headers=admin_headers).json()
    assert got.get("status") == "resolved", got
    assert got.get("resolved_at"), "resolved_at must be stamped by the enforced write path"
