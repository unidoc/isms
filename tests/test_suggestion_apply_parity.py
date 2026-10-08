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

    def pairs(of):
        r = requests.get(f"{api_url}/references", headers=admin_headers, params={"type": "asset", "id": of})
        assert r.status_code == 200, r.text
        return {(x["source_type"], x["source_id"], x["target_type"], x["target_id"]) for x in r.json()["data"]}

    # exact id equality (ASSET-12 must not match ASSET-120), seen from both ends
    assert {ident, other["identifier"]} in [{x[1], x[3]} for x in pairs(ident)]
    assert {ident, other["identifier"]} in [{x[1], x[3]} for x in pairs(other["identifier"])]


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


# --- legal requirement -----------------------------------------------------

LEGAL_FIELDS = {
    "title": "renamed legal", "description": "legal description", "jurisdiction": "DE",
    "category": "security", "reference": "Art. 32", "url": "https://example.com/law",
    "status": "open", "owner": ADMIN_EMAIL,
    "last_review": 1767225600, "next_review": 1893456000, "notes": "legal notes",
    "current_likelihood": 4, "current_impact": 3,
    "treatment": "mitigate", "treatment_plan": "comply",
    "target_likelihood": 2, "target_impact": 2, "completion": 40,
    "external_id": "LEG-EXT",
}


def _make_legal(api_url, headers):
    r = requests.post(f"{api_url}/legal", headers=headers, json={"title": f"legal {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def test_legal_update_parity(api_url, admin_headers):
    _update_parity(api_url, admin_headers, "legal_requirement", "legal",
                   lambda: _make_legal(api_url, admin_headers), LEGAL_FIELDS, unique=("external_id",))


def test_legal_create_parity(api_url, admin_headers):
    got_a, got_b = _create_parity(api_url, admin_headers, "legal_requirement", "legal", LEGAL_FIELDS,
                                  unique=("external_id", "title"))
    assert got_a["current_score"] == got_b["current_score"]


def test_legal_update_empty_category_is_400_at_apply(api_url, admin_headers):
    lr = _make_legal(api_url, admin_headers)
    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "legal_requirement", "suggestion_type": "update", "entity_id": lr["identifier"],
        "title": "bad", "payload": {"fields": {"category": ""}}})
    assert sg.status_code == 201, sg.text
    ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply", headers=admin_headers, json={"force": True})
    assert ap.status_code == 400 and "category" in ap.text, ap.text


# --- objective and program -------------------------------------------------

def _fresh_org(api_url):
    """A brand-new org (own programs list) with its own admin; returns headers."""
    t = _tag()
    email = f"parity-{t}@isms-test.local"
    pw = "TestPass123!"
    r = requests.post(f"{api_url}/auth/signup", json={"email": email, "password": pw, "name": "Parity Admin"})
    assert r.status_code in (200, 201), r.text
    r = requests.post(f"{api_url}/auth/login", json={"email": email, "password": pw})
    assert r.status_code == 200, r.text
    hdr = {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}
    slug = f"parity-{t}"
    r = requests.post(f"{api_url}/organizations", headers=hdr, json={"name": f"Parity {t}", "slug": slug})
    assert r.status_code in (200, 201), r.text
    r = requests.post(f"{api_url}/auth/login", json={"email": email, "password": pw, "organization": slug})
    assert r.status_code == 200, r.text
    return {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}


def _make_program(api_url, headers, key=None):
    key = key or f"P{_tag()[:5].upper()}"
    r = requests.post(f"{api_url}/programs", headers=headers, json={"key": key, "title": f"prog {key}"})
    assert r.status_code in (200, 201), r.text
    return r.json()


def _suggest(api_url, headers, entity_type, payload, suggestion_type="create"):
    return requests.post(f"{api_url}/suggestions", headers=headers, json={
        "entity_type": entity_type, "suggestion_type": suggestion_type,
        "title": f"parity {entity_type} {_tag()}", "payload": payload})


def _apply_raw(api_url, headers, sid):
    return requests.post(f"{api_url}/suggestions/{sid}/apply", headers=headers, json={"force": True})


def _objective_fields():
    return {
        "title": "renamed objective", "description": "objective description",
        "owner": ADMIN_EMAIL, "source": "probe", "measurement_method": "count",
        "target_value": 14.5, "target_operator": "lte", "unit": "days",
        "window_seconds": 3600, "grace_seconds": 60, "checkin_cycle": 4,
        "status": "active", "started_at": 1767225600, "notes": "objective notes",
    }


def _make_objective(api_url, headers, program_id):
    r = requests.post(f"{api_url}/objectives", headers=headers,
                      json={"title": f"obj {_tag()}", "program_id": program_id})
    assert r.status_code in (200, 201), r.text
    return r.json()


def test_objective_update_parity(api_url, admin_headers):
    prog = _make_program(api_url, admin_headers)
    _update_parity(api_url, admin_headers, "objective", "objectives",
                   lambda: _make_objective(api_url, admin_headers, prog["id"]),
                   _objective_fields(), ref="display_id")


def test_objective_create_parity(api_url, admin_headers):
    prog = _make_program(api_url, admin_headers)
    body = dict(_objective_fields(), program_id=prog["id"])
    got_a, got_b = _create_parity(api_url, admin_headers, "objective", "objectives", body,
                                  unique=("title",), get_by="ident")
    assert got_a["program_id"] == got_b["program_id"] == prog["id"]


def test_objective_create_by_program_key(api_url, admin_headers):
    prog = _make_program(api_url, admin_headers)
    ident = _apply(api_url, admin_headers, "objective", "create",
                   {"title": f"obj {_tag()}", "program_key": prog["key"].lower()})
    got = requests.get(f"{api_url}/objectives/{ident}", headers=admin_headers).json()
    assert got["program_id"] == prog["id"]


def test_objective_create_program_rules(api_url):
    hdr = _fresh_org(api_url)
    # zero programs: accepted at create time, refused at apply.
    sg = _suggest(api_url, hdr, "objective", {"title": "o"})
    assert sg.status_code == 201, sg.text
    ap = _apply_raw(api_url, hdr, sg.json()["id"])
    assert ap.status_code == 400 and "no program exists yet" in ap.text, ap.text
    # exactly one program: used.
    zzz = _make_program(api_url, hdr, "ZZZ")
    ident = _apply(api_url, hdr, "objective", "create", {"title": "o2"})
    got = requests.get(f"{api_url}/objectives/{ident}", headers=hdr).json()
    assert got["program_id"] == zzz["id"]
    # several programs, none named: 400 at create, naming the keys sorted.
    _make_program(api_url, hdr, "AAA")
    sg = _suggest(api_url, hdr, "objective", {"title": "o3"})
    assert sg.status_code == 400, sg.text
    assert "several programs (AAA, ZZZ)" in sg.text, sg.text
    assert "program_id or program_key is required" in sg.text, sg.text
    # naming one resolves it; naming both but disagreeing is 400.
    aaa = requests.get(f"{api_url}/programs/AAA", headers=hdr).json()
    sg = _suggest(api_url, hdr, "objective", {"title": "o4", "program_key": "AAA", "program_id": zzz["id"]})
    assert sg.status_code == 400 and "different programs" in sg.text, sg.text
    ident = _apply(api_url, hdr, "objective", "create", {"title": "o5", "program_key": "aaa"})
    assert requests.get(f"{api_url}/objectives/{ident}", headers=hdr).json()["program_id"] == aaa["id"]


def test_objective_create_foreign_program_is_400(api_url, admin_headers):
    other = _fresh_org(api_url)
    foreign = _make_program(api_url, other)
    sg = _suggest(api_url, admin_headers, "objective", {"title": "o", "program_id": foreign["id"]})
    assert sg.status_code == 400, sg.text
    sg = _suggest(api_url, admin_headers, "objective", {"title": "o", "program_key": foreign["key"]})
    assert sg.status_code == 400, sg.text


def test_program_create_suggestion(api_url):
    hdr = _fresh_org(api_url)
    ident = _apply(api_url, hdr, "program", "create",
                   {"key": "sec", "title": "Security", "description": "d", "notes": "n"})
    got = requests.get(f"{api_url}/programs/SEC", headers=hdr).json()
    assert got["title"] == "Security" and got["key"] == "SEC" and got["description"] == "d"
    assert got["notes"] == "n" and got["key"] == ident == "SEC"
    # second identical key: 409, not 500.
    sg = _suggest(api_url, hdr, "program", {"key": "SEC", "title": "Again"})
    assert sg.status_code == 201, sg.text
    ap = _apply_raw(api_url, hdr, sg.json()["id"])
    assert ap.status_code == 409, ap.text
    # key and title are required.
    sg = _suggest(api_url, hdr, "program", {"title": "No key"})
    assert sg.status_code == 201, sg.text
    assert _apply_raw(api_url, hdr, sg.json()["id"]).status_code == 400


# --- incident --------------------------------------------------------------

INCIDENT_FIELDS = {
    "title": "renamed incident", "description": "incident description", "severity": "high",
    "affects_c": True, "affects_i": True, "affects_a": False,
    "incident_type": "weakness", "source": "external", "status": "contained",
    "notes": "incident notes", "data_breach": True, "gdpr_role": "processor",
    "authority_notified": "notified", "authority_notified_at": 1767225600,
    "subjects_notified": "pending", "subjects_notified_at": 1767312000,
    "assignee": ADMIN_EMAIL, "root_cause": "a cause", "lessons_learned": "a lesson",
    "external_id": "INC-EXT",
}


def _make_incident(api_url, headers):
    r = requests.post(f"{api_url}/incidents", headers=headers, json={"title": f"inc {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def test_incident_update_parity(api_url, admin_headers):
    _update_parity(api_url, admin_headers, "incident", "incidents",
                   lambda: _make_incident(api_url, admin_headers), INCIDENT_FIELDS, unique=("external_id",))


def test_incident_create_parity(api_url, admin_headers):
    body = dict(INCIDENT_FIELDS, status="investigating", reporter=ADMIN_EMAIL, detected_at=1767225600)
    _create_parity(api_url, admin_headers, "incident", "incidents", body,
                   unique=("external_id", "title"), get_by="ident")


def test_incident_create_summary_is_refused(api_url, admin_headers):
    # "summary" and "affected_systems" used to be decoded and silently dropped.
    for key, val in (("summary", "s"), ("affected_systems", ["x"])):
        sg = _suggest(api_url, admin_headers, "incident", {"title": "t", key: val})
        assert sg.status_code == 400 and key in sg.text, sg.text



# --- corrective action -----------------------------------------------------

CA_FIELDS = {
    "title": "renamed CA", "description": "ca description", "source": "internal_audit",
    "severity": "major_nc", "status": "implementation", "assignee": ADMIN_EMAIL,
    "due_date": 1893456000, "root_cause": "a cause", "notes": "ca notes",
    "external_id": "CA-EXT",
}


def _make_ca(api_url, headers):
    r = requests.post(f"{api_url}/corrective-actions", headers=headers, json={"title": f"ca {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def test_corrective_action_update_parity(api_url, admin_headers):
    _update_parity(api_url, admin_headers, "corrective_action", "corrective-actions",
                   lambda: _make_ca(api_url, admin_headers), CA_FIELDS, unique=("external_id",))


def test_corrective_action_create_parity(api_url, admin_headers):
    _create_parity(api_url, admin_headers, "corrective_action", "corrective-actions", CA_FIELDS,
                   unique=("external_id", "title"), get_by="ident")


def test_corrective_action_resolve_blocked_by_open_task_is_409(api_url, admin_headers):
    ca = _make_ca(api_url, admin_headers)
    t = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
        "title": f"ca_followup {ca['identifier']} {_tag()}", "task_type": "ca_followup",
        "description": ca["identifier"]})
    assert t.status_code == 201, t.text
    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "corrective_action", "suggestion_type": "update", "entity_id": ca["identifier"],
        "title": "resolve", "payload": {"fields": {"status": "resolved"}}})
    assert sg.status_code == 201, sg.text
    ap = _apply_raw(api_url, admin_headers, sg.json()["id"])
    assert ap.status_code == 409, ap.text


# --- change request --------------------------------------------------------

CHANGE_FIELDS = {
    "type": "access_request", "title": "renamed change", "description": "change description",
    "justification": "because", "priority": "critical", "category": "technology",
    "risk_level": "high", "rollback_plan": "revert it", "notes": "change notes",
    "assigned_to": ADMIN_EMAIL, "status": "approved", "planned_at": 1893456000,
}


def _make_change(api_url, headers):
    r = requests.post(f"{api_url}/changes", headers=headers, json={"title": f"chg {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def test_change_update_parity(api_url, admin_headers):
    _update_parity(api_url, admin_headers, "change_request", "changes",
                   lambda: _make_change(api_url, admin_headers), CHANGE_FIELDS)
    # approval metadata is derived by the shared transition, on both paths.


def test_change_update_status_stamps_approval(api_url, admin_headers):
    c = _make_change(api_url, admin_headers)
    _apply(api_url, admin_headers, "change_request", "update", {"fields": {"status": "approved"}},
           entity_id=c["identifier"])
    got = requests.get(f"{api_url}/changes/{c['id']}", headers=admin_headers).json()
    assert got["status"] == "approved" and got.get("approved_at"), got


def test_change_create_parity(api_url, admin_headers):
    # status is "proposed", not CHANGE_FIELDS' own "approved" — #423/F1 made
    # creating a change in any other status a 409 on both entry points, so a
    # create-parity check can no longer exercise an already-decided status at
    # creation (the update-parity test above still does, via CHANGE_FIELDS
    # unmodified, since proposed -> approved is a legal transition on update).
    body = dict(CHANGE_FIELDS, status="proposed")
    _create_parity(api_url, admin_headers, "change_request", "changes", body,
                   unique=("title",), get_by="ident")


def test_change_create_honours_type_and_status(api_url, admin_headers):
    # status must be "proposed" at creation (#423/F1); kept explicit in the
    # payload, rather than omitted, so this still proves the suggestion-create
    # path reads and honours a caller-supplied status rather than silently
    # ignoring it and always defaulting.
    ident = _apply(api_url, admin_headers, "change_request", "create",
                   {"title": f"chg {_tag()}", "type": "access_request", "status": "proposed"})
    got = requests.get(f"{api_url}/changes/{ident}", headers=admin_headers).json()
    assert got["type"] == "access_request" and got["status"] == "proposed", got
    assert got["assigned_to"] == ADMIN_EMAIL  # defaults to the requester, like POST


# --- task ------------------------------------------------------------------

TASK_FIELDS = {
    "title": "renamed task", "description": "task description", "task_type": "training",
    "assignee": ADMIN_EMAIL, "status": "in_progress", "priority": "critical",
    "due_date": 1893456000, "recurrence_days": 30, "notes": "task notes", "private": True,
}


def _make_task(api_url, headers):
    r = requests.post(f"{api_url}/tasks", headers=headers, json={"title": f"task {_tag()}"})
    assert r.status_code == 201, r.text
    return r.json()


def test_task_update_parity(api_url, admin_headers):
    _update_parity(api_url, admin_headers, "task", "tasks",
                   lambda: _make_task(api_url, admin_headers), TASK_FIELDS)


def test_task_update_done_stamps_and_reopen_clears_completed_at(api_url, admin_headers):
    t = _make_task(api_url, admin_headers)
    _apply(api_url, admin_headers, "task", "update", {"fields": {"status": "done"}}, entity_id=t["identifier"])
    got = requests.get(f"{api_url}/tasks/{t['id']}", headers=admin_headers).json()
    assert got["status"] == "done" and got.get("completed_at"), got
    _apply(api_url, admin_headers, "task", "update", {"fields": {"status": "open"}}, entity_id=t["identifier"])
    got = requests.get(f"{api_url}/tasks/{t['id']}", headers=admin_headers).json()
    assert got["status"] == "open" and not got.get("completed_at"), got


def test_task_create_parity(api_url, admin_headers):
    _create_parity(api_url, admin_headers, "task", "tasks", TASK_FIELDS, unique=("title",), get_by="ident")


# --- audit finding ---------------------------------------------------------

def _make_audit(api_url, headers):
    prog = requests.post(f"{api_url}/audit/programmes", headers=headers,
                         json={"title": f"parity prog {_tag()}", "year": 2026})
    assert prog.status_code in (200, 201), prog.text
    aud = requests.post(f"{api_url}/audits", headers=headers, json={
        "programme_id": prog.json()["id"], "title": f"parity audit {_tag()}", "status": "planned"})
    assert aud.status_code in (200, 201), aud.text
    return aud.json()


def _make_finding(api_url, headers, audit_id):
    r = requests.post(f"{api_url}/audit-findings", headers=headers, json={
        "audit_id": audit_id, "title": f"find {_tag()}", "finding_type": "observation"})
    assert r.status_code in (200, 201), r.text
    return r.json()


def test_audit_finding_update_parity(api_url, admin_headers):
    aud = _make_audit(api_url, admin_headers)
    fields = {"title": "renamed finding", "description": "finding description",
              "owner": ADMIN_EMAIL, "due_date": 1893456000, "status": "closed"}
    _update_parity(api_url, admin_headers, "audit_finding", "audit-findings",
                   lambda: _make_finding(api_url, admin_headers, aud["id"]), fields,
                   ref="id")
    # the FIND-<n> form of the entity id resolves too
    g = _make_finding(api_url, admin_headers, aud["id"])
    _apply(api_url, admin_headers, "audit_finding", "update", {"fields": {"title": "via FIND form"}},
           entity_id=f"FIND-{g['id']}")
    assert requests.get(f"{api_url}/audit-findings/{g['id']}", headers=admin_headers).json()["title"] == "via FIND form"
    # the closure stamp comes from the shared status path on both sides
    f = _make_finding(api_url, admin_headers, aud["id"])
    _apply(api_url, admin_headers, "audit_finding", "update", {"fields": {"status": "closed"}}, entity_id=f["id"])
    got = requests.get(f"{api_url}/audit-findings/{f['id']}", headers=admin_headers).json()
    assert got["status"] == "closed" and got.get("closed_at"), got


def test_audit_finding_update_empty_title_is_400(api_url, admin_headers):
    aud = _make_audit(api_url, admin_headers)
    f = _make_finding(api_url, admin_headers, aud["id"])
    sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "audit_finding", "suggestion_type": "update", "entity_id": str(f["id"]),
        "title": "blank", "payload": {"fields": {"title": ""}}})
    assert sg.status_code == 201, sg.text
    ap = _apply_raw(api_url, admin_headers, sg.json()["id"])
    assert ap.status_code == 400 and "title" in ap.text, ap.text


def test_audit_finding_create_parity(api_url, admin_headers):
    aud = _make_audit(api_url, admin_headers)
    body = {"audit_id": aud["id"], "finding_type": "major_nc", "title": "a finding",
            "description": "finding description", "due_date": 1893456000, "owner": ADMIN_EMAIL}
    r = requests.post(f"{api_url}/audit-findings", headers=admin_headers, json=body)
    assert r.status_code == 201, r.text
    ident = _apply(api_url, admin_headers, "audit_finding", "create", body)
    assert ident.startswith("FIND-"), ident
    got_a = requests.get(f"{api_url}/audit-findings/{r.json()['id']}", headers=admin_headers).json()
    got_b = requests.get(f"{api_url}/audit-findings/{ident}", headers=admin_headers).json()
    _assert_same(got_a, got_b, body, "audit finding create")
    assert got_a["status"] == got_b["status"] == "open"


def test_audit_finding_create_defaults_and_checks(api_url, admin_headers):
    aud = _make_audit(api_url, admin_headers)
    # apply-only default finding_type
    ident = _apply(api_url, admin_headers, "audit_finding", "create", {"audit_id": aud["id"], "title": "t"})
    got = requests.get(f"{api_url}/audit-findings/{ident}", headers=admin_headers).json()
    assert got["finding_type"] == "observation"
    # bad type and a foreign/unknown audit are 4xx at apply, never 500
    for payload, code in (({"audit_id": aud["id"], "title": "t", "finding_type": "bogus"}, 400),
                          ({"audit_id": 99999999, "title": "t"}, 404)):
        sg = _suggest(api_url, admin_headers, "audit_finding", payload)
        assert sg.status_code == 201, sg.text
        assert _apply_raw(api_url, admin_headers, sg.json()["id"]).status_code == code


# --- remaining cross-cutting cases ------------------------------------------

def test_targeted_suggestion_types_require_entity_id(api_url, admin_headers):
    cases = [("asset", "update", {"fields": {"notes": "x"}}), ("risk", "update", {"fields": {"notes": "x"}}),
             ("risk", "reassess", {"current_likelihood": 2}), ("risk", "reading", {"reading_type": "x"}),
             ("supplier", "review", {"outcome": "satisfactory"}), ("incident", "link", {"links": []})]
    for entity, kind, payload in cases:
        r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": entity, "suggestion_type": kind, "title": "no entity", "payload": payload})
        assert r.status_code == 400 and "entity_id" in r.text, (entity, kind, r.status_code, r.text)


def test_unresolvable_entity_is_404_at_apply_not_500(api_url, admin_headers):
    for entity, ident, fields in (
            ("asset", "ASSET-99999", {"notes": "x"}), ("system", "SYSTEM-99999", {"notes": "x"}),
            ("supplier", "SUPPLIER-99999", {"notes": "x"}), ("risk", "RISK-99999", {"notes": "x"}),
            ("legal_requirement", "LEGAL-99999", {"notes": "x"}), ("incident", "INC-99999", {"notes": "x"}),
            ("task", "TASK-99999", {"notes": "x"}), ("change_request", "CR-99999", {"notes": "x"}),
            ("corrective_action", "CA-99999", {"notes": "x"}), ("objective", "NOPE-99999", {"title": "x"}),
            ("audit_finding", "FIND-99999", {"title": "x"})):
        sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": entity, "suggestion_type": "update", "entity_id": ident,
            "title": "gone", "payload": {"fields": fields}})
        assert sg.status_code == 201, (entity, sg.status_code, sg.text)
        ap = _apply_raw(api_url, admin_headers, sg.json()["id"])
        assert ap.status_code == 404, (entity, ap.status_code, ap.text)


def test_parity_file_covers_every_entity():
    import inspect
    src = inspect.getsource(__import__(__name__))
    for entity in ("asset", "system", "supplier", "risk", "legal", "objective", "incident",
                   "corrective_action", "change", "task", "audit_finding"):
        assert f"def test_{entity}_update_parity" in src, entity
        assert f"def test_{entity}_create_parity" in src, entity
    assert "def test_program_create_suggestion" in src


# --- review fixes ----------------------------------------------------------

def test_incident_create_ghost_reporter_is_400_and_stays_open(api_url, admin_headers):
    sg = _suggest(api_url, admin_headers, "incident", {"title": "t", "reporter": "ghost@nowhere.io"})
    assert sg.status_code == 201, sg.text
    ap = _apply_raw(api_url, admin_headers, sg.json()["id"])
    assert ap.status_code == 400, ap.text
    got = requests.get(f"{api_url}/suggestions/{sg.json()['id']}", headers=admin_headers).json()["data"]
    assert got["status"] == "open", got
    # the HTTP path refuses the same payload
    r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={"title": "t", "reporter": "ghost@nowhere.io"})
    assert r.status_code == 400, r.text


def _search_titles(api_url, headers, q):
    r = requests.get(f"{api_url}/search", headers=headers, params={"q": q})
    assert r.status_code == 200, r.text
    return [x.get("title") for x in (r.json()["data"] or [])]


def test_task_made_private_by_apply_leaves_the_search_index(api_url, admin_headers, reader_headers):
    title = f"zzprivsearch{_tag()}"
    r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={"title": title})
    assert r.status_code == 201, r.text
    t = r.json()
    assert title in _search_titles(api_url, reader_headers, title)
    _apply(api_url, admin_headers, "task", "update", {"fields": {"private": True}}, entity_id=t["identifier"])
    assert title not in _search_titles(api_url, reader_headers, title)


def test_private_task_created_by_apply_is_not_indexed(api_url, admin_headers, reader_headers):
    title = f"zzprivcreate{_tag()}"
    _apply(api_url, admin_headers, "task", "create", {"title": title, "private": True})
    assert title not in _search_titles(api_url, reader_headers, title)


def _set_custom_fields(api_url, headers, defs):
    import json as _json
    r = requests.put(f"{api_url}/admin/settings", headers=headers,
                     json={"key": "risk_custom_fields", "value": _json.dumps(defs)})
    assert r.status_code == 200, r.text


CF_DEFS = [{"key": "vendor", "label": "Vendor", "type": "text", "required": True},
           {"key": "tier", "label": "Tier", "type": "select", "options": ["Gold", "Silver"]}]


def test_risk_custom_fields_update_parity(api_url):
    hdr = _fresh_org(api_url)
    _set_custom_fields(api_url, hdr, CF_DEFS)
    def make():
        r = requests.post(f"{api_url}/risks", headers=hdr, json={"title": f"r {_tag()}", "custom_fields": {"vendor": "Init"}})
        assert r.status_code == 201, r.text
        return r.json()
    a, b = make(), make()
    fields = {"custom_fields": {"vendor": "  Acme  ", "tier": "Gold"}}
    assert requests.put(f"{api_url}/risks/{a['id']}", headers=hdr, json=fields).status_code == 200
    _apply(api_url, hdr, "risk", "update", {"fields": fields}, entity_id=b["identifier"])
    got_a = requests.get(f"{api_url}/risks/{a['id']}", headers=hdr).json()
    got_b = requests.get(f"{api_url}/risks/{b['id']}", headers=hdr).json()
    assert got_a["custom_fields"] == got_b["custom_fields"] == {"vendor": "Acme", "tier": "Gold"}, (got_a, got_b)
    # the required-field rule applies to update on both paths
    bad = {"custom_fields": {"tier": "Silver"}}
    assert requests.put(f"{api_url}/risks/{a['id']}", headers=hdr, json=bad).status_code == 400
    sg = requests.post(f"{api_url}/suggestions", headers=hdr, json={
        "entity_type": "risk", "suggestion_type": "update", "entity_id": b["identifier"],
        "title": "bad cf", "payload": {"fields": bad}})
    assert sg.status_code == 201, sg.text
    assert _apply_raw(api_url, hdr, sg.json()["id"]).status_code == 400
    # an invalid select option is refused on both paths
    worse = {"custom_fields": {"vendor": "Acme", "tier": "Bronze"}}
    assert requests.put(f"{api_url}/risks/{a['id']}", headers=hdr, json=worse).status_code == 400
    sg = requests.post(f"{api_url}/suggestions", headers=hdr, json={
        "entity_type": "risk", "suggestion_type": "update", "entity_id": b["identifier"],
        "title": "bad opt", "payload": {"fields": worse}})
    assert _apply_raw(api_url, hdr, sg.json()["id"]).status_code == 400


def test_risk_custom_fields_create_parity(api_url):
    hdr = _fresh_org(api_url)
    _set_custom_fields(api_url, hdr, CF_DEFS)
    r = requests.post(f"{api_url}/risks", headers=hdr,
                      json={"title": "cf risk", "custom_fields": {"vendor": "  Acme  ", "tier": "Silver"}})
    assert r.status_code == 201, r.text
    ident = _apply(api_url, hdr, "risk", "create",
                   {"title": "cf risk 2", "custom_fields": {"vendor": "  Acme  ", "tier": "Silver"}})
    got_a = requests.get(f"{api_url}/risks/{r.json()['id']}", headers=hdr).json()
    got_b = requests.get(f"{api_url}/risks/{ident}", headers=hdr).json()
    assert got_a["custom_fields"] == got_b["custom_fields"] == {"vendor": "Acme", "tier": "Silver"}
    # required fields: enforced on POST, deliberately not on apply (agents cannot fill forms)
    assert requests.post(f"{api_url}/risks", headers=hdr, json={"title": "no vendor"}).status_code == 400
    _apply(api_url, hdr, "risk", "create", {"title": "no vendor via apply"})
