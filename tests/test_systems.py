"""Systems CRUD tests.

Tests the full lifecycle:
1. Create system with required fields
2. List systems
3. Get by ID
4. Update fields
5. Delete
6. Auto-defaults (identifier, next_review)
7. CIA scoring
8. RBAC (reader cannot create/update/delete)
"""
import requests
from conftest import READER_EMAIL


class TestSystemsCRUD:
    """Full CRUD lifecycle for systems."""

    system_id = None

    def test_00_create_seeds_purpose_and_access_headings(self, api_url, admin_headers):
        """When description/notes empty, they're seeded with default headings."""
        r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
            "name": "Seed Test System",
            "classification": "internal",
            "criticality": "low",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert "## Purpose" in data.get("description", "")
        assert "## Access control" in data.get("notes", "")

    def test_01_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
            "name": "Production Database",
            "classification": "confidential",
            "criticality": "high",
            "rpo_hours": 1,
            "rto_hours": 4,
            "department": "Engineering",
            "description": "## Purpose\n\nPrimary data store",
            "notes": "## Access control\n\nSSO + MFA",
        })
        assert r.status_code == 201, f"Create system failed: {r.text}"
        data = r.json()
        TestSystemsCRUD.system_id = data["id"]
        assert data["name"] == "Production Database"
        assert data["classification"] == "confidential"
        assert data["criticality"] == "high"
        assert data["rpo_hours"] == 1
        assert data["rto_hours"] == 4

    def test_02_auto_identifier(self, api_url, admin_headers):
        """System gets an auto-generated SYSTEM-NNN identifier."""
        sid = TestSystemsCRUD.system_id
        r = requests.get(f"{api_url}/systems", headers=admin_headers)
        assert r.status_code == 200
        systems = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [s for s in systems if s["id"] == sid]
        assert len(match) == 1
        assert match[0]["identifier"].startswith("SYSTEM-"), \
            f"Expected SYSTEM-NNN identifier, got {match[0]['identifier']}"

    def test_03_auto_next_review(self, api_url, admin_headers):
        """High-criticality system should get auto-calculated next_review."""
        r = requests.get(f"{api_url}/systems", headers=admin_headers)
        systems = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        match = [s for s in systems if s["id"] == TestSystemsCRUD.system_id][0]
        # High criticality → 6-month review cycle by default
        assert match.get("next_review") is not None, \
            "High-criticality system should have auto-calculated next_review"

    def test_04_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/systems", headers=admin_headers)
        assert r.status_code == 200
        systems = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert len(systems) >= 1

    def test_05_update(self, api_url, admin_headers):
        sid = TestSystemsCRUD.system_id
        r = requests.put(f"{api_url}/systems/{sid}", headers=admin_headers, json={
            "name": "Production Database (Primary)",
            "description": "PostgreSQL 16 cluster",
            "classification": "confidential",
            "criticality": "high",
            "rpo_hours": 0,
            "rto_hours": 2,
        })
        assert r.status_code == 200, f"Update failed: {r.text}"
        data = r.json()
        assert data["name"] == "Production Database (Primary)"
        assert data["description"] == "PostgreSQL 16 cluster"
        assert data["rpo_hours"] == 0
        assert data["rto_hours"] == 2

    def test_06_update_cia(self, api_url, admin_headers):
        """Set CIA impact scores."""
        sid = TestSystemsCRUD.system_id
        r = requests.put(f"{api_url}/systems/{sid}", headers=admin_headers, json={
            "name": "Production Database (Primary)",
            "classification": "confidential",
            "criticality": "high",
            "rpo_hours": 0,
            "rto_hours": 2,
            "confidentiality": 5,
            "integrity": 4,
            "availability": 5,
        })
        assert r.status_code == 200, f"CIA update failed: {r.text}"
        data = r.json()
        assert data["confidentiality"] == 5
        assert data["integrity"] == 4
        assert data["availability"] == 5

    def test_07_delete(self, api_url, admin_headers):
        sid = TestSystemsCRUD.system_id
        r = requests.delete(f"{api_url}/systems/{sid}", headers=admin_headers)
        assert r.status_code == 200
        # Verify gone
        r = requests.get(f"{api_url}/systems", headers=admin_headers)
        systems = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert not any(s["id"] == sid for s in systems), "Deleted system still in list"


class TestSystemsDefaults:
    """Test default values and edge cases."""

    def test_minimal_create(self, api_url, admin_headers):
        """Create with only required fields — everything else should default."""
        r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
            "name": "Minimal System",
            "classification": "internal",
            "criticality": "low",
            "rpo_hours": 24,
            "rto_hours": 48,
        })
        assert r.status_code == 201, f"Minimal create failed: {r.text}"
        data = r.json()
        assert data["name"] == "Minimal System"
        assert data["classification"] == "internal"
        # Clean up
        requests.delete(f"{api_url}/systems/{data['id']}", headers=admin_headers)

    def test_invalid_classification_fails(self, api_url, admin_headers):
        """Invalid classification value should fail."""
        r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
            "name": "Bad Classification",
            "classification": "top_secret",
            "criticality": "low",
            "rpo_hours": 24,
            "rto_hours": 48,
        })
        assert r.status_code in [400, 422, 500], \
            f"Expected error for invalid classification, got {r.status_code}"


class TestSystemsRBAC:
    """Reader cannot mutate systems."""

    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/systems", headers=reader_headers, json={
            "name": "Unauthorized",
            "classification": "public",
            "criticality": "low",
            "rpo_hours": 24,
            "rto_hours": 48,
        })
        assert r.status_code == 403, f"Expected 403, got {r.status_code}"

    def test_reader_can_list(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/systems", headers=reader_headers)
        assert r.status_code == 200


class TestSystemsAccessReviews:
    """Access review lifecycle on a system."""

    system_id = None

    def test_01_create_system(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
            "name": "Access Review Target",
            "classification": "restricted",
            "criticality": "critical",
            "rpo_hours": 0,
            "rto_hours": 1,
        })
        assert r.status_code == 201
        TestSystemsAccessReviews.system_id = r.json()["id"]

    def test_02_create_access_review(self, api_url, admin_headers):
        sid = TestSystemsAccessReviews.system_id
        r = requests.post(f"{api_url}/systems/{sid}/access-reviews",
                          headers=admin_headers, json={
            "notes": "Q2 2026 access review",
        })
        assert r.status_code in [200, 201], f"Create access review failed: {r.text}"

    def test_03_list_access_reviews(self, api_url, admin_headers):
        sid = TestSystemsAccessReviews.system_id
        r = requests.get(f"{api_url}/systems/{sid}/access-reviews",
                         headers=admin_headers)
        assert r.status_code == 200
        reviews = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert len(reviews) >= 1

    def test_04_cleanup(self, api_url, admin_headers):
        sid = TestSystemsAccessReviews.system_id
        requests.delete(f"{api_url}/systems/{sid}", headers=admin_headers)
