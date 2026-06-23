"""Contributor and reader role enforcement (#23).

The decided model: a contributor proposes and reports *through suggestions* and
comments — it does NOT directly create or edit entities (same path as an AI
agent). A reader is read-only and cannot even suggest. Managers/admins mutate
directly and apply/reject suggestions.
"""
import requests
from conftest import CONTRIBUTOR_EMAIL, READER_EMAIL


class TestContributorCanDo:
    """Things a contributor is allowed to do: read everything, propose via suggestions."""

    def test_can_read_risks(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/risks", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_read_documents(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/documents/all", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_read_incidents(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/incidents", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_read_suppliers(self, api_url, contributor_headers):
        r = requests.get(f"{api_url}/suppliers", headers=contributor_headers)
        assert r.status_code == 200

    def test_can_update_status_of_own_assigned_task(self, api_url, admin_headers, contributor_headers):
        """Ownership exception (#23): a contributor may advance their own task's status."""
        t = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "Ownership test task", "task_type": "general",
            "assignee": CONTRIBUTOR_EMAIL,
        })
        assert t.status_code in [200, 201], t.text
        r = requests.put(f"{api_url}/tasks/{t.json()['id']}/status", headers=contributor_headers,
                         json={"status": "in_progress"})
        assert r.status_code == 200, f"contributor must update own task status: {r.text}"

    def test_can_create_suggestion(self, api_url, contributor_headers):
        """A contributor's input flows through suggestions, not direct creation."""
        r = requests.post(f"{api_url}/suggestions", headers=contributor_headers, json={
            "entity_type": "risk",
            "suggestion_type": "create",
            "title": "Contributor-proposed risk",
            "rationale": "Spotted during operations",
            "payload": {
                "title": "Contributor-proposed risk",
                "description": "Proposed by a contributor for manager review",
                "category": "technology", "risk_type": "threat", "origin": "internal",
            },
        })
        assert r.status_code in [200, 201], f"contributor must be able to suggest: {r.text}"


class TestContributorCannotDo:
    """A contributor cannot directly create or edit entities — it must suggest."""

    def test_cannot_create_incident(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/incidents", headers=contributor_headers, json={
            "title": "Should be a suggestion", "description": "d", "severity": "medium",
            "incident_type": "event", "source": "internal", "reporter": CONTRIBUTOR_EMAIL,
        })
        assert r.status_code == 403, f"contributor must not create incidents directly (#23): {r.text}"

    def test_cannot_create_change_request(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/changes", headers=contributor_headers, json={
            "title": "Should be a suggestion", "description": "d", "status": "proposed",
        })
        assert r.status_code == 403, f"contributor must not create change requests directly (#23): {r.text}"

    def test_cannot_create_task(self, api_url, contributor_headers):
        r = requests.post(f"{api_url}/tasks", headers=contributor_headers, json={
            "title": "Should be a suggestion", "task_type": "general",
            "assignee": CONTRIBUTOR_EMAIL,
        })
        assert r.status_code == 403, f"contributor must not create tasks directly (#23): {r.text}"

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

    def test_cannot_update_status_of_unassigned_task(self, api_url, admin_headers, contributor_headers):
        t = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "Unassigned task", "task_type": "general",
        })
        assert t.status_code in [200, 201], t.text
        r = requests.put(f"{api_url}/tasks/{t.json()['id']}/status", headers=contributor_headers,
                         json={"status": "in_progress"})
        assert r.status_code == 403, f"contributor must not update an unassigned task (#23): {r.text}"

    def test_cannot_update_status_of_other_persons_task(self, api_url, admin_headers, contributor_headers):
        # Assign to a real org member who isn't the contributor (task create
        # validates org membership, so the assignee must exist).
        t = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "Other person task", "task_type": "general",
            "assignee": READER_EMAIL,
        })
        assert t.status_code in [200, 201], t.text
        r = requests.put(f"{api_url}/tasks/{t.json()['id']}/status", headers=contributor_headers,
                         json={"status": "in_progress"})
        assert r.status_code == 403, f"contributor must not update another person's task (#23): {r.text}"


class TestReaderIsReadOnly:
    """A reader cannot suggest — suggestions are the contributor entry point, not reader."""

    def test_reader_cannot_create_suggestion(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/suggestions", headers=reader_headers, json={
            "entity_type": "risk",
            "suggestion_type": "create",
            "title": "Reader should not be able to suggest",
            "rationale": "read-only",
            "payload": {"title": "x", "category": "technology", "risk_type": "threat", "origin": "internal"},
        })
        assert r.status_code == 403, f"reader is read-only and must not create suggestions (#23): {r.text}"
