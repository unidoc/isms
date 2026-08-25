"""Regression: the version-downgrade guard on PUT /documents/:docId/content must
compare a requested version against the last APPROVED milestone
(document_versions, written only on merge/confirm), not against the document's
current draft frontmatter value.

A draft's version field is working state until it is re-approved — nothing
depends on it staying monotonic against its own prior unapproved saves. Before
this fix, comparing against the current draft value locked in whatever was
last saved (including a typo) as a new floor, so a user correcting an
over-bumped draft version (e.g. 0.3 -> 0.2, still well above the 0.1 that was
actually approved) was permanently blocked.
"""
import uuid
import requests
from conftest import READER_EMAIL

BODY = "# Heading\n\nBody content for the version-downgrade-guard regression test.\n"


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


def _current_version(api_url, admin_headers, doc_id):
    # PUT .../content returns only {"commit", "status"} — the applied version
    # has to be read back separately.
    r = requests.get(f"{api_url}/documents/{doc_id}/body", headers=admin_headers)
    assert r.status_code == 200, f"reading back version: {r.text}"
    return r.json()["version"]


class TestVersionDowngradeGuardComparesAgainstApprovedFloor:
    def test_draft_version_may_correct_downward_above_the_approved_floor(self, api_url, admin_headers, reader_headers):
        # Unique per run — the assertions below pin absolute version numbers
        # ("0.2", "0.3"), which only hold for a document that has never been
        # approved before this test creates it. A fixed id would keep
        # climbing on a persistent test stack across repeated runs.
        DOC = f"version-guard-test-{uuid.uuid4().hex[:8]}"

        # 1. Create a document, pin its starting version, approve + merge.
        #    No milestone exists yet at this point, so the floor check is
        #    inert here regardless of the fix.
        r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
            "folder": "iso27001", "filename": DOC + ".md",
            "document_id": DOC, "title": "Version Guard Test", "content": BODY,
        })
        assert r.status_code in (200, 201), r.text
        r = requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers,
                         json={"content": BODY, "version": "0.1"})
        assert r.status_code == 200, f"pin initial version: {r.text}"

        _approve_and_merge(api_url, admin_headers, reader_headers, DOC)
        # The approved milestone is now 0.1 — the floor every later check in
        # this test is measured against.

        # 2. Edit the approved doc with no explicit version: auto-bumps to
        #    0.2 and drops back to draft, exactly like a normal edit in the UI.
        r = requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers,
                         json={"content": BODY + "\nAn edit.\n"})
        assert r.status_code == 200, f"auto-bump edit: {r.text}"
        assert _current_version(api_url, admin_headers, DOC) == "0.2", \
            f"expected auto-bump to 0.2, got {_current_version(api_url, admin_headers, DOC)}"

        # 3. User (mis)types 0.3 and saves — still draft, still above the 0.1
        #    floor, so this must succeed both before and after the fix.
        r = requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers,
                         json={"content": BODY + "\nAn edit.\n", "version": "0.3"})
        assert r.status_code == 200, f"manual bump to 0.3: {r.text}"

        # 4. THE FIX: correcting back to 0.2 must now succeed — 0.2 is still
        #    above the 0.1 that was actually approved, even though it is
        #    below the document's current (mistaken) draft value of 0.3.
        r = requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers,
                         json={"content": BODY + "\nAn edit.\n", "version": "0.2"})
        assert r.status_code == 200, \
            f"correcting an over-bumped draft version above the approved floor must be allowed: {r.text}"
        assert _current_version(api_url, admin_headers, DOC) == "0.2"

        # 5. The floor itself still holds: 0.1 (equal to the approved
        #    milestone) and anything below it must still be rejected, with a
        #    message that explains why rather than just restating the numbers.
        r = requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers,
                         json={"content": BODY, "version": "0.1"})
        assert r.status_code == 400, "setting version back to the approved milestone itself must be rejected"
        assert "approved" in r.json()["message"].lower(), \
            f"error should explain the approved-version floor, got: {r.json()}"

        r = requests.put(f"{api_url}/documents/{DOC}/content", headers=admin_headers,
                         json={"content": BODY, "version": "0.0"})
        assert r.status_code == 400, "setting version below the approved milestone must be rejected"
