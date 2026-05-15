"""Contributor role tests.

Contributors can: create incidents, change requests, tasks, add comments.
Contributors cannot: approve reviews, create/edit risks, manage templates, admin actions.
"""
import requests
from conftest import CONTRIBUTOR_EMAIL


class TestContributorCanDo:
    """Things a contributor is allowed to do."""

    def test_can_read_risks(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/risks", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_read_documents(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/documents/all", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_read_suppliers(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/suppliers", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_read_incidents(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/incidents", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_create_incident(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/incidents", headers=contributor_headers, json={
            "title": "Contributor reported incident",
            "description": "Found an issue during maintenance",
            "severity": "medium",
            "affects_a": True,
            "incident_type": "event",
            "source": "internal",
            "reporter": CONTRIBUTOR_EMAIL,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"

    def test_can_create_change_request(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/changes", headers=contributor_headers, json={
            "title": "Update firewall rules for new office",
            "description": "Need to allow traffic from new office IP range",
            "status": "proposed",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"

    def test_can_create_task(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/tasks", headers=contributor_headers, json={
            "title": "Patch server CVE-2026-1234",
            "task_type": "incident_followup",
            "assignee": CONTRIBUTOR_EMAIL,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"


class TestContributorCannotDo:
    """Things a contributor is NOT allowed to do."""

    def test_cannot_create_risk(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/risks", headers=contributor_headers, json={
            "title": "Should fail",
            "current_likelihood": 3, "current_impact": 3,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code == 403

    def test_cannot_create_supplier(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/suppliers", headers=contributor_headers, json={
            "name": "Should fail", "supplier_type": "cloud",
            "criticality": "low",
        })
        assert r.status_code == 403

    def test_cannot_create_asset(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/assets", headers=contributor_headers, json={
            "name": "Should fail", "asset_type": "system", "status": "live",
        })
        assert r.status_code == 403

    def test_cannot_create_legal(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/legal", headers=contributor_headers, json={
            "title": "Should fail", "jurisdiction": "EU",
        })
        assert r.status_code == 403

    def test_cannot_send_for_review(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/documents/iso27001-4-1/reviews",
                          headers=contributor_headers, json={"reviewers": []})
        assert r.status_code == 403

    def test_cannot_add_template(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/templates", headers=contributor_headers,
                          json={"template": "iso9001"})
        assert r.status_code == 403

    def test_cannot_create_audit_programme(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/audit/programmes", headers=contributor_headers, json={
            "title": "Should fail", "year": 2026,
        })
        assert r.status_code == 403

    def test_cannot_access_admin(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/admin/members", headers=contributor_headers)
        assert r.status_code in [401, 403, 404]

    def test_cannot_create_overdue_tasks(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/overdue/tasks", headers=contributor_headers)
        assert r.status_code == 403

    def test_cannot_delete_risk(self, api_url, admin_headers, contributor_headers):
        # Create as admin first
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Contributor delete test",
            "current_likelihood": 1, "current_impact": 1,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        risk_id = r.json()["id"]

        d = requests.delete(f"{api_url}/risks/{risk_id}", headers=contributor_headers)
        assert d.status_code == 403
