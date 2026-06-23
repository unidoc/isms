"""Regressions for two security bugs:

#24 — a contributor could change incident / change-request status through the
      *general* edit endpoint (PUT /incidents/:id, PUT /changes/:id), bypassing
      the manager/admin role the dedicated /status endpoints enforce.

#27 — disabling OTP required no re-authentication: a hijacked session could
      silently remove 2FA. Disabling now requires a valid current OTP code.
"""
import base64
import hashlib
import hmac
import struct
import time
import uuid

import requests
from conftest import _signup, ADMIN_EMAIL


# ── #24: status changes via the general edit endpoint are manager/admin only ──

def _make_incident(api_url, admin_headers):
    r = requests.post(f"{api_url}/incidents", headers=admin_headers, json={
        "title": "RBAC probe incident", "description": "for status-rbac test",
        "severity": "medium", "incident_type": "incident", "source": "internal",
        "reporter": ADMIN_EMAIL,
    })
    assert r.status_code in (200, 201), r.text
    return r.json()["id"]


def _make_change(api_url, admin_headers):
    r = requests.post(f"{api_url}/changes", headers=admin_headers, json={
        "title": "RBAC probe change", "description": "for status-rbac test",
        "justification": "test", "priority": "medium", "category": "technology",
        "risk_level": "low", "rollback_plan": "revert",
    })
    assert r.status_code in (200, 201), r.text
    return r.json()["id"]


class TestStatusChangeRBAC:
    def test_contributor_cannot_change_incident_status_via_general_edit(
            self, api_url, admin_headers, contributor_headers):
        iid = _make_incident(api_url, admin_headers)
        r = requests.put(f"{api_url}/incidents/{iid}", headers=contributor_headers,
                         json={"status": "investigating"})
        assert r.status_code == 403, f"contributor changed incident status (#24): {r.status_code} {r.text}"

    def test_contributor_can_still_edit_nonstatus_incident_fields(
            self, api_url, admin_headers, contributor_headers):
        iid = _make_incident(api_url, admin_headers)
        r = requests.put(f"{api_url}/incidents/{iid}", headers=contributor_headers,
                         json={"description": "contributor edited the description"})
        assert r.status_code == 200, f"non-status edit must still work: {r.status_code} {r.text}"

    def test_manager_can_change_incident_status_via_general_edit(
            self, api_url, admin_headers):
        iid = _make_incident(api_url, admin_headers)
        r = requests.put(f"{api_url}/incidents/{iid}", headers=admin_headers,
                         json={"status": "investigating"})
        assert r.status_code == 200, f"admin/manager status change must work: {r.text}"

    def test_contributor_cannot_change_change_status_via_general_edit(
            self, api_url, admin_headers, contributor_headers):
        cid = _make_change(api_url, admin_headers)
        r = requests.put(f"{api_url}/changes/{cid}", headers=contributor_headers,
                         json={"status": "approved"})
        assert r.status_code == 403, f"contributor changed change-request status (#24): {r.status_code} {r.text}"

    def test_manager_can_change_change_status_via_general_edit(
            self, api_url, admin_headers):
        cid = _make_change(api_url, admin_headers)
        r = requests.put(f"{api_url}/changes/{cid}", headers=admin_headers,
                         json={"status": "approved"})
        assert r.status_code == 200, f"admin/manager change status must work: {r.text}"


# ── #27: disabling OTP requires a valid current code ──

def _totp(secret_b32, at=None):
    """RFC 6238 TOTP (SHA1, 6 digits, 30s) for a base32 secret (no padding)."""
    padded = secret_b32 + "=" * (-len(secret_b32) % 8)
    key = base64.b32decode(padded, casefold=True)
    counter = int((at or time.time()) // 30)
    digest = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
    off = digest[-1] & 0x0F
    code = (struct.unpack(">I", digest[off:off + 4])[0] & 0x7FFFFFFF) % 1_000_000
    return f"{code:06d}"


class TestOTPDisableRequiresReauth:
    def test_disable_otp_needs_current_code(self, api_url):
        # Unique throwaway user per run — never touch a shared account's 2FA
        # state on the persistent stack.
        email = f"otp-{uuid.uuid4().hex[:8]}@isms-test.local"
        token = _signup(email, "TestPass123!", "OTP Disable Test")
        assert token, "signup did not return a token"
        h = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}

        # Enable OTP.
        r = requests.post(f"{api_url}/auth/otp/setup", headers=h, json={})
        assert r.status_code == 200, f"otp setup: {r.text}"
        secret = r.json()["secret"]
        r = requests.post(f"{api_url}/auth/otp/verify", headers=h, json={"code": _totp(secret)})
        assert r.status_code == 200, f"otp verify (enable): {r.text}"

        # Disable WITHOUT a code → rejected (the #27 fix).
        r = requests.delete(f"{api_url}/auth/otp", headers=h)
        assert r.status_code == 400, f"disable without code must be rejected (#27): {r.status_code} {r.text}"

        # Disable with a WRONG code → rejected.
        r = requests.delete(f"{api_url}/auth/otp", headers=h, json={"code": "000000"})
        assert r.status_code in (400, 401), f"disable with wrong code must be rejected: {r.status_code} {r.text}"

        # Disable with a VALID current code → succeeds.
        r = requests.delete(f"{api_url}/auth/otp", headers=h, json={"code": _totp(secret)})
        assert r.status_code == 200, f"disable with valid code must succeed: {r.status_code} {r.text}"
