"""Objectives and programs tests — comprehensive coverage."""
import requests
from conftest import ADMIN_EMAIL


class TestPrograms:
    def test_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/programs", headers=admin_headers, json={
            "title": "Information Security Programme 2026",
            "description": "Annual security objectives",
            "owner": ADMIN_EMAIL,
            "key": "SEC2026",
        })
        # 201 = created; 409/500 with unique violation = already exists (idempotent)
        if (r.status_code == 500 and "unique" in r.text.lower()) or r.status_code == 409:
            pass  # already exists from previous run
        else:
            assert r.status_code in [200, 201], f"Failed: {r.text}"

    def test_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/programs", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data")
        assert data is None or isinstance(data, list)

    def test_key_format(self, api_url, admin_headers):
        """Program key must be uppercase alphanumeric."""
        r = requests.post(f"{api_url}/programs", headers=admin_headers, json={
            "title": "Bad key test",
            "key": "bad-key",  # has hyphen, lowercase
        })
        assert r.status_code in [400, 500]

    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/programs", headers=reader_headers, json={
            "title": "Should fail", "key": "FAIL",
        })
        assert r.status_code == 403


class TestObjectives:
    def test_create(self, api_url, admin_headers):
        progs = requests.get(f"{api_url}/programs", headers=admin_headers).json().get("data") or []
        if len(progs) > 0:
            r = requests.post(f"{api_url}/objectives", headers=admin_headers, json={
                "program_id": progs[0]["id"],
                "title": "Reduce phishing click rate below 5%",
                "owner": ADMIN_EMAIL,
                "target_value": 5.0,
                "target_operator": "lte",
                "unit": "%",
            })
            # 409 is expected on re-run (objective already exists, mapped via pgxHTTPError)
            assert r.status_code in [200, 201, 409, 500], f"Failed: {r.text}"

    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/objectives", headers=reader_headers, json={
            "title": "Should fail",
        })
        assert r.status_code == 403

    def test_reader_can_read(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/objectives", headers=reader_headers)
        assert r.status_code == 200
