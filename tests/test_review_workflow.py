"""End-to-end review workflow tests.

Tests the full lifecycle:
1. Admin sends document for review
2. Reviewer sees diff
3. Reviewer approves
4. Admin merges
5. Document is approved
"""
import requests
from conftest import READER_EMAIL


class TestReviewWorkflow:
    """Full review lifecycle test."""

    review_id = None

    def test_01_send_for_review(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/iso27001-4-1/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "Please review"})
        assert r.status_code in [200, 201], f"Send for review failed: {r.text}"
        data = r.json()
        assert "review_id" in data
        TestReviewWorkflow.review_id = data["review_id"]

    def test_02_review_exists(self, api_url, admin_headers):
        rid = TestReviewWorkflow.review_id
        assert rid is not None, "No review ID from previous test"
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data["status"] == "open"
        assert data["document_id"] == "iso27001-4-1"

    def test_03_review_diff(self, api_url, admin_headers):
        rid = TestReviewWorkflow.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert "diff" in data
        # Diff may be empty for first review if no prior approved version exists
        assert "new_body" in data or "diff" in data

    def test_04_review_timeline(self, api_url, admin_headers):
        rid = TestReviewWorkflow.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/timeline", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert "data" in data
        assert len(data["data"]) >= 1

    def test_05_reviewer_approves(self, api_url, reader_headers):
        rid = TestReviewWorkflow.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "LGTM"})
        assert r.status_code == 200, f"Approve failed: {r.text}"
        assert r.json()["review_status"] == "approved"

    def test_06_review_approved(self, api_url, admin_headers):
        rid = TestReviewWorkflow.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.status_code == 200
        assert r.json()["status"] == "approved"

    def test_07_admin_merges(self, api_url, admin_headers):
        rid = TestReviewWorkflow.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 200, f"Merge failed: {r.text}"
        assert r.json()["status"] == "merged"

    def test_08_review_merged(self, api_url, admin_headers):
        rid = TestReviewWorkflow.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.status_code == 200
        assert r.json()["status"] == "merged"

    def test_09_duplicate_review_blocked(self, api_url, admin_headers):
        """Cannot create another review on same doc while one exists (even merged should allow)."""
        r = requests.post(f"{api_url}/documents/iso27001-4-2/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL]})
        # Should work since iso27001-4-2 has no open review
        assert r.status_code in [200, 201], f"Second review creation failed: {r.text}"


class TestChangesRequested:
    """Test the changes_requested -> edit -> re-approve flow."""

    review_id = None

    def test_01_send_for_review(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/iso27001-5-1/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL]})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestChangesRequested.review_id = r.json()["review_id"]

    def test_02_reviewer_requests_changes(self, api_url, reader_headers):
        rid = TestChangesRequested.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "changes_requested", "comment": "Needs more detail"})
        assert r.status_code == 200
        assert r.json()["review_status"] == "changes_requested"

    def test_03_review_status(self, api_url, admin_headers):
        rid = TestChangesRequested.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.json()["status"] == "changes_requested"

    def test_04_close_review(self, api_url, admin_headers):
        rid = TestChangesRequested.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/status",
                         headers=admin_headers, json={"status": "closed"})
        assert r.status_code == 200

    def test_05_review_closed(self, api_url, admin_headers):
        rid = TestChangesRequested.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.json()["status"] == "closed"


class TestReviewRoundResubmit:
    """Test full round semantics: changes_requested → edit → resubmit.

    Verifies:
    1. Resubmit reuses same review ID and updates sent_head
    2. Previous approvals/assignments are reset to pending
    3. Diff baseline uses the new sent_head, not the original
    """

    review_id = None
    original_sent_head = None

    def test_01_send_for_review(self, api_url, admin_headers):
        """Create initial review with original content."""
        # Write known content so we can verify diff later
        r = requests.put(f"{api_url}/documents/iso27001-5-2/content",
                         headers=admin_headers,
                         json={"content": "# Round Test\n\nOriginal content for round testing."})
        assert r.status_code == 200, f"Edit failed: {r.text}"

        r = requests.post(f"{api_url}/documents/iso27001-5-2/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Round 1"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestReviewRoundResubmit.review_id = r.json()["review_id"]

    def test_02_capture_sent_head(self, api_url, admin_headers):
        """Record original sent_head for comparison. Round must be 1."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.status_code == 200
        TestReviewRoundResubmit.original_sent_head = r.json()["sent_head"]
        assert TestReviewRoundResubmit.original_sent_head, "sent_head must be set"
        assert r.json()["round"] == 1, f"Initial review should be round 1, got {r.json()['round']}"

    def test_03_reviewer_requests_changes(self, api_url, reader_headers):
        """Reviewer requests changes — assignment goes to changes_requested."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "changes_requested", "comment": "Add more detail"})
        assert r.status_code == 200
        assert r.json()["review_status"] == "changes_requested"

    def test_04_verify_assignment_is_changes_requested(self, api_url, admin_headers):
        """Before resubmit, assignment must be changes_requested."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/assignments", headers=admin_headers)
        assert r.status_code == 200
        assignments = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert len(assignments) >= 1
        reviewer_asn = [a for a in assignments if a["reviewer"] == READER_EMAIL]
        assert len(reviewer_asn) == 1
        assert reviewer_asn[0]["status"] == "changes_requested", \
            f"Expected changes_requested, got {reviewer_asn[0]['status']}"

    def test_05_admin_edits_and_resubmits(self, api_url, admin_headers):
        """Admin edits content and resubmits — must reuse same review ID."""
        rid = TestReviewRoundResubmit.review_id

        # Edit the document with new content
        r = requests.put(f"{api_url}/documents/iso27001-5-2/content",
                         headers=admin_headers,
                         json={"content": "# Round Test\n\nRevised content after changes requested."})
        assert r.status_code == 200

        # Resubmit review
        r = requests.post(f"{api_url}/documents/iso27001-5-2/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL],
                                "message": "Round 2 — addressed feedback"})
        assert r.status_code in [200, 201], f"Resubmit failed: {r.text}"
        # Must reuse same review ID
        assert r.json()["review_id"] == rid, \
            f"Resubmit must reuse same review ID: expected {rid}, got {r.json()['review_id']}"

    def test_06_sent_head_and_round_updated(self, api_url, admin_headers):
        """After resubmit, sent_head must be updated and round incremented to 2."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.status_code == 200
        new_sent_head = r.json()["sent_head"]
        assert new_sent_head != TestReviewRoundResubmit.original_sent_head, \
            "sent_head must change after resubmit with new content"
        assert r.json()["status"] == "open", \
            f"Review should be back to open after resubmit, got {r.json()['status']}"
        assert r.json()["round"] == 2, \
            f"Round should be 2 after resubmit, got {r.json()['round']}"

    def test_07_assignments_reset_to_pending(self, api_url, admin_headers):
        """After resubmit, all assignments must be reset to pending."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/assignments", headers=admin_headers)
        assert r.status_code == 200
        assignments = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert len(assignments) >= 1
        for a in assignments:
            assert a["status"] == "pending", \
                f"Assignment for {a['reviewer']} should be pending after resubmit, got {a['status']}"

    def test_07b_timeline_has_round_annotations(self, api_url, admin_headers):
        """Timeline entries must be annotated with round numbers."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/timeline", headers=admin_headers)
        assert r.status_code == 200
        entries = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert len(entries) > 0
        # All entries should have a round field
        for e in entries:
            assert "round" in e, f"Timeline entry missing round: {e.get('type')} {e.get('action')}"
            assert e["round"] >= 1, f"Round must be >= 1, got {e['round']}"
        # Should have entries from both round 1 and round 2
        rounds = set(e["round"] for e in entries)
        assert 1 in rounds, "Should have round 1 entries"
        assert 2 in rounds, "Should have round 2 entries (after resubmit)"

    def test_08_diff_uses_new_baseline(self, api_url, admin_headers):
        """Diff must show changes from new sent_head, not the original round."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        # Since sent_head was captured AFTER the "Revised content" edit and
        # no further edits happened, old_body and new_body should be the same
        # (no diff from the new baseline). The diff should NOT show "Original content"
        # as a removal — that was the old round.
        old_body = data.get("old_body", "")
        new_body = data.get("new_body", "")
        # Both should contain the revised content (current state = sent state)
        assert "Revised content" in new_body or data.get("diff", "") == "", \
            "Diff should be based on the resubmitted version, not the original"
        # Must NOT show "Original content" as if it's the baseline
        if old_body:
            assert "Original content" not in old_body, \
                "old_body still uses the original round baseline — sent_head not updated"

    def test_08b_diff_all_changes(self, api_url, admin_headers):
        """Diff with ?from=commit_hash shows all changes since original baseline."""
        rid = TestReviewRoundResubmit.review_id
        # First get the commit_hash (original baseline) from the diff response
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        commit_hash = r.json().get("commit_hash", "")
        assert r.json()["round"] == 2, "Diff response should include round"

        if commit_hash:
            # Fetch full-review diff using commit_hash
            r2 = requests.get(f"{api_url}/reviews/{rid}/diff?from={commit_hash}",
                              headers=admin_headers)
            assert r2.status_code == 200
            # The "all changes" diff should show the original content as old
            # (before any edits in this review) and current as new
            assert r2.json().get("old_body") is not None
            assert r2.json().get("new_body") is not None

    def test_09_approve_and_merge(self, api_url, admin_headers, reader_headers):
        """Full cycle: approve and merge after resubmit."""
        rid = TestReviewRoundResubmit.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "Good now"})
        assert r.status_code == 200
        assert r.json()["review_status"] == "approved"
        # Verify approval context includes round
        assert r.json()["approved_count"] == 1
        assert r.json()["round"] == 2, f"Approve response should include round 2, got {r.json().get('round')}"
        assert not r.json().get("pending_reviewers"), \
            f"No reviewers should be pending, got {r.json().get('pending_reviewers')}"

        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 200, f"Merge failed: {r.text}"
        assert r.json()["status"] == "merged"

    def test_10_merged_content_correct(self, api_url, admin_headers):
        """After merge, document must have the revised content."""
        r = requests.get(f"{api_url}/documents/iso27001-5-2/body",
                         headers=admin_headers)
        assert r.status_code == 200
        assert "Revised content" in r.json()["body"], \
            "Merged document should have the round 2 content"


class TestProposedRevision:
    """Test proposed_revision as first-class decision type."""

    review_id = None

    def test_01_setup_review(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/documents/iso27001-5-3/content",
                         headers=admin_headers,
                         json={"content": "# Revision Test\n\nOriginal content."})
        assert r.status_code == 200
        r = requests.post(f"{api_url}/documents/iso27001-5-3/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "Revision test"})
        assert r.status_code in [200, 201]
        TestProposedRevision.review_id = r.json()["review_id"]

    def test_02_reviewer_proposes_revision(self, api_url, reader_headers):
        """proposed_revision is a valid decision type."""
        rid = TestProposedRevision.review_id
        # First edit the review branch
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=reader_headers,
                         json={"content": "# Revision Test\n\nRevised content by reviewer."})
        assert r.status_code == 200
        # Then submit proposed_revision decision
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "proposed_revision", "comment": "I rewrote section 2"})
        assert r.status_code == 200, f"proposed_revision failed: {r.text}"
        assert r.json()["review_status"] == "changes_requested"

    def test_03_assignment_is_proposed_revision(self, api_url, admin_headers):
        """Assignment status must be proposed_revision, not changes_requested."""
        rid = TestProposedRevision.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/assignments", headers=admin_headers)
        assert r.status_code == 200
        assignments = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [a for a in assignments if a["reviewer"] == READER_EMAIL]
        assert len(match) == 1
        assert match[0]["status"] == "proposed_revision", \
            f"Expected proposed_revision, got {match[0]['status']}"

    def test_04_review_status_is_changes_requested(self, api_url, admin_headers):
        """Review top-level status must be changes_requested, not proposed_revision."""
        rid = TestProposedRevision.review_id
        r = requests.get(f"{api_url}/reviews/{rid}", headers=admin_headers)
        assert r.status_code == 200
        assert r.json()["status"] == "changes_requested"

    def test_05_decision_record_exists(self, api_url, admin_headers):
        """Decision log must have a proposed_revision record."""
        rid = TestProposedRevision.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/decisions", headers=admin_headers)
        assert r.status_code == 200
        decisions = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        proposed = [d for d in decisions if d["decision"] == "proposed_revision"]
        assert len(proposed) >= 1, "Decision log should have proposed_revision entry"

    def test_06_duplicate_action_blocked(self, api_url, reader_headers):
        """Reviewer cannot act again in the same round."""
        rid = TestProposedRevision.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "try again"})
        assert r.status_code == 409, f"Expected 409, got {r.status_code}"

    def test_07_timeline_shows_proposed_revision(self, api_url, admin_headers):
        """Timeline must have explicit proposed_revision activity."""
        rid = TestProposedRevision.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/timeline", headers=admin_headers)
        assert r.status_code == 200
        entries = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        proposed = [e for e in entries if e.get("action") == "review_proposed_revision"
                    or e.get("decision") == "proposed_revision"]
        assert len(proposed) >= 1, "Timeline should have proposed_revision entry"

    def test_08_merge_blocked_after_proposed_revision(self, api_url, admin_headers):
        """Merge must be blocked when review is in changes_requested from proposed_revision."""
        rid = TestProposedRevision.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 400, f"Expected 400 (merge blocked), got {r.status_code}: {r.text}"

    def test_09_author_comments_and_resubmits(self, api_url, admin_headers):
        """Author can comment, edit, and resubmit after proposed_revision — new round."""
        rid = TestProposedRevision.review_id
        # Author comments
        r = requests.post(f"{api_url}/reviews/{rid}/comment",
                          headers=admin_headers,
                          json={"body": "Thanks John, I adjusted your revision slightly."})
        assert r.status_code == 201

        # Author edits on review branch (incorporating John's revision with tweaks)
        rid = TestProposedRevision.review_id
        r = requests.put(f"{api_url}/reviews/{rid}/content",
                         headers=admin_headers,
                         json={"content": "# Revision Test\n\nFinal content after discussion."})
        assert r.status_code == 200

        # Author resubmits — same review, round increments
        r = requests.post(f"{api_url}/documents/iso27001-5-3/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "Incorporated your revision with minor tweaks"})
        assert r.status_code in [200, 201], f"Resubmit failed: {r.text}"
        assert r.json()["review_id"] == rid, "Must reuse same review ID"
        assert r.json().get("round") == 2, f"Expected round 2, got {r.json().get('round')}"

    def test_10_reviewer_approves_round_2(self, api_url, admin_headers, reader_headers):
        """Reviewer approves in round 2 — merge now allowed."""
        rid = TestProposedRevision.review_id
        # Verify assignments reset to pending
        r = requests.get(f"{api_url}/reviews/{rid}/assignments", headers=admin_headers)
        assignments = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        for a in assignments:
            assert a["status"] == "pending", f"Assignment should be pending in new round, got {a['status']}"

        # Reviewer approves
        r = requests.post(f"{api_url}/reviews/{rid}/approve",
                          headers=reader_headers,
                          json={"decision": "approved", "comment": "Looks great now"})
        assert r.status_code == 200
        assert r.json()["review_status"] == "approved"

        # Merge succeeds
        r = requests.post(f"{api_url}/reviews/{rid}/merge",
                          headers=admin_headers, json={})
        assert r.status_code == 200, f"Merge failed: {r.text}"
        assert r.json()["status"] == "merged"

    def test_11_final_content_correct(self, api_url, admin_headers):
        """Document has the final content after the full cycle."""
        r = requests.get(f"{api_url}/documents/iso27001-5-3/body", headers=admin_headers)
        assert r.status_code == 200
        assert "Final content after discussion" in r.json()["body"]


class TestFirstReviewDiff:
    """First review of a new document must show full content as new, not 'no changes'."""

    def test_first_review_diff_shows_content(self, api_url, admin_headers):
        # Use a scaffolded document that hasn't been reviewed yet
        r = requests.put(f"{api_url}/documents/iso27001-a-5-31/content",
                         headers=admin_headers,
                         json={"content": "# Awareness\n\nSecurity awareness training program."})
        assert r.status_code == 200, f"Edit failed: {r.text}"

        # Send for review (first ever review of this document)
        r = requests.post(f"{api_url}/documents/iso27001-a-5-31/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "First review"})
        assert r.status_code in [200, 201]
        rid = r.json()["review_id"]

        # Diff must show the document as new content, not empty
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data.get("new_body"), "new_body must contain document content"
        assert "awareness training" in data["new_body"].lower(), \
            f"Diff should show full document content, got: {data.get('new_body', '')[:100]}"
        # old_body should be empty (no prior version)
        assert not data.get("old_body"), \
            f"old_body should be empty for first review, got: {data.get('old_body', '')[:100]}"


class TestStaleWarningScoping:
    """The 'updated_since_sent' stale warning must be scoped to the SPECIFIC FILE
    of the review under inspection — modifying any other file in the repo must
    NOT cause the warning to fire on an unrelated review.
    """

    review_id = None

    def test_01_create_review_on_first_doc(self, api_url, admin_headers):
        # Seed known content on the first document, then send for review.
        r = requests.put(f"{api_url}/documents/iso27001-7-4/content",
                         headers=admin_headers,
                         json={"content": "# Scope Test\n\nBaseline content for scope test."})
        assert r.status_code == 200, f"Edit failed: {r.text}"
        r = requests.post(f"{api_url}/documents/iso27001-7-4/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "Scope test"})
        assert r.status_code in [200, 201], f"Send for review failed: {r.text}"
        TestStaleWarningScoping.review_id = r.json()["review_id"]

    def test_02_baseline_not_stale(self, api_url, admin_headers):
        rid = TestStaleWarningScoping.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        # Right after sending, the file content matches sent_head — must not be stale.
        assert r.json().get("updated_since_sent") is False, \
            f"updated_since_sent should be False right after send, got {r.json().get('updated_since_sent')}"

    def test_03_modify_unrelated_file(self, api_url, admin_headers):
        # Modify a DIFFERENT document — this must not affect the review on iso27001-7-4.
        r = requests.put(f"{api_url}/documents/iso27001-7-5/content",
                         headers=admin_headers,
                         json={"content": "# Other Doc\n\nUnrelated change to a different file."})
        assert r.status_code == 200, f"Unrelated edit failed: {r.text}"

    def test_04_review_still_not_stale(self, api_url, admin_headers):
        rid = TestStaleWarningScoping.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        assert r.json().get("updated_since_sent") is False, (
            "Stale warning fired after an UNRELATED file was changed. "
            "updated_since_sent must compare the specific reviewed file, not repo HEAD."
        )

    def test_05_modify_reviewed_file_marks_stale(self, api_url, admin_headers):
        # Now actually modify the reviewed file — the warning SHOULD fire.
        r = requests.put(f"{api_url}/documents/iso27001-7-4/content",
                         headers=admin_headers,
                         json={"content": "# Scope Test\n\nBaseline content for scope test.\n\nAdded after send."})
        assert r.status_code == 200
        rid = TestStaleWarningScoping.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/diff", headers=admin_headers)
        assert r.status_code == 200
        assert r.json().get("updated_since_sent") is True, \
            "Stale warning must fire when the reviewed file IS modified after send."

    def test_06_cleanup(self, api_url, admin_headers):
        rid = TestStaleWarningScoping.review_id
        # Close so subsequent runs don't see a stale review.
        requests.put(f"{api_url}/reviews/{rid}/status",
                     headers=admin_headers, json={"status": "closed"})
