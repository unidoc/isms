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

    def test_suggestion_apply_status_stamps_approved(self, api_url, admin_headers):
        """#26 slice C: status via suggestion-apply derives approved_at/by like HTTP."""
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Approve via suggestion", "description": "x"})
        assert r.status_code == 201, r.text
        cid = r.json()["id"]
        sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "change_request", "suggestion_type": "update",
            "entity_id": str(cid),
            "payload": {"fields": {"status": "approved"}},
            "rationale": "ready", "title": "approve"})
        assert sg.status_code in (200, 201), sg.text
        ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                           headers=admin_headers, json={})
        assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
        got = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
        assert got["status"] == "approved", got
        assert got.get("approved_at"), "apply→approved must stamp approved_at like HTTP"

    def test_suggestion_apply_approval_creates_followup_task(self, api_url, admin_headers):
        """#26 finding 1: approving via suggestion-apply must auto-create the
        'Implement <CR>' follow-up task, exactly like the HTTP status endpoint —
        the two paths must not diverge on this side effect."""
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Approve creates followup", "description": "x"})
        assert r.status_code == 201, r.text
        cid = r.json()["id"]
        ident = r.json()["identifier"]
        sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "change_request", "suggestion_type": "update",
            "entity_id": str(cid),
            "payload": {"fields": {"status": "approved"}},
            "rationale": "ready", "title": "approve"})
        assert sg.status_code in (200, 201), sg.text
        ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                           headers=admin_headers, json={})
        assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
        # Search by the CR identifier so pagination on the persistent stack can't hide it.
        tr = requests.get(f"{api_url}/tasks",
                          headers=admin_headers,
                          params={"q": ident, "task_type": "change_followup"})
        assert tr.status_code == 200, tr.text
        tasks = tr.json().get("data") or []
        followups = [t for t in tasks if ident in (t.get("title") or "")]
        assert followups, f"approve via suggestion-apply must create the Implement {ident} task"

    def test_suggestion_apply_status_clears_approved_on_reverse_transition(self, api_url, admin_headers):
        """#26 finding 2: a reverse transition via suggestion-apply clears
        approved_at/by, matching UpdateChangeRequestStatusTx's clearApproved branch —
        locks in parity between the hand-duplicated tx and pool status functions."""
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Reverse via suggestion", "description": "x"})
        assert r.status_code == 201, r.text
        cid = r.json()["id"]

        def apply_status(status):
            sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
                "entity_type": "change_request", "suggestion_type": "update",
                "entity_id": str(cid),
                "payload": {"fields": {"status": status}},
                "rationale": status, "title": status})
            assert sg.status_code in (200, 201), sg.text
            ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                               headers=admin_headers, json={})
            assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text

        apply_status("approved")
        got = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
        assert got["status"] == "approved" and got.get("approved_at"), got

        apply_status("rejected")
        got = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
        assert got["status"] == "rejected", got
        assert not got.get("approved_at"), "reverse transition must clear approved_at"
        assert not got.get("approved_by"), "reverse transition must clear approved_by"

    def test_suggestion_apply_change_type(self, api_url, admin_headers):
        """The suggestion-apply path (agents/MCP) can reclassify type too."""
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": "Reclassify via suggestion", "description": "x"})
        assert r.status_code == 201, r.text
        cid = r.json()["id"]
        sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "change_request", "suggestion_type": "update",
            "entity_id": str(cid),
            "payload": {"fields": {"type": "access_request"}},
            "rationale": "misclassified", "title": "reclassify"})
        assert sg.status_code in (200, 201), sg.text
        ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                           headers=admin_headers, json={})
        assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
        r = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers)
        assert r.json()["type"] == "access_request", r.text

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


class TestChangeStatusTransitionGuard:
    """#423: a change request could move to any status regardless of its
    current one — proposed straight to implemented, nobody ever approving it.
    Covers all three ways status can change: the dedicated status endpoint,
    the edit-form PUT, and suggestion-apply."""

    def _create(self, api_url, admin_headers, title):
        r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
            "title": title, "description": "x"})
        assert r.status_code == 201, r.text
        return r.json()["id"]

    def test_status_endpoint_rejects_skipping_approval(self, api_url, admin_headers):
        cid = self._create(api_url, admin_headers, "Skip approval via status endpoint")
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "implemented"})
        assert r.status_code == 409, r.text
        body = r.json()
        assert body["code"] == "change_invalid_transition"
        assert body["params"]["status"] == "proposed"
        assert body["params"]["value"] == "implemented"
        got = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
        assert got["status"] == "proposed", "rejected transition must not write"

    def test_status_endpoint_rejects_proposed_to_in_progress(self, api_url, admin_headers):
        """Approval is not optional — in_progress needs an approved change first."""
        cid = self._create(api_url, admin_headers, "Skip approval to in_progress")
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "in_progress"})
        assert r.status_code == 409, r.text

    def test_edit_form_rejects_skipping_approval(self, api_url, admin_headers):
        """The PUT edit-form endpoint shares the same guard as the status endpoint (#200)."""
        cid = self._create(api_url, admin_headers, "Skip approval via edit form")
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers, json={
            "title": "Skip approval via edit form", "description": "x", "status": "implemented"})
        assert r.status_code == 409, r.text
        assert r.json()["code"] == "change_invalid_transition"

    def test_suggestion_apply_rejects_skipping_approval(self, api_url, admin_headers):
        """Suggestion-apply shares prepareChangeUpdate too — it must refuse the
        same jump, and the suggestion must stay open rather than silently applying."""
        cid = self._create(api_url, admin_headers, "Skip approval via suggestion")
        sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "change_request", "suggestion_type": "update",
            "entity_id": str(cid),
            "payload": {"fields": {"status": "implemented"}},
            "rationale": "jump the queue", "title": "skip approval"})
        assert sg.status_code in (200, 201), sg.text
        sg_id = sg.json()["id"]
        ap = requests.post(f"{api_url}/suggestions/{sg_id}/apply", headers=admin_headers, json={})
        assert ap.status_code == 409, ap.text
        assert ap.json()["code"] == "change_invalid_transition"
        got = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
        assert got["status"] == "proposed"
        sug = requests.get(f"{api_url}/suggestions/{sg_id}", headers=admin_headers).json()["data"]
        assert sug["status"] in ("open", "in_review"), "a refused apply must not mark the suggestion applied"

    def test_rejected_is_terminal(self, api_url, admin_headers):
        cid = self._create(api_url, admin_headers, "Rejected is terminal")
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "rejected"})
        assert r.status_code == 200, r.text
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "approved"})
        assert r.status_code == 409, r.text

    def test_closed_is_terminal(self, api_url, admin_headers):
        cid = self._create(api_url, admin_headers, "Closed is terminal")
        for status in ("approved", "implemented", "closed"):
            r = requests.put(f"{api_url}/changes/{cid}/status",
                              headers=admin_headers, json={"status": status})
            assert r.status_code == 200, r.text
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "implemented"})
        assert r.status_code == 409, r.text

    def test_same_status_is_a_noop(self, api_url, admin_headers):
        """Re-sending the current status (what the web edit form always does)
        must never 409, even on a terminal status, and must not re-stamp the
        approval it already has (alip/F2 — the single-user slice of it; the
        distinct-approver-overwritten case needs two authenticated org
        members and is covered at the Go/pg level instead, see
        TestChangeStatusEndpointSameStatusPreservesOriginalApprover)."""
        cid = self._create(api_url, admin_headers, "Same status is a no-op")
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "approved"})
        assert r.status_code == 200, r.text
        first = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
        assert first.get("approved_at") and first.get("approved_by"), first

        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "approved"})
        assert r.status_code == 200, r.text
        second = requests.get(f"{api_url}/changes/{cid}", headers=admin_headers).json()
        assert second["approved_at"] == first["approved_at"], \
            "re-sending the current status must not re-stamp approved_at"
        assert second["approved_by"] == first["approved_by"]

        for status in ("implemented", "closed"):
            r = requests.put(f"{api_url}/changes/{cid}/status",
                              headers=admin_headers, json={"status": status})
            assert r.status_code == 200, r.text
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "closed"})
        assert r.status_code == 200, r.text

    def test_create_cannot_skip_approval(self, api_url, admin_headers):
        """alip/F1: a change could be created already approved/implemented/
        closed, skipping the whole update-path guard above -- the #423
        symptom, one request earlier."""
        for status in ("approved", "in_progress", "implemented", "closed", "rejected"):
            r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
                "title": f"Born {status}", "description": "x", "status": status})
            assert r.status_code == 409, f"{status}: {r.text}"
            assert r.json()["code"] == "change_invalid_transition"

    def test_create_suggestion_cannot_skip_approval(self, api_url, admin_headers):
        """The same create-time guard applies to a suggestion's create
        payload (alip/F1) -- the MCP create_suggestion tool documents it as
        accepting the same fields as the REST POST."""
        sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "change_request", "suggestion_type": "create",
            "payload": {"title": "Born via suggestion", "description": "x", "status": "implemented"},
            "rationale": "jump the queue", "title": "skip approval"})
        assert sg.status_code in (200, 201), sg.text
        sg_id = sg.json()["id"]
        ap = requests.post(f"{api_url}/suggestions/{sg_id}/apply", headers=admin_headers, json={})
        assert ap.status_code == 409, ap.text
        assert ap.json()["code"] == "change_invalid_transition"
        sug = requests.get(f"{api_url}/suggestions/{sg_id}", headers=admin_headers).json()["data"]
        assert sug["status"] in ("open", "in_review"), "a refused apply must not mark the suggestion applied"

    def test_approved_can_skip_straight_to_implemented(self, api_url, admin_headers):
        """An in_progress step is not mandatory."""
        cid = self._create(api_url, admin_headers, "Skip in_progress")
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "approved"})
        assert r.status_code == 200, r.text
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=admin_headers, json={"status": "implemented"})
        assert r.status_code == 200, r.text

    def test_reader_cannot_use_rbac_to_bypass_the_guard(self, api_url, reader_headers, admin_headers):
        """#24: the status endpoint is manager/admin-only regardless of what
        status is requested — RBAC is checked before the transition guard."""
        cid = self._create(api_url, admin_headers, "RBAC before transition")
        r = requests.put(f"{api_url}/changes/{cid}/status",
                          headers=reader_headers, json={"status": "implemented"})
        assert r.status_code == 403, r.text


class TestChangeApprovalLifecycle:
    """#197: an approval survives the move into work and is withdrawn only when
    the approved content is rewritten (or the change is sent back / rejected)."""

    WITHDRAWN_REASON = "approval withdrawn: approved content was edited"

    def _approved_change(self, api_url, headers, title="Approval lifecycle"):
        r = requests.post(f"{api_url}/changes", headers=headers, json={
            "title": title, "description": "original description",
            "justification": "original justification", "risk_level": "low",
            "rollback_plan": "original rollback"})
        assert r.status_code == 201, r.text
        cid = r.json()["id"]
        self._set_status(api_url, headers, cid, "approved")
        got = self._get(api_url, headers, cid)
        assert got["status"] == "approved" and got.get("approved_at") and got.get("approved_by"), got
        return cid, got

    @staticmethod
    def _set_status(api_url, headers, cid, status):
        r = requests.put(f"{api_url}/changes/{cid}/status", headers=headers, json={"status": status})
        assert r.status_code == 200, r.text

    @staticmethod
    def _get(api_url, headers, cid):
        r = requests.get(f"{api_url}/changes/{cid}", headers=headers)
        assert r.status_code == 200, r.text
        return r.json()

    def test_in_progress_keeps_approval(self, api_url, admin_headers):
        cid, approved = self._approved_change(api_url, admin_headers)
        self._set_status(api_url, admin_headers, cid, "in_progress")
        got = self._get(api_url, admin_headers, cid)
        assert got["status"] == "in_progress"
        assert got.get("approved_by") == approved["approved_by"], got
        assert got.get("approved_at") == approved["approved_at"], got

    def test_edit_form_in_progress_keeps_approval(self, api_url, admin_headers):
        cid, approved = self._approved_change(api_url, admin_headers)
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers, json={"status": "in_progress"})
        assert r.status_code == 200, r.text
        got = self._get(api_url, admin_headers, cid)
        assert got["status"] == "in_progress"
        assert got.get("approved_by") == approved["approved_by"], got
        assert got.get("approved_at") == approved["approved_at"], got

    def test_reopen_keeps_implemented_at_and_approval(self, api_url, admin_headers):
        cid, approved = self._approved_change(api_url, admin_headers)
        self._set_status(api_url, admin_headers, cid, "implemented")
        implemented = self._get(api_url, admin_headers, cid)
        assert implemented.get("implemented_at"), implemented
        self._set_status(api_url, admin_headers, cid, "in_progress")
        got = self._get(api_url, admin_headers, cid)
        assert got["status"] == "in_progress"
        assert got.get("implemented_at") == implemented["implemented_at"], got
        assert got.get("approved_by") == approved["approved_by"], got
        assert got.get("approved_at") == approved["approved_at"], got

    def test_rewrite_withdraws_approval(self, api_url, admin_headers):
        cid, _ = self._approved_change(api_url, admin_headers)
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers, json={
            "description": "completely different change", "justification": "different",
            "risk_level": "critical", "rollback_plan": "none"})
        assert r.status_code == 200, r.text
        got = self._get(api_url, admin_headers, cid)
        assert got["status"] == "proposed", got
        assert not got.get("approved_at") and not got.get("approved_by"), got
        r = requests.get(f"{api_url}/changelog/change_request/{cid}", headers=admin_headers)
        assert r.status_code == 200, r.text
        body = r.json()
        entries = body.get("data") if isinstance(body, dict) else body
        rows = [e for e in entries if e.get("field") == "status"
                and e.get("old_value") == "approved" and e.get("new_value") == "proposed"]
        assert rows, f"no status approved -> proposed row in {entries}"
        assert rows[0].get("reason") == self.WITHDRAWN_REASON, rows[0]

    def test_title_or_notes_edit_keeps_approval(self, api_url, admin_headers):
        cid, approved = self._approved_change(api_url, admin_headers)
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers, json={
            "title": "Retitled", "notes": "progress note"})
        assert r.status_code == 200, r.text
        got = self._get(api_url, admin_headers, cid)
        assert got["status"] == "approved", got
        assert got["title"] == "Retitled"
        assert got.get("approved_by") == approved["approved_by"], got
        assert got.get("approved_at") == approved["approved_at"], got

    def test_suggestion_rewrite_withdraws_approval(self, api_url, admin_headers):
        cid, _ = self._approved_change(api_url, admin_headers)
        sg = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "change_request", "suggestion_type": "update",
            "entity_id": str(cid),
            "payload": {"fields": {"description": "rewritten via suggestion"}},
            "rationale": "rewrite", "title": "rewrite"})
        assert sg.status_code in (200, 201), sg.text
        ap = requests.post(f"{api_url}/suggestions/{sg.json()['id']}/apply",
                           headers=admin_headers, json={})
        assert ap.status_code == 200 and ap.json().get("status") == "applied", ap.text
        got = self._get(api_url, admin_headers, cid)
        assert got["status"] == "proposed", got
        assert got["description"] == "rewritten via suggestion"
        assert not got.get("approved_at") and not got.get("approved_by"), got


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
