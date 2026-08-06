"""Comment authorization tests.

POST /comments takes a client-supplied review_id, so it must apply the same gate
as POST /reviews/:id/comment — otherwise it is a way around it. Resolving needs
its own gate, and commenting on a register entity is a contributor-and-above
write (#23).

The load-bearing case is test_05: an *assigned* reader must still be able to
comment, because assignment (not role) grants comment rights on a review. A
role-based fix would pass every refusal test here and still be wrong.
"""
import requests
from conftest import READER_EMAIL, CONTRIBUTOR_EMAIL


class TestReviewCommentAuthorization:
    """A reader who is not a participant cannot comment via either route."""

    review_id = None
    doc_id = "iso27001-a-8-11"

    def test_01_open_review_without_the_reader(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/{self.doc_id}/reviews",
                          headers=admin_headers,
                          json={"reviewers": [CONTRIBUTOR_EMAIL],
                                "message": "comment authz test"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestReviewCommentAuthorization.review_id = r.json()["review_id"]

    def test_02_gated_route_refuses_unassigned_reader(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/reviews/{self.review_id}/comment",
                          headers=reader_headers, json={"body": "should be refused"})
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"

    def test_03_generic_route_refuses_the_same_write(self, api_url, reader_headers):
        """The bypass: same intent, client-supplied review_id."""
        r = requests.post(f"{api_url}/comments", headers=reader_headers,
                          json={"document_id": self.doc_id,
                                "review_id": self.review_id,
                                "body": "injected via the generic route"})
        assert r.status_code == 403, f"Bypass still open: {r.status_code} {r.text}"

    def test_04_document_comment_without_review_id_still_allowed(self, api_url, reader_headers):
        """Readers may comment on documents — that is what the exemption is for."""
        r = requests.post(f"{api_url}/comments", headers=reader_headers,
                          json={"document_id": self.doc_id, "body": "plain document comment"})
        assert r.status_code == 201, f"Reader lost document commenting: {r.text}"
        assert r.json()["status"] == "open", "response should echo the stored status"


class TestAssignedReaderKeepsAccess:
    """Assignment grants comment rights to any role — the fix must not break this."""

    review_id = None
    doc_id = "iso27001-a-8-12"

    def test_01_open_review_assigning_the_reader(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/{self.doc_id}/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "assigned reader"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestAssignedReaderKeepsAccess.review_id = r.json()["review_id"]

    def test_02_assigned_reader_may_use_the_gated_route(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/reviews/{self.review_id}/comment",
                          headers=reader_headers, json={"body": "assigned reviewer comment"})
        assert r.status_code == 201, f"Assigned reader blocked: {r.text}"

    def test_03_assigned_reader_may_use_the_generic_route(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/comments", headers=reader_headers,
                          json={"document_id": self.doc_id,
                                "review_id": self.review_id,
                                "body": "assigned reviewer reply"})
        assert r.status_code == 201, f"Assigned reader blocked on generic route: {r.text}"

    def test_04_document_id_comes_from_the_review(self, api_url, reader_headers):
        """A mismatched document_id must not decide where the comment lands."""
        r = requests.post(f"{api_url}/comments", headers=reader_headers,
                          json={"document_id": "iso27001-4-1",
                                "review_id": self.review_id,
                                "body": "document_id should be overridden"})
        assert r.status_code == 201, r.text
        assert r.json()["document_id"] == self.doc_id


class TestMergedReviewIsClosed:
    """Comments must not land on a published review."""

    review_id = None
    doc_id = "iso27001-a-8-14"

    def test_01_open_approve_and_merge(self, api_url, admin_headers, reader_headers):
        r = requests.post(f"{api_url}/documents/{self.doc_id}/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "merge then comment"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        rid = r.json()["review_id"]
        TestMergedReviewIsClosed.review_id = rid

        assert requests.post(f"{api_url}/reviews/{rid}/approve", headers=reader_headers,
                             json={"decision": "approved", "comment": "ok"}).status_code == 200
        assert requests.post(f"{api_url}/reviews/{rid}/merge", headers=admin_headers,
                             json={}).status_code in [200, 201]

    def test_02_generic_route_rejects_comment_on_merged_review(self, api_url, admin_headers):
        """Even an admin cannot append to a merged review."""
        r = requests.post(f"{api_url}/comments", headers=admin_headers,
                          json={"document_id": self.doc_id,
                                "review_id": self.review_id,
                                "body": "comment on a merged review"})
        assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"
        assert "merged" in r.text


class TestResolveAuthorization:
    """Resolving closes someone's feedback — it needs its own gate."""

    review_id = None
    admin_comment_id = None
    reader_comment_id = None
    doc_id = "iso27001-a-8-15"

    def test_01_setup_review_with_comments(self, api_url, admin_headers, reader_headers):
        r = requests.post(f"{api_url}/documents/{self.doc_id}/reviews",
                          headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "resolve authz"})
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        TestResolveAuthorization.review_id = r.json()["review_id"]

        r = requests.post(f"{api_url}/reviews/{self.review_id}/comment",
                          headers=admin_headers, json={"body": "admin feedback"})
        assert r.status_code == 201, r.text
        TestResolveAuthorization.admin_comment_id = r.json()["id"]

        r = requests.post(f"{api_url}/reviews/{self.review_id}/comment",
                          headers=reader_headers, json={"body": "reader feedback"})
        assert r.status_code == 201, r.text
        TestResolveAuthorization.reader_comment_id = r.json()["id"]

    def test_02_author_may_resolve_own_comment(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/comments/{self.reader_comment_id}/resolve",
                          headers=reader_headers)
        assert r.status_code == 200, f"Author blocked from own comment: {r.text}"

    def test_03_participant_may_resolve_others_comment(self, api_url, reader_headers):
        """The reader is an assigned reviewer here, so resolving is legitimate."""
        r = requests.post(f"{api_url}/comments/{self.admin_comment_id}/resolve",
                          headers=reader_headers)
        assert r.status_code == 200, f"Participant blocked: {r.text}"

    def test_04_non_participant_reader_cannot_resolve(self, api_url, admin_headers, reader_headers):
        """A review the reader has nothing to do with."""
        r = requests.post(f"{api_url}/documents/iso27001-a-8-16/reviews",
                          headers=admin_headers,
                          json={"reviewers": [CONTRIBUTOR_EMAIL], "message": "not the reader's review"})
        assert r.status_code in [200, 201], r.text
        rid = r.json()["review_id"]

        r = requests.post(f"{api_url}/reviews/{rid}/comment", headers=admin_headers,
                          json={"body": "admin feedback the reader must not touch"})
        assert r.status_code == 201, r.text
        cid = r.json()["id"]

        r = requests.post(f"{api_url}/comments/{cid}/resolve", headers=reader_headers)
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"

        # and the comment is genuinely untouched
        r = requests.get(f"{api_url}/comments/open", headers=admin_headers)
        assert r.status_code == 200
        rows = r.json().get("data") or []
        assert any(row["id"] == cid for row in rows), "comment should still be open"


class TestEntityCommentAuthorization:
    """POST /entity-comments is a contributor-and-above write (#23)."""

    def test_01_reader_cannot_comment_on_a_register_entity(self, api_url, admin_headers, reader_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers,
                          json={"title": "comment authz risk", "likelihood": 3, "impact": 3})
        assert r.status_code in [200, 201], r.text
        risk = r.json()
        identifier = risk.get("identifier") or str(risk.get("id"))

        r = requests.post(f"{api_url}/entity-comments", headers=reader_headers,
                          json={"entity_type": "risk", "entity_id": identifier,
                                "body": "reader comment on a register entity"})
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"

        r = requests.post(f"{api_url}/entity-comments", headers=admin_headers,
                          json={"entity_type": "risk", "entity_id": identifier,
                                "body": "admin comment is fine"})
        assert r.status_code in [200, 201], r.text

    def test_02_contributor_may_comment(self, api_url, admin_headers, contributor_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers,
                          json={"title": "contributor comment risk", "likelihood": 2, "impact": 2})
        assert r.status_code in [200, 201], r.text
        risk = r.json()
        identifier = risk.get("identifier") or str(risk.get("id"))

        r = requests.post(f"{api_url}/entity-comments", headers=contributor_headers,
                          json={"entity_type": "risk", "entity_id": identifier,
                                "body": "contributor comment"})
        assert r.status_code in [200, 201], f"Contributor blocked: {r.text}"
