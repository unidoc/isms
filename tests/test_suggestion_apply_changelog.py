"""Regression (#334): an applied suggestion's `suggestion_applied` changelog row
must land on the entity the suggestion was applied to.

The row's entity_id used to be derived from the display identifier by stripping
the prefix ("RISK-3" -> 3). That number is the per-org sequence, not the primary
key, so the row landed on another entity with that primary key, or on one in
another organization, where no History panel shows it.

The two only agree on a single-org database, so the test works in its own fresh
organization after the shared test org already holds risks: this org's RISK-n is
then never primary key n.
"""
import uuid

import requests
from conftest import API

PASSWORD = "TestPass123!"
RISK_BODY = {
    "current_likelihood": 2, "current_impact": 3, "risk_type": "threat",
    "origin": "external", "status": "open", "treatment": "mitigate",
}


def _fresh_org_headers():
    suffix = uuid.uuid4().hex[:8]
    email = f"sg-changelog-{suffix}@isms-test.local"
    slug = f"sg-changelog-{suffix}"
    r = requests.post(f"{API}/auth/signup", json={"email": email, "password": PASSWORD, "name": "SG Changelog"})
    assert r.status_code in (200, 201), r.text
    headers = {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}
    r = requests.post(f"{API}/organizations", headers=headers, json={"name": slug, "slug": slug})
    assert r.status_code in (200, 201), r.text
    r = requests.post(f"{API}/auth/login", json={"email": email, "password": PASSWORD, "organization": slug})
    assert r.status_code == 200, r.text
    return {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}


def _create_risk(headers, title):
    r = requests.post(f"{API}/risks", headers=headers, json={"title": title, **RISK_BODY})
    assert r.status_code in (200, 201), r.text
    return r.json()


def _apply(headers, body):
    sg = requests.post(f"{API}/suggestions", headers=headers, json={"title": "#334 regression", **body})
    assert sg.status_code in (200, 201), sg.text
    sg = sg.json()
    # force=true bypasses the fresh entity's own create-changelog stale flag.
    ap = requests.post(f"{API}/suggestions/{sg['id']}/apply", headers=headers, json={"force": True})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
    return sg["id"], ap.json()["applied_entity_id"]


def _applied_rows(headers, risk_id, suggestion_id):
    r = requests.get(f"{API}/changelog/risk/{risk_id}", headers=headers)
    assert r.status_code == 200, r.text
    prefix = f"Applied suggestion #{suggestion_id}:"
    return [e for e in r.json()["data"] or []
            if e["action"] == "suggestion_applied" and e.get("reason", "").startswith(prefix)]


def test_update_suggestion_changelog_on_target(admin_headers):
    # The shared org holds risks first, so the fresh org's primary keys run ahead
    # of its identifiers.
    _create_risk(admin_headers, "shared-org risk for #334")
    headers = _fresh_org_headers()
    risks = [_create_risk(headers, f"risk {i}") for i in range(1, 4)]
    target = risks[2]
    assert target["identifier"] == "RISK-3" and target["id"] != 3, target

    sg_id, applied = _apply(headers, {
        "entity_type": "risk", "entity_id": target["identifier"], "suggestion_type": "update",
        "payload": {"fields": {"notes": "updated by suggestion"}},
    })
    assert applied == target["identifier"]

    assert len(_applied_rows(headers, target["id"], sg_id)) == 1, \
        f"{target['identifier']} (id {target['id']}) has no suggestion_applied row"
    for other in risks[:2]:
        assert not _applied_rows(headers, other["id"], sg_id), \
            f"suggestion applied to RISK-3 was logged on {other['identifier']} (id {other['id']})"


def test_create_suggestion_changelog_on_created_entity(admin_headers):
    _create_risk(admin_headers, "shared-org risk for #334 create")
    headers = _fresh_org_headers()
    _create_risk(headers, "existing risk")

    sg_id, identifier = _apply(headers, {
        "entity_type": "risk", "suggestion_type": "create",
        "payload": {"title": "risk created by suggestion"},
    })
    created = requests.get(f"{API}/risks/{identifier}", headers=headers)
    assert created.status_code == 200, created.text
    created = created.json()

    assert len(_applied_rows(headers, created["id"], sg_id)) == 1, \
        f"created {identifier} (id {created['id']}) has no suggestion_applied row"
