package db

import (
	"context"
	"errors"
	"testing"
	"time"

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

// A create that overlaps an uncommitted delete of the live program on the same
// key must wait for it, then see the key as retired. Without the lock the
// retired check passes, the insert waits on the unique index, and it succeeds
// once the delete commits, leaving a program no objective can ever be added to.
func TestCreateProgramWaitsForConcurrentDeleteOfSameKey(t *testing.T) {
	d := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	orgID, _ := newTestOrgUser(t, d, "program-delete-recreate")

	p := &Program{Key: "PDK", Title: "Deleted while a create waits"}
	if err := d.CreateProgram(ctx, orgID, p); err != nil {
		t.Fatalf("CreateProgram: %v", err)
	}
	o := &Objective{ProgramID: p.ID, Title: "keeps the key retired"}
	if err := d.CreateObjective(ctx, orgID, o); err != nil {
		t.Fatalf("CreateObjective: %v", err)
	}

	tx1, err := d.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx1.Rollback(ctx)
	if _, _, err := DeleteProgramTx(ctx, tx1, orgID, p.ID); err != nil {
		t.Fatalf("DeleteProgramTx: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- d.CreateProgram(ctx, orgID, &Program{Key: "PDK", Title: "Recreated"})
	}()

	select {
	case err := <-done:
		t.Fatalf("CreateProgram returned %v while the delete was uncommitted, want it blocked", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit delete: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrProgramKeyRetired) {
			t.Fatalf("CreateProgram after the delete committed: err = %v, want ErrProgramKeyRetired", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CreateProgram still blocked 5s after the delete committed")
	}
	progs, err := d.ListPrograms(ctx, orgID)
	if err != nil {
		t.Fatalf("ListPrograms: %v", err)
	}
	for _, lp := range progs {
		if lp.Key == "PDK" {
			t.Errorf("a live program with the retired key exists: %+v", lp)
		}
	}
}
