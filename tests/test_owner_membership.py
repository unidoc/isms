"""Owners and assignees must be org members on every create and on the objective
and program updates (#200 review). The SQL resolves emails against the global
users table, so without the check a user of another org, or nobody, is attached.

For each path: a ghost email is 400 over HTTP and at apply (the suggestion stays
open), a user of ANOTHER org is 400 on both, and a real member is accepted by both.
"""
import uuid

import pytest
import requests
from conftest import CONTRIBUTOR_EMAIL

GHOST = "ghost@nowhere.io"


def _tag():
    return uuid.uuid4().hex[:8]


@pytest.fixture(scope="module")
def foreign_email(api_url):
    """A real user who belongs to a different org."""
    t = _tag()
    email, pw = f"foreign-{t}@isms-test.local", "TestPass123!"
    r = requests.post(f"{api_url}/auth/signup", json={"email": email, "password": pw, "name": "Foreign"})
    assert r.status_code in (200, 201), r.text
    r = requests.post(f"{api_url}/auth/login", json={"email": email, "password": pw})
    hdr = {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}
    r = requests.post(f"{api_url}/organizations", headers=hdr, json={"name": f"Foreign {t}", "slug": f"foreign-{t}"})
    assert r.status_code in (200, 201), r.text
    return email


def _program(api_url, h):
    key = f"M{_tag()[:5].upper()}"
    r = requests.post(f"{api_url}/programs", headers=h, json={"key": key, "title": f"prog {key}"})
    assert r.status_code in (200, 201), r.text
    return r.json()


# Each case: (id, http(api_url, h, owner) -> Response, suggestion(api_url, h, owner) -> (entity_type, type, entity_id, payload) | None)
def _c_supplier(a, h, o):
    return requests.post(f"{a}/suppliers", headers=h, json={"name": f"s {_tag()}", "owner": o})


def _c_legal(a, h, o):
    return requests.post(f"{a}/legal", headers=h, json={"title": f"l {_tag()}", "owner": o})


def _c_change(a, h, o):
    return requests.post(f"{a}/changes", headers=h, json={"title": f"c {_tag()}", "assigned_to": o})


def _c_system(a, h, o):
    return requests.post(f"{a}/systems", headers=h, json={"name": f"sy {_tag()}", "owner": o})


def _c_asset(a, h, o):
    return requests.post(f"{a}/assets", headers=h, json={"name": f"as {_tag()}", "owner": o})


def _c_program(a, h, o):
    return requests.post(f"{a}/programs", headers=h, json={"key": f"Q{_tag()[:5].upper()}", "title": "p", "owner": o})


def _c_objective(a, h, o):
    p = _program(a, h)
    return requests.post(f"{a}/objectives", headers=h, json={"title": f"o {_tag()}", "program_id": p["id"], "owner": o})


def _u_objective(a, h, o):
    p = _program(a, h)
    obj = requests.post(f"{a}/objectives", headers=h, json={"title": f"o {_tag()}", "program_id": p["id"]}).json()
    return requests.put(f"{a}/objectives/{obj['id']}", headers=h, json={"owner": o})


def _u_program(a, h, o):
    p = _program(a, h)
    return requests.put(f"{a}/programs/{p['key']}", headers=h, json={"owner": o})


HTTP_CASES = {
    "supplier_create": _c_supplier, "legal_create": _c_legal, "change_create": _c_change,
    "system_create": _c_system, "asset_create": _c_asset, "program_create": _c_program,
    "objective_create": _c_objective, "objective_update": _u_objective, "program_update": _u_program,
}


@pytest.mark.parametrize("case", sorted(HTTP_CASES))
def test_http_owner_must_be_org_member(api_url, admin_headers, foreign_email, case):
    fn = HTTP_CASES[case]
    assert fn(api_url, admin_headers, GHOST).status_code == 400, case
    assert fn(api_url, admin_headers, foreign_email).status_code == 400, case
    assert fn(api_url, admin_headers, CONTRIBUTOR_EMAIL).status_code in (200, 201), case


def _s_create(entity, field, body):
    def build(a, h, o):
        return entity, "create", None, dict(body(), **{field: o})
    return build


def _s_objective_create(a, h, o):
    p = _program(a, h)
    return "objective", "create", None, {"title": f"o {_tag()}", "program_id": p["id"], "owner": o}


def _s_objective_update(a, h, o):
    p = _program(a, h)
    obj = requests.post(f"{a}/objectives", headers=h, json={"title": f"o {_tag()}", "program_id": p["id"]}).json()
    return "objective", "update", obj["display_id"], {"fields": {"owner": o}}


SUGGESTION_CASES = {
    "supplier_create": _s_create("supplier", "owner", lambda: {"name": f"s {_tag()}"}),
    "legal_create": _s_create("legal_requirement", "owner", lambda: {"title": f"l {_tag()}"}),
    "change_create": _s_create("change_request", "assigned_to", lambda: {"title": f"c {_tag()}"}),
    "system_create": _s_create("system", "owner", lambda: {"name": f"sy {_tag()}"}),
    "asset_create": _s_create("asset", "owner", lambda: {"name": f"as {_tag()}"}),
    "program_create": _s_create("program", "owner", lambda: {"key": f"R{_tag()[:5].upper()}", "title": "p"}),
    "objective_create": _s_objective_create,
    "objective_update": _s_objective_update,
}


def _suggest_and_apply(a, h, spec):
    entity, kind, entity_id, payload = spec
    body = {"entity_type": entity, "suggestion_type": kind, "title": f"owner {_tag()}", "payload": payload}
    if entity_id:
        body["entity_id"] = entity_id
    sg = requests.post(f"{a}/suggestions", headers=h, json=body)
    assert sg.status_code == 201, sg.text
    ap = requests.post(f"{a}/suggestions/{sg.json()['id']}/apply", headers=h, json={"force": True})
    return sg.json()["id"], ap


@pytest.mark.parametrize("case", sorted(SUGGESTION_CASES))
def test_apply_owner_must_be_org_member(api_url, admin_headers, foreign_email, case):
    build = SUGGESTION_CASES[case]
    for bad in (GHOST, foreign_email):
        sid, ap = _suggest_and_apply(api_url, admin_headers, build(api_url, admin_headers, bad))
        assert ap.status_code == 400, (case, bad, ap.status_code, ap.text)
        got = requests.get(f"{api_url}/suggestions/{sid}", headers=admin_headers).json()["data"]
        assert got["status"] == "open", (case, bad, got)
    _, ap = _suggest_and_apply(api_url, admin_headers, build(api_url, admin_headers, CONTRIBUTOR_EMAIL))
    assert ap.status_code == 200, (case, ap.status_code, ap.text)
