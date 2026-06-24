"""Regression for #32: the manual task-creation form must only offer task types
the API actually accepts.

The Add Task form used to list types (risk_review, supplier_review, ...) that
aren't in the task_type enum at all, so picking them 400'd. This pins the
contract: every type the form now offers is accepted, and the removed bogus ones
are rejected. (The *_followup types are valid but system-generated, so they're
intentionally not in the manual form — only in the filter.)
"""
import uuid

import requests
from conftest import ADMIN_EMAIL

# The types the manual create form offers after #32.
MANUAL_TYPES = ["general", "review", "onboarding", "offboarding", "training", "other"]

# Types the old form wrongly offered — not in the enum.
REMOVED_BOGUS_TYPES = [
    "risk_review", "supplier_review", "access_review",
    "legal_review", "document_review", "objective_checkin", "corrective_action",
]


def test_manual_task_types_are_accepted(api_url, admin_headers):
    for t in MANUAL_TYPES:
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": f"task-type {t} {uuid.uuid4().hex[:8]}",
            "task_type": t,
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in (200, 201), f"manual type {t!r} must be accepted: {r.status_code} {r.text}"


def test_removed_form_types_are_rejected(api_url, admin_headers):
    for t in REMOVED_BOGUS_TYPES:
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": f"bogus-type {t} {uuid.uuid4().hex[:8]}",
            "task_type": t,
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code == 400, f"removed type {t!r} should be rejected by the API: {r.status_code} {r.text}"
