package api

// Postgres-gated tests for the trust center preview (#520): the same three
// answers an anonymous visitor gets, for the signed-in manager's organization,
// whether or not the trust center is on. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run TrustPreview

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

const tpBase = "/api/v1/trust-center/preview"

func (f *tdFx) preview(h func(echo.Context) error, path, role, email string, pn, pv string) tdResp {
	return f.call(h, http.MethodGet, path, "", role, email, pn, pv)
}

// previewBodies returns the preview overview, items and the document answer for id.
func (f *tdFx) previewBodies(id string) (ov, items, doc tdResp) {
	ov = f.preview(f.s.handleTrustPreviewOverview, tpBase, "manager", f.mgr, "", "")
	items = f.preview(f.s.handleTrustPreviewItems, tpBase+"/items", "manager", f.mgr, "", "")
	doc = f.preview(f.s.handleTrustPreviewDocument, tpBase+"/documents/"+id, "manager", f.mgr, "id", id)
	return
}

func (f *tdFx) publicBodies(id string) (ov, items, doc trustResp) {
	base := "/api/v1/trust/" + f.o.slug
	return trustDo(f.s, trustTestBase, http.MethodGet, base, ""),
		trustDo(f.s, trustTestBase, http.MethodGet, base+"/items", ""),
		trustDo(f.s, trustTestBase, http.MethodGet, base+"/documents/"+id, "")
}

func TestTrustPreviewMatchesPublicAndWorksWhileOff(t *testing.T) {
	f := newTDFx(t, "tpv1", "pol-v1")
	f.mustOK(f.putSettings(`{"title":"Acme Trust","security_contact":"sec@acme.test","overview_document_id":"pol-v1"}`), "settings")
	f.save("# Pub\n\nApproved text.\n")
	f.approve()
	id := f.publishOK("public")
	gated := f.withDoc("pol-v2", "Gated Doc")
	gated.save("# Gated\n\nSecret body text.\n")
	gated.approve()
	gatedID := gated.publishOK("gated")

	// ON: preview equals what visitors get.
	pubOv, pubItems, pubDoc := f.publicBodies(id)
	if pubOv.code != 200 || pubItems.code != 200 || pubDoc.code != 200 {
		t.Fatalf("public with the trust center on: %d %d %d", pubOv.code, pubItems.code, pubDoc.code)
	}
	ov, items, doc := f.previewBodies(id)
	for name, pair := range map[string][2]string{"overview": {ov.body, pubOv.body}, "items": {items.body, pubItems.body}, "document": {doc.body, pubDoc.body}} {
		if strings.TrimSpace(pair[0]) != strings.TrimSpace(pair[1]) {
			t.Errorf("%s: preview != public\npreview: %s\npublic:  %s", name, pair[0], pair[1])
		}
	}
	// Locked items: no body in the list, and the document answer is the one 404.
	if strings.Contains(items.body, "Secret body text") || strings.Contains(items.body, `"html"`) {
		t.Errorf("preview items carry a body: %s", items.body)
	}
	lockedPub := trustDo(f.s, trustTestBase, http.MethodGet, "/api/v1/trust/"+f.o.slug+"/documents/"+gatedID, "")
	locked := f.preview(f.s.handleTrustPreviewDocument, tpBase+"/documents/"+gatedID, "manager", f.mgr, "id", gatedID)
	if locked.code != http.StatusNotFound || lockedPub.code != http.StatusNotFound || strings.Contains(locked.body, "Secret body text") || strings.Contains(locked.body, "html") {
		t.Errorf("locked document in preview = %d %s (public %d)", locked.code, locked.body, lockedPub.code)
	}

	// OFF: visitors get the one 404, the preview still answers, with the same bodies.
	f.mustOK(f.putSettings(`{"enabled":false}`), "off")
	offOv, offItems, offDoc := f.publicBodies(id)
	if offOv.code != 404 || offItems.code != 404 || offDoc.code != 404 {
		t.Fatalf("public with the trust center off: %d %d %d", offOv.code, offItems.code, offDoc.code)
	}
	ov2, items2, doc2 := f.previewBodies(id)
	if ov2.code != 200 || items2.code != 200 || doc2.code != 200 {
		t.Fatalf("preview with the trust center off: %d %d %d", ov2.code, items2.code, doc2.code)
	}
	for name, pair := range map[string][2]string{"overview": {ov2.body, pubOv.body}, "items": {items2.body, pubItems.body}, "document": {doc2.body, pubDoc.body}} {
		if strings.TrimSpace(pair[0]) != strings.TrimSpace(pair[1]) {
			t.Errorf("off %s: preview != public when on\npreview: %s\npublic:  %s", name, pair[0], pair[1])
		}
	}
}

func TestTrustPreviewSkipsHiddenAndUnknown(t *testing.T) {
	f := newTDFx(t, "tpv2", "pol-w1")
	f.save("text\n")
	f.approve()
	id := f.publishOK("public")
	f.mustOK(f.putSettings(`{"enabled":false}`), "off")
	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", f.mgr), "retire")
	items := f.preview(f.s.handleTrustPreviewItems, tpBase+"/items", "manager", f.mgr, "", "")
	if items.code != 200 || strings.TrimSpace(items.body) != "[]" {
		t.Errorf("retired item in preview items = %d %s", items.code, items.body)
	}
	if r := f.preview(f.s.handleTrustPreviewDocument, tpBase+"/documents/"+id, "manager", f.mgr, "id", id); r.code != 404 {
		t.Errorf("retired document = %d", r.code)
	}
	for _, bad := range []string{"nope", "00000000-0000-0000-0000-000000000000"} {
		if r := f.preview(f.s.handleTrustPreviewDocument, tpBase+"/documents/"+bad, "manager", f.mgr, "id", bad); r.code != 404 {
			t.Errorf("%s = %d, want 404", bad, r.code)
		}
	}
}

func TestTrustPreviewIsPerSessionOrg(t *testing.T) {
	a := newTDFx(t, "tpv3a", "pol-x")
	b := newTDFx(t, "tpv3b", "pol-x")
	for _, f := range []*tdFx{a, b} {
		f.save("text of " + f.o.slug + "\n")
		f.approve()
		f.mustOK(f.putSettings(`{"enabled":false,"title":"Title `+f.o.slug+`"}`), "settings")
	}
	idA := a.publishOK("public")
	idB := b.publishOK("public")
	ovA, itemsA, _ := a.previewBodies(idA)
	ovB, itemsB, _ := b.previewBodies(idB)
	if !strings.Contains(ovA.body, "Title "+a.o.slug) || strings.Contains(ovA.body, b.o.slug) {
		t.Errorf("A's preview = %s", ovA.body)
	}
	if !strings.Contains(ovB.body, "Title "+b.o.slug) || strings.Contains(ovB.body, a.o.slug) {
		t.Errorf("B's preview = %s", ovB.body)
	}
	if !strings.Contains(itemsA.body, idA) || strings.Contains(itemsA.body, idB) || !strings.Contains(itemsB.body, idB) || strings.Contains(itemsB.body, idA) {
		t.Errorf("items leak across orgs: A %s B %s", itemsA.body, itemsB.body)
	}
	// B's manager asking for A's document id gets the 404.
	if r := b.preview(b.s.handleTrustPreviewDocument, tpBase+"/documents/"+idA, "manager", b.mgr, "id", idA); r.code != 404 {
		t.Errorf("other org's document = %d", r.code)
	}
}

func TestTrustPreviewRolesAndAuth(t *testing.T) {
	f := newTDFx(t, "tpv4", "pol-y")
	f.save("x\n")
	f.approve()
	id := f.publishOK("public")
	type ep struct {
		path string
		h    func(echo.Context) error
		pn   string
		pv   string
	}
	eps := []ep{
		{tpBase, f.s.handleTrustPreviewOverview, "", ""},
		{tpBase + "/items", f.s.handleTrustPreviewItems, "", ""},
		{tpBase + "/documents/" + id, f.s.handleTrustPreviewDocument, "id", id},
	}
	for _, e := range eps {
		for _, who := range []struct{ role, email string }{{"reader", f.read}, {"contributor", f.contr}} {
			if r := f.preview(e.h, e.path, who.role, who.email, e.pn, e.pv); r.code != http.StatusForbidden {
				t.Errorf("%s as %s = %d %s, want 403", e.path, who.role, r.code, r.body)
			}
		}
		for _, who := range []struct{ role, email string }{{"manager", f.mgr}, {"admin", f.admin}} {
			if r := f.preview(e.h, e.path, who.role, who.email, e.pn, e.pv); r.code != http.StatusOK {
				t.Errorf("%s as %s = %d %s, want 200", e.path, who.role, r.code, r.body)
			}
		}
		// Through the router with no token: authenticated, unlike /api/v1/trust.
		if code := serveTM(f.s, newTMReq(http.MethodGet, e.path, "")); code != http.StatusUnauthorized {
			t.Errorf("%s with no token = %d, want 401", e.path, code)
		}
	}
}
