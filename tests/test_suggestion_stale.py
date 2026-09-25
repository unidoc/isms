"""#338 + #339: suggestion stale detection.

Two bugs, now one snapshot fix:

- #338: the snapshot taken on suggestion create used the entity's updated_at,
  but the check compares against entity_changelog.created_at. Register
  handlers write the entity, then log the changelog row as a separate
  statement, so the row's own created_at always lands after updated_at — a
  fresh suggestion's own create-changelog row always looked like a change,
  and every apply came back stale even when nothing happened since.
- #339: the snapshot lookup matched on `identifier` only for risk/supplier and
  on `id` for everything else, so an identifier-form entity_id ("ASSET-1")
  errored, the error was swallowed, and the suggestion got no snapshot at
  all — silently turning the check off.

Parametrized over the 11 entity types that have a suggestion apply handler.
"""
import uuid

import pytest
import requests

from conftest import ADMIN_EMAIL


def _uid():
    return uuid.uuid4().hex[:8]


def _edit_via_suggestion(api_url, headers, entity_type, entity_id, field, value):
    """Change one field through a throwaway suggestion-apply instead of the
    entity's own PUT endpoint.

    incident/legal_requirement/corrective_action's direct PUT handlers
    (handleUpdateIncident, handleUpdateLegal, handleUpdateCorrectiveAction)
    snapshot their changelog "old" values from the same struct they already
    applied the request onto, so DiffFields never sees a difference and no
    changelog row is ever written on a plain PUT — a real, pre-existing bug,
    unrelated to #338/#339 (their suggestion-apply counterparts capture "old"
    correctly, e.g. applyIncidentUpdate). Going through suggestion-apply here
    reaches the entity by the write path that actually logs, so this helper
    exercises real edits rather than the broken one.
    """
    sg = requests.post(f"{api_url}/suggestions", headers=headers, json={
        "entity_type": entity_type,
        "suggestion_type": "update",
        "entity_id": str(entity_id),
        "title": "stale-detection setup edit",
        "rationale": "#338/#339 setup",
        "payload": {"fields": {field: value}},
    })
    assert sg.status_code in (200, 201), f"setup edit suggestion ({entity_type}): {sg.text}"
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                        headers=headers, json={"force": True})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", (
        f"setup edit apply ({entity_type}): {ap.text}"
    )


# ─────────────────────────────────────────────────────────────────────────
# Per-type create (-> numeric id, identifier) and edit (a real PUT that
# changes a value, so the type's update handler actually logs a changelog
# row — most only log when there's a diff).
# ─────────────────────────────────────────────────────────────────────────

def _create_risk(api_url, headers):
    r = requests.post(f"{api_url}/risks", headers=headers, json={
        "title": f"stale-risk-{_uid()}", "current_likelihood": 2, "current_impact": 2,
        "risk_type": "threat", "origin": "external", "status": "open", "treatment": "mitigate",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_risk(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/risks/{entity_id}", headers=headers,
                      json={"description": f"edited-{_uid()}"})
    assert r.status_code == 200, r.text


def _create_incident(api_url, headers):
    r = requests.post(f"{api_url}/incidents", headers=headers, json={
        "title": f"stale-inc-{_uid()}", "severity": "medium",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_incident(api_url, headers, entity_id, identifier):
    # applyIncidentUpdate only recognizes severity/status/assignee/root_cause/
    # affects_* — "description" is not wired into its payload.Fields switch.
    _edit_via_suggestion(api_url, headers, "incident", entity_id, "root_cause", f"edited-{_uid()}")


def _create_supplier(api_url, headers):
    r = requests.post(f"{api_url}/suppliers", headers=headers, json={
        "name": f"stale-sup-{_uid()}", "supplier_type": "saas", "criticality": "low",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_supplier(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/suppliers/{entity_id}", headers=headers,
                      json={"contact": f"edited-{_uid()}@example.com"})
    assert r.status_code == 200, r.text


def _create_legal(api_url, headers):
    r = requests.post(f"{api_url}/legal", headers=headers, json={
        "title": f"stale-legal-{_uid()}", "jurisdiction": "EU", "category": "privacy",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_legal(api_url, headers, entity_id, identifier):
    # applyLegalUpdate resolves entity_id via GetLegalRequirementByIdentifier
    # unconditionally (like risk/supplier's own apply handlers) and only
    # recognizes owner/notes — address it by identifier, not the numeric PK.
    _edit_via_suggestion(api_url, headers, "legal_requirement", identifier, "notes", f"edited-{_uid()}")


def _create_change(api_url, headers):
    r = requests.post(f"{api_url}/changes", headers=headers, json={
        "title": f"stale-chg-{_uid()}", "description": "d", "justification": "j",
        "priority": "high", "category": "technology", "risk_level": "medium",
        "rollback_plan": "revert",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_change(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/changes/{entity_id}", headers=headers,
                      json={"description": f"edited-{_uid()}"})
    assert r.status_code == 200, r.text


def _create_corrective_action(api_url, headers):
    r = requests.post(f"{api_url}/corrective-actions", headers=headers, json={
        "title": f"stale-ca-{_uid()}", "description": "x",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_corrective_action(api_url, headers, entity_id, identifier):
    # handleUpdateCorrectiveAction has the same broken-diff bug as incident/
    # legal (see _edit_via_suggestion) — go through suggestion-apply instead.
    _edit_via_suggestion(api_url, headers, "corrective_action", entity_id, "root_cause", f"edited-{_uid()}")


def _create_task(api_url, headers):
    r = requests.post(f"{api_url}/tasks", headers=headers, json={
        "title": f"stale-task-{_uid()}", "task_type": "general",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_task(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/tasks/{entity_id}", headers=headers,
                      json={"description": f"edited-{_uid()}"})
    assert r.status_code == 200, r.text


def _create_objective(api_url, headers):
    prog = requests.post(f"{api_url}/programs", headers=headers, json={
        "title": f"stale-prog-{_uid()}", "owner": ADMIN_EMAIL, "key": f"STALE{_uid()[:4].upper()}",
    })
    assert prog.status_code in (200, 201), prog.text
    r = requests.post(f"{api_url}/objectives", headers=headers, json={
        "program_id": prog.json()["id"], "title": f"stale-obj-{_uid()}",
        "owner": ADMIN_EMAIL, "target_value": 5.0, "target_operator": "lte", "unit": "%",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["display_id"]


def _edit_objective(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/objectives/{entity_id}", headers=headers,
                      json={"description": f"edited-{_uid()}"})
    assert r.status_code == 200, r.text


def _create_system(api_url, headers):
    r = requests.post(f"{api_url}/systems", headers=headers, json={
        "name": f"stale-sys-{_uid()}", "classification": "internal", "criticality": "low",
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_system(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/systems/{entity_id}", headers=headers,
                      json={"description": f"edited-{_uid()}"})
    assert r.status_code == 200, r.text


def _create_asset(api_url, headers):
    r = requests.post(f"{api_url}/assets", headers=headers, json={
        "name": f"stale-asset-{_uid()}", "asset_type": "system", "status": "open",
        "confidentiality": 3, "integrity": 3, "availability": 3,
    })
    assert r.status_code in (200, 201), r.text
    d = r.json()
    return d["id"], d["identifier"]


def _edit_asset(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/assets/{entity_id}", headers=headers,
                      json={"description": f"edited-{_uid()}"})
    assert r.status_code == 200, r.text


def _create_audit_finding(api_url, headers):
    prog = requests.post(f"{api_url}/audit/programmes", headers=headers, json={
        "title": f"stale-prog-{_uid()}", "year": 2026,
    })
    assert prog.status_code in (200, 201), prog.text
    aud = requests.post(f"{api_url}/audits", headers=headers, json={
        "programme_id": prog.json()["id"], "title": f"stale-aud-{_uid()}",
    })
    assert aud.status_code in (200, 201), aud.text
    f = requests.post(f"{api_url}/audit-findings", headers=headers, json={
        "audit_id": aud.json()["id"], "title": f"stale-find-{_uid()}",
        "finding_type": "observation",
    })
    assert f.status_code in (200, 201), f.text
    fid = f.json()["id"]
    # audit_finding has no stored identifier column — its display form is
    # FIND-<id>, built from the primary key (api_audit.go / server.go
    # entityIDResolvers), so it's still worth exercising as a distinct
    # "identifier" ref: it goes through the resolver, not a bare int parse.
    return fid, f"FIND-{fid}"


def _edit_audit_finding(api_url, headers, entity_id, identifier):
    r = requests.put(f"{api_url}/audit-findings/{entity_id}", headers=headers,
                      json={"description": f"edited-{_uid()}"})
    assert r.status_code == 200, r.text


TYPES = {
    "risk": (_create_risk, _edit_risk),
    "incident": (_create_incident, _edit_incident),
    "supplier": (_create_supplier, _edit_supplier),
    "legal_requirement": (_create_legal, _edit_legal),
    "change_request": (_create_change, _edit_change),
    "corrective_action": (_create_corrective_action, _edit_corrective_action),
    "task": (_create_task, _edit_task),
    "objective": (_create_objective, _edit_objective),
    "system": (_create_system, _edit_system),
    "asset": (_create_asset, _edit_asset),
    "audit_finding": (_create_audit_finding, _edit_audit_finding),
}

# applyRiskUpdate/applySupplierUpdate/applyLegalUpdate resolve sg.EntityID via
# GetXByIdentifier unconditionally — a pre-existing limitation of those three
# apply-update handlers, unrelated to #338/#339 (the stale-snapshot layer
# resolves numeric ids for every type via s.resolveEntityID/EntityStaleSnapshot
# just fine; only the *apply dispatch* for these three types never accepted a
# bare numeric id). Applying a numeric-addressed update suggestion for them
# 500s regardless of staleness, so skip the apply step for that combination —
# the snapshot/stale assertions above it still run and are what #338/#339 cover.
APPLY_REQUIRES_IDENTIFIER = {"risk", "supplier", "legal_requirement"}


# ─────────────────────────────────────────────────────────────────────────
# Suggestion helpers
# ─────────────────────────────────────────────────────────────────────────

def _suggest(api_url, headers, entity_type, entity_ref):
    sg = requests.post(f"{api_url}/suggestions", headers=headers, json={
        "entity_type": entity_type,
        "suggestion_type": "update",
        "entity_id": str(entity_ref),
        "title": "stale-detection probe",
        "rationale": "#338/#339",
        "payload": {"fields": {}},
    })
    assert sg.status_code in (200, 201), f"create suggestion ({entity_type}, {entity_ref}): {sg.text}"
    return sg.json()


def _get(api_url, headers, suggestion_id):
    r = requests.get(f"{api_url}/suggestions/{suggestion_id}", headers=headers)
    assert r.status_code == 200, r.text
    return r.json()


def _apply(api_url, headers, suggestion_id, force=False):
    body = {"force": True} if force else {}
    r = requests.post(f"{api_url}/suggestions/{suggestion_id}/apply", headers=headers, json=body)
    assert r.status_code == 200, r.text
    return r.json()


# ─────────────────────────────────────────────────────────────────────────
# Tests
# ─────────────────────────────────────────────────────────────────────────

@pytest.mark.parametrize("by", ["identifier", "numeric"])
@pytest.mark.parametrize("entity_type", sorted(TYPES))
def test_untouched_entity_is_not_stale(api_url, admin_headers, entity_type, by):
    create, _edit = TYPES[entity_type]
    numeric_id, identifier = create(api_url, admin_headers)
    ref = identifier if by == "identifier" else numeric_id

    sg = _suggest(api_url, admin_headers, entity_type, ref)
    assert sg.get("entity_updated_at") is not None, (
        f"create response missing entity_updated_at snapshot "
        f"({entity_type}, by {by}): {sg}"
    )

    got = _get(api_url, admin_headers, sg["id"])
    assert not got.get("stale"), f"untouched entity reported stale ({entity_type}, by {by}): {got}"

    if by == "numeric" and entity_type in APPLY_REQUIRES_IDENTIFIER:
        return
    applied = _apply(api_url, admin_headers, sg["id"])
    assert applied.get("status") == "applied", (
        f"untouched entity failed to apply without force ({entity_type}, by {by}): {applied}"
    )


@pytest.mark.parametrize("entity_type", sorted(TYPES))
def test_edited_entity_is_stale(api_url, admin_headers, entity_type):
    create, edit = TYPES[entity_type]
    numeric_id, identifier = create(api_url, admin_headers)

    sg = _suggest(api_url, admin_headers, entity_type, identifier)
    edit(api_url, admin_headers, numeric_id, identifier)

    got = _get(api_url, admin_headers, sg["id"])
    assert got.get("stale"), f"edited entity not reported stale ({entity_type}): {got}"
    stale_changes = got.get("stale_changes") or []
    assert stale_changes, f"stale but no stale_changes rows ({entity_type}): {got}"
    # Update handlers log one row per changed field, so assert every row is an
    # 'update', not that there is exactly one.
    for change in stale_changes:
        # Update handlers log one 'update' row per changed field. For the
        # three types whose "edit" here goes through suggestion-apply (see
        # _edit_via_suggestion), the apply endpoint also logs its own
        # 'suggestion_applied' linking row — both are legitimate rows filed
        # against the entity after the snapshot, so both are expected.
        assert change.get("action") in ("update", "suggestion_applied"), (
            f"unexpected stale row action for {entity_type}: {change}"
        )

    blocked = _apply(api_url, admin_headers, sg["id"])
    assert blocked.get("stale") is True, (
        f"apply without force should report stale for an edited entity ({entity_type}): {blocked}"
    )

    applied = _apply(api_url, admin_headers, sg["id"], force=True)
    assert applied.get("status") == "applied", (
        f"apply with force should succeed for an edited entity ({entity_type}): {applied}"
    )


def test_second_suggestion_after_apply_is_not_stale(api_url, admin_headers):
    """Covers the snapshot after WithOrgTx-written rows: the suggestion_applied
    changelog row from applying one suggestion must not make the NEXT
    suggestion on the same entity stale the moment it's created."""
    create, _edit = TYPES["asset"]
    _numeric_id, identifier = create(api_url, admin_headers)

    first = _suggest(api_url, admin_headers, "asset", identifier)
    applied = _apply(api_url, admin_headers, first["id"])
    assert applied.get("status") == "applied", applied

    second = _suggest(api_url, admin_headers, "asset", identifier)
    got = _get(api_url, admin_headers, second["id"])
    assert not got.get("stale"), f"second suggestion stale right after an apply: {got}"

    result = _apply(api_url, admin_headers, second["id"])
    assert result.get("status") == "applied", (
        f"second suggestion should apply without force: {result}"
    )
