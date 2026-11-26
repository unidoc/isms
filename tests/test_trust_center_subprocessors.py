"""Trust center subprocessor list (#522), end to end over HTTP.

Runs in a DEDICATED organization created here: the shared conftest organization
must keep its trust center off (see tests/test_trust_center_public.py).
"""
import uuid

import pytest
import requests

PASSWORD = "TestPass123!"


def _tag():
    return uuid.uuid4().hex[:8]


def _login(api_url, email, org=None):
    body = {"email": email, "password": PASSWORD}
    if org:
        body["organization"] = org
    r = requests.post(f"{api_url}/auth/login", json=body)
    assert r.status_code == 200, r.text
    return {"Authorization": f"Bearer {r.json()['token']}"}


def _json(h):
    return {**h, "Content-Type": "application/json"}


@pytest.fixture(scope="module")
def sp(api_url):
    t = _tag()
    slug = f"tsp-{t}"
    admin_email = f"tsp-admin-{t}@isms-test.local"
    r = requests.post(f"{api_url}/auth/signup", json={"email": admin_email, "password": PASSWORD, "name": "admin"})
    assert r.status_code in (200, 201), r.text
    admin = _login(api_url, admin_email)
    r = requests.post(f"{api_url}/organizations", headers=_json(admin), json={"name": f"TC Subproc {t}", "slug": slug})
    assert r.status_code in (200, 201), r.text
    admin = _login(api_url, admin_email, slug)
    r = requests.put(f"{api_url}/trust-center/settings", headers=_json(admin),
                     json={"enabled": True, "title": "Subprocessors", "security_contact": "sec@example.com"})
    assert r.status_code == 200, r.text
    return {"slug": slug, "h": admin}


def _supplier(api_url, sp, **extra):
    body = {"name": f"Sub {_tag()}", "supplier_type": "cloud", "criticality": "low", "data_access": True,
            "notes": "SECRET-NOTES-pytest", "contract_ref": "CONTRACT-pytest", **extra}
    r = requests.post(f"{api_url}/suppliers", headers=_json(sp["h"]), json=body)
    assert r.status_code == 201, r.text
    return r.json()


def _publish(api_url, sp, supplier_id, **extra):
    return requests.post(f"{api_url}/trust-center/subprocessors", headers=_json(sp["h"]),
                         json={"supplier_id": supplier_id, **extra})


def _public(api_url, sp):
    r = requests.get(f"{api_url}/trust/{sp['slug']}/items")
    assert r.status_code == 200, r.text
    return r.text, [i for i in r.json() if i["kind"] == "subprocessor"]


def _names(api_url, sp):
    return [i["name"] for i in _public(api_url, sp)[1]]


def test_publish_public_row_keys_and_no_leak(api_url, sp):
    s = _supplier(api_url, sp, public_name="Legal Entity GmbH", public_purpose="Cloud hosting",
                  processing_location="EU (Frankfurt)", data_categories="Name, email address")
    r = _publish(api_url, sp, s["id"])
    assert r.status_code == 201, r.text
    entry = r.json()
    assert entry["eligible"] is True and entry["item"]["hidden_reason"] == ""
    body, rows = _public(api_url, sp)
    row = next(i for i in rows if i["id"] == entry["item"]["id"])
    assert sorted(row) == ["access_level", "data_categories", "id", "kind", "location", "name", "purpose", "section", "sort_order"]
    assert row["access_level"] == "public" and row["name"] == "Legal Entity GmbH"
    assert row["purpose"] == "Cloud hosting" and row["location"] == "EU (Frankfurt)"
    assert row["data_categories"] == "Name, email address"
    for leaked in ("SECRET-NOTES-pytest", "CONTRACT-pytest", s["identifier"], "supplier_id"):
        assert leaked not in body
    ov = requests.get(f"{api_url}/trust/{sp['slug']}").json()
    assert ov["subprocessors_changed_at"] > 0


def test_refusals(api_url, sp):
    term = _supplier(api_url, sp, status="terminated")
    r = _publish(api_url, sp, term["id"])
    assert r.status_code == 409 and r.json()["code"] == "trust_subprocessor_ineligible"
    assert r.json()["params"]["value"] == "terminated"
    noaccess = _supplier(api_url, sp, data_access=False)
    r = _publish(api_url, sp, noaccess["id"])
    assert r.status_code == 409 and r.json()["params"]["value"] == "no_data_access"
    ok = _supplier(api_url, sp)
    assert _publish(api_url, sp, ok["id"], access_level="gated").status_code == 400
    assert _publish(api_url, sp, ok["id"], access_level="public").status_code == 201
    r = _publish(api_url, sp, ok["id"])
    assert r.status_code == 409 and r.json()["code"] == "trust_subprocessor_already_published"
    assert _publish(api_url, sp, 99999999).status_code == 404
    item = requests.get(f"{api_url}/trust-center/subprocessors", headers=sp["h"]).json()
    pub_id = next(x for x in item["suppliers"] if x["supplier_id"] == ok["id"])["item"]["id"]
    r = requests.patch(f"{api_url}/trust-center/items/{pub_id}", headers=_json(sp["h"]), json={"access_level": "gated"})
    assert r.status_code == 400 and r.json()["code"] == "invalid_request"
    r = requests.patch(f"{api_url}/trust-center/items/{pub_id}", headers=_json(sp["h"]), json={"access_level": "public"})
    assert r.status_code == 200


def test_terminate_hides_restore_stays_hidden_republish_same_id(api_url, sp):
    s = _supplier(api_url, sp)
    entry = _publish(api_url, sp, s["id"]).json()
    pub_id = entry["item"]["id"]
    assert s["name"] in _names(api_url, sp)

    r = requests.put(f"{api_url}/suppliers/{s['id']}", headers=_json(sp["h"]), json={"status": "terminated"})
    assert r.status_code == 200, r.text
    assert s["name"] not in _names(api_url, sp)
    row = next(x for x in requests.get(f"{api_url}/trust-center/subprocessors", headers=sp["h"]).json()["suppliers"]
               if x["supplier_id"] == s["id"])
    assert row["item"]["hidden_reason"] == "terminated" and row["ineligible_reason"] == "terminated"

    r = requests.put(f"{api_url}/suppliers/{s['id']}", headers=_json(sp["h"]), json={"status": "active"})
    assert r.status_code == 200, r.text
    assert s["name"] not in _names(api_url, sp)  # a restore does not republish

    r = _publish(api_url, sp, s["id"])
    assert r.status_code == 201 and r.json()["item"]["id"] == pub_id
    assert s["name"] in _names(api_url, sp)

    # Data access off hides with its own reason; delete too.
    requests.put(f"{api_url}/suppliers/{s['id']}", headers=_json(sp["h"]), json={"data_access": False})
    assert s["name"] not in _names(api_url, sp)
    requests.put(f"{api_url}/suppliers/{s['id']}", headers=_json(sp["h"]), json={"data_access": True})
    assert _publish(api_url, sp, s["id"]).status_code == 201
    assert requests.delete(f"{api_url}/suppliers/{s['id']}", headers=sp["h"]).status_code == 200
    assert s["name"] not in _names(api_url, sp)
    row = next(x for x in requests.get(f"{api_url}/trust-center/subprocessors", headers=sp["h"]).json()["suppliers"]
               if x["supplier_id"] == s["id"])
    assert row["deleted"] is True and row["item"]["hidden_reason"] == "deleted"


def test_unpublish(api_url, sp):
    s = _supplier(api_url, sp)
    assert _publish(api_url, sp, s["id"]).status_code == 201
    assert requests.delete(f"{api_url}/trust-center/subprocessors/{s['id']}", headers=sp["h"]).status_code == 204
    assert s["name"] not in _names(api_url, sp)
    assert requests.delete(f"{api_url}/trust-center/subprocessors/{s['id']}", headers=sp["h"]).status_code == 404
