"""Legal register tests — comprehensive coverage."""
import requests


class TestLegalCRUD:
    def test_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
            "title": "GDPR - General Data Protection Regulation",
            "jurisdiction": "EU",
            "category": "privacy",
            "description": "EU data protection regulation",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data["identifier"].startswith("LEGAL-")

    def test_create_with_risk_assessment(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
            "title": "NIS2 Directive",
            "jurisdiction": "EU",
            "category": "security",
            "current_likelihood": 4,
            "current_impact": 5,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data["current_score"] == 20
        assert data["current_level"] == "critical"
        assert data["next_review"] is not None

    def test_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/legal", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data")
        assert data is None or isinstance(data, list)

    def test_update(self, api_url, admin_headers):
        # Create
        r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
            "title": "Update test law",
            "jurisdiction": "IS",
            "category": "privacy",
        })
        legal_id = r.json()["id"]

        # Update
        u = requests.put(f"{api_url}/legal/{legal_id}", headers=admin_headers, json={
            "current_likelihood": 1,
            "current_impact": 2,
            "treatment": "mitigate",
        })
        assert u.status_code == 200, f"Update failed: {u.text}"

    def test_delete(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
            "title": "To be deleted",
            "jurisdiction": "EU",
        })
        legal_id = r.json()["id"]

        d = requests.delete(f"{api_url}/legal/{legal_id}", headers=admin_headers)
        assert d.status_code == 200


class TestLegalScoring:
    def test_null_when_unassessed(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
            "title": "Unassessed law",
            "jurisdiction": "EU",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data["current_score"] is None
        assert data["current_level"] == "" or data["current_level"] is None

    def test_auto_review_date_from_risk(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
            "title": "Risk-driven review",
            "jurisdiction": "EU",
            "current_likelihood": 5,
            "current_impact": 5,
        })
        assert r.status_code in [200, 201]
        assert r.json()["next_review"] is not None


class TestLegalRBAC:
    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/legal", headers=reader_headers, json={
            "title": "test", "jurisdiction": "EU",
        })
        assert r.status_code == 403

    def test_reader_can_read(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/legal", headers=reader_headers)
        assert r.status_code == 200
