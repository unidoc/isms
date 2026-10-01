"""#381: an explicit null in an update body clears an optional field.

The update requests used **T for "absent, null, value", but encoding/json decodes
null into a **T as an outer nil, the same as an absent key. Every null was
therefore ignored and the handler answered 200 with nothing changed: a system
could not be unlinked from its supplier, a task or corrective action could not
lose its due date, a risk could not lose its likelihood.
"""
import uuid

import requests

from conftest import CONTRIBUTOR_EMAIL


def _create(api_url, headers, path, body):
    r = requests.post(f"{api_url}{path}", headers=headers, json=body)
    assert r.status_code in (200, 201), r.text
    return r.json()


def _put(api_url, headers, path, body):
    r = requests.put(f"{api_url}{path}", headers=headers, json=body)
    assert r.status_code == 200, r.text
    return r.json()


def _get(api_url, headers, path):
    r = requests.get(f"{api_url}{path}", headers=headers)
    assert r.status_code == 200, r.text
    return r.json()


def _name(prefix):
    return f"{prefix}-{uuid.uuid4().hex[:6]}"


def test_null_due_date_clears_corrective_action(api_url, admin_headers):
    ca = _create(api_url, admin_headers, "/corrective-actions", {
        "title": _name("null-ca"), "source": "other", "severity": "observation",
        "due_date": "2031-03-15",
    })
    path = f"/corrective-actions/{ca['id']}"
    assert _get(api_url, admin_headers, path).get("due_date"), "setup: due_date must be set"

    _put(api_url, admin_headers, path, {"due_date": None})

    got = _get(api_url, admin_headers, path)
    assert not got.get("due_date"), f"null must clear due_date, got {got.get('due_date')}"
    assert got["title"] == ca["title"], "clearing one field must leave the others alone"


def test_empty_string_still_clears_corrective_action_due_date(api_url, admin_headers):
    ca = _create(api_url, admin_headers, "/corrective-actions", {
        "title": _name("empty-ca"), "source": "other", "severity": "observation",
        "due_date": "2031-03-15",
    })
    path = f"/corrective-actions/{ca['id']}"

    _put(api_url, admin_headers, path, {"due_date": ""})

    assert not _get(api_url, admin_headers, path).get("due_date")


def test_null_supplier_id_unlinks_system(api_url, admin_headers):
    supplier = _create(api_url, admin_headers, "/suppliers", {
        "name": _name("null-supplier"), "supplier_type": "saas", "criticality": "low",
    })
    system = _create(api_url, admin_headers, "/systems", {
        "name": _name("null-system"), "classification": "internal", "criticality": "low",
        "supplier_id": supplier["id"],
    })
    path = f"/systems/{system['id']}"
    assert _get(api_url, admin_headers, path).get("supplier_id") == supplier["id"], \
        "setup: the system must start linked"

    _put(api_url, admin_headers, path, {"supplier_id": None})

    got = _get(api_url, admin_headers, path)
    assert not got.get("supplier_id"), f"null must unlink the supplier, got {got.get('supplier_id')}"
    assert got["name"] == system["name"]


def test_null_due_date_clears_task(api_url, admin_headers):
    task = _create(api_url, admin_headers, "/tasks", {
        "title": _name("null-task"), "task_type": "general",
        "assignee": CONTRIBUTOR_EMAIL, "due_date": "2031-03-15",
    })
    path = f"/tasks/{task['id']}"
    assert _get(api_url, admin_headers, path).get("due_date"), "setup: due_date must be set"

    _put(api_url, admin_headers, path, {"due_date": None})

    got = _get(api_url, admin_headers, path)
    assert not got.get("due_date"), f"null must clear due_date, got {got.get('due_date')}"
    assert got["title"] == task["title"]


def test_null_current_likelihood_clears_risk(api_url, admin_headers):
    risk = _create(api_url, admin_headers, "/risks", {
        "title": _name("null-risk"), "risk_type": "threat", "origin": "internal",
        "status": "open", "treatment": "mitigate",
        "current_likelihood": 3, "current_impact": 4,
    })
    path = f"/risks/{risk['id']}"
    assert _get(api_url, admin_headers, path).get("current_likelihood") == 3, "setup"

    _put(api_url, admin_headers, path, {"current_likelihood": None})

    got = _get(api_url, admin_headers, path)
    assert got.get("current_likelihood") is None, \
        f"null must clear current_likelihood, got {got.get('current_likelihood')}"
    assert got.get("current_impact") == 4, "clearing one field must leave the others alone"


def test_empty_body_changes_nothing(api_url, admin_headers):
    risk = _create(api_url, admin_headers, "/risks", {
        "title": _name("empty-risk"), "risk_type": "threat", "origin": "internal",
        "status": "open", "treatment": "mitigate",
        "current_likelihood": 2, "current_impact": 5,
    })
    ca = _create(api_url, admin_headers, "/corrective-actions", {
        "title": _name("empty-ca-body"), "source": "other", "severity": "observation",
        "due_date": "2031-03-15",
    })

    _put(api_url, admin_headers, f"/risks/{risk['id']}", {})
    _put(api_url, admin_headers, f"/corrective-actions/{ca['id']}", {})

    got_risk = _get(api_url, admin_headers, f"/risks/{risk['id']}")
    assert got_risk.get("current_likelihood") == 2
    assert got_risk.get("current_impact") == 5
    got_ca = _get(api_url, admin_headers, f"/corrective-actions/{ca['id']}")
    assert got_ca.get("due_date"), "an empty body must not clear the due date"
