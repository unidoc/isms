"""Trust center certifications (#521), end to end over HTTP.

Everything runs in a DEDICATED organization created here: the shared conftest
organization must keep its trust center off (see tests/test_trust_center_public.py
and the docstring of tests/test_trust_center_manager.py).

Needs ISMS_TRUST_RATE_LIMIT high enough for the anonymous calls below (the
default is 120 per minute per IP).
"""
import hashlib
import http.client
import uuid
from datetime import date, timedelta
from urllib.parse import urlparse

import pytest
import requests

PASSWORD = "TestPass123!"
MB = 1024 * 1024


def _tag():
    return uuid.uuid4().hex[:8]


def _pdf(size, fill=b"x"):
    head = b"%PDF-1.4\n"
    return head + fill * (size - len(head))


PNG = b"\x89PNG\r\n\x1a\n" + b"\x00\x01\x02\x03" * 64


def _login(api_url, email, org=None):
    body = {"email": email, "password": PASSWORD}
    if org:
        body["organization"] = org
    r = requests.post(f"{api_url}/auth/login", json=body)
    assert r.status_code == 200, r.text
    return {"Authorization": f"Bearer {r.json()['token']}"}


def _json(h):
    return {**h, "Content-Type": "application/json"}


@pytest.fixture(scope="module")
def tc(api_url):
    t = _tag()
    slug = f"tcc-{t}"
    emails = {role: f"tcc-{role}-{t}@isms-test.local" for role in ("admin", "manager", "contributor", "reader")}
    r = requests.post(f"{api_url}/auth/signup", json={"email": emails["admin"], "password": PASSWORD, "name": "admin"})
    assert r.status_code in (200, 201), r.text
    admin = _login(api_url, emails["admin"])
    r = requests.post(f"{api_url}/organizations", headers=_json(admin), json={"name": f"TC Certs {t}", "slug": slug})
    assert r.status_code in (200, 201), r.text
    admin = _login(api_url, emails["admin"], slug)
    heads = {"admin": admin}
    for role in ("manager", "contributor", "reader"):
        r = requests.post(f"{api_url}/auth/signup", json={"email": emails[role], "password": PASSWORD, "name": role})
        assert r.status_code in (200, 201), r.text
        r = requests.post(f"{api_url}/users", headers=_json(admin), json={"email": emails[role], "name": role, "role": role})
        assert r.status_code in (200, 201), r.text
        heads[role] = _login(api_url, emails[role], slug)
    r = requests.put(f"{api_url}/trust-center/settings", headers=_json(heads["manager"]),
                     json={"enabled": True, "title": "Certs", "security_contact": "sec@example.com"})
    assert r.status_code == 200, r.text
    return {"slug": slug, "heads": heads}


def _create_file(api_url, tc, label, data, **extra):
    fields = {"label": label, "link_type": "certificate", "issuer": "BSI", **extra}
    r = requests.post(f"{api_url}/trust-center/certifications", headers=tc["heads"]["manager"],
                      data=fields, files={"file": (f"{label}.pdf", data, "application/pdf")})
    return r


def _public_items(api_url, tc):
    r = requests.get(f"{api_url}/trust/{tc['slug']}/items")
    assert r.status_code == 200, r.text
    return r.json()


def _public_file(api_url, tc, item_id, **kw):
    return requests.get(f"{api_url}/trust/{tc['slug']}/certifications/{item_id}/file", **kw)


def _expect_413(api_url, path, content_type, length, token):
    """Send only the headers with Expect: 100-continue. A server that refuses
    the size answers 413 without ever asking for the body, so this proves the
    limit without pushing 60 MB through a connection the server closes."""
    u = urlparse(api_url)
    conn = http.client.HTTPConnection(u.hostname, u.port, timeout=30)
    conn.putrequest("POST", u.path + path)
    conn.putheader("Authorization", token)
    conn.putheader("Content-Type", content_type)
    conn.putheader("Content-Length", str(length))
    conn.putheader("Expect", "100-continue")
    conn.endheaders()
    resp = conn.getresponse()
    status = resp.status
    conn.close()
    return status


def test_upload_publish_and_public_download_matches_sha256(api_url, tc):
    data = _pdf(300_000, b"abc")
    r = _create_file(api_url, tc, "ISO 27001", data, publish="true", access_level="public",
                     valid_until=(date.today() + timedelta(days=200)).isoformat())
    assert r.status_code == 201, r.text
    cert = r.json()
    assert cert["source"] == "file" and cert["file_content_type"] == "application/pdf"
    assert cert["file_sha256"] == hashlib.sha256(data).hexdigest()
    assert cert["state"] == "current" and cert["item"]["access_level"] == "public"
    assert "file_key" not in cert

    item = next(i for i in _public_items(api_url, tc) if i["id"] == cert["item"]["id"])
    assert item["kind"] == "certification" and item["title"] == "ISO 27001" and item["source"] == "file"
    assert item["file_name"] == "ISO 27001.pdf" and item["file_size"] == len(data)
    assert "url" not in item and "file_key" not in item and "file_sha256" not in item

    dl = _public_file(api_url, tc, item["id"])
    assert dl.status_code == 200
    assert dl.content == data and hashlib.sha256(dl.content).hexdigest() == cert["file_sha256"]
    assert dl.headers["Content-Type"] == "application/pdf"
    assert dl.headers["X-Content-Type-Options"] == "nosniff"
    assert dl.headers["Cache-Control"] == "no-store"
    assert dl.headers["Content-Disposition"].startswith("attachment")

    # A bearer token changes nothing on the anonymous route.
    again = _public_file(api_url, tc, item["id"], headers=tc["heads"]["admin"])
    assert again.status_code == 200 and again.content == data


def test_gated_certification_shows_no_url_or_file(api_url, tc):
    r = requests.post(f"{api_url}/trust-center/certifications", headers=_json(tc["heads"]["manager"]), json={
        "label": "Pen test", "link_type": "penetration_test", "url": "https://example.com/pentest",
        "publish": True, "access_level": "gated"})
    assert r.status_code == 201, r.text
    item = next(i for i in _public_items(api_url, tc) if i["id"] == r.json()["item"]["id"])
    assert item["access_level"] == "gated" and item["source"] == "link" and item["title"] == "Pen test"
    assert "url" not in item and "file_name" not in item
    assert _public_file(api_url, tc, item["id"]).status_code == 404

    f = _create_file(api_url, tc, "Gated report", _pdf(2000), publish="true", access_level="gated")
    assert f.status_code == 201, f.text
    gid = f.json()["item"]["id"]
    assert _public_file(api_url, tc, gid).status_code == 404
    mgr = requests.get(f"{api_url}/trust-center/certifications/{f.json()['id']}/file", headers=tc["heads"]["manager"])
    assert mgr.status_code == 200 and mgr.content == _pdf(2000)
    prev = requests.get(f"{api_url}/trust-center/preview/certifications/{gid}/file", headers=tc["heads"]["manager"])
    assert prev.status_code == 404


def test_upload_is_checked_by_its_bytes(api_url, tc):
    m = tc["heads"]["manager"]

    def up(name, data, ctype):
        return requests.post(f"{api_url}/trust-center/certifications", headers=m,
                             data={"label": name, "link_type": "other"}, files={"file": (name, data, ctype)})

    for name, data, ctype in [
        ("x.pdf", b"<html><script>alert(1)</script></html>", "application/pdf"),
        ("x.svg", b'<svg xmlns="http://www.w3.org/2000/svg"/>', "image/svg+xml"),
        ("x.txt", b"hello", "text/plain"),
        ("empty.pdf", b"", "application/pdf"),
    ]:
        r = up(name, data, ctype)
        assert r.status_code == 400 and r.json()["code"] == "trust_file_type", f"{name}: {r.status_code} {r.text}"
    ok = up("claims-pdf.pdf", PNG, "application/pdf")
    assert ok.status_code == 201, ok.text
    assert ok.json()["file_content_type"] == "image/png"
    dl = requests.get(f"{api_url}/trust-center/certifications/{ok.json()['id']}/file", headers=m)
    assert dl.headers["Content-Type"] == "image/png"
    assert dl.headers["Content-Disposition"].rstrip('"').endswith(".png")


def test_roles_and_validation(api_url, tc):
    for role in ("contributor", "reader"):
        h = tc["heads"][role]
        for method, path in [("GET", "/trust-center/certifications"), ("POST", "/trust-center/certifications"),
                             ("DELETE", "/trust-center/certifications/1"), ("GET", "/trust-center/certifications/1/file")]:
            r = requests.request(method, f"{api_url}{path}", headers=_json(h), json={} if method == "POST" else None)
            assert r.status_code == 403, f"{role} {method} {path}: {r.status_code}"
    assert requests.get(f"{api_url}/trust-center/certifications").status_code == 401

    m = _json(tc["heads"]["manager"])
    base = {"label": "V", "link_type": "certificate", "url": "https://example.com/c"}
    for bad, code in [
        ({"valid_until": 1794000000}, "trust_date_invalid"),
        ({"valid_until": "2026-11-13T00:00:00Z"}, "trust_date_invalid"),
        ({"valid_from": ""}, "trust_date_invalid"),
        ({"valid_until": "2026-13-01"}, "trust_date_invalid"),
    ]:
        r = requests.post(f"{api_url}/trust-center/certifications", headers=m, json={**base, **bad})
        assert r.status_code == 400 and r.json()["code"] == code, (bad, r.text)
    for bad in [{"link_type": "contract"}, {"url": "ftp://x/y"}, {"valid_from": "2030-01-01", "valid_until": "2020-01-01"}]:
        r = requests.post(f"{api_url}/trust-center/certifications", headers=m, json={**base, **bad})
        assert r.status_code == 400, (bad, r.text)
    r = requests.post(f"{api_url}/trust-center/certifications", headers=m, json={"label": "none", "link_type": "other"})
    assert r.status_code == 400 and r.json()["code"] == "trust_certification_source"
    assert requests.get(f"{api_url}/trust-center/certifications/abc/file", headers=m).status_code == 400
    assert requests.get(f"{api_url}/trust-center/certifications/99999999/file", headers=m).status_code == 404


def test_expiry_hides_from_visitors_without_a_job(api_url, tc):
    m = tc["heads"]["manager"]
    r = _create_file(api_url, tc, "Expiring", _pdf(5000), publish="true", access_level="public")
    assert r.status_code == 201, r.text
    cert = r.json()
    iid = cert["item"]["id"]
    assert any(i["id"] == iid for i in _public_items(api_url, tc))
    assert _public_file(api_url, tc, iid).status_code == 200

    # A past date is accepted (a certificate can be recorded as already expired).
    past = (date.today() - timedelta(days=2)).isoformat()
    r = requests.put(f"{api_url}/trust-center/certifications/{cert['id']}", headers=_json(m), json={"valid_until": past})
    assert r.status_code == 200, r.text
    assert r.json()["state"] == "expired" and r.json()["valid_until"] == past

    assert not any(i["id"] == iid for i in _public_items(api_url, tc))
    assert _public_file(api_url, tc, iid).status_code == 404
    # Managers still have it, flagged expired, and can download it.
    listed = requests.get(f"{api_url}/trust-center/certifications", headers=m).json()
    assert next(c for c in listed if c["id"] == cert["id"])["state"] == "expired"
    assert requests.get(f"{api_url}/trust-center/certifications/{cert['id']}/file", headers=m).status_code == 200
    items = requests.get(f"{api_url}/trust-center/items", headers=m).json()
    row = next(i for i in items if i["id"] == iid)
    assert row["certification"]["state"] == "expired" and row["hidden_reason"] == ""

    # It shows up in the overdue summary for every role.
    od = requests.get(f"{api_url}/overdue", headers=tc["heads"]["reader"]).json()
    assert any(i["entity_id"] == f"CERT-{cert['id']}" and i["state"] == "expired" for i in od["trust_certifications"])

    # Renewing brings it back by itself.
    future = (date.today() + timedelta(days=90)).isoformat()
    r = requests.put(f"{api_url}/trust-center/certifications/{cert['id']}", headers=_json(m), json={"valid_until": future})
    assert r.status_code == 200
    assert any(i["id"] == iid for i in _public_items(api_url, tc))
    assert _public_file(api_url, tc, iid).status_code == 200


def test_level_order_replace_and_delete(api_url, tc):
    m = tc["heads"]["manager"]
    a = _create_file(api_url, tc, "Order A", _pdf(1000), publish="true", access_level="gated").json()
    b = _create_file(api_url, tc, "Order B", _pdf(1000), publish="true", access_level="gated").json()
    r = requests.patch(f"{api_url}/trust-center/items/{a['item']['id']}", headers=_json(m), json={"access_level": "public"})
    assert r.status_code == 200 and r.json()["access_level"] == "public" and r.json()["certification"]["label"] == "Order A"
    items = [i for i in requests.get(f"{api_url}/trust-center/items", headers=m).json() if i["kind"] == "certification"]
    ids = [i["id"] for i in items]
    r = requests.put(f"{api_url}/trust-center/items/order", headers=_json(m),
                     json={"kind": "certification", "ids": list(reversed(ids))})
    assert r.status_code == 200, r.text
    after = [i["id"] for i in r.json() if i["kind"] == "certification"]
    assert after == list(reversed(ids))

    # Replace the file: new bytes are served, and the old blob is gone from reach.
    new = _pdf(3000, b"n")
    r = requests.post(f"{api_url}/trust-center/certifications/{a['id']}/file", headers=m, files={"file": ("new.pdf", new, "application/pdf")})
    assert r.status_code == 200 and r.json()["file_sha256"] == hashlib.sha256(new).hexdigest()
    assert _public_file(api_url, tc, a["item"]["id"]).content == new
    assert requests.post(f"{api_url}/trust-center/certifications/{a['id']}/file", headers=m,
                         files={"file": ("x.pdf", b"<html>", "application/pdf")}).status_code == 400

    # Verify, unpublish, publish again, delete.
    v = requests.post(f"{api_url}/trust-center/certifications/{a['id']}/verify", headers=m)
    assert v.status_code == 200 and v.json()["verified_at"] and v.json()["verified_by"]
    assert requests.delete(f"{api_url}/trust-center/certifications/{b['id']}/publish", headers=m).status_code == 204
    assert requests.post(f"{api_url}/trust-center/certifications/{b['id']}/publish", headers=_json(m), json={"access_level": "public"}).status_code == 201
    assert requests.post(f"{api_url}/trust-center/certifications/{b['id']}/publish", headers=_json(m), json={}).status_code == 409
    assert requests.delete(f"{api_url}/trust-center/certifications/{a['id']}", headers=m).status_code == 204
    assert _public_file(api_url, tc, a["item"]["id"]).status_code == 404
    assert requests.get(f"{api_url}/trust-center/certifications/{a['id']}/file", headers=m).status_code == 404


def test_forty_megabytes_accepted_sixty_refused(api_url, tc):
    m = tc["heads"]["manager"]
    data = _pdf(40 * MB, b"z")
    r = _create_file(api_url, tc, "Large", data, publish="true", access_level="public")
    assert r.status_code == 201, r.text[:300]
    cert = r.json()
    assert cert["file_size"] == len(data)
    dl = _public_file(api_url, tc, cert["item"]["id"])
    assert dl.status_code == 200 and hashlib.sha256(dl.content).hexdigest() == hashlib.sha256(data).hexdigest()

    boundary = "----tcc"
    assert _expect_413(api_url, "/trust-center/certifications", f"multipart/form-data; boundary={boundary}",
                       60 * MB, m["Authorization"]) == 413
    assert _expect_413(api_url, f"/trust-center/certifications/{cert['id']}/file", f"multipart/form-data; boundary={boundary}",
                       60 * MB, m["Authorization"]) == 413
    # The global 10 MB cap still holds everywhere else.
    assert _expect_413(api_url, f"/trust-center/certifications/{cert['id']}/verify", "application/octet-stream",
                       11 * MB, m["Authorization"]) == 413


def test_evidence_upload_of_forty_megabytes_is_accepted(api_url, tc):
    """Evidence was capped at 10 MB by the global body limit before #521."""
    admin = tc["heads"]["admin"]
    suffix = _tag()
    p = requests.post(f"{api_url}/programs", headers=_json(admin), json={"key": f"EV{suffix[:6].upper()}", "title": "p"})
    assert p.status_code in (200, 201), p.text
    o = requests.post(f"{api_url}/objectives", headers=_json(admin), json={"program_id": p.json()["id"], "title": "o"})
    assert o.status_code in (200, 201), o.text
    ch = requests.post(f"{api_url}/objectives/{o.json()['id']}/checkins", headers=_json(admin),
                       json={"value_numeric": 1.0, "message": "m", "created_by": f"tcc-admin@isms-test.local"})
    assert ch.status_code in (200, 201), ch.text
    data = b"e" * (40 * MB)
    up = requests.post(f"{api_url}/checkins/{ch.json()['id']}/evidence", headers=admin,
                       files={"file": ("big.bin", data, "application/octet-stream")}, data={"title": "Big"})
    assert up.status_code in (200, 201), up.text[:300]
    assert _expect_413(api_url, f"/checkins/{ch.json()['id']}/evidence", "multipart/form-data; boundary=x",
                       60 * MB, admin["Authorization"]) == 413
