"""Regression for #430: PUT /documents/:docId/content must not auto-fill the
owner.

`owner` is `omitempty` in the frontmatter, so a cleared owner is
indistinguishable from one that was never set. The save used to refill an empty
owner with the saver's email, which silently undid an explicit "(not set)" on
the very next save. Owner now changes only when someone sets it: the editor's
owner dropdown (`owner` in this request), PUT .../metadata, or POST /documents
(which writes owner = author at creation). The author auto-fill is unchanged.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL

BODY = "# Heading\n\nBody content for the document-owner regression test.\n"
OWNER_X = "owner-x@isms-test.local"
OWNER_Y = "owner-y@isms-test.local"


def _create_doc(api_url, admin_headers):
    doc_id = f"owner-save-test-{uuid.uuid4().hex[:8]}"
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": "iso27001", "filename": doc_id + ".md",
        "document_id": doc_id, "title": "Owner save", "content": BODY,
    })
    assert r.status_code in (200, 201), r.text
    return doc_id


def _put_content(api_url, admin_headers, doc_id, **extra):
    r = requests.put(f"{api_url}/documents/{doc_id}/content", headers=admin_headers,
                     json={"content": BODY, **extra})
    assert r.status_code == 200, f"save content: {r.text}"


def _read(api_url, admin_headers, doc_id):
    r = requests.get(f"{api_url}/documents/{doc_id}/body", headers=admin_headers)
    assert r.status_code == 200, f"read back: {r.text}"
    return r.json()


def _find_untouched_template_doc(api_url, admin_headers):
    """A scaffolded template document: its frontmatter has no owner or author."""
    r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
    assert r.status_code == 200
    seen = set()
    for item in reversed(_flatten(r.json())):
        doc_id = item.get("document_id")
        if not doc_id or doc_id in seen:
            continue
        seen.add(doc_id)
        body = _read(api_url, admin_headers, doc_id)
        if not body["owner"] and not body["author"]:
            return doc_id
    raise AssertionError("no template document without owner and author found")


def _flatten(payload):
    out = []
    def walk(node):
        if isinstance(node, dict):
            if "document_id" in node:
                out.append(node)
            for v in node.values():
                walk(v)
        elif isinstance(node, list):
            for v in node:
                walk(v)
    walk(payload)
    return out


class TestContentSaveDoesNotRefillOwner:
    def test_cleared_owner_stays_cleared_across_later_saves(self, api_url, admin_headers):
        doc_id = _create_doc(api_url, admin_headers)
        _put_content(api_url, admin_headers, doc_id, owner=OWNER_X)
        assert _read(api_url, admin_headers, doc_id)["owner"] == OWNER_X

        # Explicitly clear it ("(not set)" in the editor).
        _put_content(api_url, admin_headers, doc_id, owner="")
        assert _read(api_url, admin_headers, doc_id)["owner"] == ""

        # A later save that leaves owner alone must not refill it (#430).
        _put_content(api_url, admin_headers, doc_id)
        assert _read(api_url, admin_headers, doc_id)["owner"] == "", \
            "a content save refilled a cleared owner"

    def test_owner_can_be_set(self, api_url, admin_headers):
        doc_id = _create_doc(api_url, admin_headers)
        assert _read(api_url, admin_headers, doc_id)["owner"] == ADMIN_EMAIL  # POST /documents: owner = author
        _put_content(api_url, admin_headers, doc_id, owner=OWNER_Y)
        assert _read(api_url, admin_headers, doc_id)["owner"] == OWNER_Y

    def test_existing_owner_kept_when_not_sent(self, api_url, admin_headers):
        doc_id = _create_doc(api_url, admin_headers)
        _put_content(api_url, admin_headers, doc_id, owner=OWNER_X)
        _put_content(api_url, admin_headers, doc_id)
        assert _read(api_url, admin_headers, doc_id)["owner"] == OWNER_X

    def test_ownerless_template_doc_gets_author_but_not_owner(self, api_url, admin_headers):
        doc_id = _find_untouched_template_doc(api_url, admin_headers)
        _put_content(api_url, admin_headers, doc_id)
        doc = _read(api_url, admin_headers, doc_id)
        assert doc["owner"] == "", "first save filled the owner"
        assert doc["author"] == ADMIN_EMAIL, "first save should still fill the author"
