"""Concurrency and data integrity tests."""
import requests
from concurrent.futures import ThreadPoolExecutor, as_completed


def test_concurrent_risk_creation(api_url, admin_headers):
    """Multiple simultaneous risk creates should all succeed."""
    def create_risk(i):
        return requests.post(f"{api_url}/risks", headers=admin_headers, json={
            "title": f"Concurrent risk {i}",
            "current_likelihood": 3, "current_impact": 3,
            "risk_type": "threat", "origin": "internal", "status": "open",
        }, timeout=10)

    with ThreadPoolExecutor(max_workers=5) as pool:
        futures = [pool.submit(create_risk, i) for i in range(5)]
        results = [f.result(timeout=15) for f in as_completed(futures)]

    for r in results:
        assert r.status_code in [200, 201], f"Concurrent create failed: {r.text}"


def test_concurrent_reads(api_url, admin_headers):
    """Multiple simultaneous reads should all succeed."""
    def read_endpoint(path):
        return requests.get(f"{api_url}{path}", headers=admin_headers, timeout=10)

    paths = ["/risks", "/assets", "/documents/all", "/reviews", "/incidents"]
    with ThreadPoolExecutor(max_workers=5) as pool:
        futures = [pool.submit(read_endpoint, p) for p in paths]
        results = [f.result(timeout=15) for f in as_completed(futures)]

    for r in results:
        assert r.status_code == 200
