"""Trust center public read API (#517): no session, one 404 for everything.

There are no manager endpoints yet, so no trust center can be enabled over
HTTP. The conftest organization therefore has none, which is exactly what lets
this file prove the uniformity: an organization that exists but has the trust
center off must look the same as one that does not exist. Enabled-trust-center
behaviour (items, host/slug mismatch, unpublish) is covered by the database
tests in internal/isms/api/trust_public_pg_test.go.

Everything here is unauthenticated on purpose.
"""
import http.client
import os
import uuid
from urllib.parse import urlparse

import pytest
import requests

from conftest import BASE_URL, ORG_SLUG


def _missing_slug():
    return f"no-such-org-{uuid.uuid4().hex[:12]}"


def test_disabled_org_looks_like_unknown_org(api_url, test_setup):
    """The org exists, its trust center is off: same bytes as an unknown slug."""
    unknown = requests.get(f"{api_url}/trust/{_missing_slug()}")
    assert unknown.status_code == 404
    assert unknown.headers.get("Cache-Control") == "no-store"
    body = unknown.json()
    assert body.get("code") == "not_found"

    for suffix in ("", "/items"):
        r = requests.get(f"{api_url}/trust/{ORG_SLUG}{suffix}")
        assert r.status_code == 404, f"{suffix!r}: {r.status_code} {r.text}"
        assert r.content == unknown.content
        assert r.headers.get("Cache-Control") == "no-store"
        assert r.headers.get("Content-Type", "").startswith("application/json")


def test_every_other_path_and_method_is_the_same_404(api_url, test_setup):
    want = requests.get(f"{api_url}/trust/{_missing_slug()}").content
    probes = [
        ("GET", f"/trust/{ORG_SLUG}/whatever"),
        ("GET", f"/trust/{ORG_SLUG}/items/extra"),
        ("GET", f"/trust/{ORG_SLUG}/"),
        ("GET", "/trust/"),
        ("GET", "/trust"),
        ("POST", f"/trust/{ORG_SLUG}/items"),
        ("PUT", f"/trust/{ORG_SLUG}/items"),
        ("DELETE", f"/trust/{ORG_SLUG}/items"),
        ("POST", f"/trust/{ORG_SLUG}"),
    ]
    for method, path in probes:
        r = requests.request(method, f"{api_url}{path}")
        assert r.status_code == 404, f"{method} {path}: {r.status_code} {r.text}"
        assert r.content == want, f"{method} {path}: body differs"
        assert r.headers.get("Cache-Control") == "no-store"


def test_prefix_is_public_but_member_routes_are_not(api_url, test_setup):
    # Never a 401 for the trust prefix without credentials, whatever the answer.
    assert requests.get(f"{api_url}/trust/{ORG_SLUG}").status_code != 401
    assert requests.get(f"{api_url}/trust/{ORG_SLUG}/items").status_code != 401
    # A bogus bearer is ignored, not rejected.
    r = requests.get(f"{api_url}/trust/{ORG_SLUG}/items",
                     headers={"Authorization": "Bearer isms_not-a-real-key"})
    assert r.status_code == 404
    # Control: a member route still needs credentials.
    assert requests.get(f"{api_url}/risks").status_code == 401


def _raw_get(path):
    """GET with the path sent byte for byte (requests/urllib3 would collapse
    dot segments before sending)."""
    u = urlparse(BASE_URL)
    conn = http.client.HTTPConnection(u.hostname, u.port or 80, timeout=10)
    try:
        conn.putrequest("GET", path, skip_host=False)
        conn.endheaders()
        resp = conn.getresponse()
        return resp.status, resp.read()
    finally:
        conn.close()


def test_dot_segments_do_not_inherit_the_public_skip(test_setup):
    status, _ = _raw_get("/api/v1/trust/../risks")
    assert status == 401
    # Discriminating check: if the server had collapsed the dots first, this
    # would be /api/v1/trust/<slug>/items and answer the trust 404, not 401.
    status, _ = _raw_get(f"/api/v1/trust/../trust/{ORG_SLUG}/items")
    assert status == 401
    status, _ = _raw_get("/api/v1/trust/..%2Frisks")
    assert status in (401, 404)
    # Control: the same path without dots is the public 404.
    status, _ = _raw_get(f"/api/v1/trust/{ORG_SLUG}/items")
    assert status == 404


@pytest.mark.skipif(
    os.environ.get("ISMS_RATE_LIMIT", "0") == "0",
    reason="the suite runs against a server with rate limiting disabled "
    "(ISMS_RATE_LIMIT=0, see conftest.py), which also turns the trust limit "
    "off; set ISMS_RATE_LIMIT to a non-zero value in the pytest env to run "
    "this against a limited server. The limiter is unit-tested in "
    "internal/isms/api/trust_rate_limit_test.go",
)
def test_trust_rate_limit(api_url):
    """Last in the file: it uses up this client's budget."""
    last = None
    for _ in range(1000):
        last = requests.get(f"{api_url}/trust/{_missing_slug()}")
        if last.status_code == 429:
            break
    assert last is not None and last.status_code == 429
    assert last.json() == {"message": "too many requests"}
    assert int(last.headers["Retry-After"]) >= 1
    assert last.headers.get("Cache-Control") == "no-store"
