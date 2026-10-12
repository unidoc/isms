"""POST /entity-comments validates and canonicalises the record (#438).

An unknown type is a 400 (not a 500 carrying the Postgres CHECK text), a
comment on a record that doesn't exist is a 400 not_found_in_org (not a stored
201), and a record has ONE thread whichever spelling of its id the caller uses:
AUDIT-3 and 3 are the same audit, a program's row id and its key are the same
program. Comments are stored under the id /references stores (AUDIT-/FIND-/AP-
prefixed, program key, objective display id).
"""
import uuid

import pytest
import requests
from conftest import ADMIN_EMAIL


def _post(api_url, headers, etype, eid, body="c", parent_id=None):
    payload = {"entity_type": etype, "entity_id": eid, "body": body}
    if parent_id is not None:
        payload["parent_id"] = parent_id
    return requests.post(f"{api_url}/entity-comments", headers=headers, json=payload)


def _list(api_url, headers, etype, eid):
    r = requests.get(f"{api_url}/entity-comments/{etype}/{eid}", headers=headers)
    assert r.status_code == 200, r.text
    return r.json().get("data") or []


def _create(api_url, headers, path, payload):
    r = requests.post(f"{api_url}{path}", headers=headers, json=payload)
    assert r.status_code in (200, 201), f"{path}: {r.text}"
    return r.json()


@pytest.fixture(scope="module")
def audit(api_url, admin_headers):
    programme = _create(api_url, admin_headers, "/audit/programmes",
                        {"title": f"cv-programme-{uuid.uuid4().hex[:8]}", "year": 2026})
    a = _create(api_url, admin_headers, "/audits", {
        "programme_id": programme["id"], "title": f"cv-audit-{uuid.uuid4().hex[:8]}",
        "scope": "iso27001-4.*", "auditor": ADMIN_EMAIL, "audit_type": "internal",
        "status": "planned", "planned_date": 1781740800,
    })
    return {"audit": a, "programme": programme}


def test_unknown_entity_type_is_400_without_database_text(api_url, admin_headers):
    r = _post(api_url, admin_headers, "bogus", "X-1")
    assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"
    low = r.text.lower()
    assert "sqlstate" not in low and "constraint" not in low and "violates" not in low


MISSING = [
    ("risk", "RISK-999999"),
    ("audit", "999999"),
    ("audit", "AUDIT-999999"),
    ("audit_finding", "999999"),
    ("audit_programme", "999999"),
    ("program", "NOSUCHKEY"),
    ("objective", "NOPE-999"),
    ("task", "TASK-999999"),
    ("document", "no-such-doc-438"),
    ("checkin", "999999"),
    ("access_review", "999999"),
    ("review", "999999"),
]


@pytest.mark.parametrize("etype,eid", MISSING)
def test_comment_on_a_missing_record_is_refused(api_url, admin_headers, etype, eid):
    r = _post(api_url, admin_headers, etype, eid)
    assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"
    assert r.json().get("code") == "not_found_in_org", r.text
    assert _list(api_url, admin_headers, etype, eid) == [], "a refused comment must not be stored"


@pytest.mark.parametrize("etype,key", [
    ("audit", "audit"), ("audit_programme", "programme"),
])
def test_audit_spellings_share_one_thread(api_url, admin_headers, audit, etype, key):
    row = audit[key]["id"]
    prefix = {"audit": "AUDIT", "audit_programme": "AP"}[etype]
    canonical = f"{prefix}-{row}"
    a = _post(api_url, admin_headers, etype, str(row), "by bare id")
    b = _post(api_url, admin_headers, etype, canonical, "by prefixed id")
    assert a.status_code == 201 and b.status_code == 201, (a.text, b.text)
    assert a.json()["entity_id"] == canonical
    assert b.json()["entity_id"] == canonical
    want = {a.json()["id"], b.json()["id"]}
    for spelling in (str(row), canonical):
        got = {c["id"] for c in _list(api_url, admin_headers, etype, spelling)}
        assert want <= got, f"{spelling}: {got}"


def test_audit_finding_spellings_share_one_thread(api_url, admin_headers, audit):
    f = _create(api_url, admin_headers, "/audit-findings", {
        "audit_id": audit["audit"]["id"], "finding_type": "minor_nc",
        "title": "cv finding", "description": "cv finding description",
    })
    canonical = f"FIND-{f['id']}"
    a = _post(api_url, admin_headers, "audit_finding", str(f["id"]), "bare")
    b = _post(api_url, admin_headers, "audit_finding", canonical, "prefixed")
    assert a.status_code == 201 and b.status_code == 201, (a.text, b.text)
    assert a.json()["entity_id"] == canonical and b.json()["entity_id"] == canonical
    for spelling in (str(f["id"]), canonical):
        assert len(_list(api_url, admin_headers, "audit_finding", spelling)) == 2


def test_program_row_id_and_key_share_one_thread(api_url, admin_headers):
    key = f"CV{uuid.uuid4().hex[:6]}".upper()
    p = _create(api_url, admin_headers, "/programs", {"key": key, "title": f"cv program {key}"})
    a = _post(api_url, admin_headers, "program", str(p["id"]), "by row id")
    b = _post(api_url, admin_headers, "program", p["key"], "by key")
    assert a.status_code == 201 and b.status_code == 201, (a.text, b.text)
    assert a.json()["entity_id"] == p["key"] and b.json()["entity_id"] == p["key"]
    for spelling in (str(p["id"]), p["key"]):
        assert len(_list(api_url, admin_headers, "program", spelling)) == 2


def test_objective_row_id_and_display_id_share_one_thread(api_url, admin_headers):
    key = f"CO{uuid.uuid4().hex[:6]}".upper()
    p = _create(api_url, admin_headers, "/programs", {"key": key, "title": f"cv program {key}"})
    o = _create(api_url, admin_headers, "/objectives",
                {"program_id": p["id"], "title": f"cv objective {key}"})
    display = o["display_id"]
    a = _post(api_url, admin_headers, "objective", str(o["id"]), "by row id")
    b = _post(api_url, admin_headers, "objective", display, "by display id")
    assert a.status_code == 201 and b.status_code == 201, (a.text, b.text)
    assert a.json()["entity_id"] == display and b.json()["entity_id"] == display
    assert len(_list(api_url, admin_headers, "objective", str(o["id"]))) == 2


def test_reply_with_the_other_spelling_is_accepted(api_url, admin_headers, audit):
    row = audit["audit"]["id"]
    parent = _post(api_url, admin_headers, "audit", f"AUDIT-{row}", "parent")
    assert parent.status_code == 201, parent.text
    reply = _post(api_url, admin_headers, "audit", str(row), "reply", parent.json()["id"])
    assert reply.status_code == 201, f"Expected 201, got {reply.status_code}: {reply.text}"
    assert reply.json()["parent_id"] == parent.json()["id"]


def test_checkin_and_review_comments_on_real_records(api_url, admin_headers):
    key = f"CK{uuid.uuid4().hex[:6]}".upper()
    p = _create(api_url, admin_headers, "/programs", {"key": key, "title": f"cv program {key}"})
    o = _create(api_url, admin_headers, "/objectives",
                {"program_id": p["id"], "title": f"cv objective {key}"})
    ch = _create(api_url, admin_headers, f"/objectives/{o['id']}/checkins",
                 {"value_numeric": 1.0, "message": "cv", "created_by": ADMIN_EMAIL})
    r = _post(api_url, admin_headers, "checkin", str(ch["id"]))
    assert r.status_code == 201, r.text
    assert r.json()["entity_id"] == str(ch["id"])

    doc_id = f"cv-doc-{uuid.uuid4().hex[:8]}"
    _create(api_url, admin_headers, "/documents", {
        "folder": "iso27001", "filename": f"{doc_id}.md", "document_id": doc_id,
        "title": "cv doc", "content": "# cv\n\nbody",
    })
    rv = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin_headers,
                       json={"reviewers": [ADMIN_EMAIL], "message": "cv"})
    if rv.status_code in (200, 201) and rv.json().get("review_id"):
        rid = str(rv.json()["review_id"])
        r = _post(api_url, admin_headers, "review", rid)
        assert r.status_code == 201, r.text
        assert r.json()["entity_id"] == rid


def test_controls_risk_contributor_and_reply_rules(api_url, admin_headers, contributor_headers):
    risk = _create(api_url, admin_headers, "/risks",
                   {"title": "cv risk", "likelihood": 2, "impact": 2})
    ident = risk.get("identifier") or str(risk.get("id"))
    other = _create(api_url, admin_headers, "/risks",
                    {"title": "cv other risk", "likelihood": 2, "impact": 2})
    other_ident = other.get("identifier") or str(other.get("id"))

    r = _post(api_url, admin_headers, "risk", ident)
    assert r.status_code == 201, r.text
    assert r.json()["entity_id"] == ident

    parent = _post(api_url, contributor_headers, "risk", ident, "contributor")
    assert parent.status_code == 201, parent.text

    r = _post(api_url, contributor_headers, "risk", other_ident, "reply elsewhere", parent.json()["id"])
    assert r.status_code == 400, r.text
    r = _post(api_url, contributor_headers, "risk", ident, "reply to nothing", 999999)
    assert r.status_code == 404, r.text


def test_access_review_comment_on_a_real_record(api_url, admin_headers):
    system = _create(api_url, admin_headers, "/systems", {
        "name": f"cv system {uuid.uuid4().hex[:6]}", "classification": "restricted",
        "criticality": "critical", "rpo_hours": 0, "rto_hours": 1,
    })
    ar = _create(api_url, admin_headers, f"/systems/{system['id']}/access-reviews",
                 {"notes": "cv access review"})
    r = _post(api_url, admin_headers, "access_review", str(ar["id"]))
    assert r.status_code == 201, r.text
    assert r.json()["entity_id"] == str(ar["id"])
