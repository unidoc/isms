"""Universal search tests — verify search index works across all entity types."""
import requests
from conftest import ADMIN_EMAIL


class TestSearchIndex:
    """Search should find entities across all types."""

    def test_01_create_test_data(self, api_url, admin_headers):
        """Create entities to search for."""
        # Risk
        requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Searchable ransomware risk",
            "description": "Ransomware attack on production",
            "risk_type": "threat", "origin": "external", "status": "open",
            "current_likelihood": 4, "current_impact": 4,
        })
        # Supplier
        requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Searchable Cloud Corp",
            "supplier_type": "cloud", "criticality": "high",
        })
        # Asset
        requests.post(f"{api_url}/assets", headers=admin_headers, json={
            "name": "Searchable Production DB",
            "asset_type": "software", "status": "open",
        })

    def test_02_search_empty_returns_all(self, api_url, admin_headers):
        """Empty query returns a variety of entities (up to 50)."""
        r = requests.get(f"{api_url}/search?q=", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        assert len(data) > 0, "Expected at least some results"
        # Should return multiple entity types — exact selection is non-deterministic
        # (depends on goroutine completion order during index build)
        types = set(d["type"] for d in data)
        assert len(types) >= 2, f"Expected multiple entity types, got: {types}"

    def test_03_search_by_title(self, api_url, admin_headers):
        """Search by entity title."""
        r = requests.get(f"{api_url}/search?q=ransomware", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        risk_results = [d for d in data if d["type"] == "risk"]
        assert len(risk_results) >= 1, "Expected to find ransomware risk"
        assert "ransomware" in risk_results[0]["title"].lower()

    def test_04_search_by_identifier(self, api_url, admin_headers):
        """Search by entity identifier."""
        r = requests.get(f"{api_url}/search?q=RISK-", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        assert any(d["type"] == "risk" for d in data), "Expected risk results for RISK- query"

    def test_05_search_supplier(self, api_url, admin_headers):
        """Search finds suppliers."""
        r = requests.get(f"{api_url}/search?q=cloud+corp", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        supplier_results = [d for d in data if d["type"] == "supplier"]
        assert len(supplier_results) >= 1, "Expected to find Cloud Corp supplier"

    def test_06_search_asset(self, api_url, admin_headers):
        """Search finds assets."""
        r = requests.get(f"{api_url}/search?q=production+db", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        asset_results = [d for d in data if d["type"] == "asset"]
        assert len(asset_results) >= 1, "Expected to find Production DB asset"

    def test_07_search_documents(self, api_url, admin_headers):
        """Search finds documents by document_id."""
        # Search by document ID prefix — templates always scaffold iso27001 docs
        # Document type in results comes from frontmatter (clause, control, policy, etc.)
        r = requests.get(f"{api_url}/search?q=iso27001", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        doc_types = {'document', 'clause', 'control', 'policy', 'procedure', 'requirement', 'record', 'guideline'}
        doc_results = [d for d in data if d["type"] in doc_types]
        assert len(doc_results) >= 1, f"Expected documents matching 'iso27001', got {[d['type'] for d in data]}"

    def test_08_search_case_insensitive(self, api_url, admin_headers):
        """Search is case insensitive."""
        r1 = requests.get(f"{api_url}/search?q=RANSOMWARE", headers=admin_headers)
        r2 = requests.get(f"{api_url}/search?q=ransomware", headers=admin_headers)
        assert r1.status_code == 200
        assert r2.status_code == 200
        assert len(r1.json().get("data") or []) == len(r2.json().get("data") or [])

    def test_09_search_returns_type_badges(self, api_url, admin_headers):
        """Each result has type, id, and title."""
        r = requests.get(f"{api_url}/search?q=", headers=admin_headers)
        data = r.json().get("data") or []
        for item in data[:5]:
            assert "type" in item, "Missing type"
            assert "id" in item, "Missing id"
            assert "title" in item, "Missing title"

    def test_10_search_max_50_results(self, api_url, admin_headers):
        """Results are capped at 50."""
        r = requests.get(f"{api_url}/search?q=", headers=admin_headers)
        data = r.json().get("data") or []
        assert len(data) <= 50

    def test_11_second_search_is_cached(self, api_url, admin_headers):
        """Second search should be fast (served from cache)."""
        import time
        # First search builds the index
        requests.get(f"{api_url}/search?q=test", headers=admin_headers)
        # Second should be instant (cached)
        start = time.time()
        r = requests.get(f"{api_url}/search?q=test", headers=admin_headers)
        elapsed = time.time() - start
        assert r.status_code == 200
        assert elapsed < 0.1, f"Cached search took {elapsed:.3f}s — should be <100ms"

    def test_12_reader_can_search(self, api_url, reader_headers):
        """Readers should be able to search too."""
        r = requests.get(f"{api_url}/search?q=", headers=reader_headers)
        assert r.status_code == 200
