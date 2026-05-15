"""Corrective action tests — CRUD + seed-template + validation coverage."""
import requests
from conftest import ADMIN_EMAIL


class TestCACRUD:
    ca_id = None

    def test_create_seeds_action_plan_template(self, api_url, admin_headers):
        """When notes is empty on create, server seeds the action plan markdown template."""
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Address NC-7 from 2026 internal audit",
            "source": "internal_audit",
            "severity": "minor_nc",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        TestCACRUD.ca_id = data["id"]
        notes = data.get("notes", "")
        assert "## Action plan" in notes
        assert "## Implementation" in notes
        assert "## Verification" in notes
        assert "## Evidence" in notes

    def test_auto_identifier(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/corrective-actions/{TestCACRUD.ca_id}", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert data["identifier"].startswith("CA-"), f"Expected CA-N, got {data['identifier']}"

    def test_user_notes_not_overwritten_by_template(self, api_url, admin_headers):
        """If user provides notes at create, server should NOT seed the template over them."""
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "User-authored CA",
            "source": "feedback",
            "severity": "observation",
            "notes": "User-provided context, no template please.",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert "## Action plan" not in data.get("notes", "")
        assert "User-provided context" in data["notes"]

    def test_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/corrective-actions", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        items = data.get("data", data) if isinstance(data, dict) else data
        assert isinstance(items, list)
        assert len(items) >= 2

    def test_update_root_cause(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/corrective-actions/{TestCACRUD.ca_id}", headers=admin_headers, json={
            "title": "Address NC-7 from 2026 internal audit",
            "source": "internal_audit",
            "severity": "minor_nc",
            "root_cause": "Documentation gap in onboarding procedure",
        })
        assert r.status_code == 200, f"Failed: {r.text}"
        data = r.json()
        assert data["root_cause"] == "Documentation gap in onboarding procedure"

    def test_update_status_via_dedicated_endpoint(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/corrective-actions/{TestCACRUD.ca_id}/status",
                         headers=admin_headers, json={"status": "implementation"})
        assert r.status_code == 200, f"Failed: {r.text}"

    def test_delete(self, api_url, admin_headers):
        r = requests.delete(f"{api_url}/corrective-actions/{TestCACRUD.ca_id}", headers=admin_headers)
        assert r.status_code == 200


class TestCAValidation:
    def test_invalid_severity_rejected(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Bad CA",
            "source": "other",
            "severity": "catastrophic",
        })
        assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"

    def test_invalid_source_rejected(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "Bad CA",
            "source": "made_up_source",
            "severity": "observation",
        })
        assert r.status_code == 400, f"Expected 400, got {r.status_code}: {r.text}"


class TestCARBAC:
    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=reader_headers, json={
            "title": "Sneaky CA",
            "source": "other",
            "severity": "observation",
        })
        assert r.status_code == 403
