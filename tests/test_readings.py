"""Entity readings tests — risk, legal, supplier readings and reading suggestions."""
import requests


class TestRiskReadings:
    """Risk reading lifecycle: submit readings, verify risk updates, check history."""

    risk_id = None
    first_reading_id = None
    second_reading_id = None

    def test_01_create_risk(self, api_url, admin_headers):
        """Create a risk to test readings on."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Reading test risk - data breach",
            "current_likelihood": 2,
            "current_impact": 2,
            "risk_type": "threat",
            "origin": "external",
            "status": "open",
            "treatment": "mitigate",
        })
        assert r.status_code in [200, 201], f"Create risk failed: {r.text}"
        data = r.json()
        assert data["id"]
        TestRiskReadings.risk_id = data["id"]

    def test_02_submit_reading(self, api_url, admin_headers):
        """POST /risks/:id/readings with assessment values should return 201."""
        rid = TestRiskReadings.risk_id
        r = requests.post(f"{api_url}/risks/{rid}/readings", headers=admin_headers, json={
            "current_likelihood": 4,
            "current_impact": 3,
            "confidentiality": 3,
            "integrity": 4,
            "availability": 2,
            "notes": "Initial assessment",
        })
        assert r.status_code in [200, 201], f"Submit reading failed: {r.text}"
        data = r.json()
        assert data.get("id"), "Expected reading ID"
        TestRiskReadings.first_reading_id = data["id"]

    def test_03_reading_has_fields(self, api_url, admin_headers):
        """Verify returned reading has all expected fields."""
        rid = TestRiskReadings.risk_id
        r = requests.get(f"{api_url}/risks/{rid}/readings", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        readings = data.get("data") if isinstance(data, dict) else data
        assert len(readings) >= 1
        reading = next(rd for rd in readings if rd["id"] == TestRiskReadings.first_reading_id)
        assert reading["current_likelihood"] == 4
        assert reading["current_impact"] == 3
        assert reading["confidentiality"] == 3
        assert reading["integrity"] == 4
        assert reading["availability"] == 2
        assert reading.get("notes") == "Initial assessment"
        assert reading.get("assessed_by"), "Expected assessed_by to be set"
        assert reading.get("created_at"), "Expected created_at to be set"

    def test_04_risk_updated(self, api_url, admin_headers):
        """GET the risk and verify current values updated from reading."""
        rid = TestRiskReadings.risk_id
        # Fetch by id directly — avoids pagination flakiness on a busy DB.
        r = requests.get(f"{api_url}/risks/{rid}", headers=admin_headers)
        assert r.status_code == 200, r.text
        risk = r.json()

        assert risk["current_likelihood"] == 4
        assert risk["current_impact"] == 3
        assert risk["current_score"] == 12
        assert risk["confidentiality_impact"] == 3
        assert risk["integrity_impact"] == 4
        assert risk["availability_impact"] == 2
        assert risk.get("last_review") is not None, "Expected last_review to be set"
        assert risk.get("next_review") is not None, "Expected next_review to be advanced"

    def test_05_list_readings(self, api_url, admin_headers):
        """GET /risks/:id/readings returns at least 1 reading."""
        rid = TestRiskReadings.risk_id
        r = requests.get(f"{api_url}/risks/{rid}/readings", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        readings = data.get("data") if isinstance(data, dict) else data
        assert isinstance(readings, list)
        assert len(readings) >= 1

    def test_06_submit_second_reading(self, api_url, admin_headers):
        """Submit another reading with different values, verify risk updates."""
        rid = TestRiskReadings.risk_id
        r = requests.post(f"{api_url}/risks/{rid}/readings", headers=admin_headers, json={
            "current_likelihood": 2,
            "current_impact": 5,
            "confidentiality": 5,
            "integrity": 3,
            "availability": 4,
            "notes": "Re-assessment after controls",
        })
        assert r.status_code in [200, 201], f"Submit second reading failed: {r.text}"
        data = r.json()
        TestRiskReadings.second_reading_id = data["id"]

        # Verify risk updated to new values — fetch by id.
        risk = requests.get(f"{api_url}/risks/{rid}", headers=admin_headers).json()
        assert risk["current_likelihood"] == 2
        assert risk["current_impact"] == 5
        assert risk["current_score"] == 10
        assert risk["confidentiality_impact"] == 5
        assert risk["integrity_impact"] == 3
        assert risk["availability_impact"] == 4

    def test_07_reading_history(self, api_url, admin_headers):
        """List readings shows both, most recent first."""
        rid = TestRiskReadings.risk_id
        r = requests.get(f"{api_url}/risks/{rid}/readings", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        readings = data.get("data") if isinstance(data, dict) else data
        assert len(readings) >= 2

        # Most recent first
        assert readings[0]["id"] == TestRiskReadings.second_reading_id
        assert readings[1]["id"] == TestRiskReadings.first_reading_id

    def test_08_reader_cannot_submit(self, api_url, reader_headers):
        """Reader role gets 403 when submitting a reading."""
        rid = TestRiskReadings.risk_id
        r = requests.post(f"{api_url}/risks/{rid}/readings", headers=reader_headers, json={
            "current_likelihood": 1,
            "current_impact": 1,
            "confidentiality": 1,
            "integrity": 1,
            "availability": 1,
            "notes": "Should be rejected",
        })
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"


class TestLegalReadings:
    """Legal requirement reading lifecycle."""

    legal_id = None

    def test_01_create_legal(self, api_url, admin_headers):
        """Create a legal requirement to test readings on."""
        r = requests.post(f"{api_url}/legal", headers=admin_headers, json={
            "title": "Reading test - Data Protection Act",
            "jurisdiction": "IS",
            "category": "privacy",
            "description": "National data protection law",
        })
        assert r.status_code in [200, 201], f"Create legal failed: {r.text}"
        data = r.json()
        assert data["id"]
        TestLegalReadings.legal_id = data["id"]

    def test_02_submit_reading(self, api_url, admin_headers):
        """POST /legal/:id/readings with assessment values."""
        lid = TestLegalReadings.legal_id
        r = requests.post(f"{api_url}/legal/{lid}/readings", headers=admin_headers, json={
            "current_likelihood": 3,
            "current_impact": 2,
        })
        assert r.status_code in [200, 201], f"Submit legal reading failed: {r.text}"
        data = r.json()
        assert data.get("id"), "Expected reading ID"

    def test_03_legal_updated(self, api_url, admin_headers):
        """Verify legal requirement updated with new values from reading."""
        lid = TestLegalReadings.legal_id
        r = requests.get(f"{api_url}/legal/{lid}", headers=admin_headers)
        assert r.status_code == 200, r.text
        legal = r.json()

        assert legal["current_likelihood"] == 3
        assert legal["current_impact"] == 2
        assert legal["current_score"] == 6

    def test_04_list_readings(self, api_url, admin_headers):
        """Verify reading appears in list."""
        lid = TestLegalReadings.legal_id
        r = requests.get(f"{api_url}/legal/{lid}/readings", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        readings = data.get("data") if isinstance(data, dict) else data
        assert isinstance(readings, list)
        assert len(readings) >= 1


class TestSupplierReview:
    """Supplier review lifecycle (not reading — review is confirm/verify)."""

    supplier_id = None

    def test_01_create_supplier(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Review Test Cloud Provider",
            "supplier_type": "cloud",
            "criticality": "high",
        })
        assert r.status_code in [200, 201], f"Create supplier failed: {r.text}"
        TestSupplierReview.supplier_id = r.json()["id"]

    def test_02_submit_review(self, api_url, admin_headers):
        """POST /suppliers/:id/reviews with notes (required)."""
        sid = TestSupplierReview.supplier_id
        r = requests.post(f"{api_url}/suppliers/{sid}/reviews", headers=admin_headers, json={
            "outcome": "satisfactory",
            "certifications_verified": True,
            "data_handling_verified": True,
            "sla_met": True,
            "notes": "Verified ISO 27001 cert valid until 2027. SLA metrics within bounds.",
        })
        assert r.status_code in [200, 201], f"Submit review failed: {r.text}"
        data = r.json()
        assert data.get("id")
        assert data["outcome"] == "satisfactory"

    def test_03_review_without_notes_fails(self, api_url, admin_headers):
        """Review without notes should fail — audit evidence requires notes."""
        sid = TestSupplierReview.supplier_id
        r = requests.post(f"{api_url}/suppliers/{sid}/reviews", headers=admin_headers, json={
            "outcome": "satisfactory",
        })
        assert r.status_code == 400, f"Expected 400, got {r.status_code}"

    def test_04_list_reviews(self, api_url, admin_headers):
        sid = TestSupplierReview.supplier_id
        r = requests.get(f"{api_url}/suppliers/{sid}/reviews", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") if isinstance(r.json(), dict) else r.json()
        assert len(data) >= 1

    def test_05_supplier_review_date_updated(self, api_url, admin_headers):
        """Supplier last_review should be updated after review."""
        sid = TestSupplierReview.supplier_id
        sup = requests.get(f"{api_url}/suppliers/{sid}", headers=admin_headers).json()
        assert sup.get("last_review"), "last_review should be set after review"


class TestSupplierReadings:
    """Supplier CIA reading lifecycle — regression for #161.

    The supplier reading path shipped but entity_readings' entity_type CHECK
    constraint never listed 'supplier', so POST /suppliers/:id/readings failed
    with entity_readings_entity_type_check (SQLSTATE 23514). This is the exact
    flow from the report: create supplier -> save CIA reading -> verify.
    """

    supplier_id = None
    reading_id = None

    def test_01_create_supplier(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": "Reading test - Managed Hosting Provider",
            "supplier_type": "cloud",
            "criticality": "high",
        })
        assert r.status_code in [200, 201], f"Create supplier failed: {r.text}"
        TestSupplierReadings.supplier_id = r.json()["id"]

    def test_02_submit_cia_reading(self, api_url, admin_headers):
        """POST /suppliers/:id/readings with CIA values should return 201 (#161)."""
        sid = TestSupplierReadings.supplier_id
        r = requests.post(f"{api_url}/suppliers/{sid}/readings", headers=admin_headers, json={
            "confidentiality": 4,
            "integrity": 3,
            "availability": 5,
            "notes": "Initial supplier CIA assessment",
        })
        assert r.status_code in [200, 201], f"Submit supplier reading failed: {r.text}"
        data = r.json()
        assert data.get("id"), "Expected reading ID"
        assert data["confidentiality"] == 4
        assert data["integrity"] == 3
        assert data["availability"] == 5
        TestSupplierReadings.reading_id = data["id"]

    def test_03_supplier_updated(self, api_url, admin_headers):
        """Supplier CIA classification + last_review updated from the reading."""
        sid = TestSupplierReadings.supplier_id
        r = requests.get(f"{api_url}/suppliers/{sid}", headers=admin_headers)
        assert r.status_code == 200, r.text
        sup = r.json()
        assert sup["confidentiality"] == 4
        assert sup["integrity"] == 3
        assert sup["availability"] == 5
        assert sup.get("last_review"), "Expected last_review to be set after reading"

    def test_04_list_readings(self, api_url, admin_headers):
        sid = TestSupplierReadings.supplier_id
        r = requests.get(f"{api_url}/suppliers/{sid}/readings", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        readings = data.get("data") if isinstance(data, dict) else data
        assert isinstance(readings, list)
        assert any(rd["id"] == TestSupplierReadings.reading_id for rd in readings)

    def test_05_reader_cannot_submit(self, api_url, reader_headers):
        """Reader role gets 403 when submitting a supplier reading."""
        sid = TestSupplierReadings.supplier_id
        r = requests.post(f"{api_url}/suppliers/{sid}/readings", headers=reader_headers, json={
            "confidentiality": 1,
            "integrity": 1,
            "availability": 1,
            "notes": "Should be rejected",
        })
        assert r.status_code == 403, f"Expected 403, got {r.status_code}: {r.text}"


class TestReadingSuggestion:
    """Reading via suggestion workflow: suggest reading -> apply -> verify."""

    suggestion_id = None
    risk_id = None

    def test_01_suggest_risk_reading(self, api_url, admin_headers):
        """Create a risk, then suggest a reading via the suggestion system."""
        # First create a risk to target
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Suggestion reading target risk",
            "current_likelihood": 1,
            "current_impact": 1,
            "risk_type": "threat",
            "origin": "internal",
            "status": "open",
        })
        assert r.status_code in [200, 201], f"Create risk failed: {r.text}"
        risk_data = r.json()
        TestReadingSuggestion.risk_id = risk_data["id"]
        risk_identifier = risk_data["identifier"]

        # Now suggest a reading — entity_id must be the string identifier (e.g. "RISK-1")
        r = requests.post(f"{api_url}/suggestions", headers=admin_headers, json={
            "entity_type": "risk",
            "entity_id": risk_identifier,
            "suggestion_type": "reading",
            "title": "Quarterly risk re-assessment",
            "rationale": "Scheduled quarterly review of risk levels",
            "payload": {
                "current_likelihood": 3,
                "current_impact": 4,
                "confidentiality": 2,
                "integrity": 3,
                "availability": 4,
                "notes": "Suggested from quarterly review",
            },
        })
        assert r.status_code in [200, 201], f"Create reading suggestion failed: {r.text}"
        data = r.json()
        assert data.get("id"), "Expected suggestion ID"
        assert data.get("status") == "open"
        TestReadingSuggestion.suggestion_id = data["id"]

    def test_02_apply_reading_suggestion(self, api_url, admin_headers):
        """Apply the reading suggestion, verify it succeeds."""
        sid = TestReadingSuggestion.suggestion_id
        r = requests.post(f"{api_url}/suggestions/{sid}/apply", headers=admin_headers, json={"force": True})
        assert r.status_code == 200, f"Apply reading suggestion failed: {r.text}"
        data = r.json()
        assert data.get("status") == "applied", f"Expected applied, got: {data}"

    def test_03_risk_updated_from_suggestion(self, api_url, admin_headers):
        """Verify risk has the suggested values after applying reading suggestion."""
        rid = TestReadingSuggestion.risk_id
        r = requests.get(f"{api_url}/risks/{rid}", headers=admin_headers)
        assert r.status_code == 200, r.text
        risk = r.json()

        assert risk["current_likelihood"] == 3
        assert risk["current_impact"] == 4
        assert risk["current_score"] == 12
        assert risk["confidentiality_impact"] == 2
        assert risk["integrity_impact"] == 3
        assert risk["availability_impact"] == 4

    def test_04_reading_exists(self, api_url, admin_headers):
        """Verify a reading record was created from the applied suggestion."""
        rid = TestReadingSuggestion.risk_id
        r = requests.get(f"{api_url}/risks/{rid}/readings", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        readings = data.get("data") if isinstance(data, dict) else data
        assert isinstance(readings, list)
        assert len(readings) >= 1, "Expected at least one reading from applied suggestion"
