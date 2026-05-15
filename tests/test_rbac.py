"""Role-Based Access Control tests.

Verifies that each role can only access what it should:
- admin: full access
- manager: edit + manage, no admin settings
- contributor: edit registers + participate in reviews
- reader: read only (+ approve/comment when assigned to a review)
"""
import requests
from conftest import ADMIN_EMAIL


class TestReaderCannotWrite:
    """Reader should be blocked from all write operations except comments and approvals on assigned reviews."""

    def test_cannot_create_risk(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/risks", headers=reader_headers, json={
            "title": "test", "current_likelihood": 1, "current_impact": 1,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code == 403

    def test_cannot_create_asset(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/assets", headers=reader_headers, json={
            "name": "test", "asset_type": "system",
        })
        assert r.status_code == 403

    def test_cannot_create_supplier(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/suppliers", headers=reader_headers, json={
            "name": "test",
        })
        assert r.status_code == 403

    def test_cannot_send_for_review(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/documents/iso27001-4-1/reviews",
                          headers=reader_headers, json={"reviewers": [ADMIN_EMAIL]})
        assert r.status_code == 403

    def test_cannot_create_audit(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/audits", headers=reader_headers, json={
            "title": "test", "scope": "all",
        })
        assert r.status_code == 403

    def test_cannot_access_admin(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/admin/members", headers=reader_headers)
        assert r.status_code == 403

    def test_cannot_add_template(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/templates", headers=reader_headers,
                          json={"template": "soc2"})
        assert r.status_code == 403


class TestReaderCanRead:
    """Reader should be able to read all data."""

    def test_can_read_documents(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/documents/all", headers=reader_headers)
        assert r.status_code == 200

    def test_can_search(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/documents/search?q=security", headers=reader_headers)
        assert r.status_code == 200

    def test_can_read_risks(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/risks", headers=reader_headers)
        assert r.status_code == 200

    def test_can_read_reviews(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/reviews", headers=reader_headers)
        assert r.status_code == 200


class TestAdminCanWrite:
    """Admin should be able to perform all operations."""

    def test_can_read_admin(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/admin/members", headers=admin_headers)
        assert r.status_code == 200

    def test_can_read_documents(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
        assert r.status_code == 200
