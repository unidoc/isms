"""Universal search tests — verify search index works across all entity types."""
import uuid

import requests
from conftest import ADMIN_EMAIL, API


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

    def test_02_search_empty_returns_results(self, api_url, admin_headers):
        """Empty query returns indexed entries, capped at 50.

        No assertion on type variety: the endpoint returns the first 50
        entries in index-build order (goroutine completion order), so once
        any single type accumulates 50+ contiguous entries the first page is
        homogeneous. Type reachability is covered by the targeted-query tests
        below (title, identifier, documents).
        """
        r = requests.get(f"{api_url}/search?q=", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        assert len(data) > 0, "Expected at least some results"
        assert len(data) <= 50, f"Expected the 50-entry cap, got {len(data)}"

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


def _search(api_url, headers, q):
    r = requests.get(f"{api_url}/search", params={"q": q}, headers=headers)
    assert r.status_code == 200, r.text
    return r.json().get("data") or []


def _find(results, type_, id_):
    return [d for d in results if d["type"] == type_ and d["id"] == id_]


class TestAuditSearch:
    """Audits, findings and audit programmes are searchable (#436).

    SearchIndex.Upsert is a no-op while no index exists for the org, and the
    first search builds the index synchronously. So every freshness test forces
    the build with a search FIRST, mutates, and only then searches again.
    """

    @staticmethod
    def _seed(api_url, headers, tag):
        r = requests.post(f"{api_url}/audit/programmes", headers=headers, json={
            "title": f"Prog {tag}", "year": 2026})
        assert r.status_code == 201, r.text
        prog = r.json()
        r = requests.post(f"{api_url}/audits", headers=headers, json={
            "title": f"Audit {tag}", "status": "planned", "programme_id": prog["id"]})
        assert r.status_code == 201, r.text
        audit = r.json()
        r = requests.post(f"{api_url}/audit-findings", headers=headers, json={
            "audit_id": audit["id"], "title": f"Finding {tag}", "finding_type": "minor_nc"})
        assert r.status_code == 201, r.text
        return prog, audit, r.json()

    def test_01_found_by_title_and_display_id(self, api_url, admin_headers):
        tag = uuid.uuid4().hex[:10]
        prog, audit, finding = self._seed(api_url, admin_headers, tag)
        cases = [
            ("audit_programme", f"AP-{prog['id']}", f"Prog {tag}"),
            ("audit", f"AUDIT-{audit['id']}", f"Audit {tag}"),
            ("audit_finding", f"FIND-{finding['id']}", f"Finding {tag}"),
        ]
        for type_, display_id, title in cases:
            by_title = _find(_search(api_url, admin_headers, tag), type_, display_id)
            assert len(by_title) == 1, f"{type_} not found by title fragment"
            assert by_title[0]["title"] == title
            by_id = _find(_search(api_url, admin_headers, display_id), type_, display_id)
            assert len(by_id) == 1, f"{type_} not found by display id {display_id}"

    def test_02_create_is_found_immediately(self, api_url, admin_headers):
        _search(api_url, admin_headers, "")  # force the index to exist
        tag = uuid.uuid4().hex[:10]
        prog, audit, finding = self._seed(api_url, admin_headers, tag)
        results = _search(api_url, admin_headers, tag)
        assert _find(results, "audit_programme", f"AP-{prog['id']}"), "new programme missing"
        assert _find(results, "audit", f"AUDIT-{audit['id']}"), "new audit missing"
        assert _find(results, "audit_finding", f"FIND-{finding['id']}"), "new finding missing"

    def test_03_rename_is_reflected_immediately(self, api_url, admin_headers):
        old = uuid.uuid4().hex[:10]
        new = uuid.uuid4().hex[:10]
        prog, audit, finding = self._seed(api_url, admin_headers, old)
        _search(api_url, admin_headers, "")  # force the index to exist
        r = requests.put(f"{api_url}/audit/programmes/{prog['id']}", headers=admin_headers,
                         json={"title": f"Prog {new}"})
        assert r.status_code == 200, r.text
        r = requests.put(f"{api_url}/audits/{audit['id']}", headers=admin_headers,
                         json={"title": f"Audit {new}"})
        assert r.status_code == 200, r.text
        r = requests.put(f"{api_url}/audit-findings/{finding['id']}", headers=admin_headers,
                         json={"title": f"Finding {new}"})
        assert r.status_code == 200, r.text

        found = _search(api_url, admin_headers, new)
        stale = _search(api_url, admin_headers, old)
        for type_, display_id, title in [
            ("audit_programme", f"AP-{prog['id']}", f"Prog {new}"),
            ("audit", f"AUDIT-{audit['id']}", f"Audit {new}"),
            ("audit_finding", f"FIND-{finding['id']}", f"Finding {new}"),
        ]:
            hit = _find(found, type_, display_id)
            assert len(hit) == 1, f"renamed {type_} not found under new title"
            assert hit[0]["title"] == title
            assert not _find(stale, type_, display_id), f"renamed {type_} still found under old title"

    def test_04_delete_removes_from_search(self, api_url, admin_headers):
        tag = uuid.uuid4().hex[:10]
        # A programme with no audits (deletable) and a finding on another audit.
        r = requests.post(f"{api_url}/audit/programmes", headers=admin_headers, json={
            "title": f"Lonely {tag}", "year": 2026})
        assert r.status_code == 201, r.text
        prog = r.json()
        _, _, finding = self._seed(api_url, admin_headers, tag)
        _search(api_url, admin_headers, "")  # force the index to exist
        assert _find(_search(api_url, admin_headers, tag), "audit_programme", f"AP-{prog['id']}")
        assert _find(_search(api_url, admin_headers, tag), "audit_finding", f"FIND-{finding['id']}")

        r = requests.delete(f"{api_url}/audit-findings/{finding['id']}", headers=admin_headers)
        assert r.status_code == 200, r.text
        r = requests.delete(f"{api_url}/audit/programmes/{prog['id']}", headers=admin_headers)
        assert r.status_code == 200, r.text

        results = _search(api_url, admin_headers, tag)
        assert not _find(results, "audit_finding", f"FIND-{finding['id']}"), "deleted finding still found"
        assert not _find(results, "audit_programme", f"AP-{prog['id']}"), "deleted programme still found"


class TestFindingSeedHeadingNotSearchable:
    """A finding created without a description is seeded with a "## Corrective
    Action" heading. That scaffolding must not make every finding match
    "corrective action" (#451); text the user wrote still does."""

    PASSWORD = "TestPass123!"

    def _fresh_org_headers(self):
        suffix = uuid.uuid4().hex[:8]
        email = f"find-search-{suffix}@isms-test.local"
        slug = f"find-search-{suffix}"
        r = requests.post(f"{API}/auth/signup", json={"email": email, "password": self.PASSWORD, "name": "Finding Search"})
        assert r.status_code in (200, 201), r.text
        headers = {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}
        r = requests.post(f"{API}/organizations", headers=headers, json={"name": slug, "slug": slug})
        assert r.status_code in (200, 201), r.text
        r = requests.post(f"{API}/auth/login", json={"email": email, "password": self.PASSWORD, "organization": slug})
        assert r.status_code == 200, r.text
        return email, {"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"}

    def _finding(self, headers, audit_id, title, **extra):
        r = requests.post(f"{API}/audit-findings", headers=headers, json={
            "audit_id": audit_id, "finding_type": "minor_nc", "title": title, **extra,
        })
        assert r.status_code in (200, 201), r.text
        return r.json()["id"]

    def _finding_ids(self, headers, q):
        r = requests.get(f"{API}/search", headers=headers, params={"q": q})
        assert r.status_code == 200, r.text
        return {i["id"] for i in (r.json().get("data") or []) if i.get("type") == "audit_finding"}

    def test_seeded_heading_does_not_match(self):
        email, headers = self._fresh_org_headers()
        token = uuid.uuid4().hex[:8]

        r = requests.post(f"{API}/audit/programmes", headers=headers, json={"title": "Seed programme", "year": 2026})
        assert r.status_code in (200, 201), r.text
        r = requests.post(f"{API}/audits", headers=headers, json={
            "programme_id": r.json()["id"], "title": "Seed audit", "scope": "iso27001-*",
            "auditor": email, "audit_type": "internal", "planned_date": 1781740800,
        })
        assert r.status_code in (200, 201), r.text
        audit_id = r.json()["id"]
        requests.put(f"{API}/audits/{audit_id}/status", headers=headers, json={"status": "in_progress"})

        # A goes through the index build, B through the handler upsert.
        a = self._finding(headers, audit_id, f"Plain finding A {token}")
        assert self._finding_ids(headers, token) == {f"FIND-{a}"}
        b = self._finding(headers, audit_id, f"Plain finding B {token}")
        assert self._finding_ids(headers, token) == {f"FIND-{a}", f"FIND-{b}"}

        c = self._finding(headers, audit_id, f"Corrective action overdue {token}")
        d = self._finding(headers, audit_id, f"Finding D {token}",
                          description=f"## Corrective Action\n\nRotate {token} keys")

        for q in ("corrective action", "corrective", "action"):
            assert self._finding_ids(headers, q) == {f"FIND-{c}"}, q
        assert self._finding_ids(headers, f"rotate {token}") == {f"FIND-{d}"}
