"""Publishing documents to the trust center (#518), over HTTP.

A trust center cannot be switched on over HTTP yet (no settings endpoint), so
the conftest organization has none and the public document endpoint can only be
shown to answer the one 404. What the HTTP API does allow is covered end to end
here: the approval flow, the manager endpoints, who may call them, what they
report while a draft is in progress, and retire. What visitors actually receive
(approved text only, no frontmatter identities, link handling, gated items) is
covered by internal/isms/api/trust_documents_pg_test.go.
"""
import uuid

import requests

from conftest import CONTRIBUTOR_EMAIL


def _new_document(api_url, admin_headers, body="# Policy\n\nFirst version.\n"):
    doc_id = f"e2e-trust-{uuid.uuid4().hex[:10]}"
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": "e2e-trust", "filename": f"{doc_id}.md", "document_id": doc_id,
        "title": "E2E Trust Doc", "content": body,
    })
    assert r.status_code == 201, r.text
    return doc_id


def _approve(api_url, admin_headers, contributor_headers, doc_id):
    r = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin_headers, json={
        "reviewers": [CONTRIBUTOR_EMAIL], "message": "trust center e2e",
    })
    assert r.status_code == 201, r.text
    rid = r.json()["review_id"]
    r = requests.post(f"{api_url}/reviews/{rid}/approve", headers=contributor_headers,
                      json={"decision": "approved"})
    assert r.status_code == 200, r.text
    r = requests.post(f"{api_url}/reviews/{rid}/merge", headers=admin_headers, json={})
    assert r.status_code == 200, r.text


def _tc(api_url, doc_id):
    return f"{api_url}/documents/{doc_id}/trust-center"


def test_publish_flow_and_draft_in_progress(api_url, test_setup, admin_headers, contributor_headers):
    doc_id = _new_document(api_url, admin_headers)

    # Never approved: cannot be published, and says why.
    r = requests.get(_tc(api_url, doc_id), headers=admin_headers)
    assert r.status_code == 200, r.text
    st = r.json()
    assert st["published"] is False and st["can_publish"] is False
    assert st["cannot_publish_reason"] == "no_approved_version"
    assert st["served_version"] is None
    r = requests.post(_tc(api_url, doc_id), headers=admin_headers, json={"access_level": "public"})
    assert r.status_code == 409 and r.json()["code"] == "trust_no_approved_version", r.text

    _approve(api_url, admin_headers, contributor_headers, doc_id)
    st = requests.get(_tc(api_url, doc_id), headers=admin_headers).json()
    assert st["can_publish"] is True and st["served_version"]["version"]
    served = st["served_version"]

    r = requests.post(_tc(api_url, doc_id), headers=admin_headers, json={"access_level": "bogus"})
    assert r.status_code == 400, r.text
    r = requests.post(_tc(api_url, doc_id), headers=admin_headers, json={"access_level": "public"})
    assert r.status_code == 201, r.text
    st = r.json()
    assert st["published"] is True and st["access_level"] == "public" and st["id"]
    r = requests.post(_tc(api_url, doc_id), headers=admin_headers, json={})
    assert r.status_code == 409 and r.json()["code"] == "trust_already_published", r.text

    # Edit the approved document into a draft: visitors keep the approved version.
    r = requests.put(f"{api_url}/documents/{doc_id}/content", headers=admin_headers,
                     json={"content": "# Policy\n\nDraft, not approved.\n"})
    assert r.status_code == 200, r.text
    st = requests.get(_tc(api_url, doc_id), headers=admin_headers).json()
    assert st["document_status"] == "draft"
    assert st["served_version"] == served
    assert st["published"] is True

    # Not enabled over HTTP, so visitors get the one 404 whatever the id.
    pub = requests.get(f"{api_url}/trust/test-org/documents/{st['id']}")
    unknown = requests.get(f"{api_url}/trust/test-org/documents/{uuid.uuid4()}")
    assert pub.status_code == 404 and pub.content == unknown.content

    r = requests.delete(_tc(api_url, doc_id), headers=admin_headers)
    assert r.status_code == 204, r.text
    assert requests.delete(_tc(api_url, doc_id), headers=admin_headers).status_code == 404
    assert requests.get(_tc(api_url, doc_id), headers=admin_headers).json()["published"] is False


def test_retire_hides_and_blocks_publishing(api_url, test_setup, admin_headers, contributor_headers):
    doc_id = _new_document(api_url, admin_headers)
    _approve(api_url, admin_headers, contributor_headers, doc_id)
    assert requests.post(_tc(api_url, doc_id), headers=admin_headers,
                         json={"access_level": "public"}).status_code == 201

    r = requests.put(f"{api_url}/documents/{doc_id}/metadata", headers=admin_headers,
                     json={"fields": {"status": "retired"}})
    assert r.status_code == 200, r.text
    st = requests.get(_tc(api_url, doc_id), headers=admin_headers).json()
    assert st["published"] is False and st["hidden_reason"] == "retired"
    assert st["can_publish"] is False and st["cannot_publish_reason"] == "retired"
    r = requests.post(_tc(api_url, doc_id), headers=admin_headers, json={"access_level": "public"})
    assert r.status_code == 409 and r.json()["code"] == "trust_document_retired", r.text

    # Un-retiring does not bring it back; publishing again does.
    requests.put(f"{api_url}/documents/{doc_id}/metadata", headers=admin_headers,
                 json={"fields": {"status": "approved"}})
    assert requests.get(_tc(api_url, doc_id), headers=admin_headers).json()["published"] is False
    r = requests.post(_tc(api_url, doc_id), headers=admin_headers, json={"access_level": "public"})
    assert r.status_code == 200, r.text
    assert r.json()["published"] is True and r.json()["hidden_reason"] == ""


def test_unknown_document_is_404(api_url, test_setup, admin_headers):
    r = requests.get(_tc(api_url, "no-such-document"), headers=admin_headers)
    assert r.status_code == 404, r.text


def test_only_managers_and_admins(api_url, test_setup, admin_headers, reader_headers, contributor_headers):
    doc_id = _new_document(api_url, admin_headers)
    _approve(api_url, admin_headers, contributor_headers, doc_id)
    for who, headers in (("reader", reader_headers), ("contributor", contributor_headers)):
        for method in ("GET", "POST", "DELETE"):
            r = requests.request(method, _tc(api_url, doc_id), headers=headers, json={})
            assert r.status_code == 403, f"{who} {method}: {r.status_code} {r.text}"
    st = requests.get(_tc(api_url, doc_id), headers=admin_headers).json()
    assert st["published"] is False, "a forbidden publish must not create an item"
