"""Notification tests."""
import requests


def test_get_notifications(api_url, admin_headers):
    r = requests.get(f"{api_url}/notifications", headers=admin_headers)
    assert r.status_code == 200


def test_unread_count(api_url, admin_headers):
    r = requests.get(f"{api_url}/notifications/count", headers=admin_headers)
    assert r.status_code == 200
    assert "count" in r.json()


def test_mark_all_read(api_url, admin_headers):
    r = requests.post(f"{api_url}/notifications/read-all", headers=admin_headers, json={})
    assert r.status_code == 200
