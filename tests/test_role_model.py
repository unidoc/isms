"""Role model tests — verify reader + assignment based review access.

After removing the reviewer role, any assigned user (including reader)
can approve, comment, and edit on reviews they're assigned to.
"""
import requests
from conftest import READER_EMAIL, ADMIN_EMAIL


class TestAssignedReaderCanReview:
    """Reader assigned to a review can approve and edit."""

    review_id = None

    def test_01_create_review_with_reader_assigned(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/iso27001-a-7-1/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Reader review test"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestAssignedReaderCanReview.review_id = r.json()["review_id"]

    def test_02_reader_can_approve(self, api_url, reader_headers):
        """Assigned reader can approve."""
        rid = TestAssignedReaderCanReview.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "Reader approved"})
        assert r.status_code == 200, f"Reader approve failed: {r.text}"

    def test_03_reader_can_edit_review_content(self, api_url, reader_headers):
        """Assigned reader can edit review branch."""
        rid = TestAssignedReaderCanReview.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=reader_headers,
                         json={"content": "# Reader edited this\n\nNew content."})
        assert r.status_code == 200, f"Reader edit failed: {r.text}"

    def test_04_reader_can_comment(self, api_url, reader_headers):
        """Assigned reader can add review comment."""
        rid = TestAssignedReaderCanReview.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/comment",
                          headers=reader_headers,
                          json={"body": "Reader comment"})
        assert r.status_code in [200, 201], f"Reader comment failed: {r.text}"


class TestUnassignedReaderBlocked:
    """Reader NOT assigned to a review is blocked."""

    review_id = None

    def test_01_create_review_without_reader(self, api_url, admin_headers):
        """Create review with only admin as reviewer — reader not assigned."""
        r = requests.post(f"{api_url}/documents/iso27001-a-7-2/reviews",
                          headers=admin_headers,
                          json={"reviewers": [ADMIN_EMAIL],
                                "message": "Unassigned test"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestUnassignedReaderBlocked.review_id = r.json()["review_id"]

    def test_02_unassigned_reader_cannot_approve(self, api_url, reader_headers):
        rid = TestUnassignedReaderBlocked.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "Should fail"})
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"

    def test_03_unassigned_reader_cannot_comment(self, api_url, reader_headers):
        rid = TestUnassignedReaderBlocked.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/comment",
                          headers=reader_headers,
                          json={"body": "Should fail"})
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"

    def test_04_unassigned_reader_cannot_edit(self, api_url, reader_headers):
        rid = TestUnassignedReaderBlocked.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=reader_headers,
                         json={"content": "Should fail"})
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"


class TestSupplierAssuranceOnCreate:
    """Supplier create respects provided status."""

    def test_custom_status_persists(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Custom Status Supplier",
            "supplier_type": "saas",
            "criticality": "high",
            "status": "under_review",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data["status"] == "under_review"

    def test_default_status_when_empty(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Default Status Supplier",
            "supplier_type": "cloud",
            "criticality": "low",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data["status"] == "active"


