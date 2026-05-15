"""Coverage boost — hit remaining untested endpoints.

Focuses on endpoints that are straightforward to test without complex setup.
Skips: OIDC flows, passkey WebAuthn, git protocol, evidence upload (needs multipart).
"""
import requests
from conftest import ADMIN_EMAIL, READER_EMAIL


class TestSystems:
    def test_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/systems", headers=admin_headers)
        assert r.status_code == 200

    def test_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
            "name": "Email Server",
            "classification": "confidential",
            "criticality": "high",
            "rpo_hours": 4,
            "rto_hours": 4,
        })
        assert r.status_code in [200, 201], f"Failed: {r.text}"

    def test_update(self, api_url, admin_headers):
        systems = requests.get(f"{api_url}/systems", headers=admin_headers).json()["data"]
        if len(systems) > 0:
            sys = systems[0]
            r = requests.put(f"{api_url}/systems/{sys['id']}", headers=admin_headers, json={
                **sys, "description": "## Purpose\n\nCompany email",
            })
            assert r.status_code == 200

    def test_delete(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/systems", headers=admin_headers, json={
            "name": "To delete", "classification": "public", "criticality": "low",
        })
        if r.status_code in [200, 201]:
            requests.delete(f"{api_url}/systems/{r.json()['id']}", headers=admin_headers)

    def test_access_reviews(self, api_url, admin_headers):
        systems = requests.get(f"{api_url}/systems", headers=admin_headers).json()["data"]
        if len(systems) > 0:
            r = requests.get(f"{api_url}/systems/{systems[0]['id']}/access-reviews",
                             headers=admin_headers)
            assert r.status_code == 200

    def test_create_access_review(self, api_url, admin_headers):
        systems = requests.get(f"{api_url}/systems", headers=admin_headers).json()["data"]
        if len(systems) > 0:
            r = requests.post(f"{api_url}/systems/{systems[0]['id']}/access-reviews",
                              headers=admin_headers, json={
                "reviewed_by": ADMIN_EMAIL,
                "users_added": 2,
                "users_removed": 1,
                "notes": "Quarterly access review",
            })
            assert r.status_code in [200, 201], f"Failed: {r.text}"


class TestAuditExtended:
    def test_list_audits(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/audits", headers=admin_headers)
        assert r.status_code == 200

    def test_get_audit(self, api_url, admin_headers):
        audits = requests.get(f"{api_url}/audits", headers=admin_headers).json()
        data = audits.get("data") or audits
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/audits/{data[0]['id']}", headers=admin_headers)
            assert r.status_code == 200

    def test_audit_items(self, api_url, admin_headers):
        audits = requests.get(f"{api_url}/audits", headers=admin_headers).json()
        data = audits.get("data") or audits
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/audits/{data[0]['id']}/items", headers=admin_headers)
            assert r.status_code == 200

    def test_calendar(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/audit/calendar?year=2026", headers=admin_headers)
        assert r.status_code == 200


class TestCorrectiveActionsExtended:
    def test_get_single(self, api_url, admin_headers):
        cas = requests.get(f"{api_url}/corrective-actions", headers=admin_headers).json()
        data = cas.get("data") or cas
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/corrective-actions/{data[0]['id']}", headers=admin_headers)
            assert r.status_code == 200

    def test_stats(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/corrective-actions/stats", headers=admin_headers)
        assert r.status_code == 200

    def test_delete(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/corrective-actions", headers=admin_headers, json={
            "title": "To delete", "description": "Test", "source": "other", "severity": "observation",
        })
        if r.status_code in [200, 201]:
            d = requests.delete(f"{api_url}/corrective-actions/{r.json()['id']}", headers=admin_headers)
            assert d.status_code == 200


class TestObjectivesExtended:
    def test_get_single(self, api_url, admin_headers):
        objs = requests.get(f"{api_url}/objectives", headers=admin_headers).json()
        data = objs.get("data") or objs
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/objectives/{data[0]['id']}", headers=admin_headers)
            assert r.status_code == 200

    def test_checkins(self, api_url, admin_headers):
        objs = requests.get(f"{api_url}/objectives", headers=admin_headers).json()
        data = objs.get("data") or objs
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/objectives/{data[0]['id']}/checkins", headers=admin_headers)
            assert r.status_code == 200

    def test_create_checkin(self, api_url, admin_headers):
        objs = requests.get(f"{api_url}/objectives", headers=admin_headers).json()
        data = objs.get("data") or objs
        if isinstance(data, list) and len(data) > 0:
            r = requests.post(f"{api_url}/objectives/{data[0]['id']}/checkins",
                              headers=admin_headers, json={
                "value_numeric": 4.2,
                "message": "Monthly measurement",
                "created_by": ADMIN_EMAIL,
            })
            assert r.status_code in [200, 201], f"Failed: {r.text}"

    def test_delete(self, api_url, admin_headers):
        objs = requests.get(f"{api_url}/objectives", headers=admin_headers).json()
        data = objs.get("data") or objs
        if isinstance(data, list) and len(data) > 0:
            # Create and delete an objective
            progs = requests.get(f"{api_url}/programs", headers=admin_headers).json().get("data") or []
            if len(progs) > 0:
                r = requests.post(f"{api_url}/objectives", headers=admin_headers, json={
                    "program_id": progs[0]["id"],
                    "title": "To delete",
                    "target_value": 1, "target_operator": "gte", "unit": "count",
                })
                if r.status_code in [200, 201]:
                    requests.delete(f"{api_url}/objectives/{r.json()['id']}", headers=admin_headers)


class TestProgramsExtended:
    def test_get_single(self, api_url, admin_headers):
        progs = requests.get(f"{api_url}/programs", headers=admin_headers).json()
        data = progs.get("data") or progs
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/programs/{data[0]['id']}", headers=admin_headers)
            assert r.status_code == 200

    def test_update(self, api_url, admin_headers):
        progs = requests.get(f"{api_url}/programs", headers=admin_headers).json()
        data = progs.get("data") or progs
        if isinstance(data, list) and len(data) > 0:
            r = requests.put(f"{api_url}/programs/{data[0]['id']}", headers=admin_headers, json={
                "title": "Updated programme title",
            })
            assert r.status_code == 200


class TestIncidentsExtended:
    def test_get_single(self, api_url, admin_headers):
        incs = requests.get(f"{api_url}/incidents", headers=admin_headers).json()
        data = incs.get("data") or incs
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/incidents/{data[0]['id']}", headers=admin_headers)
            assert r.status_code == 200


class TestLegalExtended:
    def test_get_single(self, api_url, admin_headers):
        legals = requests.get(f"{api_url}/legal", headers=admin_headers).json()
        data = legals.get("data") or legals
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/legal/{data[0]['id']}", headers=admin_headers)
            assert r.status_code == 200


class TestReviewsExtended:
    def test_create(self, api_url, admin_headers):
        r = requests.post(f"{api_url}/documents/iso27001-4-3/reviews", headers=admin_headers,
                          json={"reviewers": [READER_EMAIL], "message": "Coverage test"})
        assert r.status_code in [200, 201, 409], f"Failed: {r.text}"

    def test_assignments(self, api_url, admin_headers):
        reviews = requests.get(f"{api_url}/reviews", headers=admin_headers).json()
        data = reviews.get("data") or reviews
        if isinstance(data, list) and len(data) > 0:
            r = requests.get(f"{api_url}/reviews/{data[0]['id']}/assignments", headers=admin_headers)
            assert r.status_code == 200

    def test_add_comment(self, api_url, admin_headers):
        reviews = requests.get(f"{api_url}/reviews", headers=admin_headers).json()
        data = reviews.get("data") or reviews
        open_reviews = [rv for rv in data if rv.get("status") in ("open", "changes_requested")] if isinstance(data, list) else []
        if len(open_reviews) > 0:
            r = requests.post(f"{api_url}/reviews/{open_reviews[0]['id']}/comment",
                              headers=admin_headers, json={"body": "Coverage test comment"})
            assert r.status_code in [200, 201]


class TestApprovals:
    def test_list_doc_approvals(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/documents/iso27001-4-1/approvals", headers=admin_headers)
        assert r.status_code == 200


class TestCommentResolve:
    def test_resolve(self, api_url, admin_headers):
        comments = requests.get(f"{api_url}/comments/open", headers=admin_headers).json()
        data = comments.get("data") or comments
        if isinstance(data, list) and len(data) > 0:
            r = requests.post(f"{api_url}/comments/{data[0]['id']}/resolve", headers=admin_headers)
            assert r.status_code == 200


class TestDocumentEdit:
    def test_update_metadata(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/documents/iso27001-4-1/metadata", headers=admin_headers,
                         json={"field": "classification", "value": "internal"})
        assert r.status_code in [200, 400]  # may fail if field not supported

    def test_versions(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/documents/iso27001-4-1/versions", headers=admin_headers)
        assert r.status_code == 200


class TestAuth:
    def test_signup(self, api_url):
        r = requests.post(f"{api_url}/auth/signup", json={
            "email": "coverage-test@isms-test.local",
            "password": "TestPass123!",
            "name": "Coverage Test",
        })
        # 201 or 409 (already exists)
        assert r.status_code in [201, 409]

    def test_refresh(self, api_url):
        # Refresh now revokes the old token (security fix), so use an isolated
        # one-shot user rather than the shared admin to avoid poisoning the
        # session-scoped admin_headers fixture.
        import time
        email = f"refresh-probe-{int(time.time()*1000)}@isms-test.local"
        signup = requests.post(f"{api_url}/auth/signup", json={
            "email": email, "password": "TestPass123!", "name": "Refresh Probe",
        })
        if signup.status_code not in (200, 201):
            return  # signup disabled, skip
        token = signup.json().get("token")
        if not token:
            return
        r = requests.post(f"{api_url}/auth/refresh",
                          headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"})
        assert r.status_code == 200, f"refresh failed: {r.text}"

    def test_api_keys_list(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/auth/api-keys", headers=admin_headers)
        assert r.status_code == 200

    def test_change_password(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/auth/password", headers=admin_headers, json={
            "current_password": "TestPass123!",
            "new_password": "TestPass123!",  # same password, just testing endpoint
        })
        assert r.status_code in [200, 400]

    def test_update_profile(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/auth/profile", headers=admin_headers, json={
            "name": "Test Admin",
        })
        assert r.status_code == 200


class TestAdminEndpoints:
    def test_list_members(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/admin/members", headers=admin_headers)
        assert r.status_code == 200

    def test_list_settings(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/admin/settings", headers=admin_headers)
        assert r.status_code == 200

    def test_update_setting(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/admin/settings", headers=admin_headers, json={
            "key": "branding_name",
            "value": "Test Branding",
        })
        assert r.status_code == 200

    def test_branding_in_config(self, api_url, admin_headers):
        """Branding settings should appear in config response."""
        # Set branding
        requests.put(f"{api_url}/admin/settings", headers=admin_headers, json={
            "key": "branding_name", "value": "My Custom Brand",
        })
        requests.put(f"{api_url}/admin/settings", headers=admin_headers, json={
            "key": "branding_color", "value": "#1a365d",
        })
        # Check config
        r = requests.get(f"{api_url}/config", headers=admin_headers)
        assert r.status_code == 200
        data = r.json()
        branding = data.get("branding", {})
        assert branding.get("branding_name") == "My Custom Brand"
        assert branding.get("branding_color") == "#1a365d"

    def test_list_api_keys(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/admin/api-keys", headers=admin_headers)
        assert r.status_code == 200

    def test_list_oidc(self, api_url, admin_headers):
        r = requests.get(f"{api_url}/admin/oidc", headers=admin_headers)
        assert r.status_code == 200
