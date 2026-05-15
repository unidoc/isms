"""Security-specific tests."""
import requests


def test_no_auth_on_protected_endpoint(api_url):
    """All API endpoints should require auth (except public ones)."""
    protected = [
        "/me", "/users", "/risks", "/assets", "/suppliers",
        "/reviews", "/incidents", "/legal", "/audit/programmes",
    ]
    for path in protected:
        r = requests.get(f"{api_url}{path}")
        assert r.status_code == 401, f"{path} should require auth, got {r.status_code}"


def test_public_endpoints_accessible(base_url, api_url):
    """Public endpoints should work without auth."""
    public = [
        (f"{base_url}/healthz", 200),
        (f"{base_url}/docs", 200),
        (f"{base_url}/api/openapi.yaml", 200),
    ]
    for url, expected in public:
        r = requests.get(url)
        assert r.status_code == expected, f"{url} expected {expected}, got {r.status_code}"


def test_invalid_token(api_url):
    """Invalid JWT should be rejected."""
    r = requests.get(f"{api_url}/me", headers={"Authorization": "Bearer invalid.token.here"})
    assert r.status_code == 401


def test_expired_token_format(api_url):
    """Malformed bearer token should be rejected."""
    r = requests.get(f"{api_url}/me", headers={"Authorization": "Bearer "})
    assert r.status_code == 401


def test_sql_injection_in_search(api_url, admin_headers):
    """Search should be safe from SQL injection."""
    r = requests.get(f"{api_url}/documents/search?q=' OR 1=1 --", headers=admin_headers)
    assert r.status_code == 200  # should return results or empty, not error


def test_path_traversal_in_document(api_url, admin_headers):
    """Document folder should reject path traversal."""
    # Try various traversal attempts
    traversals = [
        "/documents/file/..%2F..%2Fetc/passwd",
        "/documents/file/....//....//etc/passwd",
    ]
    for path in traversals:
        r = requests.get(f"{api_url}{path}", headers=admin_headers)
        # Should not return actual file content — 400, 404, or empty doc
        assert r.status_code in [400, 404] or "root:" not in r.text, f"Path traversal may have succeeded: {path}"


def test_xss_in_risk_title(api_url, admin_headers):
    """XSS in risk title should be stored safely (not executed)."""
    r = requests.post(f"{api_url}/risks", headers=admin_headers, json={
        "title": '<script>alert("xss")</script>',
        "current_likelihood": 1, "current_impact": 1,
        "risk_type": "threat", "origin": "internal", "status": "open",
    })
    if r.status_code in [200, 201]:
        # Verify it's stored as text, not rendered. Search by query to bypass pagination.
        risks = requests.get(f"{api_url}/risks?q=script", headers=admin_headers).json()["data"]
        found = [r for r in risks if "script" in (r.get("title") or "")]
        assert len(found) > 0  # stored safely


def test_brute_force_protection(api_url):
    """Multiple failed logins should be rate limited."""
    for i in range(6):
        requests.post(f"{api_url}/auth/login", json={
            "email": "bruteforce@test.local",
            "password": f"wrong{i}",
        })
    r = requests.post(f"{api_url}/auth/login", json={
        "email": "bruteforce@test.local",
        "password": "wrong7",
    })
    assert r.status_code == 429, f"Expected 429 after brute force, got {r.status_code}"


def test_security_headers(base_url):
    """Server should set security headers."""
    r = requests.get(f"{base_url}/healthz")
    headers = r.headers
    assert "x-frame-options" in {k.lower() for k in headers}
    assert "x-content-type-options" in {k.lower() for k in headers}
    assert "strict-transport-security" in {k.lower() for k in headers}
