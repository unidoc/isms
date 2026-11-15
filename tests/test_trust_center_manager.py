"""Trust center manager API (#520), end to end over HTTP.

Everything runs in a DEDICATED organization created here: the shared conftest
organization must keep its trust center off, because
tests/test_trust_center_public.py asserts exactly that.
"""
import uuid

import pytest
import requests

PASSWORD = "TestPass123!"


def _tag():
    return uuid.uuid4().hex[:8]


def _signup_login(api_url, email, org=None):
    r = requests.post(f"{api_url}/auth/signup", json={"email": email, "password": PASSWORD, "name": email.split("@")[0]})
    assert r.status_code in (200, 201), r.text
    body = {"email": email, "password": PASSWORD}
    if org:
        body["organization"] = org
    r = requests.post(f"{api_url}/auth/login", json=body)
    assert r.status_code == 200, r.text
    return {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}


@pytest.fixture(scope="module")
def tc(api_url):
    """A fresh organization with an admin, a manager, a contributor and a reader."""
    t = _tag()
    slug = f"tcm-{t}"
    emails = {role: f"tcm-{role}-{t}@isms-test.local" for role in ("admin", "manager", "contributor", "reader")}
    admin = _signup_login(api_url, emails["admin"])
    r = requests.post(f"{api_url}/organizations", headers=admin, json={"name": f"TC Manager {t}", "slug": slug})
    assert r.status_code in (200, 201), r.text
    admin = _signup_login_existing(api_url, emails["admin"], slug)
    heads = {"admin": admin}
    for role in ("manager", "contributor", "reader"):
        _signup_login(api_url, emails[role])
        r = requests.post(f"{api_url}/users", headers=admin, json={"email": emails[role], "name": role, "role": role})
        assert r.status_code in (200, 201), r.text
        heads[role] = _signup_login_existing(api_url, emails[role], slug)
    return {"slug": slug, "heads": heads, "emails": emails}


def _signup_login_existing(api_url, email, org):
    r = requests.post(f"{api_url}/auth/login", json={"email": email, "password": PASSWORD, "organization": org})
    assert r.status_code == 200, r.text
    return {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}


def _approved_doc(api_url, tc, title):
    admin = tc["heads"]["admin"]
    doc_id = f"tcm-doc-{_tag()}"
    r = requests.post(f"{api_url}/documents", headers=admin, json={
        "folder": "tcm", "filename": f"{doc_id}.md", "document_id": doc_id, "title": title,
        "content": f"# {title}\n\nApproved text.\n",
    })
    assert r.status_code == 201, r.text
    r = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin, json={
        "reviewers": [tc["emails"]["contributor"]], "message": "trust center manager e2e",
    })
    assert r.status_code == 201, r.text
    rid = r.json()["review_id"]
    r = requests.post(f"{api_url}/reviews/{rid}/approve", headers=tc["heads"]["contributor"], json={"decision": "approved"})
    assert r.status_code == 200, r.text
    r = requests.post(f"{api_url}/reviews/{rid}/merge", headers=admin, json={})
    assert r.status_code == 200, r.text
    return doc_id


def test_settings_defaults_then_enable(api_url, tc):
    mgr = tc["heads"]["manager"]
    r = requests.get(f"{api_url}/trust-center/settings", headers=mgr)
    assert r.status_code == 200, r.text
    s = r.json()
    assert s["enabled"] is False and s["default_access_level"] == "gated"
    assert s["updated_at"] is None and s["updated_by"] is None and s["overview_document_id"] is None
    # Switched off: visitors get the one 404.
    assert requests.get(f"{api_url}/trust/{tc['slug']}").status_code == 404

    r = requests.put(f"{api_url}/trust-center/settings", headers=mgr,
                     json={"enabled": True, "title": "Acme Trust", "default_access_level": "public"})
    assert r.status_code == 200, r.text
    s = r.json()
    assert s["enabled"] is True and s["title"] == "Acme Trust" and s["default_access_level"] == "public"
    assert s["updated_at"] and s["updated_by"] == tc["emails"]["manager"]
    # A partial update leaves the other keys alone.
    r = requests.put(f"{api_url}/trust-center/settings", headers=mgr, json={"security_contact": "sec@acme.test"})
    assert r.status_code == 200 and r.json()["title"] == "Acme Trust" and r.json()["enabled"] is True
    assert requests.get(f"{api_url}/trust/{tc['slug']}").status_code == 200

    for body in ({"default_access_level": "secret"}, {"title": "x" * 201}):
        assert requests.put(f"{api_url}/trust-center/settings", headers=mgr, json=body).status_code == 400


def test_overview_document(api_url, tc):
    admin = tc["heads"]["admin"]
    url = f"{api_url}/trust-center/settings"
    r = requests.put(url, headers=admin, json={"overview_document_id": "no-such-document"})
    assert r.status_code == 404, r.text
    doc_id = _approved_doc(api_url, tc, "Overview Doc")
    r = requests.put(url, headers=admin, json={"overview_document_id": doc_id.upper()})
    assert r.status_code == 200 and r.json()["overview_document_id"] == doc_id, r.text
    r = requests.put(url, headers=admin, json={"overview_document_id": None})
    assert r.status_code == 200 and r.json()["overview_document_id"] is None, r.text


def test_publish_reorder_and_public_order(api_url, tc):
    mgr = tc["heads"]["manager"]
    requests.put(f"{api_url}/trust-center/settings", headers=mgr, json={"enabled": True})
    docs = [_approved_doc(api_url, tc, t) for t in ("Alpha Policy", "Bravo Policy")]
    ids = []
    for d in docs:
        r = requests.post(f"{api_url}/documents/{d}/trust-center", headers=mgr, json={"access_level": "public"})
        assert r.status_code == 201, r.text
        ids.append(r.json()["id"])

    r = requests.get(f"{api_url}/trust-center/items", headers=mgr)
    assert r.status_code == 200, r.text
    items = r.json()
    assert [i["id"] for i in items] == ids, "new items append in publish order"
    assert [i["sort_order"] for i in items] == [1, 2]
    assert all(i["kind"] == "document" and i["document"]["status"] == "published" for i in items)
    assert items[0]["document"]["title"] == "Alpha Policy"

    pub = requests.get(f"{api_url}/trust/{tc['slug']}/items")
    assert pub.status_code == 200, pub.text
    assert [i["id"] for i in pub.json()] == ids

    r = requests.put(f"{api_url}/trust-center/items/order", headers=mgr,
                     json={"kind": "document", "ids": list(reversed(ids))})
    assert r.status_code == 200, r.text
    assert [i["id"] for i in r.json()] == list(reversed(ids))
    pub = requests.get(f"{api_url}/trust/{tc['slug']}/items")
    assert [i["id"] for i in pub.json()] == list(reversed(ids))

    # A list that is not the current set is refused and changes nothing.
    r = requests.put(f"{api_url}/trust-center/items/order", headers=mgr, json={"kind": "document", "ids": ids[:1]})
    assert r.status_code == 409 and r.json()["code"] == "trust_order_stale", r.text
    r = requests.put(f"{api_url}/trust-center/items/order", headers=mgr, json={"kind": "nope", "ids": []})
    assert r.status_code == 400, r.text
    assert [i["id"] for i in requests.get(f"{api_url}/trust/{tc['slug']}/items").json()] == list(reversed(ids))


def test_roles_and_anonymous(api_url, tc):
    calls = [
        ("GET", "/trust-center/settings", None),
        ("PUT", "/trust-center/settings", {"enabled": True}),
        ("GET", "/trust-center/items", None),
        ("PUT", "/trust-center/items/order", {"kind": "document", "ids": []}),
    ]
    for method, path, body in calls:
        for role in ("reader", "contributor"):
            r = requests.request(method, f"{api_url}{path}", headers=tc["heads"][role], json=body)
            assert r.status_code == 403, f"{role} {method} {path}: {r.status_code} {r.text}"
        r = requests.request(method, f"{api_url}{path}", json=body)
        assert r.status_code == 401, f"anonymous {method} {path}: {r.status_code} {r.text}"


def _classified_doc(api_url, tc, title, classification="Confidential"):
    admin = tc["heads"]["admin"]
    doc_id = f"tcm-doc-{_tag()}"
    r = requests.post(f"{api_url}/documents", headers=admin, json={
        "folder": "tcm", "filename": f"{doc_id}.md", "document_id": doc_id, "title": title,
        "content": f"# {title}\n\nApproved text.\n",
    })
    assert r.status_code == 201, r.text
    r = requests.put(f"{api_url}/documents/{doc_id}/metadata", headers=admin,
                     json={"fields": {"classification": classification}})
    assert r.status_code == 200, r.text
    r = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin, json={
        "reviewers": [tc["emails"]["contributor"]], "message": "classified e2e",
    })
    assert r.status_code == 201, r.text
    rid = r.json()["review_id"]
    assert requests.post(f"{api_url}/reviews/{rid}/approve", headers=tc["heads"]["contributor"],
                         json={"decision": "approved"}).status_code == 200
    assert requests.post(f"{api_url}/reviews/{rid}/merge", headers=admin, json={}).status_code == 200
    return doc_id


def test_confidential_document_needs_confirmation_to_go_public(api_url, tc):
    mgr = tc["heads"]["manager"]
    doc_id = _classified_doc(api_url, tc, "Secret Policy")
    url = f"{api_url}/documents/{doc_id}/trust-center"
    r = requests.post(url, headers=mgr, json={"access_level": "public"})
    assert r.status_code == 409 and r.json()["code"] == "trust_classification_confirm", r.text
    assert requests.get(url, headers=mgr).json()["published"] is False

    r = requests.post(url, headers=mgr, json={"access_level": "gated"})
    assert r.status_code == 201, r.text
    item_id = r.json()["id"]

    patch = f"{api_url}/trust-center/items/{item_id}"
    r = requests.patch(patch, headers=mgr, json={"access_level": "public"})
    assert r.status_code == 409 and r.json()["code"] == "trust_classification_confirm", r.text
    assert requests.get(url, headers=mgr).json()["access_level"] == "gated"
    r = requests.patch(patch, headers=mgr, json={"access_level": "public", "confirm_classification": True})
    assert r.status_code == 200, r.text
    row = r.json()
    assert row["id"] == item_id and row["access_level"] == "public"
    assert row["document"]["classification"] == "Confidential" and row["document"]["status"] == "published"

    assert requests.patch(f"{api_url}/trust-center/items/{uuid.uuid4()}", headers=mgr,
                          json={"access_level": "gated"}).status_code == 404
    assert requests.patch(patch, headers=mgr, json={"access_level": "bogus"}).status_code == 400
    for role in ("reader", "contributor"):
        assert requests.patch(patch, headers=tc["heads"][role], json={"access_level": "gated"}).status_code == 403
    assert requests.patch(patch, json={"access_level": "gated"}).status_code == 401
