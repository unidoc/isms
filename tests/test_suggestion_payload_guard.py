"""#298 / #200: a suggestion is never marked applied with its proposal dropped."""
import uuid

import requests


def _uid():
    return uuid.uuid4().hex[:8]


def _incident(api_url, headers):
    r = requests.post(f"{api_url}/incidents", headers=headers,
                      json={"title": f"guard-{_uid()}", "severity": "low", "description": "d"})
    assert r.status_code in (200, 201), r.text
    return r.json()


def _suggest(api_url, headers, **body):
    body.setdefault("title", f"guard suggestion {_uid()}")
    return requests.post(f"{api_url}/suggestions", headers=headers, json=body)


def _apply(api_url, headers, sid):
    return requests.post(f"{api_url}/suggestions/{sid}/apply", headers=headers, json={"force": True})


def _status(api_url, headers, sid):
    r = requests.get(f"{api_url}/suggestions/{sid}", headers=headers)
    assert r.status_code == 200, r.text
    return r.json()["data"]["status"]


def test_298_top_level_values_rejected_at_create(api_url, admin_headers):
    inc = _incident(api_url, admin_headers)
    r = _suggest(api_url, admin_headers, entity_type="incident", entity_id=inc["identifier"],
                 suggestion_type="update", payload={"status": "resolved"})
    assert r.status_code == 400, r.text
    assert "fields" in r.text


def test_free_text_update_is_created_but_not_applied(api_url, admin_headers):
    inc = _incident(api_url, admin_headers)
    r = _suggest(api_url, admin_headers, entity_type="incident", entity_id=inc["identifier"],
                 suggestion_type="update", rationale="please look at this", payload={})
    assert r.status_code in (200, 201), r.text
    sg = r.json()
    got = requests.get(f"{api_url}/suggestions/{sg['id']}", headers=admin_headers).json()["data"]
    payload = got.get("payload") or {}
    assert "title" not in payload and "description" not in payload, (
        f"title/rationale must not be copied into an update payload: {payload}")
    ap = _apply(api_url, admin_headers, sg["id"])
    assert ap.status_code == 400, ap.text
    assert "nothing to apply" in ap.text
    assert _status(api_url, admin_headers, sg["id"]) == "open"


def test_unknown_field_rejected_with_supported_list(api_url, admin_headers):
    inc = _incident(api_url, admin_headers)
    r = _suggest(api_url, admin_headers, entity_type="incident", entity_id=inc["identifier"],
                 suggestion_type="update", payload={"fields": {"title": "renamed"}})
    assert r.status_code == 400, r.text
    assert "title" in r.text and "supported" in r.text


def test_wrong_type_rejected(api_url, admin_headers):
    inc = _incident(api_url, admin_headers)
    r = _suggest(api_url, admin_headers, entity_type="incident", entity_id=inc["identifier"],
                 suggestion_type="update", payload={"fields": {"affects_c": "yes"}})
    assert r.status_code == 400, r.text
    assert "affects_c" in r.text


def test_valid_update_applies_and_changes_entity(api_url, admin_headers):
    inc = _incident(api_url, admin_headers)
    value = f"rc-{_uid()}"
    r = _suggest(api_url, admin_headers, entity_type="incident", entity_id=inc["identifier"],
                 suggestion_type="update", payload={"fields": {"root_cause": value}})
    assert r.status_code in (200, 201), r.text
    ap = _apply(api_url, admin_headers, r.json()["id"])
    assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
    got = requests.get(f"{api_url}/incidents/{inc['id']}", headers=admin_headers).json()
    assert got.get("root_cause") == value


def test_edit_cannot_introduce_bad_payload(api_url, admin_headers):
    inc = _incident(api_url, admin_headers)
    r = _suggest(api_url, admin_headers, entity_type="incident", entity_id=inc["identifier"],
                 suggestion_type="update", payload={"fields": {"root_cause": "x"}})
    assert r.status_code in (200, 201), r.text
    e = requests.put(f"{api_url}/suggestions/{r.json()['id']}", headers=admin_headers,
                     json={"payload": {"status": "resolved"}})
    assert e.status_code == 400, e.text


def test_200_create_unknown_key_rejected(api_url, admin_headers):
    r = _suggest(api_url, admin_headers, entity_type="objective", suggestion_type="create",
                 payload={"title": f"MTTP {_uid()}", "target_value": 14, "target_operator": "lte"})
    assert r.status_code == 400, r.text
    assert "target_operator" in r.text


def test_supplier_create_with_rationale_still_applies(api_url, admin_headers):
    name = f"guard-sup-{_uid()}"
    r = _suggest(api_url, admin_headers, entity_type="supplier", suggestion_type="create",
                 title=name, rationale="needed for payroll", payload={"name": name})
    assert r.status_code in (200, 201), r.text
    ap = _apply(api_url, admin_headers, r.json()["id"])
    assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
