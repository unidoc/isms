"""Regression: /config must report signup_enabled, and it must match the gate.

The bug: the login/signup UI hard-coded a "Sign up" link, so a deployment with
ISMS_USER_SIGNUP unset (self-registration off) still advertised signup even
though POST /auth/signup returns 403. /config now exposes `signup_enabled` so the
frontend can hide the affordance. This test pins the flag to the real gate: the
value /config reports must agree with whether the signup endpoint accepts a
registration (403 = disabled, anything else = enabled).
"""
import uuid

import requests


def test_config_signup_enabled_matches_gate(api_url):
    cfg = requests.get(f"{api_url}/config")
    assert cfg.status_code == 200, cfg.text
    body = cfg.json()
    assert "signup_enabled" in body, "/config must expose signup_enabled"
    enabled = body["signup_enabled"]
    assert isinstance(enabled, bool), f"signup_enabled must be a bool, got {enabled!r}"

    # Probe the actual gate with a throwaway registration.
    email = f"signup-probe-{uuid.uuid4().hex[:8]}@isms-test.local"
    r = requests.post(f"{api_url}/auth/signup",
                      json={"email": email, "name": "Probe", "password": "probe-pw-123"})

    if enabled:
        # Flag says on → the endpoint must NOT be gated off (403).
        assert r.status_code != 403, \
            f"signup_enabled=true but /auth/signup returned 403: {r.text}"
    else:
        # Flag says off → the endpoint must be gated (403).
        assert r.status_code == 403, \
            f"signup_enabled=false but /auth/signup returned {r.status_code}: {r.text}"
