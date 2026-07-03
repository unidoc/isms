"""Regression for #4: @-mentioning an org member in a review comment notifies them.

The commenter writes "@handle …" (handle = the member's email local-part). The
mentioned member gets an in-app notification linking to the review. Handles that
don't match a member, and self-mentions, produce no notification.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL, READER_EMAIL


def _notifications(api_url, headers):
    r = requests.get(f"{api_url}/notifications", headers=headers)
    assert r.status_code == 200, r.text
    body = r.json()
    items = body.get("data", body) if isinstance(body, dict) else body
    return items or []  # GET /notifications serialises an empty result as null


def test_review_comment_mention_notifies_the_member(api_url, admin_headers, reader_headers):
    suffix = uuid.uuid4().hex[:8]
    doc_id = f"mention-{suffix}"
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "Mention test", "content": "# Mention test\n\nbody",
    })
    assert r.status_code in (200, 201), r.text

    rid = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin_headers,
                        json={"reviewers": [READER_EMAIL], "message": "review"}).json()["review_id"]

    handle = READER_EMAIL.split("@")[0]  # "testreviewer"
    c = requests.post(f"{api_url}/reviews/{rid}/comment", headers=admin_headers,
                      json={"body": f"@{handle} please take a look"})
    assert c.status_code in (200, 201), c.text

    # The mentioned reader has a notification linking to this review.
    items = _notifications(api_url, reader_headers)
    assert any("mentioned you" in (n.get("title", "")) and f"/reviews/{rid}" in (n.get("link") or "")
               for n in items), f"reader should have a mention notification for review {rid}"


def test_unknown_handle_and_self_mention_produce_no_notification(api_url, admin_headers):
    suffix = uuid.uuid4().hex[:8]
    doc_id = f"mention-none-{suffix}"
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "Mention none", "content": "# x\n\nbody",
    })
    assert r.status_code in (200, 201), r.text
    rid = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin_headers,
                        json={"reviewers": [READER_EMAIL], "message": "review"}).json()["review_id"]

    admin_handle = ADMIN_EMAIL.split("@")[0]
    # Unknown handle + a self-mention by the admin author — neither should notify anyone.
    c = requests.post(f"{api_url}/reviews/{rid}/comment", headers=admin_headers,
                      json={"body": f"@nobody-{suffix} and @{admin_handle} (me) — no pings"})
    assert c.status_code in (200, 201), c.text

    # Admin (the author) must not have self-notified for this review.
    items = _notifications(api_url, admin_headers)
    assert not any(f"/reviews/{rid}" in (n.get("link") or "") for n in items), \
        "author self-mention must not create a notification"


def test_scoped_package_token_is_not_a_mention(api_url, admin_headers, reader_headers):
    """`@handle/...` (e.g. @babel/core) is a package path, not a mention (#4 review)."""
    suffix = uuid.uuid4().hex[:8]
    doc_id = f"mention-scope-{suffix}"
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": "iso27001", "filename": f"{doc_id}.md",
        "document_id": doc_id, "title": "Mention scope", "content": "# x\n\nbody",
    })
    assert r.status_code in (200, 201), r.text
    rid = requests.post(f"{api_url}/documents/{doc_id}/reviews", headers=admin_headers,
                        json={"reviewers": [READER_EMAIL], "message": "review"}).json()["review_id"]

    # The reader's handle followed by "/..." must read as a package path, not a ping.
    handle = READER_EMAIL.split("@")[0]
    c = requests.post(f"{api_url}/reviews/{rid}/comment", headers=admin_headers,
                      json={"body": f"see @{handle}/core for the config, not a mention"})
    assert c.status_code in (200, 201), c.text

    # The reader is a reviewer, so they legitimately have a review-assignment
    # notification for this rid — assert only that no *mention* notification exists.
    items = _notifications(api_url, reader_headers)
    assert not any("mentioned you" in (n.get("title", "")) and f"/reviews/{rid}" in (n.get("link") or "")
                   for n in items), \
        "a @handle/pkg token must not create a mention notification"
