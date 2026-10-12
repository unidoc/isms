"""Comments on a private task follow the task's visibility rule.

A member who cannot see a private task (GET /tasks/<id> is 404) must not read
or post its comments, see their text in the activity feed, get it in an
@-mention notification, or react to them. A hidden task answers exactly like a
task that does not exist.
"""
import uuid

import pytest
import requests
from conftest import (CONTRIBUTOR_EMAIL, ORG_SLUG, READER_EMAIL, _login,
                      _signup)

OUT_EMAIL = "testoutsider@isms-test.local"
OUT_PASSWORD = "TestPass123!"


@pytest.fixture(scope="module")
def outsider_headers(api_url, admin_headers):
    """A second contributor who is neither assignee nor creator."""
    _signup(OUT_EMAIL, OUT_PASSWORD, "Test Outsider")
    requests.post(f"{api_url}/users", headers=admin_headers,
                  json={"email": OUT_EMAIL, "name": "Test Outsider", "role": "contributor"})
    tok = _login(OUT_EMAIL, OUT_PASSWORD, ORG_SLUG)
    assert tok, "outsider login failed"
    return {"Authorization": f"Bearer {tok}", "Content-Type": "application/json"}


def _task(api_url, admin_headers, private):
    r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
        "title": f"ptc-{uuid.uuid4().hex[:8]}", "priority": "high", "task_type": "general",
        "private": private, "assignee": CONTRIBUTOR_EMAIL})
    assert r.status_code in (200, 201), r.text
    return r.json()["identifier"]


def _post(api_url, headers, ident, body):
    return requests.post(f"{api_url}/entity-comments", headers=headers,
                         json={"entity_type": "task", "entity_id": ident, "body": body})


def _list(api_url, headers, ident):
    r = requests.get(f"{api_url}/entity-comments/task/{ident}", headers=headers)
    assert r.status_code == 200, r.text
    return r.json().get("data") or []


def _notifications(api_url, headers):
    r = requests.get(f"{api_url}/notifications", headers=headers)
    assert r.status_code == 200, r.text
    body = r.json()
    return (body.get("data", body) if isinstance(body, dict) else body) or []


def _activity_text(api_url, headers):
    r = requests.get(f"{api_url}/activity", headers=headers, params={"limit": 500})
    assert r.status_code == 200, r.text
    j = r.json()
    return " ".join((a.get("detail") or "") for a in ((j.get("data", j) if isinstance(j, dict) else j) or []))


def test_outsider_lists_nothing_like_a_missing_task(api_url, admin_headers, outsider_headers, reader_headers):
    ident = _task(api_url, admin_headers, True)
    tag = uuid.uuid4().hex
    assert _post(api_url, admin_headers, ident, f"secret {tag}").status_code == 201
    for h in (outsider_headers, reader_headers):
        assert _list(api_url, h, ident) == []
    assert _list(api_url, outsider_headers, "TASK-999999") == []
    assert len(_list(api_url, admin_headers, ident)) == 1


def test_outsider_cannot_post_on_a_private_task(api_url, admin_headers, outsider_headers):
    ident = _task(api_url, admin_headers, True)
    r = _post(api_url, outsider_headers, ident, "should not land")
    assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"
    assert r.json().get("code") == "not_found_in_org"
    assert _list(api_url, admin_headers, ident) == []


def test_authorised_members_and_public_tasks_are_unchanged(api_url, admin_headers, contributor_headers, outsider_headers):
    priv = _task(api_url, admin_headers, True)
    for h in (contributor_headers, admin_headers):
        assert _post(api_url, h, priv, "visible to me").status_code == 201
    for h in (contributor_headers, admin_headers):
        assert len(_list(api_url, h, priv)) == 2
    pub = _task(api_url, admin_headers, False)
    assert _post(api_url, outsider_headers, pub, "public ok").status_code == 201
    assert len(_list(api_url, outsider_headers, pub)) == 1


def test_comment_body_not_in_activity_feed(api_url, admin_headers, outsider_headers):
    ident = _task(api_url, admin_headers, True)
    tag = f"actleak{uuid.uuid4().hex}"
    c = _post(api_url, admin_headers, ident, f"{tag} body text")
    assert c.status_code == 201, c.text
    feed = _activity_text(api_url, outsider_headers)
    assert tag not in feed, "comment body leaked into the activity feed"
    assert tag not in _activity_text(api_url, admin_headers), "admin feed should not carry the body either"
    r = requests.delete(f"{api_url}/entity-comments/{c.json()['id']}", headers=admin_headers)
    assert r.status_code == 200, r.text
    assert tag not in _activity_text(api_url, outsider_headers)
    assert tag not in _activity_text(api_url, admin_headers)


def test_mentions_only_reach_members_who_can_see_the_task(api_url, admin_headers, reader_headers, contributor_headers):
    ident = _task(api_url, admin_headers, True)
    tag = uuid.uuid4().hex
    out, asg = READER_EMAIL.split("@")[0], CONTRIBUTOR_EMAIL.split("@")[0]
    r = _post(api_url, admin_headers, ident, f"@{out} @{asg} please look {tag}")
    assert r.status_code == 201, r.text
    def hit(h):
        return [n for n in _notifications(api_url, h)
                if "mentioned you" in (n.get("title") or "") and tag in (n.get("body") or "")]
    assert not hit(reader_headers), "outsider was notified about a private task comment"
    assert hit(contributor_headers), "assignee should be notified"


def test_reactions_on_a_hidden_comment_are_404(api_url, admin_headers, contributor_headers, outsider_headers):
    ident = _task(api_url, admin_headers, True)
    cid = _post(api_url, admin_headers, ident, "react to me").json()["id"]
    r = requests.post(f"{api_url}/reactions", headers=outsider_headers,
                      json={"target_type": "entity_comment", "target_id": cid, "emoji": "\U0001F44D"})
    assert r.status_code == 404, f"Expected 404, got {r.status_code}: {r.text}"
    r = requests.get(f"{api_url}/reactions/entity_comment/{cid}", headers=outsider_headers)
    assert r.status_code == 200 and (r.json().get("data") or []) == [], r.text
    r = requests.post(f"{api_url}/reactions", headers=contributor_headers,
                      json={"target_type": "entity_comment", "target_id": cid, "emoji": "\U0001F44D"})
    assert r.status_code == 200, r.text
    r = requests.get(f"{api_url}/reactions/entity_comment/{cid}", headers=contributor_headers)
    assert len(r.json().get("data") or []) == 1
