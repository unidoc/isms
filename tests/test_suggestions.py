"""Suggestion proposal tests.

Tests the paragraph-level suggestion workflow:
1. Reviewer creates suggestion with proposed text
2. Author accepts suggestion → text applied to review branch
3. Author rejects suggestion → document unchanged
4. Authorization: only author/admin can accept/reject
5. Double-accept blocked
"""
import requests
from conftest import READER_EMAIL, ADMIN_EMAIL


class TestSuggestionWorkflow:
    """Full suggestion lifecycle: create → accept → verify."""

    review_id = None
    suggestion_id = None

    def test_01_setup_review(self, api_url, admin_headers):
        """Create a document with known content and send for review."""
        r = requests.put(f"{api_url}/documents/iso27001-6-1/content",
                         headers=admin_headers,
                         json={"content": "# Planning\n\nThis is the original paragraph.\n\nAnother paragraph here."})
        assert r.status_code == 200, f"Edit failed: {r.text}"
        r = requests.post(f"{api_url}/documents/iso27001-6-1/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "Suggestion test"})
        assert r.status_code in [200, 201], f"Create review failed: {r.text}"
        TestSuggestionWorkflow.review_id = r.json()["review_id"]

    def test_02_reviewer_creates_suggestion(self, api_url, reader_headers):
        """Reviewer proposes replacement text for a paragraph."""
        rid = TestSuggestionWorkflow.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/comment",
                          headers=reader_headers,
                          json={
                              "body": "Suggested replacement for this paragraph",
                              "suggestion_body": "This is the improved paragraph with better wording.",
                              "paragraph_index": 1,
                              "paragraph_hash": "test",
                              "quote": "This is the original paragraph.",
                          })
        assert r.status_code == 201, f"Create suggestion failed: {r.text}"
        data = r.json()
        assert data.get("suggestion_body") == "This is the improved paragraph with better wording."
        assert data.get("suggestion_status") == "pending"
        TestSuggestionWorkflow.suggestion_id = data["id"]

    def test_03_suggestion_in_list(self, api_url, admin_headers):
        """Suggestion appears in the suggestions list."""
        rid = TestSuggestionWorkflow.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/suggestions", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert len(data) >= 1
        assert any(s["id"] == TestSuggestionWorkflow.suggestion_id for s in data)

    def test_04_reviewer_cannot_accept(self, api_url, reader_headers):
        """Reviewer (not author) cannot accept their own suggestion."""
        sid = TestSuggestionWorkflow.suggestion_id
        r = requests.post(f"{api_url}/comments/{sid}/accept", headers=reader_headers)
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"

    def test_05_admin_accepts_suggestion(self, api_url, admin_headers):
        """Author (admin) accepts the suggestion → text applied to review branch."""
        sid = TestSuggestionWorkflow.suggestion_id
        r = requests.post(f"{api_url}/comments/{sid}/accept", headers=admin_headers)
        assert r.status_code == 200, f"Accept failed: {r.text}"
        assert r.json()["status"] == "accepted"
        assert "commit" in r.json()
        assert "branch" in r.json()

    def test_06_suggestion_status_accepted(self, api_url, admin_headers):
        """After accept, suggestion_status is 'accepted' and comment is resolved."""
        rid = TestSuggestionWorkflow.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/suggestions", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [s for s in data if s["id"] == TestSuggestionWorkflow.suggestion_id]
        assert len(match) == 1
        assert match[0]["suggestion_status"] == "accepted"
        assert match[0]["status"] == "resolved"

    def test_07_document_content_updated(self, api_url, admin_headers):
        """Review branch content should contain the accepted suggestion text."""
        rid = TestSuggestionWorkflow.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/content", headers=admin_headers)
        assert r.status_code == 200
        body = r.json()["body"]
        assert "improved paragraph with better wording" in body, \
            f"Accepted suggestion text not found in review branch: {body[:200]}"
        assert "original paragraph" not in body, \
            "Original paragraph should have been replaced"

    def test_08_duplicate_accept_fails(self, api_url, admin_headers):
        """Cannot accept an already-accepted suggestion."""
        sid = TestSuggestionWorkflow.suggestion_id
        r = requests.post(f"{api_url}/comments/{sid}/accept", headers=admin_headers)
        assert r.status_code == 400, f"Expected 400 for duplicate accept, got {r.status_code}"


class TestSuggestionReject:
    """Reject workflow: document stays unchanged."""

    review_id = None
    suggestion_id = None

    def test_01_setup(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/documents/iso27001-6-2/content",
                         headers=admin_headers,
                         json={"content": "# Roles\n\nOriginal text here.\n\nSecond paragraph."})
        assert r.status_code == 200
        r = requests.post(f"{api_url}/documents/iso27001-6-2/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "Reject test"})
        assert r.status_code in [200, 201]
        TestSuggestionReject.review_id = r.json()["review_id"]

    def test_02_create_suggestion(self, api_url, reader_headers):
        rid = TestSuggestionReject.review_id
        r = requests.post(f"{api_url}/reviews/{rid}/comment",
                          headers=reader_headers,
                          json={
                              "body": "Suggested edit",
                              "suggestion_body": "Replacement text that should NOT be applied.",
                              "paragraph_index": 1,
                              "quote": "Original text here.",
                          })
        assert r.status_code == 201
        TestSuggestionReject.suggestion_id = r.json()["id"]

    def test_03_reject_suggestion(self, api_url, admin_headers):
        sid = TestSuggestionReject.suggestion_id
        r = requests.post(f"{api_url}/comments/{sid}/reject", headers=admin_headers)
        assert r.status_code == 200
        assert r.json()["status"] == "rejected"

    def test_04_document_unchanged(self, api_url, admin_headers):
        """Document should still have original text after rejection."""
        rid = TestSuggestionReject.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/content", headers=admin_headers)
        assert r.status_code == 200
        body = r.json()["body"]
        assert "Original text here" in body
        assert "Replacement text" not in body

    def test_05_suggestion_status_rejected(self, api_url, admin_headers):
        rid = TestSuggestionReject.review_id
        r = requests.get(f"{api_url}/reviews/{rid}/suggestions", headers=admin_headers)
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [s for s in data if s["id"] == TestSuggestionReject.suggestion_id]
        assert match[0]["suggestion_status"] == "rejected"
        # Comment should still be open (not auto-resolved on reject)
        assert match[0]["status"] == "open"


class TestEntitySuggestionCreate:
    """Entity suggestion: create a risk via suggestion → apply → risk exists."""

    suggestion_id = None

    def test_01_create_risk_suggestion(self, api_url, admin_headers):
        """Create a suggestion to add a new risk."""
        r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "risk",
            "suggestion_type": "create",
            "title": "Phishing attack risk",
            "rationale": "Recent phishing attempts detected",
            "payload": {
                "title": "Phishing attack risk",
                "description": "Risk of successful phishing attack on employees",
                "category": "technology",
                "risk_type": "threat",
                "origin": "external",
            },
        })
        assert r.status_code in [200, 201], f"Create suggestion failed: {r.text}"
        data = r.json()
        assert data.get("id"), "Expected suggestion ID"
        assert data.get("status") == "open"
        TestEntitySuggestionCreate.suggestion_id = data["id"]

    def test_02_suggestion_in_list(self, api_url, admin_headers):
        """Suggestion appears in the suggestions list."""
        r = requests.get(f"{api_url}/suggestions?status=open", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert any(s["id"] == TestEntitySuggestionCreate.suggestion_id for s in (data or []))

    def test_03_apply_creates_risk(self, api_url, admin_headers):
        """Applying the suggestion creates the risk."""
        sid = TestEntitySuggestionCreate.suggestion_id
        r = requests.post(f"{api_url}/suggestions/{sid}/apply", headers=admin_headers, json={})
        assert r.status_code == 200, f"Apply failed: {r.text}"
        data = r.json()
        assert data.get("status") == "applied", f"Expected applied, got: {data}"
        assert data.get("applied_entity_id"), "Expected applied_entity_id"

    def test_04_risk_exists(self, api_url, admin_headers):
        """The created risk should exist in the risk register."""
        r = requests.get(f"{api_url}/risks?q=Phishing&limit=200", headers=admin_headers)
        assert r.status_code == 200
        risks = r.json() if isinstance(r.json(), list) else r.json().get("data", [])
        phishing = [ri for ri in risks if "Phishing" in ri.get("title", "")]
        assert len(phishing) >= 1, "Risk created by suggestion not found"

    def test_05_suggestion_status_applied(self, api_url, admin_headers):
        """Suggestion status should be 'applied' after apply."""
        sid = TestEntitySuggestionCreate.suggestion_id
        r = requests.get(f"{api_url}/suggestions?status=applied", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [s for s in (data or []) if s["id"] == sid]
        assert len(match) == 1
        assert match[0]["status"] == "applied"

    def test_06_duplicate_apply_fails(self, api_url, admin_headers):
        """Applying an already-applied suggestion should fail."""
        sid = TestEntitySuggestionCreate.suggestion_id
        r = requests.post(f"{api_url}/suggestions/{sid}/apply", headers=admin_headers, json={})
        assert r.status_code in [400, 409], f"Expected 400/409 for duplicate apply, got {r.status_code}"


class TestEntitySuggestionReject:
    """Entity suggestion: create → reject with optional reason."""

    suggestion_id = None

    def test_01_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "incident",
            "suggestion_type": "create",
            "title": "Rejected incident suggestion",
            "rationale": "Testing rejection",
            "payload": {"title": "Test incident", "description": "Should be rejected"},
        })
        assert r.status_code in [200, 201], f"Create failed: {r.text}"
        TestEntitySuggestionReject.suggestion_id = r.json()["id"]

    def test_02_reject_with_reason(self, api_url, admin_headers):
        sid = TestEntitySuggestionReject.suggestion_id
        r = requests.post(f"{api_url}/suggestions/{sid}/reject", headers=admin_headers,
                          json={"reason": "Not relevant"})
        assert r.status_code == 200, f"Reject failed: {r.text}"

    def test_03_status_rejected(self, api_url, admin_headers):
        sid = TestEntitySuggestionReject.suggestion_id
        r = requests.get(f"{api_url}/suggestions?status=rejected", headers=admin_headers)
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [s for s in (data or []) if s["id"] == sid]
        assert len(match) == 1
        assert match[0]["status"] == "rejected"
        assert match[0].get("reject_reason") == "Not relevant"


class TestEntitySuggestionMinimalPayload:
    """Entity suggestion with minimal payload — title auto-populated from suggestion title."""

    suggestion_id = None

    def test_01_create_with_empty_payload(self, api_url, admin_headers):
        """Suggestion with no payload fields — backend should auto-populate title."""
        r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "risk",
            "suggestion_type": "create",
            "title": "Auto-populated risk title",
            "rationale": "Testing auto-populate",
            "payload": {},
        })
        assert r.status_code in [200, 201], f"Create failed: {r.text}"
        TestEntitySuggestionMinimalPayload.suggestion_id = r.json()["id"]

    def test_02_apply_succeeds(self, api_url, admin_headers):
        """Apply should succeed — backend auto-populates payload from suggestion title."""
        sid = TestEntitySuggestionMinimalPayload.suggestion_id
        r = requests.post(f"{api_url}/suggestions/{sid}/apply", headers=admin_headers, json={})
        assert r.status_code == 200, f"Apply with minimal payload failed: {r.text}"
        assert r.json().get("applied_entity_id"), "Expected entity to be created"

    def test_03_risk_has_correct_title(self, api_url, admin_headers):
        """The created risk should have the suggestion title."""
        r = requests.get(f"{api_url}/risks?q=Auto-populated&limit=200", headers=admin_headers)
        risks = r.json() if isinstance(r.json(), list) else r.json().get("data", [])
        match = [ri for ri in risks if ri.get("title") == "Auto-populated risk title"]
        assert len(match) >= 1, "Risk with auto-populated title not found"
