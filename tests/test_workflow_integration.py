"""Integration tests for cross-entity workflow features added in P2/P3:
- Orphan validation: cannot close incident with open CAs
- Orphan validation: cannot resolve CA with open implementation tasks
- Auto-task on Change approval
- Inbox surfaces (incidents/CAs filterable by assignee)
"""
import requests
from conftest import ADMIN_EMAIL


class TestIncidentOrphanValidation:
    """Cannot close an incident if it has open corrective actions linked to it."""

    incident_id = None
    ca_id = None

    def test_01_create_incident(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
            "title": "P3 orphan test incident",
            "description": "Testing orphan validation",
            "severity": "high",
            "affects_c": True,
            "incident_type": "incident",
            "source": "internal",
            "reporter": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Create failed: {r.text}"
        TestIncidentOrphanValidation.incident_id = r.json()["id"]
        TestIncidentOrphanValidation.incident_identifier = r.json()["identifier"]

    def test_02_link_ca_to_incident(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Linked CA",
            "description": "Linked corrective action",
            "source": "security_incident",
            "severity": "minor_nc",
        })
        assert r.status_code in [200, 201], f"Create CA failed: {r.text}"
        ca = r.json()
        TestIncidentOrphanValidation.ca_id = ca["id"]
        # Register the CA → Incident link via entity_references — references
        # use per-org identifiers (INC-N), not numeric row ids.
        ref = requests.post(f"{api_url}/references", headers=admin_headers, json={
            "source_type": "corrective_action",
            "source_id": ca["identifier"],
            "target_type": "incident",
            "target_id": TestIncidentOrphanValidation.incident_identifier,
        })
        assert ref.status_code in [200, 201], f"Create reference failed: {ref.text}"

    def test_03_cannot_close_incident_with_open_ca(self, api_url, admin_headers):
        r = requests.put(
            f"{api_url}/incidents/{TestIncidentOrphanValidation.incident_id}/status",
            headers=admin_headers,
            json={"status": "closed"},
        )
        assert r.status_code == 409, f"Expected 409 Conflict, got {r.status_code}: {r.text}"
        assert "corrective action" in r.text.lower()

    def test_04_cannot_resolve_incident_with_open_ca(self, api_url, admin_headers):
        r = requests.put(
            f"{api_url}/incidents/{TestIncidentOrphanValidation.incident_id}/status",
            headers=admin_headers,
            json={"status": "resolved"},
        )
        assert r.status_code == 409, f"Expected 409 Conflict, got {r.status_code}: {r.text}"

    def test_05_resolve_ca_then_close_incident(self, api_url, admin_headers):
        # Resolve the CA first
        r = requests.put(
            f"{api_url}/corrective-actions/{TestIncidentOrphanValidation.ca_id}/status",
            headers=admin_headers,
            json={"status": "resolved"},
        )
        assert r.status_code == 200, f"Resolve CA failed: {r.text}"
        # Now closing incident should succeed
        r = requests.put(
            f"{api_url}/incidents/{TestIncidentOrphanValidation.incident_id}/status",
            headers=admin_headers,
            json={"status": "closed"},
        )
        assert r.status_code == 200, f"Close incident failed: {r.text}"


class TestCAOrphanValidation:
    """Cannot resolve a CA if there is an open implementation task referencing it."""

    ca_id = None
    ca_identifier = None
    task_id = None

    def test_01_create_ca(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "P3 ca-task orphan test",
            "description": "Testing CA orphan validation",
            "source": "internal_audit",
            "severity": "minor_nc",
        })
        assert r.status_code in [200, 201], f"Create CA failed: {r.text}"
        ca = r.json()
        TestCAOrphanValidation.ca_id = ca["id"]
        TestCAOrphanValidation.ca_identifier = ca["identifier"]

    def test_02_create_implementation_task(self, api_url, admin_headers):
        # Title contains the CA identifier — heuristic for linkage
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": f"Implement {TestCAOrphanValidation.ca_identifier}: testing",
            "description": "linked impl task",
            "task_type": "ca_followup",
            "priority": "medium",
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Create task failed: {r.text}"
        TestCAOrphanValidation.task_id = r.json()["id"]

    def test_03_cannot_resolve_ca_with_open_task(self, api_url, admin_headers):
        r = requests.put(
            f"{api_url}/corrective-actions/{TestCAOrphanValidation.ca_id}/status",
            headers=admin_headers,
            json={"status": "resolved"},
        )
        assert r.status_code == 409, f"Expected 409, got {r.status_code}: {r.text}"
        assert "implementation task" in r.text.lower() or "open" in r.text.lower()

    def test_04_finish_task_then_resolve_ca(self, api_url, admin_headers):
        # Mark task done
        r = requests.put(
            f"{api_url}/tasks/{TestCAOrphanValidation.task_id}/status",
            headers=admin_headers,
            json={"status": "done"},
        )
        assert r.status_code == 200, f"Mark task done failed: {r.text}"
        # Now resolve CA should succeed
        r = requests.put(
            f"{api_url}/corrective-actions/{TestCAOrphanValidation.ca_id}/status",
            headers=admin_headers,
            json={"status": "resolved"},
        )
        assert r.status_code == 200, f"Resolve CA failed: {r.text}"


class TestAutoTaskOnChangeApproval:
    """When a change is approved, an implementation task is auto-created."""

    change_id = None
    change_identifier = None

    def test_01_create_change(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "P2 auto-task change",
            "description": "Testing auto-task on approval",
            "priority": "medium",
            "category": "process",
            "risk_level": "low",
        })
        assert r.status_code in [200, 201], f"Create change failed: {r.text}"
        cr = r.json()
        TestAutoTaskOnChangeApproval.change_id = cr["id"]
        TestAutoTaskOnChangeApproval.change_identifier = cr["identifier"]

    def test_02_approve_creates_task(self, api_url, admin_headers):
        # Snapshot tasks before
        before = requests.get(f"{api_url}/tasks?limit=200", headers=admin_headers).json()
        before_ids = {t["id"] for t in before.get("data", [])}

        r = requests.put(
            f"{api_url}/changes/{TestAutoTaskOnChangeApproval.change_id}/status",
            headers=admin_headers,
            json={"status": "approved"},
        )
        assert r.status_code == 200, f"Approve failed: {r.text}"

        # Look for a new task referencing this change
        ident = TestAutoTaskOnChangeApproval.change_identifier
        r = requests.get(f"{api_url}/tasks?q={ident}&limit=200", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data", [])
        match = [t for t in data if ident in t.get("title", "") and t.get("task_type") == "change_followup"]
        assert len(match) >= 1, f"Expected auto-task referencing {ident}, got: {[t.get('title') for t in data]}"

    def test_03_double_approve_does_not_duplicate(self, api_url, admin_headers):
        # Re-approving (same status) should not create another task
        ident = TestAutoTaskOnChangeApproval.change_identifier
        r = requests.put(
            f"{api_url}/changes/{TestAutoTaskOnChangeApproval.change_id}/status",
            headers=admin_headers,
            json={"status": "approved"},
        )
        # Either 200 (no-op) or returns same status; not creating a new task is what we test
        assert r.status_code == 200
        r = requests.get(f"{api_url}/tasks?q={ident}&limit=200", headers=admin_headers)
        data = r.json().get("data", [])
        match = [t for t in data if ident in t.get("title", "") and t.get("task_type") == "change_followup"]
        assert len(match) == 1, f"Expected exactly 1 auto-task, got {len(match)}"


class TestInboxFilterByAssignee:
    """Inbox tab queries should filter incidents/CAs by assignee."""

    incident_id = None
    ca_id = None

    def test_01_create_incident_assigned_to_me(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
            "title": "P1 inbox test incident",
            "description": "for inbox query",
            "severity": "low",
            "affects_a": True,
            "incident_type": "event",
            "source": "internal",
            "reporter": ADMIN_EMAIL,
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Create failed: {r.text}"
        TestInboxFilterByAssignee.incident_id = r.json()["id"]

    def test_02_query_incidents_by_assignee(self, api_url, admin_headers):
        # Use a wide limit so the just-created record is in the page even on a busy DB.
        r = requests.get(f"{api_url}/incidents?assignee={ADMIN_EMAIL}&limit=200", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data", [])
        ids = {i["id"] for i in data}
        assert TestInboxFilterByAssignee.incident_id in ids, f"Expected to find incident assigned to admin in {ids}"

    def test_03_create_ca_assigned_to_me(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "P1 inbox test CA",
            "description": "for inbox query",
            "source": "feedback",
            "severity": "observation",
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Create failed: {r.text}"
        TestInboxFilterByAssignee.ca_id = r.json()["id"]

    def test_04_query_cas_by_assignee(self, api_url, admin_headers):
        # Use a wide limit so the just-created record is in the page even on a busy DB.
        r = requests.get(f"{api_url}/corrective-actions?assignee={ADMIN_EMAIL}&limit=200", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data", [])
        ids = {c["id"] for c in data}
        assert TestInboxFilterByAssignee.ca_id in ids, f"Expected to find CA assigned to admin in {ids}"


class TestTaskCompletedAtClearing:
    """Data correctness: completed_at must clear when task reverts from done."""

    task_id = None

    def test_01_create_and_finish(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "P3 completed_at test",
            "description": "verify completed_at is cleared on revert",
            "task_type": "general",
            "priority": "medium",
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201]
        TestTaskCompletedAtClearing.task_id = r.json()["id"]

        # Mark done
        r = requests.put(
            f"{api_url}/tasks/{TestTaskCompletedAtClearing.task_id}/status",
            headers=admin_headers,
            json={"status": "done"},
        )
        assert r.status_code == 200

        # Verify completed_at is set
        t = requests.get(f"{api_url}/tasks/{TestTaskCompletedAtClearing.task_id}", headers=admin_headers).json()
        assert t.get("completed_at"), f"completed_at should be set after done, got {t.get('completed_at')}"

    def test_02_revert_to_in_progress_clears_completed_at(self, api_url, admin_headers):
        # Revert to in_progress
        r = requests.put(
            f"{api_url}/tasks/{TestTaskCompletedAtClearing.task_id}/status",
            headers=admin_headers,
            json={"status": "in_progress"},
        )
        assert r.status_code == 200

        # Verify completed_at is now null/empty
        t = requests.get(f"{api_url}/tasks/{TestTaskCompletedAtClearing.task_id}", headers=admin_headers).json()
        assert not t.get("completed_at"), \
            f"completed_at should be cleared after revert, got {t.get('completed_at')}"

    def test_03_revert_to_open_also_clears(self, api_url, admin_headers):
        # Mark done again
        requests.put(
            f"{api_url}/tasks/{TestTaskCompletedAtClearing.task_id}/status",
            headers=admin_headers,
            json={"status": "done"},
        )
        # Revert directly to open
        requests.put(
            f"{api_url}/tasks/{TestTaskCompletedAtClearing.task_id}/status",
            headers=admin_headers,
            json={"status": "open"},
        )
        t = requests.get(f"{api_url}/tasks/{TestTaskCompletedAtClearing.task_id}", headers=admin_headers).json()
        assert not t.get("completed_at"), \
            f"completed_at should be cleared after revert to open, got {t.get('completed_at')}"


class TestCrossEntityLinkage:
    """Verify that CA linked to incident is connected via entity_references and visible in both directions."""

    incident_id = None
    ca_id = None
    ca_identifier = None

    def test_01_create_incident_and_linked_ca(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
            "title": "Linkage test incident",
            "description": "for ref test",
            "severity": "medium",
            "affects_i": True,
            "incident_type": "event",
            "source": "internal",
            "reporter": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201]
        TestCrossEntityLinkage.incident_id = r.json()["id"]

        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Linkage test CA",
            "description": "linked",
            "source": "security_incident",
            "severity": "observation",
        })
        assert r.status_code in [200, 201], f"Create CA failed: {r.text}"
        ca = r.json()
        TestCrossEntityLinkage.ca_id = ca["id"]
        TestCrossEntityLinkage.ca_identifier = ca["identifier"]

        ref = requests.post(f"{api_url}/references", headers=admin_headers, json={
            "source_type": "corrective_action",
            "source_id": ca["identifier"],
            "target_type": "incident",
            "target_id": f"INC-{TestCrossEntityLinkage.incident_id}",
        })
        assert ref.status_code in [200, 201], f"Create reference failed: {ref.text}"

    def test_02_reference_visible_from_ca(self, api_url, admin_headers):
        r = requests.get(
            f"{api_url}/references?type=corrective_action&id={TestCrossEntityLinkage.ca_identifier}",
            headers=admin_headers,
        )
        assert r.status_code == 200, r.text
        body = r.json()
        refs = body.get("data") if isinstance(body, dict) else body
        assert any(
            (rr.get("target_type") == "incident" or rr.get("source_type") == "incident")
            for rr in (refs or [])
        ), f"Expected incident reference in {refs}"
