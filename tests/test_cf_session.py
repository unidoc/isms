"""Cloudflare Access → web session endpoint (#98).

The test stack has no Cloudflare Access configured, so this pins the *safe
default*: the endpoint is a public route (reachable without a token) and refuses
to mint a session when CF Access isn't configured — it never falls open.

Coverage gap: the happy path (valid Cf-Access-Jwt-Assertion → JIT-provision +
session) and the JWT-validation branches require a live Cloudflare Access tunnel
(the JWKS are fetched from the team domain and the JWT is signed by Cloudflare's
private key), which CI can't provide — same limitation as the SMTP paths (#16/
#42). Those are verified manually against a real CF Access deployment. The pure
resolve helper's name derivation is unit-tested in api_cf_test.go.
"""
import requests


def test_cf_session_is_public_and_fails_safe(api_url):
    # No auth header at all — the route must be reachable (public), and with no
    # Cloudflare Access configured it must refuse rather than mint a session.
    r = requests.get(f"{api_url}/auth/cf-session")
    assert r.status_code == 401, f"expected 401 when CF Access is unconfigured: {r.status_code} {r.text}"
    assert "cloudflare access" in r.text.lower(), f"unexpected body: {r.text}"
