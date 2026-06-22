"""Active-status filter for the task list (#79 default-view declutter).

`?status=active` is a filter pseudo-status: open + in_progress (the work that
still needs attention). done/cancelled are excluded so they don't clutter the
default Tasks view. Regression cover for the combined tasks-fix PR.
"""
import requests
from conftest import ADMIN_EMAIL

# The list endpoint is paginated (default page_size 50) and the envelope is
# {"data": [...], "total": ...}. Pull a large page so freshly-created tasks are
# included regardless of the (priority ASC) default sort.
BIG = {"limit": 2000}


def _create(api_url, admin_headers, title):
    r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
        "title": title,
        "task_type": "review",
        "priority": "medium",
        "assignee": ADMIN_EMAIL,
    })
    assert r.status_code in [200, 201], f"create failed: {r.text}"
    return r.json()["id"]


def _list(api_url, admin_headers, **params):
    r = requests.get(f"{api_url}/tasks", headers=admin_headers, params={**BIG, **params})
    assert r.status_code == 200, r.text
    return r.json()["data"]


class TestTaskActiveFilter:
    def test_active_excludes_done_and_cancelled(self, api_url, admin_headers):
        open_id = _create(api_url, admin_headers, "active-filter open task")
        done_id = _create(api_url, admin_headers, "active-filter done task")
        cancelled_id = _create(api_url, admin_headers, "active-filter cancelled task")

        for tid, status in [(done_id, "done"), (cancelled_id, "cancelled")]:
            r = requests.put(f"{api_url}/tasks/{tid}/status", headers=admin_headers,
                             json={"status": status})
            assert r.status_code == 200, f"status update failed: {r.text}"

        ids = {t["id"] for t in _list(api_url, admin_headers, status="active")}
        assert open_id in ids, "open task must appear in the active view"
        assert done_id not in ids, "done task must be hidden from the active view"
        assert cancelled_id not in ids, "cancelled task must be hidden from the active view"

    def test_explicit_status_still_filters_exactly(self, api_url, admin_headers):
        """The 'active' pseudo-status must not break a concrete status filter."""
        done_id = _create(api_url, admin_headers, "explicit-done filter task")
        r = requests.put(f"{api_url}/tasks/{done_id}/status", headers=admin_headers,
                         json={"status": "done"})
        assert r.status_code == 200, r.text

        rows = _list(api_url, admin_headers, status="done")
        assert all(t["status"] == "done" for t in rows), \
            "?status=done must return only done tasks"
        assert any(t["id"] == done_id for t in rows), \
            "the just-completed task must appear under ?status=done"
