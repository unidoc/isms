"""Regression (#179 review of #178): a private task must not leak its title,
existence, or aggregate presence through the surfaces that read/write the tasks
table beyond the list/get paths. Covers Alip's six findings — overdue summary,
activity feed, suggestions, search index, task stats.

A private task is created by admin and assigned to CONTRIBUTOR (a legitimate
viewer); READER is the unrelated viewer that must see nothing. Titles are unique
per test so the assertions are deterministic on the shared/persistent stack.

The stats test uses a same-viewer before/after delta — run sequentially (not with
pytest -n) for its exact-count assertion to hold.
"""
import uuid

import requests
from conftest import CONTRIBUTOR_EMAIL

PAST_DUE = 1704067200  # 2024-01-01, safely overdue relative to test-time


def _mk_private(api_url, admin_headers, title, due=None):
    body = {"title": title, "priority": "high", "task_type": "general",
            "private": True, "assignee": CONTRIBUTOR_EMAIL}
    if due:
        body["due_date"] = due
    r = requests.post(f"{api_url}/tasks", headers=admin_headers, json=body)
    assert r.status_code in (200, 201), r.text
    return r.json()


def _data(r):
    j = r.json()
    v = j.get("data", j) if isinstance(j, dict) else j
    return v or []  # a nil Go slice serializes as null → treat as empty


def test_private_task_absent_from_search_for_reader(api_url, admin_headers, reader_headers):
    title = f"priv-search-{uuid.uuid4().hex[:8]}"
    _mk_private(api_url, admin_headers, title)
    r = requests.get(f"{api_url}/search", headers=reader_headers, params={"q": title})
    assert r.status_code == 200, r.text
    assert not [e for e in _data(r) if e.get("title") == title], \
        "private task title leaked into the search index"


def test_private_task_absent_from_activity_for_reader(api_url, admin_headers, reader_headers):
    title = f"priv-activity-{uuid.uuid4().hex[:8]}"
    _mk_private(api_url, admin_headers, title)
    r = requests.get(f"{api_url}/activity", headers=reader_headers, params={"limit": 200})
    assert r.status_code == 200, r.text
    assert not [a for a in _data(r) if title in (a.get("detail") or "")], \
        "private task title leaked into the activity feed"


def test_private_overdue_task_scoped_by_viewer(api_url, admin_headers, reader_headers, contributor_headers):
    title = f"priv-overdue-{uuid.uuid4().hex[:8]}"
    _mk_private(api_url, admin_headers, title, due=PAST_DUE)
    # Unrelated reader must not see it in the overdue summary.
    r = requests.get(f"{api_url}/overdue", headers=reader_headers)
    assert r.status_code == 200, r.text
    assert not [t for t in (r.json().get("tasks") or []) if t.get("title") == title], \
        "private overdue task leaked to an unrelated reader"
    # The assignee (contributor) must still see their own.
    r2 = requests.get(f"{api_url}/overdue", headers=contributor_headers)
    assert r2.status_code == 200, r2.text
    assert [t for t in (r2.json().get("tasks") or []) if t.get("title") == title], \
        "assignee should see their own private overdue task"


def test_suggestion_on_private_task_scoped_by_viewer(api_url, admin_headers, reader_headers, contributor_headers):
    task = _mk_private(api_url, admin_headers, f"priv-sugg-{uuid.uuid4().hex[:8]}")
    sugg_title = f"sugg-{uuid.uuid4().hex[:8]}"
    r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "task", "entity_id": task["identifier"],
        "suggestion_type": "update", "title": sugg_title,
        "rationale": "review", "payload": {"fields": {"priority": "low"}},
    })
    assert r.status_code in (200, 201), r.text
    # Unrelated reader must not see the suggestion via the open list.
    rr = requests.get(f"{api_url}/suggestions", headers=reader_headers)
    assert rr.status_code == 200, rr.text
    assert not [x for x in _data(rr) if x.get("title") == sugg_title], \
        "suggestion on a private task leaked to an unrelated reader"
    # Assignee (contributor) sees it.
    rc = requests.get(f"{api_url}/suggestions", headers=contributor_headers)
    assert rc.status_code == 200, rc.text
    assert [x for x in _data(rc) if x.get("title") == sugg_title], \
        "assignee should see a suggestion on their own private task"


def test_private_task_not_counted_in_reader_stats(api_url, admin_headers, reader_headers, contributor_headers):
    def total(hdrs):
        r = requests.get(f"{api_url}/tasks/stats", headers=hdrs)
        assert r.status_code == 200, r.text
        return r.json().get("total", 0)

    reader_before = total(reader_headers)
    contributor_before = total(contributor_headers)
    _mk_private(api_url, admin_headers, f"priv-stats-{uuid.uuid4().hex[:8]}")
    assert total(reader_headers) == reader_before, \
        "private task leaked into an unrelated reader's stats total"
    assert total(contributor_headers) >= contributor_before + 1, \
        "assignee's stats total should include their own private task"
