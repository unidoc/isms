"""/config advertises the document export formats that are licensed.

Export needs a UniPDF/UniOffice licence. /config reports the formats that are
licensed right now as `export_formats` so the UI can hide the Export button.
The integration job runs without a licence key, so the list must be present and
empty: a missing key (old server) or a non-list value both fail here.
"""
import requests


def test_config_export_formats_empty_without_licence(api_url):
    cfg = requests.get(f"{api_url}/config")
    assert cfg.status_code == 200, cfg.text
    body = cfg.json()
    assert "export_formats" in body, "/config must expose export_formats"
    assert body["export_formats"] == [], \
        f"no licence configured, expected [] got {body['export_formats']!r}"
