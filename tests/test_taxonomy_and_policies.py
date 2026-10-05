"""Approval policies and entity references tests."""
import uuid

import pytest
import requests
from conftest import READER_EMAIL, CONTRIBUTOR_EMAIL


# ═══════════════════════════════════════════════════════════════════════
# 1. Approval policy enforcement
# ═══════════════════════════════════════════════════════════════════════

class TestApprovalPolicyEnforcement:
    """Test approval policy enforcement on merge."""

    policy_id = None
    review_id = None

    def test_01_create_policy(self, api_url, admin_headers):
        """Create a policy requiring 2 approvals for iso27001-6-* docs."""
        r = requests.post(f"{api_url}/admin/policies", headers=admin_headers, json={
            "name": "Two approvals for planning",
            "path_pattern": "*",
            "min_approvals": 2,
            "required_roles": [],
            "required_users": [],
        })
        assert r.status_code in [200, 201], f"Create policy failed: {r.text}"
        TestApprovalPolicyEnforcement.policy_id = r.json()["id"]

    def test_02_create_review(self, api_url, admin_headers):
        """Create a review on a doc matching the policy path."""
        r = requests.post(f"{api_url}/documents/iso27001-a-6-2/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL, CONTRIBUTOR_EMAIL],
                                "message": "Policy enforcement test"})
        assert r.status_code in [200, 201], f"Create review failed: {r.text}"
        TestApprovalPolicyEnforcement.review_id = r.json()["review_id"]

    def test_03_one_approval(self, api_url, reader_headers):
        """Reviewer approves (1 of 2)."""
        rid = TestApprovalPolicyEnforcement.review_id
        assert rid is not None, "No review ID from previous test"
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "Looks good"})
        assert r.status_code == 200, f"Approve failed: {r.text}"

    def test_04_merge_blocked(self, api_url, admin_headers):
        """Merge should be blocked — only 1 of 2 required approvals."""
        rid = TestApprovalPolicyEnforcement.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code in (400, 403), f"Expected 400/403, got {r.status_code}: {r.text}"

    def test_05_second_approval(self, api_url, contributor_headers):
        """Contributor approves (2 of 2)."""
        rid = TestApprovalPolicyEnforcement.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=contributor_headers,
                          json={"decision": "approved", "comment": "Also good"})
        assert r.status_code == 200, f"Second approve failed: {r.text}"

    def test_06_merge_succeeds(self, api_url, admin_headers):
        """Merge should succeed now with 2 approvals."""
        rid = TestApprovalPolicyEnforcement.review_id
        requests.put(f"{api_url}/reviews/{rid}/status",
                     headers=admin_headers, json={"status": "approved"})
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 200, f"Merge failed: {r.text}"
        assert r.json()["status"] == "merged"

    def test_07_cleanup_policy(self, api_url, admin_headers):
        """Delete the test policy."""
        pid = TestApprovalPolicyEnforcement.policy_id
        if pid:
            requests.delete(f"{api_url}/admin/policies/{pid}", headers=admin_headers)


# ═══════════════════════════════════════════════════════════════════════
# 2. Entity references (bidirectional)
# ═══════════════════════════════════════════════════════════════════════

def _links(refs, typ, ident):
    """Entries whose either end is the given (type, identifier)."""
    return [x for x in refs
            if (x.get("target_type"), x.get("target_id")) == (typ, ident)
            or (x.get("source_type"), x.get("source_id")) == (typ, ident)]


@pytest.fixture(scope="class")
def entity_reference_records(request, api_url, admin_headers):
    """Create the asset and legal requirement this class links, so it doesn't
    depend on records another test file happened to create (#405)."""
    suffix = uuid.uuid4().hex[:8]
    r = requests.post(f"{api_url}/assets", headers=admin_headers, json={
        "name": f"ref-test-asset-{suffix}", "asset_type": "system", "status": "open",
    })
    assert r.status_code in (200, 201), f"Create asset failed: {r.text}"
    request.cls.asset_id = r.json()["identifier"]
    r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
        "title": f"ref-test-legal-{suffix}", "jurisdiction": "EU", "category": "privacy",
    })
    assert r.status_code in (200, 201), f"Create legal requirement failed: {r.text}"
    request.cls.legal_id = r.json()["identifier"]


@pytest.mark.usefixtures("entity_reference_records")
class TestEntityReferences:
    """Create, verify bidirectional, delete, verify cleanup."""

    ref_id = None
    asset_id = None
    legal_id = None

    def test_01_create_reference(self, api_url, admin_headers):
        """Create a reference between asset and legal requirement."""
        r = requests.post(f"{api_url}/references", headers=admin_headers, json={
            "source_type": "asset",
            "source_id": self.asset_id,
            "target_type": "legal_requirement",
            "target_id": self.legal_id,
        })
        assert r.status_code == 201, f"Create ref failed: {r.text}"
        TestEntityReferences.ref_id = r.json().get("id")
        assert TestEntityReferences.ref_id, f"No id in create response: {r.text}"

    def test_02_bidirectional_from_source(self, api_url, admin_headers):
        """Reference visible from source side."""
        r = requests.get(f"{api_url}/references?type=asset&id={self.asset_id}", headers=admin_headers)
        assert r.status_code == 200
        refs = r.json()["data"]
        legal_refs = _links(refs, "legal_requirement", self.legal_id)
        assert len(legal_refs) >= 1, "Expected to find legal_requirement reference from asset side"

    def test_03_bidirectional_from_target(self, api_url, admin_headers):
        """Reference visible from target side."""
        r = requests.get(f"{api_url}/references?type=legal_requirement&id={self.legal_id}", headers=admin_headers)
        assert r.status_code == 200
        refs = r.json()["data"]
        asset_refs = _links(refs, "asset", self.asset_id)
        assert len(asset_refs) >= 1, "Expected to find asset reference from legal_requirement side"

    def test_04_no_duplicates(self, api_url, admin_headers):
        """Listing should not return duplicate entries for same pair."""
        r = requests.get(f"{api_url}/references?type=asset&id={self.asset_id}", headers=admin_headers)
        assert r.status_code == 200
        refs = r.json()["data"]
        pairs = set()
        for ref in refs:
            pair = frozenset([
                (ref.get("source_type", ""), ref.get("source_id", "")),
                (ref.get("target_type", ""), ref.get("target_id", "")),
            ])
            assert pair not in pairs, f"Duplicate reference pair found: {pair}"
            pairs.add(pair)

    def test_05_delete_reference(self, api_url, admin_headers):
        """Delete the reference."""
        ref_id = TestEntityReferences.ref_id
        assert ref_id, "test_01 stored no reference id"
        r = requests.delete(f"{api_url}/references/{ref_id}", headers=admin_headers)
        assert r.status_code == 200, f"Delete ref failed: {r.text}"

    def test_06_both_sides_cleared(self, api_url, admin_headers):
        """After delete, both sides should no longer see the reference."""
        r1 = requests.get(f"{api_url}/references?type=asset&id={self.asset_id}", headers=admin_headers)
        r2 = requests.get(f"{api_url}/references?type=legal_requirement&id={self.legal_id}", headers=admin_headers)
        assert r1.status_code == 200
        assert r2.status_code == 200
        legal_refs = _links(r1.json()["data"], "legal_requirement", self.legal_id)
        asset_refs = _links(r2.json()["data"], "asset", self.asset_id)
        assert len(legal_refs) == 0, "Legal ref still visible from asset side after delete"
        assert len(asset_refs) == 0, "Asset ref still visible from legal side after delete"
