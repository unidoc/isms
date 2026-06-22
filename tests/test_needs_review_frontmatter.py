"""Regression for #3: changing a frontmatter metadata field (e.g. owner) must
NOT mark an approved document as "changed since last approval" — only a change
to the reviewed body should. The needs-review check used to compare the git
commit hash, so a metadata-only edit (which still produces a commit) wrongly
flagged the document for re-review.
"""
import requests
from conftest import READER_EMAIL

DOC = "needs-review-fm-test"
BODY = "# Heading\n\nReviewed body content — a metadata edit must not re-flag this.\n"


def _approve_and_merge(api_url, admin_headers, reader_headers, doc_id):
    r = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin_headers,
                      json={"reviewers": [READER_EMAIL], "message": "review"})
    assert r.status_code in (200, 201), f"send for review: {r.text}"
    rid = r.json()["review_id"]
    r = requests.post(f"{api_url}/reviews/{rid}/approve", headers=reader_headers,
                      json={"decision": "approved", "comment": "LGTM"})
    assert r.status_code == 200, f"approve: {r.text}"
    r = requests.post(f"{api_url}/reviews/{rid}/merge", headers=admin_headers, json={})
    assert r.status_code == 200, f"merge: {r.text}"


def _needs_review(api_url, admin_headers, doc_id):
    r = requests.get(f"{api_url}/documents/needs-review", headers=admin_headers)
    assert r.status_code == 200
    return any(d["document_id"] == doc_id for d in r.json()["data"])


class TestNeedsReviewIgnoresMetadata:
    def test_owner_change_does_not_flag_but_body_change_does(self, api_url, admin_headers, reader_headers):
        # 1. Create a document with real body content.
        r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
            "folder": "iso27001", "filename": DOC + ".md",
            "document_id": DOC, "title": "Needs Review FM", "content": BODY,
        })
        assert r.status_code in (200, 201, 409), r.text
        requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers, json={"content": BODY})

        # 2. Approve + merge → there is now an approved baseline.
        _approve_and_merge(api_url, admin_headers, reader_headers, DOC)

        # 3. Freshly approved: must NOT be in needs-review.
        assert not _needs_review(api_url, admin_headers, DOC), "flagged immediately after approval"

        # 4. Change ONLY a frontmatter metadata field (owner).
        r = requests.put(f"{api_url}/documents/{DOC}/metadata", headers=admin_headers,
                         json={"fields": {"owner": "someone-else@test.local"}})
        assert r.status_code == 200, f"owner update: {r.text}"

        # 5. THE FIX (#3): a metadata-only edit must NOT mark it changed-since-approval.
        assert not _needs_review(api_url, admin_headers, DOC), \
            "owner change wrongly flagged the document as needing review (#3)"

        # 6. Sanity: a real body change DOES still flag it — detection isn't broken.
        requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers,
                     json={"content": BODY + "\nA new paragraph — a genuine content change.\n"})
        assert _needs_review(api_url, admin_headers, DOC), \
            "a real body change should flag needs-review"
