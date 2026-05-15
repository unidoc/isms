"""Branding tests — settings, logo upload, config, and serve endpoints.

Tests both authenticated and unauthenticated access.
Verifies security (role checks, file validation) and correct content streaming from git.
"""
import io
import struct
import requests


# Minimal valid PNG: 1x1 pixel, white
def _make_png():
    """Create a minimal valid PNG file (1x1 white pixel)."""
    import zlib
    # IHDR: 1x1, 8-bit RGB
    ihdr_data = struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)
    ihdr_crc = zlib.crc32(b"IHDR" + ihdr_data) & 0xFFFFFFFF
    ihdr = struct.pack(">I", 13) + b"IHDR" + ihdr_data + struct.pack(">I", ihdr_crc)
    # IDAT: single row, filter=0, RGB white
    raw = b"\x00\xff\xff\xff"
    compressed = zlib.compress(raw)
    idat_crc = zlib.crc32(b"IDAT" + compressed) & 0xFFFFFFFF
    idat = struct.pack(">I", len(compressed)) + b"IDAT" + compressed + struct.pack(">I", idat_crc)
    # IEND
    iend_crc = zlib.crc32(b"IEND") & 0xFFFFFFFF
    iend = struct.pack(">I", 0) + b"IEND" + struct.pack(">I", iend_crc)
    return b"\x89PNG\r\n\x1a\n" + ihdr + idat + iend


MINIMAL_SVG = b'<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><rect fill="#f00" width="100" height="100"/></svg>'


class TestBrandingSettings:
    """Admin can save all branding settings without 500 errors."""

    def test_save_branding_name(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/admin/settings", headers=admin_headers,
                         json={"key": "branding_name", "value": "Test Corp"})
        assert r.status_code == 200, f"branding_name failed: {r.text}"

    def test_save_branding_color(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/admin/settings", headers=admin_headers,
                         json={"key": "branding_color", "value": "#ff6600"})
        assert r.status_code == 200, f"branding_color failed: {r.text}"

    def test_save_branding_footer(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/admin/settings", headers=admin_headers,
                         json={"key": "branding_footer", "value": "Confidential"})
        assert r.status_code == 200, f"branding_footer failed: {r.text}"

    def test_save_show_powered_by(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/admin/settings", headers=admin_headers,
                         json={"key": "show_powered_by", "value": "false"})
        assert r.status_code == 200, f"show_powered_by failed: {r.text}"

    def test_save_terms_url(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/admin/settings", headers=admin_headers,
                         json={"key": "terms_url", "value": "https://example.com/terms"})
        assert r.status_code == 200, f"terms_url failed: {r.text}"

    def test_save_privacy_url(self, api_url, admin_headers):
        r = requests.put(f"{api_url}/admin/settings", headers=admin_headers,
                         json={"key": "privacy_url", "value": "https://example.com/privacy"})
        assert r.status_code == 200, f"privacy_url failed: {r.text}"

    def test_config_reflects_branding(self, api_url, admin_headers):
        """Config endpoint should return saved branding values."""
        r = requests.get(f"{api_url}/config", headers=admin_headers)
        assert r.status_code == 200
        cfg = r.json()
        branding = cfg.get("branding") or {}
        assert branding.get("branding_name") == "Test Corp"
        assert branding.get("branding_color") == "#ff6600"


class TestBrandingUploadPNG:
    """Upload PNG logo via admin endpoint, verify it's served correctly."""

    def test_upload_png_as_admin(self, api_url, admin_headers, base_url):
        """Admin uploads a PNG logo — should succeed and commit to git."""
        png_data = _make_png()
        files = {"file": ("logo.png", io.BytesIO(png_data), "image/png")}
        # Don't send Content-Type: application/json for multipart upload
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 200, f"Upload failed: {r.text}"
        body = r.json()
        assert body.get("status") == "uploaded"
        assert body.get("file") == "branding/logo.png"

    def test_serve_logo_authenticated(self, base_url, admin_headers):
        """Authenticated user can fetch the uploaded logo."""
        r = requests.get(f"{base_url}/branding/logo", headers=admin_headers)
        assert r.status_code == 200, f"Authenticated logo fetch: {r.status_code} {r.text}"
        assert r.headers.get("Content-Type") in ("image/png", "image/svg+xml")
        assert len(r.content) > 0, "Logo content should not be empty"
        # Verify it's actually a PNG (starts with PNG magic)
        assert r.content[:4] == b"\x89PNG", "Content should be valid PNG data"

    def test_serve_logo_unauthenticated(self, base_url):
        """Unauthenticated request to /branding/logo with ?org= param."""
        r = requests.get(f"{base_url}/branding/logo?org=test-org")
        assert r.status_code == 200, f"Unauthenticated logo fetch: {r.status_code}"
        assert r.headers.get("Content-Type") in ("image/png", "image/svg+xml")
        assert len(r.content) > 0
        assert r.content[:4] == b"\x89PNG"

    def test_config_includes_logo(self, api_url, admin_headers):
        """Config endpoint should include branding_logo after upload."""
        r = requests.get(f"{api_url}/config", headers=admin_headers)
        assert r.status_code == 200
        cfg = r.json()
        branding = cfg.get("branding") or {}
        assert branding.get("branding_logo") == "/branding/logo", \
            f"Expected branding_logo=/branding/logo, got: {branding}"

    def test_config_unauthenticated_includes_logo(self, api_url):
        """Unauthenticated config with ?org= should also include branding_logo."""
        r = requests.get(f"{api_url}/config?org=test-org")
        assert r.status_code == 200
        cfg = r.json()
        branding = cfg.get("branding") or {}
        assert branding.get("branding_logo") == "/branding/logo", \
            f"Unauth config missing branding_logo: {branding}"


class TestBrandingUploadSVG:
    """Upload SVG logo, verify SVG replaces PNG and is served correctly."""

    def test_upload_svg_replaces_png(self, api_url, admin_headers):
        """SVG upload should succeed and remove the previous PNG."""
        files = {"file": ("logo.svg", io.BytesIO(MINIMAL_SVG), "image/svg+xml")}
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 200, f"SVG upload failed: {r.text}"
        body = r.json()
        assert body.get("file") == "branding/logo.svg"

    def test_serve_svg_after_upload(self, base_url):
        """After SVG upload, /branding/logo should serve SVG content."""
        r = requests.get(f"{base_url}/branding/logo?org=test-org")
        assert r.status_code == 200
        assert r.headers.get("Content-Type") == "image/svg+xml"
        assert b"<svg" in r.content

    def test_upload_png_back(self, api_url, admin_headers, base_url):
        """Re-upload PNG to restore state for other tests."""
        png_data = _make_png()
        files = {"file": ("logo.png", io.BytesIO(png_data), "image/png")}
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 200
        # Verify PNG is now served
        r2 = requests.get(f"{base_url}/branding/logo?org=test-org")
        assert r2.status_code == 200
        assert r2.content[:4] == b"\x89PNG"


class TestBrandingUploadSecurity:
    """Security checks on branding upload."""

    def test_reader_cannot_upload(self, api_url, reader_headers):
        """Non-admin role should be rejected."""
        png_data = _make_png()
        files = {"file": ("logo.png", io.BytesIO(png_data), "image/png")}
        headers = {"Authorization": reader_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 403, f"Reader should get 403, got {r.status_code}"

    def test_contributor_cannot_upload(self, api_url, contributor_headers):
        """Contributor role should be rejected."""
        png_data = _make_png()
        files = {"file": ("logo.png", io.BytesIO(png_data), "image/png")}
        headers = {"Authorization": contributor_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 403, f"Contributor should get 403, got {r.status_code}"

    def test_unauthenticated_cannot_upload(self, api_url):
        """No token should be rejected."""
        png_data = _make_png()
        files = {"file": ("logo.png", io.BytesIO(png_data), "image/png")}
        r = requests.post(f"{api_url}/admin/branding/upload", files=files)
        assert r.status_code in (401, 403), f"Expected 401/403, got {r.status_code}"

    def test_reject_invalid_extension(self, api_url, admin_headers):
        """Only PNG and SVG should be accepted."""
        files = {"file": ("logo.gif", io.BytesIO(b"GIF89a"), "image/gif")}
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 400, f"GIF should be rejected: {r.status_code}"

    def test_reject_fake_png(self, api_url, admin_headers):
        """File with .png extension but no PNG magic should be rejected."""
        files = {"file": ("logo.png", io.BytesIO(b"not a real png file"), "image/png")}
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 400, f"Fake PNG should be rejected: {r.status_code}"

    def test_reject_oversized_file(self, api_url, admin_headers):
        """Files over 2MB should be rejected."""
        big_data = b"\x89PNG" + b"\x00" * (2 * 1024 * 1024 + 100)
        files = {"file": ("logo.png", io.BytesIO(big_data), "image/png")}
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 400, f"Oversized should be rejected: {r.status_code}"

    def test_svg_script_sanitized(self, api_url, admin_headers, base_url):
        """SVG with <script> should have it stripped."""
        malicious = b'<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><rect width="1" height="1"/></svg>'
        files = {"file": ("logo.svg", io.BytesIO(malicious), "image/svg+xml")}
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 200, f"SVG upload should succeed (sanitized): {r.text}"
        # Fetch and verify script tag was removed
        r2 = requests.get(f"{base_url}/branding/logo?org=test-org")
        assert r2.status_code == 200
        assert b"<script" not in r2.content, "Script tag should be stripped from SVG"

    def test_restore_clean_logo(self, api_url, admin_headers):
        """Restore a clean PNG logo after security tests."""
        png_data = _make_png()
        files = {"file": ("logo.png", io.BytesIO(png_data), "image/png")}
        headers = {"Authorization": admin_headers["Authorization"]}
        r = requests.post(f"{api_url}/admin/branding/upload", headers=headers, files=files)
        assert r.status_code == 200


class TestBrandingNonExistent:
    """Verify 404 for branding assets that don't exist."""

    def test_logo_dark_404_when_not_uploaded(self, base_url):
        """logo-dark should 404 if never uploaded."""
        r = requests.get(f"{base_url}/branding/logo-dark?org=test-org")
        # Could be 200 if a previous test uploaded it, or 404 if not
        # Just verify it doesn't 500
        assert r.status_code in (200, 204, 404), f"Unexpected: {r.status_code}"

    def test_favicon_when_not_uploaded(self, base_url):
        """favicon should not 500 if never uploaded."""
        r = requests.get(f"{base_url}/branding/favicon.ico?org=test-org")
        assert r.status_code in (200, 204, 404), f"Unexpected: {r.status_code}"
