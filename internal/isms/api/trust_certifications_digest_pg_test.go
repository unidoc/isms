package api

import (
	"context"
	"testing"
)

func TestTrustCertDigestOverdueRowsAddUp(t *testing.T) {
	f := newCertFx(t, "tcdigest")
	soon := f.mkLink("Soon", ``)
	setUntil(t, soon.ID, intp(3))
	gone := f.mkLink("Gone", `,"valid_until":"2020-01-01"`)
	_ = gone
	d, err := BuildDigest(context.Background(), f.s.db, f.o.id, f.mgr, "manager")
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, c := range d.Overdue {
		sum += c.Count
	}
	if got := overdueCount(d, "Trust center certifications"); got != 2 || overdueCount(d, "External links") != 0 {
		t.Errorf("rows = %+v", d.Overdue)
	}
	if sum != d.OverdueTotal {
		t.Errorf("rows add up to %d, total says %d: %+v", sum, d.OverdueTotal, d.Overdue)
	}
}
