package api

// confirm-review records an approval, a decision and a version snapshot, so it
// must only confirm a document that is approved. Run with:
//
//	ISMS_TEST_DATABASE_URL=... go test ./internal/isms/api/ -run ConfirmReview

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

type confirmCounts struct {
	approvals, decisions, versions int
	head                           string
}

func (f *tdFx) counts() confirmCounts {
	f.t.Helper()
	ctx := context.Background()
	a, err := f.s.db.ApprovalsForDocument(ctx, f.o.id, f.docID)
	if err != nil {
		f.t.Fatal(err)
	}
	d, err := f.s.db.ListDecisionRecords(ctx, f.o.id, f.docID)
	if err != nil {
		f.t.Fatal(err)
	}
	v, err := f.s.db.ListVersions(ctx, f.o.id, f.docID)
	if err != nil {
		f.t.Fatal(err)
	}
	h, _ := f.st.HeadHash()
	return confirmCounts{len(a), len(d), len(v), h}
}

func (f *tdFx) confirm(role, email string) tdResp {
	return f.docCall(f.s.handleConfirmDocumentReview, http.MethodPost, `{"comment":"annual"}`, role, email)
}

func (f *tdFx) assertRefused(r tdResp, before confirmCounts, why string) {
	f.t.Helper()
	if r.code != http.StatusConflict || !strings.Contains(r.body, CodeDocumentNotApproved) {
		f.t.Fatalf("%s: confirm = %d %s, want 409 %s", why, r.code, r.body, CodeDocumentNotApproved)
	}
	if after := f.counts(); after != before {
		f.t.Errorf("%s: a refused confirm wrote something: before %+v after %+v", why, before, after)
	}
}

// a. Control: an approved document is confirmed as before.
func TestConfirmReviewApprovedDocumentIsConfirmed(t *testing.T) {
	f := newTDFx(t, "cra", "pol-ca")
	f.save("text\n")
	f.approve()
	before := f.counts()
	f.mustOK(f.confirm("admin", f.admin), "confirm")
	after := f.counts()
	if after.approvals != before.approvals+1 || after.decisions != before.decisions+1 {
		t.Errorf("approval/decision not written: before %+v after %+v", before, after)
	}
	if after.versions != before.versions { // same version number: upserted, not added
		t.Errorf("versions before %d after %d", before.versions, after.versions)
	}
}

// b. Approved, then edited back into a draft: refused, nothing written.
func TestConfirmReviewDraftAfterApprovalIsRefused(t *testing.T) {
	f := newTDFx(t, "crb", "pol-cb")
	f.save("approved text\n")
	f.approve()
	f.save("unapproved edit\n")
	f.assertRefused(f.confirm("admin", f.admin), f.counts(), "draft after approval")
	r := f.confirm("admin", f.admin)
	if !strings.Contains(r.body, "full review") {
		t.Errorf("message should say a changed document needs a full review: %s", r.body)
	}
}

// c. Never approved: draft, then in review.
func TestConfirmReviewNeverApprovedIsRefused(t *testing.T) {
	f := newTDFx(t, "crc", "pol-cc")
	f.assertRefused(f.confirm("admin", f.admin), f.counts(), "draft")
	f.mustOK(f.docCall(f.s.handleReviewSend, http.MethodPost, `{"reviewers":["`+f.rev+`"]}`, "admin", f.admin), "send")
	if st := f.state(); st.DocumentStatus != "in_review" {
		t.Fatalf("precondition: status = %q, want in_review", st.DocumentStatus)
	}
	f.assertRefused(f.confirm("admin", f.admin), f.counts(), "in_review")
}

// d. Retired.
func TestConfirmReviewRetiredIsRefused(t *testing.T) {
	f := newTDFx(t, "crd", "pol-cd")
	f.save("text\n")
	f.approve()
	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"status":"retired"}}`, "manager", f.mgr), "retire")
	f.assertRefused(f.confirm("admin", f.admin), f.counts(), "retired")
}

// e. The owner rule still comes first: a non-owner contributor gets 403 (never
// 409), an owning contributor can confirm an approved document.
func TestConfirmReviewOwnerRuleUnchanged(t *testing.T) {
	f := newTDFx(t, "cre", "pol-ce")
	f.save("text\n")
	f.approve()
	f.mustOK(f.docCall(f.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"owner":"`+f.mgr+`"}}`, "manager", f.mgr), "set owner")
	before := f.counts()
	if r := f.confirm("contributor", f.contr); r.code != http.StatusForbidden {
		t.Errorf("non-owner contributor on an approved doc = %d %s, want 403", r.code, r.body)
	}
	f.save("edit\n") // now a draft, still owned by someone else
	if r := f.confirm("contributor", f.contr); r.code != http.StatusForbidden {
		t.Errorf("non-owner contributor on a draft = %d %s, want 403, not 409", r.code, r.body)
	}
	_ = before

	g := newTDFx(t, "cre2", "pol-ce2")
	g.save("text\n")
	g.approve()
	g.mustOK(g.docCall(g.s.handleUpdateDocumentMetadata, http.MethodPut, `{"fields":{"owner":"`+g.contr+`"}}`, "manager", g.mgr), "set owner")
	g.mustOK(g.confirm("contributor", g.contr), "owner confirms")
}
