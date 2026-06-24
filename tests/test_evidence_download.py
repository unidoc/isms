"""Regression for #31: evidence download on the local (file) blob backend.

The S3 backend returns JSON {url} (a presigned link); the local/file backend
streams the file directly. The frontend now branches on Content-Type to handle
both. This test pins the backend contract the frontend relies on: on the file
backend, GET /evidence/:id/download returns the raw file bytes with the file's
content type — NOT a JSON envelope — so the client can save it.

Coverage gap: the S3 backend path (JSON {url} response) is not covered here —
the test stack runs ISMS_STORAGE_BACKEND=file and there's no mocked S3 (minio/
localstack) fixture. The server-side S3 path is unchanged by this fix; only the
client gained the Content-Type branch. Add a mock-S3 fixture if that path needs
regression coverage.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL


def test_evidence_download_local_backend_streams_file_bytes(api_url, admin_headers):
    suffix = uuid.uuid4().hex[:8]
    auth = {"Authorization": admin_headers["Authorization"]}  # no JSON CT for multipart

    p = requests.post(f"{api_url}/programs", headers=admin_headers, json={
        "key": f"EVT{suffix}", "title": f"Evidence test program {suffix}",
    })
    assert p.status_code in (200, 201), f"program: {p.text}"
    pid = p.json()["id"]

    o = requests.post(f"{api_url}/objectives", headers=admin_headers, json={
        "program_id": pid, "title": f"Evidence test objective {suffix}",
    })
    assert o.status_code in (200, 201), f"objective: {o.text}"
    oid = o.json()["id"]

    ch = requests.post(f"{api_url}/objectives/{oid}/checkins", headers=admin_headers, json={
        "value_numeric": 1.0, "message": "evidence test", "created_by": ADMIN_EMAIL,
    })
    assert ch.status_code in (200, 201), f"checkin: {ch.text}"
    cid = ch.json()["id"]

    content = f"proof-bytes-{suffix}".encode()
    up = requests.post(f"{api_url}/checkins/{cid}/evidence", headers=auth,
                       files={"file": ("proof.txt", content, "text/plain")},
                       data={"title": "Proof"})
    assert up.status_code in (200, 201), f"upload: {up.text}"
    eid = up.json()["id"]

    # The file backend must stream the file, not return a JSON {url} envelope.
    dl = requests.get(f"{api_url}/evidence/{eid}/download", headers=admin_headers)
    assert dl.status_code == 200, dl.text
    assert "application/json" not in dl.headers.get("content-type", ""), \
        f"file backend should stream the file, got JSON: {dl.headers.get('content-type')}"
    assert dl.content == content, "downloaded bytes must match the uploaded file"
