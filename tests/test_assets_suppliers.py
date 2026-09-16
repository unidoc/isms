"""Asset and supplier register tests — comprehensive coverage."""
import requests


class TestAssets:
    def test_create_asset(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/assets", headers=admin_headers, json={
            "name": "Production Database",
            "asset_type": "system",
            "status": "open",
            "confidentiality": 4,
            "integrity": 5,
            "availability": 5,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data["identifier"].startswith("ASSET-")

    def test_create_asset_null_cia(self, api_url, admin_headers):
        """Assets with no CIA should have null values, not zero."""
        r = requests.post(f"{api_url}/assets", headers=admin_headers, json={
            "name": "Test Asset No CIA",
            "asset_type": "other",
            "status": "open",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data.get("confidentiality") is None
        assert data.get("integrity") is None
        assert data.get("availability") is None

    def test_list_assets(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/assets", headers=admin_headers)
        assert r.status_code == 200
        assert isinstance(r.json()["data"], list)
        assert len(r.json()["data"]) >= 1

    def test_delete_asset(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/assets", headers=admin_headers, json={
            "name": "To delete", "asset_type": "other", "status": "open",
        })
        asset_id = r.json()["id"]
        d = requests.delete(f"{api_url}/assets/{asset_id}", headers=admin_headers)
        assert d.status_code == 200

    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/assets", headers=reader_headers, json={
            "name": "test", "asset_type": "system",
        })
        assert r.status_code == 403

    def test_reader_can_read(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/assets", headers=reader_headers)
        assert r.status_code == 200


class TestSuppliers:
    def test_create_seeds_services_notes(self, api_url, admin_headers):
        """When notes is empty on create, it's seeded with the Services heading."""
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Seed Test Supplier",
            "supplier_type": "saas",
            "criticality": "low",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        assert "## Services" in r.json().get("notes", "")

    def test_create_supplier(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "AWS",
            "supplier_type": "cloud",
            "criticality": "critical",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data["identifier"].startswith("SUPPLIER-")
        # Cycle is derived from criticality (critical=1mo) — verify next_review is set
        assert data["next_review"] is not None

    def test_create_low_criticality(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Coffee Supplier",
            "supplier_type": "other",
            "criticality": "low",
        })
        assert r.status_code in [200, 201]
        # Low criticality → 12mo cycle. next_review should be roughly 12 months out.
        assert r.json()["next_review"] is not None

    def test_supplier_cia_null(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "No CIA supplier",
            "supplier_type": "consulting",
            "criticality": "low",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data.get("confidentiality") is None
        assert data.get("integrity") is None
        assert data.get("availability") is None

    def test_list_suppliers(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/suppliers", headers=admin_headers)
        assert r.status_code == 200
        assert isinstance(r.json()["data"], list)

    def test_update_supplier(self, api_url, admin_headers):
        # Create
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Update Test", "supplier_type": "saas",
            "criticality": "medium",
        })
        sup = r.json()

        # Update criticality to critical — next_review should change
        u = requests.put(f"{api_url}/suppliers/{sup['id']}", headers=admin_headers, json={
            **sup,
            "criticality": "critical",
        })
        assert u.status_code == 200, f"Update failed: {u.text}"

    def test_create_with_new_fields(self, api_url, admin_headers):
        """New supplier fields: status, owner, contract_expiry."""
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Full Supplier",
            "supplier_type": "saas",
            "criticality": "high",
            "status": "active",
            "contract_expiry": "2027-01-15",
            "data_access": True,
            "confidentiality": 4,
            "integrity": 3,
            "availability": 5,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        data = r.json()
        assert data["status"] == "active"

    def test_update_status(self, api_url, admin_headers):
        """Can update supplier status."""
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Status Test", "supplier_type": "cloud", "criticality": "medium",
        })
        sup = r.json()
        u = requests.put(f"{api_url}/suppliers/{sup['id']}", headers=admin_headers, json={
            **sup, "status": "under_review",
        })
        assert u.status_code == 200, f"Update failed: {u.text}"

    def test_delete_supplier(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "To delete", "supplier_type": "other",
            "criticality": "low",
        })
        d = requests.delete(f"{api_url}/suppliers/{r.json()['id']}", headers=admin_headers)
        assert d.status_code == 200

    def test_invalid_type(self, api_url, admin_headers):
        """Supplier type CHECK constraint."""
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Bad type", "supplier_type": "invalid_type",
            "criticality": "low",
        })
        assert r.status_code in [400, 500]

    def test_invalid_criticality(self, api_url, admin_headers):
        """Supplier criticality CHECK constraint."""
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Bad crit", "supplier_type": "cloud",
            "criticality": "extreme",
        })
        assert r.status_code in [400, 500]

    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/suppliers", headers=reader_headers, json={
            "name": "test",
        })
        assert r.status_code == 403

    def test_reader_can_read(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/suppliers", headers=reader_headers)
        assert r.status_code == 200


class TestAssetExternalID:
    """Optional external_id (#39) — the org's own reference, free text, not a key."""

    def _create(self, api_url, admin_headers, external_id=None):
        body = dict({"name": "External ID asset", "asset_type": "other", "status": "open"})
        if external_id is not None:
            body["external_id"] = external_id
        r = requests.post(f"{api_url}/assets", headers=admin_headers, json=body)
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        return r.json()

    def test_create_with_external_id(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "ASSET-2026-001")
        r = requests.get(f"{api_url}/assets/{created['id']}", headers=admin_headers)
        assert r.status_code == 200
        assert r.json().get("external_id") == "ASSET-2026-001"

    def test_update_clears_external_id(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "ASSET-2026-002")
        u = requests.put(f"{api_url}/assets/{created['id']}", headers=admin_headers,
                         json={"external_id": ""})
        assert u.status_code == 200, u.text
        r = requests.get(f"{api_url}/assets/{created['id']}", headers=admin_headers)
        assert not r.json().get("external_id")

    def test_update_omitting_external_id_leaves_it_alone(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "ASSET-2026-003")
        u = requests.put(f"{api_url}/assets/{created['id']}", headers=admin_headers, json={})
        assert u.status_code == 200, u.text
        r = requests.get(f"{api_url}/assets/{created['id']}", headers=admin_headers)
        assert r.json().get("external_id") == "ASSET-2026-003"

    def test_search_by_external_id(self, api_url, admin_headers):
        self._create(api_url, admin_headers, "ASSETZZ-2026-777")
        # The register list search parameter is `q`, not `search`.
        r = requests.get(f"{api_url}/assets", headers=admin_headers, params={"q": "ASSETZZ-2026"})
        assert r.status_code == 200
        payload = r.json()
        data = payload.get("data") or payload
        assert any(i.get("external_id") == "ASSETZZ-2026-777" for i in data), payload


class TestSupplierExternalID:
    """Optional external_id (#39) — the org's own reference, free text, not a key."""

    def _create(self, api_url, admin_headers, external_id=None):
        body = dict({"name": "External ID supplier", "supplier_type": "saas", "criticality": "medium"})
        if external_id is not None:
            body["external_id"] = external_id
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json=body)
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        return r.json()

    def test_create_with_external_id(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "SUP-2026-001")
        r = requests.get(f"{api_url}/suppliers/{created['id']}", headers=admin_headers)
        assert r.status_code == 200
        assert r.json().get("external_id") == "SUP-2026-001"

    def test_update_clears_external_id(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "SUP-2026-002")
        u = requests.put(f"{api_url}/suppliers/{created['id']}", headers=admin_headers,
                         json={"external_id": ""})
        assert u.status_code == 200, u.text
        r = requests.get(f"{api_url}/suppliers/{created['id']}", headers=admin_headers)
        assert not r.json().get("external_id")

    def test_update_omitting_external_id_leaves_it_alone(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, "SUP-2026-003")
        u = requests.put(f"{api_url}/suppliers/{created['id']}", headers=admin_headers, json={})
        assert u.status_code == 200, u.text
        r = requests.get(f"{api_url}/suppliers/{created['id']}", headers=admin_headers)
        assert r.json().get("external_id") == "SUP-2026-003"

    def test_search_by_external_id(self, api_url, admin_headers):
        self._create(api_url, admin_headers, "SUPZZ-2026-777")
        # The register list search parameter is `q`, not `search`.
        r = requests.get(f"{api_url}/suppliers", headers=admin_headers, params={"q": "SUPZZ-2026"})
        assert r.status_code == 200
        payload = r.json()
        data = payload.get("data") or payload
        assert any(i.get("external_id") == "SUPZZ-2026-777" for i in data), payload
