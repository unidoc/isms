"""Weekly digest preference (#232): GET/PUT /me/digest, per org, self only."""
import pytest
import requests


@pytest.fixture(autouse=True)
def digest_defaults(api_url, admin_headers, contributor_headers, reader_headers):
    """The users are shared across the suite: start and end with the default (on)."""
    def reset():
        for h in (admin_headers, contributor_headers, reader_headers):
            requests.put(f"{api_url}/me/digest", headers=h, json={"enabled": True})
    reset()
    yield
    reset()


def _get(api_url, headers):
    r = requests.get(f"{api_url}/me/digest", headers=headers)
    assert r.status_code == 200, r.text
    return r.json()["enabled"]


def test_default_is_on(api_url, admin_headers):
    assert _get(api_url, admin_headers) is True


def test_put_false_then_get_false_and_back(api_url, admin_headers):
    r = requests.put(f"{api_url}/me/digest", headers=admin_headers, json={"enabled": False})
    assert r.status_code == 200, r.text
    assert r.json() == {"enabled": False}
    assert _get(api_url, admin_headers) is False
    r = requests.put(f"{api_url}/me/digest", headers=admin_headers, json={"enabled": True})
    assert r.status_code == 200
    assert _get(api_url, admin_headers) is True


def test_reader_can_toggle_own(api_url, reader_headers):
    r = requests.put(f"{api_url}/me/digest", headers=reader_headers, json={"enabled": False})
    assert r.status_code == 200, r.text
    assert _get(api_url, reader_headers) is False


@pytest.mark.parametrize("body", [{}, {"enabled": None}, {"enabled": "yes"}, {"enabled": 1}, {"user": "x"}])
def test_bad_body_is_400_and_changes_nothing(api_url, admin_headers, body):
    r = requests.put(f"{api_url}/me/digest", headers=admin_headers, json=body)
    assert r.status_code == 400, r.text
    assert _get(api_url, admin_headers) is True


def test_one_users_toggle_does_not_touch_another(api_url, admin_headers, contributor_headers):
    r = requests.put(f"{api_url}/me/digest", headers=contributor_headers, json={"enabled": False})
    assert r.status_code == 200
    assert _get(api_url, contributor_headers) is False
    assert _get(api_url, admin_headers) is True


def test_a_user_id_in_the_body_is_ignored(api_url, admin_headers, contributor_headers):
    """There is no way to name another user: the caller's own row changes."""
    r = requests.put(f"{api_url}/me/digest", headers=admin_headers,
                     json={"enabled": False, "user_id": 1, "email": "testreviewer@isms-test.local"})
    assert r.status_code == 200
    assert _get(api_url, admin_headers) is False
    assert _get(api_url, contributor_headers) is True


def test_requires_authentication(api_url):
    assert requests.get(f"{api_url}/me/digest").status_code in (401, 403)
    assert requests.put(f"{api_url}/me/digest", json={"enabled": False}).status_code in (401, 403)
