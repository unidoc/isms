"""#26 slice B: update consistency — a status transition applied via a suggestion
must derive the same closure metadata as the HTTP path.

Before: applying an audit-finding suggestion that set status=closed errored
outright (the field-level update rejected 'status'); it now goes through the same
SetAuditFindingStatus closure path the HTTP handler uses.

(Risk 'accepted' was intentionally left out: 'accepted' is not a valid risk
status — RiskStatuses is draft/open/closed — so there is no real HTTP-vs-apply
divergence there; the HTTP status=='accepted' block is vestigial.)

force=true on apply bypasses stale-detection (a fresh entity's own create
changelog would otherwise flag it), so the enforced path is what's exercised.
"""
import uuid

import requests


def _suggest_update(api_url, headers, entity_type, entity_id, fields):
    sg = requests.post(f"{api_url}/suggestions", headers=headers, json={
        "entity_type": entity_type,
        "suggestion_type": "update",
        "entity_id": str(entity_id),
        "title": "writepath update",
        "rationale": "writepath consistency",
        "payload": {"fields": fields},
    })
    assert sg.status_code in (200, 201), sg.text
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                       headers=headers, json={"force": True})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text


def _make_finding(api_url, headers):
    prog = requests.post(f"{api_url}/audit/programmes", headers=headers, json={
        "title": f"wp-prog-{uuid.uuid4().hex[:6]}", "year": 2026,
    })
    assert prog.status_code in (200, 201), prog.text
    aud = requests.post(f"{api_url}/audits", headers=headers, json={
        "programme_id": prog.json()["id"], "title": f"wp-aud-{uuid.uuid4().hex[:6]}",
    })
    assert aud.status_code in (200, 201), aud.text
    f = requests.post(f"{api_url}/audit-findings", headers=headers, json={
        "audit_id": aud.json()["id"], "title": f"wp-find-{uuid.uuid4().hex[:6]}",
        "finding_type": "observation",
    })
    assert f.status_code in (200, 201), f.text
    return f.json()["id"]


def test_audit_finding_apply_close_stamps_closure(api_url, admin_headers):
    fid = _make_finding(api_url, admin_headers)
    _suggest_update(api_url, admin_headers, "audit_finding", fid, {"status": "closed"})
    got = requests.get(f"{api_url}/audit-findings/{fid}", headers=admin_headers).json()
    assert got["status"] == "closed", got
    assert got.get("closed_at"), "apply→closed must stamp closed_at like the HTTP path"


def test_audit_finding_apply_reopen_clears_closure(api_url, admin_headers):
    fid = _make_finding(api_url, admin_headers)
    _suggest_update(api_url, admin_headers, "audit_finding", fid, {"status": "closed"})
    _suggest_update(api_url, admin_headers, "audit_finding", fid, {"status": "open"})
    got = requests.get(f"{api_url}/audit-findings/{fid}", headers=admin_headers).json()
    assert got["status"] == "open", got
    assert not got.get("closed_at"), "reopen must clear closed_at"
    assert not got.get("closed_by"), "reopen must clear closed_by"
