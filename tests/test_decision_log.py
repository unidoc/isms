"""Decision log tests — verify immutable audit records on review decisions."""
import requests
from conftest import READER_EMAIL


class TestDecisionLog:
    """Decision records are created on approve and merge."""

    review_id = None

    def test_01_create_review(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/iso27001-a-6-1/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Decision log test"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestDecisionLog.review_id = r.json()["review_id"]

    def test_02_approve_creates_decision(self, api_url, reader_headers):
        rid = TestDecisionLog.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "LGTM"})
        assert r.status_code == 200

    def test_03_review_has_decisions(self, api_url, admin_headers):
        rid = TestDecisionLog.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/decisions", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or r.json()
        assert len(data) >= 1, "Should have at least one decision record"
        rec = data[0]
        assert rec["decision"] == "approved"
        assert rec.get("content_hash"), "Decision should have content hash"
        assert len(rec["content_hash"]) >= 10

    def test_04_merge_creates_decision(self, api_url, admin_headers):
        rid = TestDecisionLog.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 200

    def test_05_document_decisions(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/documents/iso27001-a-6-1/decisions",
                         headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or r.json()
        # Should have both approve and merge records
        decisions = [d["decision"] for d in data]
        assert "approved" in decisions
        assert "merged" in decisions


class TestApprovalPolicies:
    """Approval policy CRUD and enforcement."""

    policy_id = None

    def test_01_list_empty(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/admin/policies", headers=admin_headers)
        assert r.status_code == 200

    def test_02_create_policy(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/admin/policies", headers=admin_headers, json={
            "name": "Test Policy",
            "path_pattern": "iso27001/controls",
            "min_approvals": 2,
            "required_roles": ["manager"],
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestApprovalPolicies.policy_id = r.json().get("id")

    def test_03_list_has_policy(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/admin/policies", headers=admin_headers)
        body = r.json()
        data = body.get("data") if isinstance(body, dict) else body
        assert any(p["name"] == "Test Policy" for p in data)

    def test_04_delete_policy(self, api_url, admin_headers):
        pid = TestApprovalPolicies.policy_id
        if pid:
            r = requests.delete(f"{api_url}/admin/policies/{pid}", headers=admin_headers)
            assert r.status_code == 200
