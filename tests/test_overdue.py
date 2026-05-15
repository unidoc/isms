"""Overdue reviews and auto-task creation tests."""
import requests


def test_overdue_summary(api_url, admin_headers):
    """GET /overdue returns summary structure."""
    r = requests.get(f"{api_url}/overdue", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "total_count" in data
    assert "risks" in data
    assert "suppliers" in data
    assert "systems" in data
    assert "legal" in data
    assert "tasks" in data


def test_create_overdue_tasks(api_url, admin_headers):
    """POST /overdue/tasks creates tasks (or returns empty if nothing overdue)."""
    r = requests.post(f"{api_url}/overdue/tasks", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "created" in data
    assert "skipped" in data
    assert "total" in data


def test_create_overdue_tasks_idempotent(api_url, admin_headers):
    """Running twice doesn't create duplicates."""
    r1 = requests.post(f"{api_url}/overdue/tasks", headers=admin_headers)
    assert r1.status_code == 200
    r2 = requests.post(f"{api_url}/overdue/tasks", headers=admin_headers)
    assert r2.status_code == 200
    data2 = r2.json()
    # Second run should create 0 new tasks
    assert len(data2.get("created") or []) == 0


def test_create_overdue_tasks_requires_role(api_url, reader_headers):
    """Only admin/manager can trigger overdue task creation — reader is blocked."""
    r = requests.post(f"{api_url}/overdue/tasks", headers=reader_headers)
    assert r.status_code == 403


def test_risk_has_next_review(api_url, admin_headers):
    """Risks with scores should have auto-calculated next_review."""
    r = requests.get(f"{api_url}/risks", headers=admin_headers)
    assert r.status_code == 200
    risks = r.json().get("data", [])
    for risk in risks:
        score = risk.get("current_score")
        if score is not None and score > 0:
            assert risk.get("next_review") is not None, \
                f"Risk {risk.get('identifier')} has score but no next_review"


def test_supplier_has_next_review(api_url, admin_headers):
    """Suppliers should have auto-calculated next_review derived from criticality."""
    # Create a supplier
    r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
        "name": "Review Test Supplier",
        "supplier_type": "cloud",
        "criticality": "critical",
    })
    assert r.status_code in [200, 201], f"Create failed: {r.text}"
    data = r.json()
    assert data.get("next_review") is not None, "Supplier should have auto-calculated next_review"
