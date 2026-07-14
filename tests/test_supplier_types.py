"""Supplier register accepts every supplier_type, including `contractor`.

Regression for adding `contractor` to the supplier_type enum — the IT-centric set
(cloud/saas/consulting/hosting/infrastructure/software/other) had no home for
physical works contractors, which were landing in `other`.
"""
import uuid

import requests

SUPPLIER_TYPES = [
    "cloud", "saas", "consulting", "hosting",
    "infrastructure", "software", "contractor", "other",
]


def test_every_supplier_type_accepted(api_url, admin_headers):
    created = {}
    for t in SUPPLIER_TYPES:
        r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
            "name": f"stype-{t}-{uuid.uuid4().hex[:8]}",
            "supplier_type": t,
            "criticality": "medium",
        })
        assert r.status_code in (200, 201), f"create supplier_type={t} failed: {r.text}"
        s = r.json()
        assert s["supplier_type"] == t, f"expected {t}, got {s.get('supplier_type')}"
        created[t] = s["id"]

    # Each is stored and retrievable with its type (robust vs list pagination).
    for t, sid in created.items():
        got = requests.get(f"{api_url}/suppliers/{sid}", headers=admin_headers)
        assert got.status_code == 200, got.text
        assert got.json()["supplier_type"] == t


def test_reclassify_other_to_contractor(api_url, admin_headers):
    r = requests.post(f"{api_url}/suppliers", headers=admin_headers, json={
        "name": f"reclass-{uuid.uuid4().hex[:8]}",
        "supplier_type": "other",
        "criticality": "low",
    })
    assert r.status_code in (200, 201), r.text
    sid = r.json()["id"]

    up = requests.put(f"{api_url}/suppliers/{sid}", headers=admin_headers,
                      json={"supplier_type": "contractor"})
    assert up.status_code in (200, 204), f"reclassify failed: {up.text}"

    got = requests.get(f"{api_url}/suppliers/{sid}", headers=admin_headers)
    assert got.status_code == 200
    assert got.json()["supplier_type"] == "contractor"
