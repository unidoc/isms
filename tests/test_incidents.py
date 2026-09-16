"""Incident management tests — comprehensive coverage."""
import requests
from conftest import ADMIN_EMAIL, READER_EMAIL


class TestIncidentCRUD:
    def test_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
            "title": "Phishing email targeting employees",
            "description": "Three employees received targeted phishing",
            "severity": "high",
            "affects_c": True,
            "incident_type": "incident",
            "source": "external",
            "reporter": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"

    def test_create_with_cia_flags(self, api_url, admin_headers):
        """affects_c/i/a are independent booleans for CIA impact."""
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
            "title": "Data breach via compromised credentials",
            "description": "Customer data exposed",
            "severity": "critical",
            "affects_c": True,
            "affects_i": True,
            "affects_a": False,
            "incident_type": "incident",
            "source": "external",
            "reporter": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data["affects_c"] is True
        assert data["affects_i"] is True
        assert data["affects_a"] is False

    def test_create_seeds_timeline_notes(self, api_url, admin_headers):
        """When notes is empty on create, it's seeded with a Timeline template."""
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
            "title": "Incident with seeded timeline",
            "description": "Should get Timeline template",
            "severity": "low",
            "incident_type": "event",
            "source": "internal",
            "reporter": ADMIN_EMAIL,
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert "## Timeline" in data.get("notes", "")
        assert "Incident raised by" in data.get("notes", "")

    def test_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/incidents", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or r.json()
        assert isinstance(data, list)
        assert len(data) >= 1

    def test_stats(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/incidents/stats", headers=admin_headers)
        assert r.status_code == 200

    def test_update_status(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
            "title": "Status update test",
            "description": "Testing status transitions",
            "severity": "medium",
            "affects_i": True,
            "incident_type": "event",
            "source": "internal",
            "reporter": ADMIN_EMAIL,
        })
        inc_id = r.json()["id"]

        u = requests.put(f"{api_url}/incidents/{inc_id}/status", headers=admin_headers,
                         json={"status": "investigating"})
        assert u.status_code == 200


class TestIncidentRBAC:
    def test_reader_cannot_create(self, api_url, reader_headers):
        """Reader (formerly reviewer) cannot create incidents — requires contributor+."""
        r = requests.post(f"{api_url}/incidents", headers=reader_headers, json={
            "title": "Reader-reported incident",
            "description": "Found suspicious activity",
            "severity": "medium",
            "affects_c": True,
            "incident_type": "event",
            "source": "internal",
        })
        assert r.status_code == 403

    def test_reader_cannot_update(self, api_url, admin_headers, reader_headers):
        incidents = requests.get(f"{api_url}/incidents", headers=admin_headers).json()
        data = incidents.get("data") or incidents
        if len(data) > 0:
            inc_id = data[0]["id"]
            r = requests.put(f"{api_url}/incidents/{inc_id}", headers=reader_headers,
                             json={"severity": "critical"})
            assert r.status_code == 403

    def test_reader_can_read(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/incidents", headers=reader_headers)
        assert r.status_code == 200


class TestIncidentExternalID:
    """Optional external_id (#39) — the org's own reference, free text, not a key."""

    def _create(self, api_url, admin_headers, external_id=None):
        body = {
            "title": "External ID round-trip",
            "description": "Checks the optional external reference",
            "severity": "low",
            "affects_c": True,
            "incident_type": "event",
            "source": "internal",
            "reporter": ADMIN_EMAIL,
        }
        if external_id is not None:
            body["external_id"] = external_id
        r = requests.post(f"{api_url}/incidents", headers=admin_headers, json=body)
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        return r.json()

    def test_create_with_external_id(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "ACME-2026-001")
        r = requests.get(f"{api_url}/incidents/{created['id']}", headers=admin_headers)
        assert r.status_code == 200
        assert r.json().get("external_id") == "ACME-2026-001"

    def test_update_clears_external_id(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "ACME-2026-002")
        u = requests.put(f"{api_url}/incidents/{created['id']}", headers=admin_headers,
                         json={"external_id": ""})
        assert u.status_code == 200, u.text
        r = requests.get(f"{api_url}/incidents/{created['id']}", headers=admin_headers)
        assert not r.json().get("external_id")

    def test_update_omitting_external_id_leaves_it_alone(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "ACME-2026-003")
        u = requests.put(f"{api_url}/incidents/{created['id']}", headers=admin_headers,
                         json={"severity": "medium"})
        assert u.status_code == 200, u.text
        r = requests.get(f"{api_url}/incidents/{created['id']}", headers=admin_headers)
        assert r.json().get("external_id") == "ACME-2026-003"

    def test_search_by_external_id(self, api_url, admin_headers):
        self._create(api_url, admin_headers, "ZETA-2026-777")
        # The register list search parameter is `q`, not `search`.
        r = requests.get(f"{api_url}/incidents", headers=admin_headers,
                         params={"q": "ZETA-2026"})
        assert r.status_code == 200
        body = r.json()
        data = body.get("data") or body
        assert any(i.get("external_id") == "ZETA-2026-777" for i in data), body
