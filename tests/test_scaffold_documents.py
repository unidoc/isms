"""Template scaffold + document visibility tests.

Verifies that after scaffolding a template, documents actually appear
in the document tree and are readable. Includes test for fresh org
with no prior commits (HEAD doesn't exist yet).
"""
import requests


class TestScaffoldProducesDocuments:
    """After scaffold, documents must be visible and readable."""

    def test_01_documents_all_has_folders(self, api_url, admin_headers):
        """GET /documents/all must return at least one folder after setup."""
        r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
        assert r.status_code == 200
        data = r.json().get("data") or []
        assert len(data) > 0, "No document folders found — scaffold may have failed"

    def test_02_iso27001_folder_exists(self, api_url, admin_headers):
        """iso27001 folder must exist after template scaffold."""
        r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
        folders = [f["name"] for f in (r.json().get("data") or [])]
        assert "iso27001" in folders, f"iso27001 not in folders: {folders}"

    def test_03_iso27001_has_files(self, api_url, admin_headers):
        """iso27001 folder must contain documents (not be empty)."""
        r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
        data = r.json().get("data") or []
        iso = next((f for f in data if f["name"] == "iso27001"), None)
        assert iso is not None, "iso27001 folder not found"
        # Count files recursively
        def count_files(folder):
            n = len(folder.get("files") or [])
            for sub in (folder.get("subfolders") or []):
                n += count_files(sub)
            return n
        total = count_files(iso)
        assert total > 0, f"iso27001 has 0 files — scaffold produced empty folder"

    def test_04_document_body_readable(self, api_url, admin_headers):
        """A scaffolded document should be readable by ID."""
        r = requests.get(f"{api_url}/documents/iso27001-4-1/body", headers=admin_headers)
        assert r.status_code == 200, f"Cannot read iso27001-4-1: {r.text}"
        body = r.json().get("body") or ""
        assert len(body) > 0, "Document body is empty"

    def test_05_document_blame_works(self, api_url, admin_headers):
        """Blame should return lines for a scaffolded document."""
        r = requests.get(f"{api_url}/documents/iso27001-4-1/blame", headers=admin_headers)
        assert r.status_code == 200, f"Blame failed: {r.text}"
        lines = r.json().get("lines") or []
        assert len(lines) > 0, "Blame returned no lines"
        assert "author" in lines[0], "Blame line missing author"
        assert "date" in lines[0], "Blame line missing date"

    def test_06_add_second_template(self, api_url, admin_headers):
        """Adding a second template should scaffold its documents."""
        r = requests.post(f"{api_url}/templates", headers=admin_headers,
                          json={"template": "soc2"})
        assert r.status_code == 201, f"Add soc2 failed: {r.text}"
        # Verify soc2 folder appears
        r2 = requests.get(f"{api_url}/documents/all", headers=admin_headers)
        folders = [f["name"] for f in (r2.json().get("data") or [])]
        assert "soc2" in folders, f"soc2 not in folders after scaffold: {folders}"
