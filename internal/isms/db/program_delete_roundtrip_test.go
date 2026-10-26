package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Regression for #367: a create that passed its program pre-check just before
// a delete committed reaches nextObjectiveSeqTx with the program already gone.
// The lock must refuse it instead of allocating a seq for an orphan.
func TestNextObjectiveSeqRefusesDeletedProgram(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "program-delete-seq")

	p := &Program{Key: "PDS", Title: "Program deleted under a create"}
	if err := d.CreateProgram(ctx, orgID, p); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}

	seqFor := func() (int, error) {
		var seq int
		err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			seq, err = nextObjectiveSeqTx(ctx, tx, p.ID)
			return err
		})
		return seq, err
	}
	if seq, err := seqFor(); err != nil || seq != 1 {
		t.Fatalf("live program: seq = %d, err = %v, want 1, nil", seq, err)
	}

	if err := d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, _, err := DeleteProgramTx(ctx, tx, orgID, p.ID)
		return err
	}); err != nil {
		t.Fatalf("DeleteProgramTx: %v", err)
	}
	if _, err := seqFor(); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted program: err = %v, want pgx.ErrNoRows", err)
	}
}
