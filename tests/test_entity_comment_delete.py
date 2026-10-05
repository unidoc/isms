"""DELETE /entity-comments/:id (#403).

A comment that still has replies is refused with 409 (no cascade: it would
silently remove other people's replies), a missing id is 404, and the delete
order reply-then-parent works.
"""
import requests


class TestEntityCommentDelete:
    risk = None
    parent = None
    reply = None

    def _post(self, api_url, headers, body, parent_id=None):
        payload = {"entity_type": "risk", "entity_id": self.risk, "body": body}
        if parent_id is not None:
            payload["parent_id"] = parent_id
        r = requests.post(f"{api_url}/entity-comments", headers=headers, json=payload)
        assert r.status_code in [200, 201], r.text
        return r.json()["id"]

    def test_01_parent_with_a_reply_is_refused(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers,
                          json={"title": "comment delete risk", "likelihood": 2, "impact": 2})
        assert r.status_code in [200, 201], r.text
        risk = r.json()
        TestEntityCommentDelete.risk = risk.get("identifier") or str(risk.get("id"))

        TestEntityCommentDelete.parent = self._post(api_url, admin_headers, "parent")
        TestEntityCommentDelete.reply = self._post(api_url, admin_headers, "reply", self.parent)

        r = requests.delete(f"{api_url}/entity-comments/{self.parent}", headers=admin_headers)
        assert r.status_code == 409, f"Expected 409, got {r.status_code}: {r.text}"
        body = r.json()
        assert body["code"] == "comment_has_replies"
        assert body["params"]["count"] == "1"
        assert "foreign key" not in r.text.lower()

    def test_02_reply_then_parent_can_be_deleted(self, api_url, admin_headers):
        r = requests.delete(f"{api_url}/entity-comments/{self.reply}", headers=admin_headers)
        assert r.status_code == 200, r.text
        r = requests.delete(f"{api_url}/entity-comments/{self.parent}", headers=admin_headers)
        assert r.status_code == 200, r.text

        r = requests.get(f"{api_url}/entity-comments/risk/{self.risk}", headers=admin_headers)
        assert r.status_code == 200, r.text
        ids = [c["id"] for c in (r.json().get("data") or [])]
        assert self.parent not in ids and self.reply not in ids

    def test_03_missing_comment_is_404(self, api_url, admin_headers):
        r = requests.delete(f"{api_url}/entity-comments/999999999", headers=admin_headers)
        assert r.status_code == 404, f"Expected 404, got {r.status_code}: {r.text}"

    def test_04_contributor_cannot_delete(self, api_url, admin_headers, contributor_headers):
        cid = self._post(api_url, admin_headers, "contributor cannot delete this")
        r = requests.delete(f"{api_url}/entity-comments/{cid}", headers=contributor_headers)
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"
        r = requests.delete(f"{api_url}/entity-comments/{cid}", headers=admin_headers)
        assert r.status_code == 200, r.text
