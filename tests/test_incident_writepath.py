"""#26 slice 1: one enforced incident write path.

Both writers — the HTTP update endpoint and suggestion-apply — go through the same
enforced function, so the open-CA guard (can't resolve/close an incident with open
corrective actions linked) fires identically. Before #26, suggestion-apply
bypassed the guard entirely: this test locks the parity.
"""
import uuid

import requests


def _mk_incident(api_url, headers):
    r = requests.post(f"{api_url}/incidents", headers=headers, json={
        "title": f"writepath-{uuid.uuid4().hex[:8]}", "severity": "medium",
    })
    assert r.status_code in (200, 201), r.text
    inc = r.json()
    return inc["id"], inc["identifier"]


def _mk_open_ca_linked(api_url, headers, incident_identifier):
    """Create a (non-resolved) corrective action and link it to the incident."""
    r = requests.post(f"{api_url}/corrective-actions", headers=headers, json={
        "title": f"ca-{uuid.uuid4().hex[:8]}", "description": "open action",
    })
    assert r.status_code in (200, 201), r.text
    ca = r.json()
    link = requests.post(f"{api_url}/references", headers=headers, json={
        "source_type": "corrective_action", "source_id": ca["identifier"],
        "target_type": "incident", "target_id": incident_identifier,
    })
    assert link.status_code in (200, 201), link.text
    return ca["identifier"]


def test_http_path_blocks_resolve_with_open_ca(api_url, admin_headers):
    inc_id, inc_ident = _mk_incident(api_url, admin_headers)
    _mk_open_ca_linked(api_url, admin_headers, inc_ident)

    r = requests.put(f"{api_url}/incidents/{inc_id}/status",
                     headers=admin_headers, json={"status": "resolved"})
    assert r.status_code == 409, f"HTTP resolve should be blocked by open CA, got {r.status_code}: {r.text}"


def test_apply_path_blocks_resolve_with_open_ca(api_url, admin_headers):
    """The bypass #26 closes: suggestion-apply must hit the SAME open-CA guard."""
    inc_id, inc_ident = _mk_incident(api_url, admin_headers)
    _mk_open_ca_linked(api_url, admin_headers, inc_ident)

    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "incident",
        "suggestion_type": "update",
        "entity_id": str(inc_id),
        "title": "Resolve incident",
        "rationale": "done",
        "payload": {"fields": {"status": "resolved"}},
    })
    assert sg.status_code in (200, 201), sg.text
    sid = sg.json()["id"]

    # force=true bypasses stale-detection so the ONLY thing that can block the
    # apply is the enforced open-CA guard (what we're testing).
    apply = requests.post(f"{api_url}/suggestions/{sid}/apply", headers=admin_headers, json={"force": True})
    assert not (apply.status_code == 200 and apply.json().get("status") == "applied"), (
        f"apply should be blocked by the open-CA guard, got {apply.status_code}: {apply.text}"
    )

    # And the incident stays unresolved (guard held, no partial write).
    got = requests.get(f"{api_url}/incidents/{inc_id}", headers=admin_headers).json()
    assert got.get("status") != "resolved", "incident must remain unresolved after blocked apply"


def test_apply_path_sets_lifecycle_timestamp(api_url, admin_headers):
    """With no open CA, apply resolves the incident AND stamps resolved_at —
    the lifecycle-timestamp behavior suggestion-apply previously skipped."""
    inc_id, _ = _mk_incident(api_url, admin_headers)

    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "incident",
        "suggestion_type": "update",
        "entity_id": str(inc_id),
        "title": "Resolve incident",
        "rationale": "done",
        "payload": {"fields": {"status": "resolved"}},
    })
    assert sg.status_code in (200, 201), sg.text
    apply = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                          headers=admin_headers, json={"force": True})
    assert apply.status_code == 200 and apply.json().get("status") == "applied", apply.text

    got = requests.get(f"{api_url}/incidents/{inc_id}", headers=admin_headers).json()
    assert got.get("status") == "resolved", got
    assert got.get("resolved_at"), "resolved_at must be stamped by the enforced write path"
