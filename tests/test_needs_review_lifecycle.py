"""Regression for #19: "Needs Review keeps listing documents that are already
approved."

Empirically (against the live stack) the core of this is already handled by the
#3 fix (merge_commit baseline + body comparison): an approved, unchanged
document does not appear, and re-approving a changed document clears it again.
The #3 test only covered a single approval + a metadata-vs-body edit; it never
exercised the *re-approval cycle* that #19 describes ("keeps listing"). These
tests lock in the full lifecycle so the "keeps listing" regression can't return.

Conclusion captured here: an approved document only appears in needs-review when
its reviewed body has genuinely drifted from the approved baseline, and going
through review again always clears it. A never-reviewed document is surfaced
(correct — it has no governance baseline) and clears once it is actually
reviewed in-system.
"""
import uuid

import requests
from conftest import READER_EMAIL

BODY = "# Control\n\nApproved, reviewed body.\n"
BODY2 = BODY + "\nA genuine second-round content change.\n"


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


def _listed(api_url, admin_headers, doc_id):
    r = requests.get(f"{api_url}/documents/needs-review", headers=admin_headers)
    assert r.status_code == 200, r.text
    return any(d["document_id"] == doc_id for d in r.json()["data"])


def _create(api_url, admin_headers, doc_id):
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": "iso27001", "filename": doc_id + ".md",
        "document_id": doc_id, "title": "NR lifecycle", "content": BODY})
    assert r.status_code in (200, 201, 409), r.text
    requests.put(f"{api_url}/documents/{doc_id}/content", headers=admin_headers, json={"content": BODY})


class TestNeedsReviewLifecycle:
    def test_approved_doc_does_not_keep_listing_across_reapproval(
            self, api_url, admin_headers, reader_headers):
        # Unique id per run — the test stack persists state, and these assertions
        # depend on the document's review history, so each run needs a fresh doc.
        doc = "nr-cycle-" + uuid.uuid4().hex[:8]
        _create(api_url, admin_headers, doc)

        # First approval → cleared.
        _approve_and_merge(api_url, admin_headers, reader_headers, doc)
        assert not _listed(api_url, admin_headers, doc), \
            "freshly approved document must not be listed"

        # Genuine body change → correctly flagged.
        requests.put(f"{api_url}/documents/{doc}/content", headers=admin_headers,
                     json={"content": BODY2})
        assert _listed(api_url, admin_headers, doc), \
            "a real body change must flag needs-review"

        # Re-approval must clear it again — this is the #19 "keeps listing" guard.
        _approve_and_merge(api_url, admin_headers, reader_headers, doc)
        assert not _listed(api_url, admin_headers, doc), \
            "#19: re-approved document must NOT keep listing in needs-review"

    def test_never_reviewed_doc_is_surfaced_then_clears_after_review(
            self, api_url, admin_headers, reader_headers):
        # Unique id per run — on the persistent stack a fixed id would already be
        # approved from a prior run, so "never reviewed" would no longer hold.
        doc = "nr-never-" + uuid.uuid4().hex[:8]
        _create(api_url, admin_headers, doc)

        # A document with no in-system review baseline is correctly surfaced —
        # frontmatter status alone is not a governance trail.
        assert _listed(api_url, admin_headers, doc), \
            "a never-reviewed document should be surfaced for review"

        # Reviewing it once clears it — so an imported repo settles after review,
        # it does not keep listing forever.
        _approve_and_merge(api_url, admin_headers, reader_headers, doc)
        assert not _listed(api_url, admin_headers, doc), \
            "once actually reviewed, the document must clear from needs-review"
