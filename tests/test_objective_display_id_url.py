"""#166: GET /objectives/<display_id> resolves the same objective as the numeric
id, so identifier deep-links work for off-page objectives too. handleGetObjective
was strconv.ParseInt (numeric-only); it now routes through resolveObjectiveID
(the same helper suggestion-apply uses), which accepts the display_id form.
"""
import uuid

import requests
from conftest import ADMIN_EMAIL


def test_objective_get_by_display_id(api_url, admin_headers):
    # Ensure a programme exists (objectives require one; display_id = <key>-<seq>).
    requests.post(f"{api_url}/programs", headers=admin_headers, json={
        "title": "Sec Programme", "owner": ADMIN_EMAIL, "key": "SEC2026"})
    progs = requests.get(f"{api_url}/programs", headers=admin_headers).json().get("data") or []
    assert progs, "need at least one programme"

    r = requests.post(f"{api_url}/objectives", headers=admin_headers, json={
        "program_id": progs[0]["id"], "title": f"obj-{uuid.uuid4().hex[:8]}",
        "owner": ADMIN_EMAIL, "target_value": 5.0, "target_operator": "lte", "unit": "%"})
    assert r.status_code in (200, 201), r.text
    obj = r.json()
    num_id, display_id = obj["id"], obj["display_id"]
    assert display_id, "objective must have a display_id"

    by_num = requests.get(f"{api_url}/objectives/{num_id}", headers=admin_headers)
    by_disp = requests.get(f"{api_url}/objectives/{display_id}", headers=admin_headers)
    assert by_num.status_code == 200, by_num.text
    assert by_disp.status_code == 200, \
        f"GET /objectives/{display_id} must resolve by display_id (#166): {by_disp.status_code} {by_disp.text}"
    assert by_disp.json()["id"] == num_id, "display_id and numeric id must resolve the same objective"
