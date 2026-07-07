"""Change Management tests.

Tests the full change request lifecycle including
priority, category, risk assessment, and status transitions.
"""
import requests
from conftest import READER_EMAIL, ADMIN_EMAIL


class TestChangesCRUD:
    """Full CRUD lifecycle with assessment fields."""

    change_id = None

    def test_01_create_with_assessment(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Migrate to OIDC",
            "description": "Replace password auth with OIDC SSO.",
            "justification": "Security improvement and user convenience.",
            "priority": "high",
            "category": "technology",
            "risk_level": "medium",
            "rollback_plan": "Revert to password auth within 30 minutes.",
            "planned_at": "2026-06-01T09:00:00Z",
        })
        assert r.status_code == 201, f"Create failed: {r.text}"
        data = r.json()
        TestChangesCRUD.change_id = data["id"]
        assert data["status"] == "proposed"
        assert data["priority"] == "high"
        assert data["category"] == "technology"
        assert data["risk_level"] == "medium"
        assert data.get("planned_at") is not None

    def test_02_get_has_all_fields(self, api_url, admin_headers):
        cid = TestChangesCRUD.change_id
        r = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data["priority"] == "high"
        assert data["category"] == "technology"
        assert data["risk_level"] == "medium"
        assert data["rollback_plan"] == "Revert to password auth within 30 minutes."
        assert data["justification"] == "Security improvement and user convenience."
        assert data.get("planned_at") is not None

    def test_03_list_has_fields(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/changes", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [c for c in data if c["id"] == TestChangesCRUD.change_id]
        assert len(match) == 1
        assert match[0]["priority"] == "high"
        assert match[0]["category"] == "technology"

    def test_04_defaults(self, api_url, admin_headers):
        """Create with minimal fields — defaults must be sensible."""
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Minor process tweak",
            "description": "Small update.",
        })
        assert r.status_code == 201
        data = r.json()
        assert data["priority"] == "medium"
        assert data["category"] == "process"
        assert data["risk_level"] == "low"


class TestChangesStatusFlow:
    """Status transitions: proposed → approved → implemented → closed."""

    change_id = None

    def test_01_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Status flow test",
            "description": "Testing full lifecycle.",
            "priority": "critical",
            "category": "infrastructure",
            "risk_level": "high",
        })
        assert r.status_code == 201
        TestChangesStatusFlow.change_id = r.json()["id"]
        assert r.json()["status"] == "proposed"

    def test_02_approve(self, api_url, admin_headers):
        cid = TestChangesStatusFlow.change_id
        r = requests.put(f"{api_url}/changes/{cid}/status",
                         headers=admin_headers, json={"status": "approved"})
        assert r.status_code == 200

    def test_03_implement(self, api_url, admin_headers):
        cid = TestChangesStatusFlow.change_id
        r = requests.put(f"{api_url}/changes/{cid}/status",
                         headers=admin_headers, json={"status": "implemented"})
        assert r.status_code == 200

    def test_04_close(self, api_url, admin_headers):
        cid = TestChangesStatusFlow.change_id
        r = requests.put(f"{api_url}/changes/{cid}/status",
                         headers=admin_headers, json={"status": "closed"})
        assert r.status_code == 200
        r = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers)
        assert r.json()["status"] == "closed"

    def test_05_reject_flow(self, api_url, admin_headers):
        """Proposed → rejected."""
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Reject test",
            "description": "Will be rejected.",
        })
        cid = r.json()["id"]
        r = requests.put(f"{api_url}/changes/{cid}/status",
                         headers=admin_headers, json={"status": "rejected"})
        assert r.status_code == 200
        r = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers)
        assert r.json()["status"] == "rejected"


class TestChangeType:
    """#128: change requests carry a type — 'change' (default) or 'access_request'."""

    def test_default_type_is_change(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Type default test",
            "description": "No type supplied.",
        })
        assert r.status_code == 201, r.text
        assert r.json()["type"] == "change"

    def test_create_access_request(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Grant finance read access",
            "description": "Access request captured as a change record.",
            "type": "access_request",
        })
        assert r.status_code == 201, r.text
        cid = r.json()["id"]
        assert r.json()["type"] == "access_request"
        # Type persists on read.
        r = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers)
        assert r.json()["type"] == "access_request"

    def test_invalid_type_rejected(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Bad type",
            "description": "Should be rejected.",
            "type": "not_a_type",
        })
        assert r.status_code == 400, r.text

    def test_type_editable_after_create(self, api_url, admin_headers):
        """Type is settable on an existing change (misclassification is fixable)."""
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Reclassify me", "description": "starts as change",
        })
        assert r.status_code == 201, r.text
        cid = r.json()["id"]
        assert r.json()["type"] == "change"
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers,
                         json={"type": "access_request"})
        assert r.status_code == 200, r.text
        r = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers)
        assert r.json()["type"] == "access_request"

    def test_update_invalid_type_rejected(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Reclassify bad", "description": "x",
        })
        cid = r.json()["id"]
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers,
                         json={"type": "bogus"})
        assert r.status_code == 400, r.text


class TestChangesRBAC:
    """Reader cannot create or change status."""

    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/changes", headers=reader_headers, json={
            "title": "Unauthorized",
            "description": "Should fail.",
        })
        assert r.status_code == 403

    def test_reader_cannot_change_status(self, api_url, admin_headers, reader_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "RBAC test",
            "description": "Testing role access.",
        })
        cid = r.json()["id"]
        r = requests.put(f"{api_url}/changes/{cid}/status",
                         headers=reader_headers, json={"status": "approved"})
        assert r.status_code == 403


class TestChangesUpdate:
    """Update change request fields after creation."""

    change_id = None

    def test_01_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Update test",
            "description": "Original description.",
            "priority": "low",
            "category": "process",
            "risk_level": "low",
        })
        assert r.status_code == 201
        TestChangesUpdate.change_id = r.json()["id"]

    def test_02_update_fields(self, api_url, admin_headers):
        cid = TestChangesUpdate.change_id
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers, json={
            "title": "Update test (revised)",
            "description": "Revised description with more detail.",
            "priority": "high",
            "category": "technology",
            "risk_level": "medium",
            "rollback_plan": "Revert within 1 hour.",
            "justification": "Now has proper justification.",
            "planned_at": "2026-07-15T14:30:00Z",
        })
        assert r.status_code == 200, f"Update failed: {r.text}"
        data = r.json()
        assert data["title"] == "Update test (revised)"
        assert data["priority"] == "high"
        assert data["category"] == "technology"
        assert data["risk_level"] == "medium"
        assert data["rollback_plan"] == "Revert within 1 hour."

    def test_03_get_reflects_update(self, api_url, admin_headers):
        cid = TestChangesUpdate.change_id
        r = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers)
        assert r.status_code == 200
        assert r.json()["priority"] == "high"
        assert r.json()["justification"] == "Now has proper justification."
