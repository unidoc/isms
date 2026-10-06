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


# --- generic helpers (used from the system section on) ---------------------

def _uniq(v):
    return f"{v}-{_tag()}"


def _update_parity(api_url, headers, entity_type, path, make, fields, unique=(), ref="identifier"):
    """PUT `fields` on twin A, apply the same fields as a suggestion on twin B,
    and assert every sent key reads back equal. Keys in `unique` must differ per
    entity (unique constraints) and are checked against their own values."""
    a, b = make(), make()
    fa = dict(fields)
    fb = dict(fields)
    for k in unique:
        fa[k] = _uniq(fields[k])
        fb[k] = _uniq(fields[k])
    r = requests.put(f"{api_url}/{path}/{a['id']}", headers=headers, json=fa)
    assert r.status_code == 200, f"PUT {entity_type}: {r.status_code} {r.text}"
    _apply(api_url, headers, entity_type, "update", {"fields": fb}, entity_id=b[ref])
    got_a = requests.get(f"{api_url}/{path}/{a['id']}", headers=headers).json()
    got_b = requests.get(f"{api_url}/{path}/{b['id']}", headers=headers).json()
    _assert_same(got_a, got_b, {k: v for k, v in fields.items() if k not in unique}, f"{entity_type} update")
    for k in unique:
        assert got_a.get(k) == fa[k] and got_b.get(k) == fb[k], (k, got_a.get(k), got_b.get(k))


def _create_parity(api_url, headers, entity_type, path, body, unique=(), get_by="id", ident_key="identifier"):
    """POST the maximal body and apply it as a create suggestion; compare every key."""
    ba = dict(body)
    bb = dict(body)
    for k in unique:
        ba[k] = _uniq(body[k])
        bb[k] = _uniq(body[k])
    r = requests.post(f"{api_url}/{path}", headers=headers, json=ba)
    assert r.status_code == 201, f"POST {entity_type}: {r.status_code} {r.text}"
    ra = r.json()
    ident = _apply(api_url, headers, entity_type, "create", bb)
    if get_by == "id":
        # applied_entity_id is the identifier; resolve to the row via the list.
        lst = requests.get(f"{api_url}/{path}", headers=headers, params={"limit": 500}).json()
        rows = lst["data"] if isinstance(lst, dict) and "data" in lst else lst
        rb = next(x for x in rows if x.get(ident_key) == ident)
    else:
        rb = requests.get(f"{api_url}/{path}/{ident}", headers=headers).json()
    got_a = requests.get(f"{api_url}/{path}/{ra['id']}", headers=headers).json()
    got_b = requests.get(f"{api_url}/{path}/{rb['id']}", headers=headers).json()
    _assert_same(got_a, got_b, {k: v for k, v in body.items() if k not in unique and k != "references"}, f"{entity_type} create")
    for k in unique:
        assert got_a.get(k) == ba[k] and got_b.get(k) == bb[k], (k, got_a.get(k), got_b.get(k))
    return got_a, got_b


# --- system ----------------------------------------------------------------

def _make_system(api_url, headers):
    r = requests.post(f"{api_url}/systems", headers=headers, json={"name": f"sys {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def _make_supplier(api_url, headers):
    r = requests.post(f"{api_url}/suppliers", headers=headers, json={"name": f"sup {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def _system_fields(supplier_id):
    return {
        "name": "renamed system", "description": "new description", "supplier_id": supplier_id,
        "department": "Platform", "classification": "restricted", "criticality": "critical",
        "status": "under_review", "rpo_hours": 4, "rto_hours": 8,
        "confidentiality": 3, "integrity": 2, "availability": 1,
        "last_review": 1767225600, "next_review": 1893456000,
        "owner": ADMIN_EMAIL, "notes": "system notes", "external_id": "SYS-EXT",
    }


def test_system_update_parity(api_url, admin_headers):
    sup = _make_supplier(api_url, admin_headers)
    _update_parity(api_url, admin_headers, "system", "systems",
                   lambda: _make_system(api_url, admin_headers),
                   _system_fields(sup["id"]), unique=("external_id",))


def test_system_create_parity(api_url, admin_headers):
    sup = _make_supplier(api_url, admin_headers)
    _create_parity(api_url, admin_headers, "system", "systems",
                   _system_fields(sup["id"]), unique=("external_id", "name"))


def test_system_create_foreign_supplier_is_400_at_apply(api_url, admin_headers):
    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "system", "suggestion_type": "create", "title": "bad supplier",
        "payload": {"name": f"sys {_tag()}", "supplier_id": 99999999},
    })
    assert sg.status_code == 201, sg.text
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply", headers=admin_headers, json={"force": True})
    assert ap.status_code == 400, ap.text


# --- supplier --------------------------------------------------------------

SUPPLIER_FIELDS = {
    "name": "renamed supplier", "supplier_type": "saas", "criticality": "critical",
    "data_access": True, "contact": "ops@example.com", "contract_ref": "C-200",
    "status": "under_review", "owner": ADMIN_EMAIL,
    "contract_expiry": 1893456000, "confidentiality": 3, "integrity": 2, "availability": 1,
    "last_review": 1767225600, "next_review": 1893456000,
    "notes": "supplier notes", "external_id": "SUP-EXT",
}


def test_supplier_update_parity(api_url, admin_headers):
    _update_parity(api_url, admin_headers, "supplier", "suppliers",
                   lambda: _make_supplier(api_url, admin_headers), SUPPLIER_FIELDS, unique=("external_id",))


def test_supplier_create_parity(api_url, admin_headers):
    _create_parity(api_url, admin_headers, "supplier", "suppliers", SUPPLIER_FIELDS,
                   unique=("external_id", "name"))


def test_supplier_create_still_accepts_description(api_url, admin_headers):
    # The server copies the rationale into "description"; it is accepted and ignored.
    _apply(api_url, admin_headers, "supplier", "create",
           {"name": f"sup {_tag()}", "description": "ignored"})


# --- risk ------------------------------------------------------------------

RISK_FIELDS = {
    "title": "renamed risk", "description": "risk description", "risk_type": "opportunity",
    "origin": "external", "category": "technology",
    "current_likelihood": 4, "current_impact": 3,
    "confidentiality_impact": 2, "integrity_impact": 3, "availability_impact": 1,
    "inherent_likelihood": 5, "inherent_impact": 4,
    "inherent_confidentiality_impact": 3, "inherent_integrity_impact": 2,
    "inherent_availability_impact": 1,
    "target_likelihood": 2, "target_impact": 2,
    "treatment": "mitigate", "treatment_plan": "do the thing",
    "treatment_due_date": 1893456000, "owner": ADMIN_EMAIL, "status": "open",
    "last_review": 1767225600, "next_review": 1893456000,
    "notes": "risk notes", "external_id": "RISK-EXT",
}


def _make_risk(api_url, headers):
    r = requests.post(f"{api_url}/risks", headers=headers,
                      json={"title": f"risk {_tag()}", "category": "technology"})
    assert r.status_code == 201, r.text
    return r.json()


def test_risk_update_parity(api_url, admin_headers):
    _update_parity(api_url, admin_headers, "risk", "risks",
                   lambda: _make_risk(api_url, admin_headers), RISK_FIELDS, unique=("external_id",))


def test_risk_update_recomputes_score(api_url, admin_headers):
    r = _make_risk(api_url, admin_headers)
    _apply(api_url, admin_headers, "risk", "update",
           {"fields": {"current_likelihood": 4, "current_impact": 5}}, entity_id=r["identifier"])
    got = requests.get(f"{api_url}/risks/{r['id']}", headers=admin_headers).json()
    assert got["current_likelihood"] == 4 and got["current_score"] == 20, got


def test_risk_update_out_of_range_is_400(api_url, admin_headers):
    r = _make_risk(api_url, admin_headers)
    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "risk", "suggestion_type": "update", "entity_id": r["identifier"],
        "title": "bad", "payload": {"fields": {"current_likelihood": 9}}})
    assert sg.status_code == 201, sg.text
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply", headers=admin_headers, json={"force": True})
    assert ap.status_code == 400 and "0-5" in ap.text, ap.text


def test_risk_create_parity(api_url, admin_headers):
    got_a, got_b = _create_parity(api_url, admin_headers, "risk", "risks", RISK_FIELDS,
                                  unique=("external_id", "title"))
    assert got_a["current_score"] == got_b["current_score"]
    assert got_a["target_score"] == got_b["target_score"]
