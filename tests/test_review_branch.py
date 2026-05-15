"""Review branch editing, merge correctness, and authz tests.

Tests the review branch workflow:
- Edit content on review branch
- Authz: only assigned review participant/requester/manager/admin can edit
- Merge correctness: git first, DB after
- Stale merge detection (409)
- Merge with no branch edits
- Blame at review branch ref
"""
import requests
from conftest import READER_EMAIL, CONTRIBUTOR_EMAIL


class TestReviewBranchEdit:
    """Test editing documents on a review branch."""

    review_id = None

    def test_01_create_review(self, api_url, admin_headers):
        """Create a review to test branch editing."""
        r = requests.post(f"{api_url}/documents/iso27001-a-5-2/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Review branch test"})
        assert r.status_code in [200, 201], f"Create review failed: {r.text}"
        TestReviewBranchEdit.review_id = r.json()["review_id"]

    def test_02_review_has_sent_head(self, api_url, admin_headers):
        """Review must capture sent_head at creation time."""
        rid = TestReviewBranchEdit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data.get("sent_head"), "sent_head should be set on review create"
        assert len(data["sent_head"]) >= 7, "sent_head should be a commit hash"

    def test_03_review_has_message(self, api_url, admin_headers):
        """Review must have the message from creation."""
        rid = TestReviewBranchEdit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.json()["message"] == "Review branch test"

    def test_04_get_review_content(self, api_url, admin_headers):
        """GET /reviews/:id/content returns document body."""
        rid = TestReviewBranchEdit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/content", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert "body" in data
        assert len(data["body"]) > 0

    def test_05_reviewer_can_edit(self, api_url, reader_headers):
        """Assigned reviewer can edit review content."""
        rid = TestReviewBranchEdit.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=reader_headers,
                         json={"content": "# Updated by reviewer\n\nNew content here."})
        assert r.status_code == 200, f"Reviewer edit failed: {r.text}"
        data = r.json()
        assert "commit" in data
        assert "branch" in data
        assert data["branch"] == f"review/{rid}"

    def test_06_contributor_cannot_edit(self, api_url, contributor_headers):
        """Contributor (not assigned) cannot edit review content."""
        rid = TestReviewBranchEdit.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=contributor_headers,
                         json={"content": "# Unauthorized edit"})
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"

    def test_07_review_diff_shows_branch(self, api_url, admin_headers):
        """After branch edit, diff should show has_branch=true and old/new body."""
        rid = TestReviewBranchEdit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data.get("has_branch") is True, "has_branch should be true after branch edit"
        assert data.get("new_body"), "new_body should be populated"
        assert "Updated by reviewer" in data["new_body"]

    def test_08_review_content_from_branch(self, api_url, admin_headers):
        """GET /reviews/:id/content should return branch content after edit."""
        rid = TestReviewBranchEdit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/content", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data.get("from_branch") is True
        assert "Updated by reviewer" in data["body"]

    def test_09_blame_at_review_branch(self, api_url, admin_headers):
        """Blame with ?ref=review/<id> should work."""
        rid = TestReviewBranchEdit.review_id
        r = requests.get(f"{api_url}/documents/iso27001-a-5-2/blame?ref=review/{rid}",
                         headers=admin_headers)
        assert r.status_code == 200
        lines = r.json().get("lines") or []
        assert len(lines) > 0, "Blame at review branch returned no lines"

    def test_10_approve_and_merge(self, api_url, admin_headers, reader_headers):
        """Approve then merge — branch content should land on main."""
        rid = TestReviewBranchEdit.review_id
        # Approve
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "Looks good"})
        assert r.status_code == 200
        # Merge
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 200, f"Merge failed: {r.text}"
        assert r.json()["status"] == "merged"
        # Verify document on main has branch content
        r = requests.get(f"{api_url}/documents/iso27001-a-5-2/body",
                         headers=admin_headers)
        assert r.status_code == 200
        assert "Updated by reviewer" in r.json()["body"], \
            "Branch content not merged to main"


class TestMergeWithNoBranchEdit:
    """Test merging a review where no edits were made on the branch."""

    review_id = None

    def test_01_create_and_approve(self, api_url, admin_headers, reader_headers):
        """Create review, approve without editing on branch."""
        r = requests.post(f"{api_url}/documents/iso27001-a-5-3/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "No-edit merge test"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestMergeWithNoBranchEdit.review_id = r.json()["review_id"]

        rid = TestMergeWithNoBranchEdit.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "OK"})
        assert r.status_code == 200

    def test_02_merge_succeeds(self, api_url, admin_headers):
        """Merge should succeed even with no branch (no edits)."""
        rid = TestMergeWithNoBranchEdit.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 200, f"Merge with no branch failed: {r.text}"
        assert r.json()["status"] == "merged"


class TestMergeCannotMergeUnapproved:
    """Merge requires approved status."""

    def test_merge_open_review_fails(self, api_url, admin_headers):
        """Cannot merge an open (unapproved) review."""
        # Create a review
        r = requests.post(f"{api_url}/documents/iso27001-a-5-4/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Should not merge"})
        assert r.status_code in [200, 201]
        rid = r.json()["review_id"]

        # Try to merge without approval
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"


class TestReviewEditAuthz:
    """Authorization tests for review content editing."""

    review_id = None

    def test_01_setup_review(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/iso27001-a-5-5/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Authz test"})
        assert r.status_code in [200, 201]
        TestReviewEditAuthz.review_id = r.json()["review_id"]

    def test_02_admin_can_edit(self, api_url, admin_headers):
        """Admin can always edit review content."""
        rid = TestReviewEditAuthz.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=admin_headers,
                         json={"content": "Admin edit"})
        assert r.status_code == 200

    def test_03_assigned_reviewer_can_edit(self, api_url, reader_headers):
        """Assigned reviewer can edit."""
        rid = TestReviewEditAuthz.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=reader_headers,
                         json={"content": "Reviewer edit"})
        assert r.status_code == 200

    def test_04_contributor_cannot_edit(self, api_url, contributor_headers):
        """Contributor not assigned to review cannot edit."""
        rid = TestReviewEditAuthz.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=contributor_headers,
                         json={"content": "Should fail"})
        assert r.status_code == 403

    def test_05_closed_review_cannot_edit(self, api_url, admin_headers):
        """Cannot edit a closed review."""
        rid = TestReviewEditAuthz.review_id
        # Close the review
        requests.put(f"{api_url}/reviews/{rid}/status",
                     headers=admin_headers, json={"status": "closed"})
        # Try to edit
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=admin_headers,
                         json={"content": "Should fail"})
        assert r.status_code == 400, f"Expected 400 for closed review edit, got {r.status_code}"


class TestStaleMergeConflict:
    """Regression: merge must fail with 409 if document was modified on main after review was sent."""

    review_id = None

    def test_01_create_review(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/iso27001-a-5-9/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Stale merge test"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestStaleMergeConflict.review_id = r.json()["review_id"]

    def test_02_modify_document_on_main(self, api_url, admin_headers):
        """Modify the same document on main after review was sent."""
        r = requests.put(f"{api_url}/documents/iso27001-a-5-9/content",
                         headers=admin_headers,
                         json={"content": "# Modified on main after review sent\n\nThis should cause conflict."})
        assert r.status_code == 200, f"Edit failed: {r.text}"

    def test_03_approve(self, api_url, reader_headers):
        rid = TestStaleMergeConflict.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "OK"})
        assert r.status_code == 200

    def test_04_merge_returns_409(self, api_url, admin_headers):
        """Merge must fail because document was modified on main since sent_head."""
        rid = TestStaleMergeConflict.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 409, f"Expected 409 conflict, got {r.status_code}: {r.text}"

    def test_05_review_not_merged(self, api_url, admin_headers):
        """Review status must still be approved, not merged."""
        rid = TestStaleMergeConflict.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.json()["status"] == "approved", "Review should not be merged after conflict"


class TestDocumentContentSave:
    """Test that document edit saves content+metadata in one commit."""

    def test_save_content_with_version_and_author(self, api_url, admin_headers):
        """PUT /documents/:id/content with version+author should work."""
        r = requests.put(f"{api_url}/documents/iso27001-4-1/content",
                         headers=admin_headers,
                         json={"content": "# Test content\n\nUpdated.",
                               "version": "1.0",
                               "author": "testadmin@isms-test.local"})
        assert r.status_code == 200, f"Save failed: {r.text}"
        assert "commit" in r.json()

    def test_metadata_multi_field_update(self, api_url, admin_headers):
        """PUT /documents/:id/metadata with multiple fields in one commit."""
        r = requests.put(f"{api_url}/documents/iso27001-4-1/metadata",
                         headers=admin_headers,
                         json={"fields": {"version": "1.1", "status": "draft"}})
        assert r.status_code == 200, f"Metadata update failed: {r.text}"
        assert "commit" in r.json()


class TestChangeRequestDefaultStatus:
    """Regression: change request default status must be 'proposed', not 'draft'."""

    def test_create_change_request_default_status(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers,
                          json={"title": "Test change", "description": "Testing default status"})
        assert r.status_code == 201, f"Create change failed: {r.text}"
        # Status should be 'proposed' (matches CHECK constraint)
        assert r.json().get("status") == "proposed", \
            f"Expected 'proposed', got '{r.json().get('status')}'"
