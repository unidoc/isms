"""Regression: applying a task-create suggestion with no assignee must not blow
up with a raw SQL constraint error.

tasks.assignee_id is NOT NULL, but a suggested task often carries no assignee.
applyTaskCreate used to insert it as-is, surfacing
  "null value in column assignee_id ... violates not-null constraint"
to the manager. It now defaults the assignee to the applier, who can reassign
in edit afterwards.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL


def test_apply_task_suggestion_without_assignee_defaults_to_applier(api_url, admin_headers):
    title = f"apply-task-noassignee {uuid.uuid4().hex[:8]}"

    # A task suggestion with NO assignee — the exact repro.
    r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
        "entity_type": "task",
        "suggestion_type": "create",
        "title": title,
        "rationale": "follow-up proposed during review",
        "payload": {
            "title": title,
            "description": "Investigate the reported issue",
            "priority": "high",
            "task_type": "general",
            # deliberately no "assignee"
        },
    })
    assert r.status_code in (200, 201), f"create suggestion: {r.text}"
    sid = r.json()["id"]

    # Apply must succeed, not crash on the NOT NULL constraint.
    r = requests.post(f"{api_url}/suggestions/{sid}/apply", headers=admin_headers, json={})
    assert r.status_code == 200, f"apply must not crash on missing assignee: {r.status_code} {r.text}"
    assert r.json().get("status") == "applied", r.text

    # The created task is assigned to the applier (admin), who can reassign later.
    r = requests.get(f"{api_url}/tasks", headers=admin_headers, params={"limit": 2000})
    assert r.status_code == 200, r.text
    rows = r.json().get("data", r.json())
    match = [t for t in rows if t.get("title") == title]
    assert match, "task created by the applied suggestion not found"
    assert match[0]["assignee"] == ADMIN_EMAIL, \
        f"expected assignee to default to the applier {ADMIN_EMAIL}, got {match[0].get('assignee')!r}"
