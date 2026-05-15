"""Risk register tests — comprehensive coverage."""
import requests
from conftest import ADMIN_EMAIL


class TestRiskCRUD:
    """Basic create, read, update, delete."""

    def test_create_seeds_potential_consequences(self, api_url, admin_headers):
        """When description is empty on create, it's seeded with the heading template."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Seed test risk",
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"
        assert "## Potential consequences" in r.json().get("description", "")

    def test_create_risk(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Ransomware attack on primary systems",
            "current_likelihood": 4,
            "current_impact": 5,
            "risk_type": "threat",
            "origin": "external",
            "status": "open",
            "treatment": "mitigate",
        })
        assert r.status_code in [200, 201], f"Create failed: {r.text}"
        data = r.json()
        assert data["identifier"].startswith("RISK-")
        assert data["current_score"] == 20
        assert data["current_level"] == "critical"
        assert data["next_review"] is not None

    def test_create_risk_minimal_succeeds(self, api_url, admin_headers):
        """Light-create form: title alone is sufficient; backend defaults the rest."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Minimal risk via light form",
        })
        assert r.status_code in [200, 201], r.text
        data = r.json()
        # Server-side defaults — user refines via edit modal
        assert data["risk_type"] == "threat"
        assert data["origin"] == "internal"
        assert data["status"] == "open"

    def test_list_risks(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/risks", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        assert "data" in data
        assert isinstance(data["data"], list)
        assert len(data["data"]) >= 1

    def test_update_risk(self, api_url, admin_headers):
        """Update a risk and verify changes persist."""
        # Get first risk
        risks = requests.get(f"{api_url}/risks", headers=admin_headers).json()["data"]
        risk_id = risks[0]["id"]

        r = requests.put(f"{api_url}/risks/{risk_id}", headers=admin_headers, json={
            **risks[0],
            "title": "Updated ransomware risk",
            "current_likelihood": 3,
            "current_impact": 4,
            "treatment": "transfer",
        })
        assert r.status_code == 200, f"Update failed: {r.text}"

        # Verify — fetch by id to avoid pagination flakiness when the DB has many risks.
        risk = requests.get(f"{api_url}/risks/{risk_id}", headers=admin_headers).json()
        assert risk["title"] == "Updated ransomware risk"
        assert risk["current_likelihood"] == 3
        assert risk["current_impact"] == 4
        assert risk["current_score"] == 12
        assert risk["current_level"] == "high"
        assert risk["treatment"] == "transfer"

    def test_delete_risk(self, api_url, admin_headers):
        """Create and delete a risk."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "To be deleted",
            "current_likelihood": 1, "current_impact": 1,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201]
        risk_id = r.json()["id"]

        d = requests.delete(f"{api_url}/risks/{risk_id}", headers=admin_headers)
        assert d.status_code == 200


class TestRiskScoring:
    """Auto-calculated scores and levels."""

    def test_score_computation(self, api_url, admin_headers):
        """Score = likelihood * impact, level derived from score."""
        cases = [
            (1, 1, 1, "low"),
            (2, 2, 4, "low"),
            (3, 2, 6, "medium"),
            (4, 3, 12, "high"),
            (4, 4, 16, "critical"),
            (5, 5, 25, "critical"),
        ]
        for likelihood, impact, expected_score, expected_level in cases:
            r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
                "title": f"Score test L{likelihood}xI{impact}",
                "current_likelihood": likelihood,
                "current_impact": impact,
                "risk_type": "threat", "origin": "internal", "status": "open",
            })
            assert r.status_code in [200, 201], f"Create failed: {r.text}"
            data = r.json()
            assert data["current_score"] == expected_score, \
                f"L{likelihood}xI{impact}: expected score {expected_score}, got {data['current_score']}"
            assert data["current_level"] == expected_level, \
                f"Score {expected_score}: expected {expected_level}, got {data['current_level']}"

    def test_null_assessment(self, api_url, admin_headers):
        """Risks with no assessment fields should have null scores."""
        r = requests.get(f"{api_url}/risks", headers=admin_headers)
        for risk in r.json()["data"]:
            if risk.get("inherent_likelihood") is None:
                assert risk.get("inherent_score") is None, \
                    f"{risk['identifier']}: null inherent_likelihood but non-null inherent_score"

    def test_inherent_score(self, api_url, admin_headers):
        """Inherent assessment should auto-calculate."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Inherent score test",
            "current_likelihood": 3, "current_impact": 3,
            "inherent_likelihood": 5, "inherent_impact": 4,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data["inherent_score"] == 20

    def test_target_score(self, api_url, admin_headers):
        """Target assessment should auto-calculate."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Target score test",
            "current_likelihood": 4, "current_impact": 4,
            "target_likelihood": 2, "target_impact": 2,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data["target_score"] == 4
        assert data["target_level"] == "low"


class TestRiskReviewDates:
    """Auto-calculated review dates based on current_level."""

    def test_critical_next_review(self, api_url, admin_headers):
        """Critical risks get ~30 day review cycle."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Critical review date test",
            "current_likelihood": 5, "current_impact": 5,
            "risk_type": "threat", "origin": "external", "status": "open",
        })
        assert r.status_code in [200, 201]
        assert r.json()["next_review"] is not None

    def test_next_review_updates_on_score_change(self, api_url, admin_headers):
        """Review date should update when score changes."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Review date update test",
            "current_likelihood": 5, "current_impact": 5,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        risk_id = r.json()["id"]
        original_review = r.json()["next_review"]

        # Lower the score — review date should push out
        updated = requests.put(f"{api_url}/risks/{risk_id}", headers=admin_headers, json={
            **r.json(),
            "current_likelihood": 1,
            "current_impact": 1,
        })
        assert updated.status_code == 200
        new_review = updated.json()["next_review"]
        assert new_review != original_review, "Review date should change when level changes"
        assert new_review > original_review, "Low risk should have later review date than critical"


class TestRiskOrigins:
    """Valid origin values."""

    def test_internal(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Internal origin", "current_likelihood": 2, "current_impact": 2,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201]

    def test_external(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "External origin", "current_likelihood": 2, "current_impact": 2,
            "risk_type": "threat", "origin": "external", "status": "open",
        })
        assert r.status_code in [200, 201]

    def test_internal_and_external(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Both origins", "current_likelihood": 2, "current_impact": 2,
            "risk_type": "threat", "origin": "internal and external", "status": "open",
        })
        assert r.status_code in [200, 201]

    def test_invalid_origin(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Bad origin", "current_likelihood": 2, "current_impact": 2,
            "risk_type": "threat", "origin": "somewhere", "status": "open",
        })
        assert r.status_code in [400, 500]


class TestRiskValidation:
    """Field validation."""

    def test_likelihood_out_of_range(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Bad likelihood", "current_likelihood": 6, "current_impact": 3,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [400, 500]

    def test_impact_zero_allowed(self, api_url, admin_headers):
        """Zero impact is valid (not assessed)."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Zero impact", "current_likelihood": 3, "current_impact": 0,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201]

    def test_invalid_status(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Bad status", "current_likelihood": 3, "current_impact": 3,
            "risk_type": "threat", "origin": "internal", "status": "invalid",
        })
        assert r.status_code in [400, 500]

    def test_invalid_treatment(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Bad treatment", "current_likelihood": 3, "current_impact": 3,
            "risk_type": "threat", "origin": "internal", "status": "open",
            "treatment": "ignore",
        })
        assert r.status_code in [400, 500]

    def test_invalid_risk_type(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Bad type", "current_likelihood": 3, "current_impact": 3,
            "risk_type": "unknown", "origin": "internal", "status": "open",
        })
        assert r.status_code in [400, 500]


class TestRiskRequiredFields:
    """Missing required fields should return 400/500, not crash."""

    def test_missing_risk_type_defaults(self, api_url, admin_headers):
        """risk_type defaults to 'threat' when omitted (light-create form)."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Defaults risk_type",
            "current_likelihood": 3, "current_impact": 3,
            "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201], r.text
        assert r.json()["risk_type"] == "threat"

    def test_missing_origin_defaults(self, api_url, admin_headers):
        """origin defaults to 'internal' when omitted (light-create form)."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Defaults origin",
            "current_likelihood": 3, "current_impact": 3,
            "risk_type": "threat", "status": "open",
        })
        assert r.status_code in [200, 201], r.text
        assert r.json()["origin"] == "internal"

    def test_create_with_defaults(self, api_url, admin_headers):
        """Create with only title + L/I + risk_type + origin should work."""
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Minimal risk",
            "current_likelihood": 2, "current_impact": 2,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201], f"Minimal create failed: {r.text}"


class TestRiskIdentifiers:
    """Per-org sequential identifiers."""

    def test_sequential(self, api_url, admin_headers):
        """Identifiers should be sequential within org."""
        ids = []
        for i in range(3):
            r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
                "title": f"Sequential test {i}",
                "current_likelihood": 2, "current_impact": 2,
                "risk_type": "threat", "origin": "internal", "status": "open",
            })
            assert r.status_code in [200, 201]
            ids.append(r.json()["identifier"])

        # All should be RISK-NNN format
        for ident in ids:
            assert ident.startswith("RISK-")

        # Numbers should be sequential
        nums = [int(i.split("-")[1]) for i in ids]
        assert nums == sorted(nums)
        assert nums[-1] - nums[0] == 2


class TestRiskRBAC:
    """Role-based access control."""

    def test_reader_cannot_create(self, api_url, reader_headers):
        r = requests.post(f"{api_url}/risks", headers=reader_headers, json={
            "title": "Should fail", "current_likelihood": 3, "current_impact": 3,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code == 403

    def test_reader_can_read(self, api_url, reader_headers):
        r = requests.get(f"{api_url}/risks", headers=reader_headers)
        assert r.status_code == 200

    def test_reader_cannot_delete(self, api_url, admin_headers, reader_headers):
        # Create as admin
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "RBAC delete test", "current_likelihood": 1, "current_impact": 1,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        risk_id = r.json()["id"]

        # Delete as reader should fail
        d = requests.delete(f"{api_url}/risks/{risk_id}", headers=reader_headers)
        assert d.status_code == 403


class TestRiskCIA:
    """CIA impact fields."""

    def test_cia_stored(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "CIA test",
            "current_likelihood": 3, "current_impact": 3,
            "confidentiality_impact": 5,
            "integrity_impact": 3,
            "availability_impact": 1,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data["confidentiality_impact"] == 5
        assert data["integrity_impact"] == 3
        assert data["availability_impact"] == 1

    def test_cia_null_when_not_set(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "CIA null test",
            "current_likelihood": 2, "current_impact": 2,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201]
        data = r.json()
        assert data["confidentiality_impact"] is None
        assert data["integrity_impact"] is None
        assert data["availability_impact"] is None


class TestRiskEditZeroValues:
    """Editing a risk with zero-value fields should not crash."""

    risk_id = None

    def test_01_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": "Zero value edit test",
            "current_likelihood": 3, "current_impact": 3,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code in [200, 201], f"Create failed: {r.text}"
        TestRiskEditZeroValues.risk_id = r.json()["id"]

    def test_02_edit_with_zero_cia(self, api_url, admin_headers):
        """Zero CIA values (not assessed) should be accepted."""
        rid = TestRiskEditZeroValues.risk_id
        r = requests.put(f"{api_url}/risks/{rid}", headers=admin_headers, json={
            "title": "Zero value edit test",
            "current_likelihood": 3, "current_impact": 3,
            "confidentiality_impact": 0, "integrity_impact": 0, "availability_impact": 0,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code == 200, f"Edit with zero CIA failed: {r.text}"

    def test_03_edit_with_zero_inherent(self, api_url, admin_headers):
        """Zero inherent scores (not assessed) should be accepted."""
        rid = TestRiskEditZeroValues.risk_id
        r = requests.put(f"{api_url}/risks/{rid}", headers=admin_headers, json={
            "title": "Zero value edit test",
            "current_likelihood": 3, "current_impact": 3,
            "inherent_likelihood": 0, "inherent_impact": 0,
            "target_likelihood": 0, "target_impact": 0,
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code == 200, f"Edit with zero inherent/target failed: {r.text}"

    def test_04_edit_with_treatment_plan(self, api_url, admin_headers):
        """Treatment plan with markdown should be saved."""
        rid = TestRiskEditZeroValues.risk_id
        r = requests.put(f"{api_url}/risks/{rid}", headers=admin_headers, json={
            "title": "Zero value edit test",
            "current_likelihood": 3, "current_impact": 3,
            "treatment_plan": "Mitigate via [Access Control](/documents/iso27001-a-5-1)",
            "notes": "See also [BCP](/documents/iso27001-a-5-30)",
            "risk_type": "threat", "origin": "internal", "status": "open",
        })
        assert r.status_code == 200, f"Edit with markdown fields failed: {r.text}"
        # Verify fields saved
        risk = requests.get(f"{api_url}/risks/{rid}", headers=admin_headers).json()
        assert "Access Control" in risk["treatment_plan"]
        assert "BCP" in risk["notes"]
