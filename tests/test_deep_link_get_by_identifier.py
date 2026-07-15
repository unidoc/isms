"""#166 review (#185): GET /<module>/<identifier> must resolve the same record as
the numeric id, so identifier deep-links work for off-page items (fresh tab /
shared link / reload). The frontend now sends raw identifiers (RISK-3, ASSET-7,
CR-12, SYSTEM-5, SUPPLIER-8) to these GET handlers, which were numeric-only
(asset/risk/change) or prefix-stripping (system/supplier); they now route through
resolveXID DB lookups.
"""
import uuid

import pytest
import requests

ENTITIES = [
    {"name": "risk", "path": "risks", "field": "title", "create": {}},
    {"name": "asset", "path": "assets", "field": "name",
     "create": {"asset_type": "system", "status": "open",
                "confidentiality": 3, "integrity": 3, "availability": 3}},
    {"name": "change", "path": "changes", "field": "title",
     "create": {"description": "d", "justification": "j", "priority": "high",
                "category": "technology", "risk_level": "medium", "rollback_plan": "revert"}},
    {"name": "system", "path": "systems", "field": "name",
     "create": {"classification": "internal", "criticality": "low"}},
    {"name": "supplier", "path": "suppliers", "field": "name",
     "create": {"supplier_type": "saas", "criticality": "low"}},
]


@pytest.mark.parametrize("cfg", ENTITIES, ids=[e["name"] for e in ENTITIES])
def test_get_by_identifier_resolves_same_record(cfg, api_url, admin_headers):
    path = cfg["path"]
    payload = dict(cfg["create"])
    payload[cfg["field"]] = f"idurl-{cfg['name']}-{uuid.uuid4().hex[:8]}"

    r = requests.post(f"{api_url}/{path}", headers=admin_headers, json=payload)
    assert r.status_code in (200, 201), f"create {cfg['name']}: {r.text}"
    ent = r.json()
    num_id, ident = ent["id"], ent["identifier"]
    assert ident, f"{cfg['name']} must have an identifier"

    by_num = requests.get(f"{api_url}/{path}/{num_id}", headers=admin_headers)
    by_ident = requests.get(f"{api_url}/{path}/{ident}", headers=admin_headers)
    assert by_num.status_code == 200, by_num.text
    assert by_ident.status_code == 200, \
        f"GET /{path}/{ident} must resolve by identifier (#185): {by_ident.status_code} {by_ident.text}"
    assert by_ident.json()["id"] == num_id, \
        f"identifier and numeric id must resolve the same {cfg['name']} row"
