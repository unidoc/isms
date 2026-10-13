"""Regression for #439: a document's History tab was always empty.

GET /changelog/document/<document_id> answered 400 invalid_id because the route
only understood numeric ids. A document's history now comes from git (every
content, metadata and status change is a commit, including pushed ones), shaped
like the other entity types' changelog rows.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL, CONTRIBUTOR_EMAIL

BODY = "# Heading\n\nBody content for the document-history test.\n"
OWNER = "history-owner@isms-test.local"


def _create_doc(api_url, admin_headers):
    doc_id = f"history-test-{uuid.uuid4().hex[:8]}"
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": "iso27001", "filename": doc_id + ".md",
        "document_id": doc_id, "title": "History", "content": BODY,
    })
    assert r.status_code in (200, 201), r.text
    return doc_id


class TestDocumentHistory:
    def test_edits_listed_with_who_and_what(self, api_url, admin_headers):
        doc_id = _create_doc(api_url, admin_headers)
        r = requests.put(f"{api_url}/documents/{doc_id}/metadata",
                         headers=admin_headers, json={"fields": {"owner": OWNER}})
        assert r.status_code == 200, r.text
        r = requests.put(f"{api_url}/documents/{doc_id}/content",
                         headers=admin_headers, json={"content": BODY + "\nMore.\n"})
        assert r.status_code == 200, r.text

        r = requests.get(f"{api_url}/changelog/document/{doc_id}", headers=admin_headers)
        assert r.status_code == 200, r.text
        rows = r.json()["data"]
        by = {(x["action"], x.get("field", "")): x for x in rows}
        assert ("update", "owner") in by, rows
        assert by[("update", "owner")]["new_value"] == OWNER
        assert ("update", "content") in by, rows
        assert ("create", "") in by, rows
        assert all(x["changed_by"] == ADMIN_EMAIL for x in rows), rows
        assert all(x["created_at"] for x in rows), rows
        # Newest first: the create row is last.
        assert rows[-1]["action"] == "create", rows

    def test_unknown_document_is_404(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/changelog/document/no-such-doc-{uuid.uuid4().hex[:8]}",
                         headers=admin_headers)
        assert r.status_code == 404, r.text

    def test_reader_can_read_history(self, api_url, admin_headers, reader_headers):
        doc_id = _create_doc(api_url, admin_headers)
        r = requests.get(f"{api_url}/changelog/document/{doc_id}", headers=reader_headers)
        assert r.status_code == 200, r.text
        assert any(x["action"] == "create" for x in r.json()["data"])

    def test_risk_changelog_unchanged(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers,
                          json={"title": "history control risk", "likelihood": 2, "impact": 2})
        assert r.status_code in (200, 201), r.text
        risk_id = r.json()["id"]
        r = requests.get(f"{api_url}/changelog/risk/{risk_id}", headers=admin_headers)
        assert r.status_code == 200, r.text
        assert any(x["action"] == "create" for x in r.json()["data"]), r.text


class TestDocumentHistoryDecisions:
    def test_review_decisions_listed_with_git_rows(self, api_url, admin_headers, contributor_headers):
        doc_id = _create_doc(api_url, admin_headers)
        r = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin_headers,
                          json={"reviewers": [CONTRIBUTOR_EMAIL], "message": "please review"})
        assert r.status_code in (200, 201), r.text
        rid = r.json()["review_id"]
        r = requests.post(f"{api_url}/reviews/{rid}/approve", headers=contributor_headers,
                          json={"decision": "approved", "comment": "lgtm"})
        assert r.status_code == 200, r.text
        r = requests.post(f"{api_url}/reviews/{rid}/merge", headers=admin_headers, json={})
        assert r.status_code == 200, r.text
        r = requests.post(f"{api_url}/documents/{doc_id}/confirm-review", headers=admin_headers,
                          json={"comment": "annual check"})
        assert r.status_code == 200, r.text

        r = requests.get(f"{api_url}/changelog/document/{doc_id}", headers=admin_headers)
        assert r.status_code == 200, r.text
        rows = r.json()["data"]
        by = {x["action"]: x for x in rows if x["action"] in ("approved", "merged", "confirmed")}
        assert set(by) == {"approved", "merged", "confirmed"}, rows
        assert by["approved"]["changed_by"] == CONTRIBUTOR_EMAIL, rows
        assert by["approved"]["reason"] == "lgtm"
        assert by["merged"]["changed_by"] == ADMIN_EMAIL
        assert by["confirmed"]["changed_by"] == ADMIN_EMAIL
        assert by["confirmed"]["reason"] == "annual check"
        for x in by.values():
            assert x["new_value"], x
        # Git rows from the create and the status changes are still there.
        assert any(x["action"] == "create" for x in rows), rows
        assert any(x["action"] == "update" and x.get("field") == "status" for x in rows), rows
        # Newest first across both sources.
        stamps = [x["created_at"] for x in rows]
        assert stamps == sorted(stamps, reverse=True), stamps
