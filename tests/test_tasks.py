"""Task management tests — basic CRUD + validation coverage."""
import requests
from conftest import ADMIN_EMAIL


class TestTasksCRUD:
    task_id = None

    def test_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "Review risk register",
            "task_type": "review",
            "priority": "medium",
            "assignee": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        TestTasksCRUD.task_id = data["id"]
        assert data["title"] == "Review risk register"
        assert data["task_type"] == "review"
        assert data["status"] == "open"

    def test_auto_identifier(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/tasks/{TestTasksCRUD.task_id}", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data["identifier"].startswith("TASK-"), f"Expected TASK-N, got {data['identifier']}"

    def test_due_date_optional(self, api_url, admin_headers):
        """due_date should be optional — task creates fine without it."""
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "Ad-hoc task",
            "task_type": "general",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data.get("due_date") is None or data.get("due_date") == 0

    def test_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/tasks", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        items = data.get("data", data) if isinstance(data, dict) else data
        assert isinstance(items, list)
        assert len(items) >= 1

    def test_update_status(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/tasks/{TestTasksCRUD.task_id}/status", headers=admin_headers,
                         json={"status": "in_progress"})
        assert r.status_code == 200, f"Failed: {r.text}"

    def test_update_fields(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/tasks/{TestTasksCRUD.task_id}", headers=admin_headers, json={
            "title": "Review risk register (Q2)",
            "priority": "high",
            "notes": "## Context\n\nQuarterly review per ISO 27001 6.1.2",
        })
        assert r.status_code == 200, f"Failed: {r.text}"
        data = r.json()
        assert data["title"] == "Review risk register (Q2)"
        assert data["priority"] == "high"

    def test_delete(self, api_url, admin_headers):
        r = requests.delete(f"{api_url}/tasks/{TestTasksCRUD.task_id}", headers=admin_headers)
        assert r.status_code == 200


class TestTaskValidation:
    def test_invalid_task_type_rejected(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "Bad task",
            "task_type": "imaginary-flavour",
        })
        assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"

    def test_invalid_priority_rejected(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/tasks", headers=admin_headers, json={
            "title": "Bad task",
            "task_type": "general",
            "priority": "supercritical",
        })
        assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"


class TestTaskRBAC:
    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/tasks", headers=reader_headers, json={
            "title": "Sneaky task",
            "task_type": "general",
        })
        assert r.status_code == 403
