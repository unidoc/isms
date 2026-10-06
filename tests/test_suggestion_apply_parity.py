"""Parity (#200): applying a suggestion writes every field the equivalent REST
PUT (update) or POST (create) accepts, with the same validation.

Update parity: two twin entities, A updated by PUT, B by an update suggestion
carrying the same fields; every sent key must read back equal on both.
Create parity: the maximal POST body sent as a REST POST and as a create
suggestion; every sent key must read back equal on both.

The file grows one entity per commit.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL


def _tag():
    return uuid.uuid4().hex[:8]


def _apply(api_url, headers, entity_type, suggestion_type, payload, entity_id=None):
    body = {
        "entity_type": entity_type,
        "suggestion_type": suggestion_type,
        "title": f"parity {entity_type} {_tag()}",
        "rationale": "#200 parity",
        "payload": payload,
    }
    if entity_id is not None:
        body["entity_id"] = str(entity_id)
    sg = requests.post(f"{api_url}/suggestions", headers=headers, json=body)
    assert sg.status_code in (200, 201), f"create suggestion: {sg.status_code} {sg.text}"
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                       headers=headers, json={"force": True})
    assert ap.status_code == 200 and ap.json().get("status") == "applied", \
        f"apply: {ap.status_code} {ap.text}"
    return ap.json()["applied_entity_id"]


def _assert_same(a, b, sent, label):
    for key in sent:
        assert a.get(key) == b.get(key), \
            f"{label}: field {key!r} differs: REST={a.get(key)!r} suggestion={b.get(key)!r}"
        assert a.get(key) == sent[key], \
            f"{label}: field {key!r} not stored as sent: sent={sent[key]!r} got={a.get(key)!r}"


# --- asset -----------------------------------------------------------------

ASSET_FIELDS = {
    "name": "renamed asset",
    "description": "a new description",
    "asset_type": "network",
    "status": "archived",
    "owner": ADMIN_EMAIL,
    "primary_location": "datacentre 2",
    "confidentiality": 3,
    "integrity": 2,
    "availability": 1,
    "last_review": 1767225600,
    "next_review": 1893456000,
    "notes": "updated notes",
    "external_id": "EXT-200",
}


def _make_asset(api_url, headers):
    r = requests.post(f"{api_url}/assets", headers=headers, json={"name": f"asset {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def test_asset_update_parity(api_url, admin_headers):
    a = _make_asset(api_url, admin_headers)
    b = _make_asset(api_url, admin_headers)
    fields = dict(ASSET_FIELDS, external_id=f"EXT-{_tag()}")
    r = requests.put(f"{api_url}/assets/{a['id']}", headers=admin_headers, json=fields)
    assert r.status_code == 200, r.text
    # External ids are unique per org, so B gets its own; compare the rest.
    fields_b = dict(fields, external_id=f"EXT-{_tag()}")
    _apply(api_url, admin_headers, "asset", "update", {"fields": fields_b}, entity_id=b["identifier"])
    got_a = requests.get(f"{api_url}/assets/{a['id']}", headers=admin_headers).json()
    got_b = requests.get(f"{api_url}/assets/{b['id']}", headers=admin_headers).json()
    same = {k: v for k, v in fields.items() if k != "external_id"}
    _assert_same(got_a, got_b, same, "asset update")
    assert got_a["external_id"] == fields["external_id"]
    assert got_b["external_id"] == fields_b["external_id"]


def test_asset_update_null_clears_optional_only(api_url, admin_headers):
    a = _make_asset(api_url, admin_headers)
    requests.put(f"{api_url}/assets/{a['id']}", headers=admin_headers, json={"confidentiality": 3})
    _apply(api_url, admin_headers, "asset", "update", {"fields": {"confidentiality": None}},
           entity_id=a["identifier"])
    got = requests.get(f"{api_url}/assets/{a['id']}", headers=admin_headers).json()
    assert got.get("confidentiality") is None
    r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "asset", "suggestion_type": "update", "entity_id": a["identifier"],
        "title": "null notes", "payload": {"fields": {"notes": None}},
    })
    assert r.status_code == 400 and "notes" in r.text, r.text


def test_asset_update_requires_entity_id(api_url, admin_headers):
    r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "asset", "suggestion_type": "update",
        "title": "no entity", "payload": {"fields": {"notes": "x"}},
    })
    assert r.status_code == 400 and "entity_id" in r.text, r.text


def test_asset_update_invalid_enum_is_400_at_apply(api_url, admin_headers):
    a = _make_asset(api_url, admin_headers)
    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "asset", "suggestion_type": "update", "entity_id": a["identifier"],
        "title": "bad status", "payload": {"fields": {"status": "bogus"}},
    })
    assert sg.status_code == 201, sg.text
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply", headers=admin_headers, json={"force": True})
    assert ap.status_code == 400, ap.text


def test_asset_create_parity(api_url, admin_headers):
    body = dict(ASSET_FIELDS, status="draft", asset_type="software",
                external_id=f"EXT-{_tag()}", name=f"asset {_tag()}")
    r = requests.post(f"{api_url}/assets", headers=admin_headers, json=body)
    assert r.status_code == 201, r.text
    got_a = requests.get(f"{api_url}/assets/{r.json()['id']}", headers=admin_headers).json()

    body_b = dict(body, external_id=f"EXT-{_tag()}", name=f"asset {_tag()}")
    ident = _apply(api_url, admin_headers, "asset", "create", body_b)
    got_b = requests.get(f"{api_url}/assets/{ident}", headers=admin_headers).json()

    skip = {"name", "external_id"}
    _assert_same(got_a, got_b, {k: v for k, v in body.items() if k not in skip}, "asset create")
    assert got_b["name"] == body_b["name"] and got_b["external_id"] == body_b["external_id"]


def test_asset_create_with_reference_links_both_ways(api_url, admin_headers):
    other = _make_asset(api_url, admin_headers)
    ident = _apply(api_url, admin_headers, "asset", "create", {
        "name": f"asset {_tag()}", "references": [{"type": "asset", "id": other["identifier"]}],
    })
    refs = requests.get(f"{api_url}/references", headers=admin_headers,
                        params={"type": "asset", "id": other["identifier"]}).json()
    assert any(ident in str(r) for r in (refs if isinstance(refs, list) else refs.get("data", []))), refs
