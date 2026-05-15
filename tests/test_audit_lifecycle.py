"""Audit findings and corrective actions lifecycle tests.

Tests the ISO 27001 core workflow:
  Audit → Finding → Corrective Action → Resolution
"""
import time

import requests
from conftest import ADMIN_EMAIL


class TestAuditFindingsLifecycle:
    """Audit → Items → Findings → Corrective Actions."""

    programme_id = None
    audit_id = None
    finding_id = None
    ca_id = None

    def test_01_create_programme(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
            "title": "Lifecycle Test Programme",
            "year": 2026,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestAuditFindingsLifecycle.programme_id = r.json()["id"]

    def test_02_create_audit(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/audits", headers=admin_headers, json={
            "programme_id": self.programme_id,
            "title": "Lifecycle test audit",
            "scope": "iso27001-4.*",
            "auditor": ADMIN_EMAIL,
            "audit_type": "internal",
            "planned_date": 1781740800,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestAuditFindingsLifecycle.audit_id = r.json()["id"]

    def test_03_start_audit(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/audits/{self.audit_id}/status", headers=admin_headers,
                         json={"status": "in_progress"})
        assert r.status_code == 200, f"Failed: {r.text}"

    def test_04_create_finding(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/audit-findings", headers=admin_headers, json={
            "audit_id": self.audit_id,
            "finding_type": "minor_nc",
            "title": "Incomplete access review records",
            "description": "Access reviews for 3 systems have no evidence of completion\n\n## Corrective Action\n\nDocument completion for the 3 flagged systems.",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestAuditFindingsLifecycle.finding_id = r.json()["id"]

    def test_05_list_findings(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/audits/{self.audit_id}/findings", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or r.json()
        assert isinstance(data, list)
        assert len(data) >= 1

    def test_06_create_corrective_action(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Complete access review for all systems",
            "description": "Perform and document access reviews for the 3 flagged systems",
            "source": "internal_audit",
            "severity": "minor_nc",
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        ca = r.json()
        TestAuditFindingsLifecycle.ca_id = ca["id"]
        # Link CA → audit finding via entity_references
        ref = requests.post(f"{api_url}/references", headers=admin_headers, json={
            "source_type": "corrective_action",
            "source_id": ca["identifier"],
            "target_type": "audit_finding",
            "target_id": f"FIND-{self.finding_id}",
        })
        assert ref.status_code in [200, 201], f"Create reference failed: {ref.text}"

    def test_07_list_corrective_actions(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/corrective-actions", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or r.json()
        assert isinstance(data, list)
        assert len(data) >= 1

    def test_08_update_ca_status_assessment(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/corrective-actions/{self.ca_id}/status", headers=admin_headers,
                         json={"status": "assessment"})
        assert r.status_code == 200

    def test_09_update_ca_with_root_cause(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/corrective-actions/{self.ca_id}", headers=admin_headers, json={
            "root_cause": "No automated reminders for access review deadlines",
            "notes": "## Action plan\n\nImplement automated review task creation via isms server manager",
        })
        assert r.status_code == 200, f"Failed: {r.text}"

    def test_10_progress_to_implementation(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/corrective-actions/{self.ca_id}/status", headers=admin_headers,
                         json={"status": "implementation"})
        assert r.status_code == 200

    def test_11_resolve_ca(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/corrective-actions/{self.ca_id}/status", headers=admin_headers,
                         json={"status": "resolved"})
        assert r.status_code == 200

    def test_12_close_finding(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/audit-findings/{self.finding_id}/status", headers=admin_headers,
                         json={"status": "closed"})
        assert r.status_code == 200, f"Failed: {r.text}"

    def test_13_complete_audit(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/audits/{self.audit_id}/status", headers=admin_headers,
                         json={"status": "completed"})
        assert r.status_code == 200


class TestCorrectiveActionSeeding:
    def test_create_seeds_notes_template(self, api_url, admin_headers):
        """When notes is empty on create, it's seeded with the action plan template."""
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Seeding test CA",
            "description": "Verify default notes template",
            "source": "other",
            "severity": "observation",
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        notes = data.get("notes", "")
        assert "## Action plan" in notes
        assert "## Implementation" in notes
        assert "## Verification" in notes
        assert "## Evidence" in notes


class TestCorrectiveActionRBAC:
    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=reader_headers, json={
            "title": "Should fail",
            "description": "Reader should not create CAs",
            "source": "other",
            "severity": "observation",
        })
        assert r.status_code == 403

    def test_reader_can_read(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/corrective-actions", headers=reader_headers)
        assert r.status_code == 200


class TestAuditFindingCorrectness:
    """Server-side correctness — reopen logic, partial updates, aggregate list."""

    programme_id = None
    audit_id = None
    finding_id = None

    def _ensure_finding(self, api_url, admin_headers):
        if self.programme_id is None:
            r = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
                "title": "Correctness programme", "year": 2026,
            })
            assert r.status_code in (200, 201), r.text
            TestAuditFindingCorrectness.programme_id = r.json()["id"]
        if self.audit_id is None:
            r = requests.post(f"{api_url}/audits", headers=admin_headers, json={
                "programme_id": self.programme_id,
                "title": "Correctness audit", "scope": "iso27001-*",
                "auditor": ADMIN_EMAIL, "audit_type": "internal",
                "planned_date": int(time.time()),
            })
            assert r.status_code in (200, 201), r.text
            TestAuditFindingCorrectness.audit_id = r.json()["id"]
            requests.put(f"{api_url}/audits/{self.audit_id}/status", headers=admin_headers, json={"status": "in_progress"})
        if self.finding_id is None:
            r = requests.post(f"{api_url}/audit-findings", headers=admin_headers, json={
                "audit_id": self.audit_id,
                "finding_type": "minor_nc",
                "title": "Reopen-test finding",
                "description": "Initial description",
            })
            assert r.status_code in (200, 201), r.text
            TestAuditFindingCorrectness.finding_id = r.json()["id"]

    def test_00_create_seeds_corrective_action_heading(self, api_url, admin_headers):
        """When description is empty on create, it's seeded with the Corrective Action heading."""
        self._ensure_finding(api_url, admin_headers)
        r = requests.post(f"{api_url}/audit-findings", headers=admin_headers, json={
            "audit_id": self.audit_id,
            "finding_type": "observation",
            "title": "Seed test finding",
        })
        assert r.status_code in (200, 201), r.text
        assert "## Corrective Action" in r.json().get("description", "")

    def test_01_invalid_finding_type_rejected(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        r = requests.post(f"{api_url}/audit-findings", headers=admin_headers, json={
            "audit_id": self.audit_id,
            "finding_type": "non_conformity",  # not in allowlist
            "title": "Bad type",
            "description": "Should be rejected",
        })
        assert r.status_code == 400, r.text

    def test_02_invalid_status_rejected(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        r = requests.put(f"{api_url}/audit-findings/{self.finding_id}", headers=admin_headers, json={
            "status": "in_remediation",
        })
        assert r.status_code == 400, r.text

    def test_03_close_then_reopen_clears_metadata(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        # Close
        r = requests.put(f"{api_url}/audit-findings/{self.finding_id}", headers=admin_headers, json={"status": "closed"})
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["status"] == "closed"
        assert body.get("closed_at"), "closed_at must be stamped on close"
        assert body.get("closed_by"), "closed_by must be set on close"
        # Reopen — the bug we fixed: closed_at/closed_by must be cleared.
        r = requests.put(f"{api_url}/audit-findings/{self.finding_id}", headers=admin_headers, json={"status": "open"})
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["status"] == "open"
        assert not body.get("closed_at"), f"closed_at should be cleared on reopen, got {body.get('closed_at')}"
        assert not body.get("closed_by"), f"closed_by should be cleared on reopen, got {body.get('closed_by')}"

    def test_04_partial_update_description_with_corrective_action_heading(self, api_url, admin_headers):
        # corrective_action column was dropped; corrective action content now lives
        # in description under a ## Corrective Action heading. Verify partial update
        # of description still works.
        self._ensure_finding(api_url, admin_headers)
        body = "Issue summary.\n\n## Corrective Action\n\nDo X, Y, Z."
        r = requests.put(f"{api_url}/audit-findings/{self.finding_id}", headers=admin_headers, json={
            "description": body,
        })
        assert r.status_code == 200, r.text
        assert "## Corrective Action" in r.json().get("description", "")

    def test_05_get_finding_endpoint(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        r = requests.get(f"{api_url}/audit-findings/{self.finding_id}", headers=admin_headers)
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["id"] == self.finding_id
        assert body.get("audit_title"), "audit_title should be populated for the detail view"

    def test_06_aggregate_findings_list(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        # Default list
        r = requests.get(f"{api_url}/audit/findings", headers=admin_headers)
        assert r.status_code == 200, r.text
        body = r.json()
        assert "data" in body and "total" in body
        # Status filter
        r = requests.get(f"{api_url}/audit/findings?status=open", headers=admin_headers)
        assert r.status_code == 200
        for f in r.json()["data"]:
            assert f["status"] == "open"
        # Bogus status filter
        r = requests.get(f"{api_url}/audit/findings?status=invalid", headers=admin_headers)
        assert r.status_code == 400

    def test_07_cross_org_audit_id_rejected(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        # Audit id 999999 doesn't belong to this org.
        r = requests.post(f"{api_url}/audit-findings", headers=admin_headers, json={
            "audit_id": 999999,
            "finding_type": "minor_nc",
            "title": "Cross-org attempt",
            "description": "Should be rejected",
        })
        assert r.status_code in (400, 404), r.text

    def test_08_cannot_delete_finding_with_open_ca(self, api_url, admin_headers):
        # Re-create a finding with linked CA
        r = requests.post(f"{api_url}/audit-findings", headers=admin_headers, json={
            "audit_id": self.audit_id,
            "finding_type": "minor_nc",
            "title": "Finding with CA",
            "description": "Will get a CA attached",
        })
        assert r.status_code in (200, 201), r.text
        fid = r.json()["id"]
        ca = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Block-delete CA",
            "description": "This CA blocks deletion of its finding",
            "source": "internal_audit",
            "severity": "minor_nc",
            "assignee": ADMIN_EMAIL,
        })
        if ca.status_code not in (200, 201):
            # CA creation failed, can't run the rest
            return
        ca_body = ca.json()
        # Link CA → audit finding via entity_references
        ref = requests.post(f"{api_url}/references", headers=admin_headers, json={
            "source_type": "corrective_action",
            "source_id": ca_body["identifier"],
            "target_type": "audit_finding",
            "target_id": f"FIND-{fid}",
        })
        assert ref.status_code in (200, 201), ref.text
        r = requests.delete(f"{api_url}/audit-findings/{fid}", headers=admin_headers)
        assert r.status_code == 409, r.text

    def test_09_audit_update_endpoint(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        # PUT /audits/:id should accept partial updates and persist them
        r = requests.put(f"{api_url}/audits/{self.audit_id}", headers=admin_headers, json={
            "summary": "Audit summary written via PUT",
            "notes": "Some internal notes",
        })
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["summary"] == "Audit summary written via PUT"
        assert body["notes"] == "Some internal notes"

    def test_10_audit_changelog_emits_on_update(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        # Update writes to entity_changelog under entity_type=audit.
        r = requests.put(f"{api_url}/audits/{self.audit_id}", headers=admin_headers, json={
            "summary": "Changelog probe " + str(time.time()),
        })
        assert r.status_code == 200, r.text
        # Read the changelog
        r = requests.get(f"{api_url}/changelog/audit/{self.audit_id}", headers=admin_headers)
        assert r.status_code == 200, r.text
        entries = r.json().get("data") or []
        # We should have at least one summary-update entry
        summary_entries = [e for e in entries if e.get("field") == "summary"]
        assert len(summary_entries) >= 1, f"expected summary changelog entry, got {entries}"

    def test_11_audit_supports_comments(self, api_url, admin_headers):
        self._ensure_finding(api_url, admin_headers)
        # entity_type=audit must be allowed by the comments allowlist.
        r = requests.post(f"{api_url}/entity-comments", headers=admin_headers, json={
            "entity_type": "audit",
            "entity_id": str(self.audit_id),
            "body": "Test comment on audit",
        })
        assert r.status_code in (200, 201), r.text
        # And listing returns it
        r = requests.get(f"{api_url}/entity-comments/audit/{self.audit_id}", headers=admin_headers)
        assert r.status_code == 200, r.text
