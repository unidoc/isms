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


class TestSupplierPublicFields:
    """public_name, public_purpose, processing_location, data_categories (#522):
    free text, trimmed, length-limited, partial-update safe."""

    FIELDS = ("public_name", "public_purpose", "processing_location", "data_categories")

    def _create(self, api_url, admin_headers, **extra):
        body = {"name": "Public fields supplier", "supplier_type": "saas", "criticality": "medium"}
        body.update(extra)
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json=body)
        assert r.status_code == 201, r.text
        return r.json()

    def _get(self, api_url, admin_headers, sid):
        r = requests.get(f"{api_url}/suppliers/{sid}", headers=admin_headers)
        assert r.status_code == 200, r.text
        return r.json()

    def test_default_is_empty_and_keys_present(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers)
        got = self._get(api_url, admin_headers, created["id"])
        for k in self.FIELDS:
            assert got[k] == "", (k, got)
        assert "trust_public_changed_at" not in got

    def test_create_and_update_store_trimmed_values(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, public_name="  Acme GmbH ", public_purpose="Hosting",
                               processing_location="EU", data_categories="Email")
        got = self._get(api_url, admin_headers, created["id"])
        assert [got[k] for k in self.FIELDS] == ["Acme GmbH", "Hosting", "EU", "Email"], got
        u = requests.put(f"{api_url}/suppliers/{created['id']}", headers=admin_headers,
                         json={"public_purpose": " Payments "})
        assert u.status_code == 200, u.text
        got = self._get(api_url, admin_headers, created["id"])
        assert got["public_purpose"] == "Payments"
        assert got["name"] == "Public fields supplier"  # no fallback stored

    def test_partial_put_keeps_and_empty_string_clears(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, public_name="Acme GmbH", public_purpose="Hosting")
        u = requests.put(f"{api_url}/suppliers/{created['id']}", headers=admin_headers, json={"notes": "n"})
        assert u.status_code == 200, u.text
        got = self._get(api_url, admin_headers, created["id"])
        assert got["public_name"] == "Acme GmbH" and got["public_purpose"] == "Hosting", got
        u = requests.put(f"{api_url}/suppliers/{created['id']}", headers=admin_headers, json={"public_purpose": ""})
        assert u.status_code == 200, u.text
        got = self._get(api_url, admin_headers, created["id"])
        assert got["public_purpose"] == "" and got["public_name"] == "Acme GmbH", got

    def test_over_length_is_400(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers)
        for field, limit in (("public_name", 200), ("public_purpose", 1000),
                             ("processing_location", 200), ("data_categories", 1000)):
            ok = requests.put(f"{api_url}/suppliers/{created['id']}", headers=admin_headers,
                              json={field: "é" * limit})
            assert ok.status_code == 200, (field, ok.text)
            bad = requests.put(f"{api_url}/suppliers/{created['id']}", headers=admin_headers,
                               json={field: "x" * (limit + 1)})
            assert bad.status_code == 400 and field in bad.text, (field, bad.status_code, bad.text)
            bad = requests.post(f"{api_url}/suppliers", headers=admin_headers,
                                json={"name": "too long", "supplier_type": "saas", "criticality": "low",
                                      field: "x" * (limit + 1)})
            assert bad.status_code == 400, (field, bad.status_code, bad.text)

    def test_review_keeps_public_fields(self, api_url, admin_headers):
        created = self._create(api_url, admin_headers, public_name="Acme GmbH", data_categories="Email")
        r = requests.post(f"{api_url}/suppliers/{created['id']}/reviews", headers=admin_headers,
                          json={"outcome": "satisfactory", "notes": "checked"})
        assert r.status_code == 201, r.text
        got = self._get(api_url, admin_headers, created["id"])
        assert got["public_name"] == "Acme GmbH" and got["data_categories"] == "Email", got


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
