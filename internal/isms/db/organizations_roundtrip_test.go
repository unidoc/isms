package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// A duplicate slug (in any case) must surface as ErrSlugTaken, not a raw
// pgx error carrying the constraint name (#296).
func TestCreateOrganizationDuplicateSlug(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	slug := fmt.Sprintf("dup-slug-%d", time.Now().UnixNano())

	first := &Organization{Name: "A", Slug: slug, RepoPath: "/tmp/none"}
	if err := d.CreateOrganization(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, first.ID)
	})

	second := &Organization{Name: "B", Slug: "DUP" + slug[3:], RepoPath: "/tmp/none"}
	err := d.CreateOrganization(ctx, second)
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("second create err = %v, want ErrSlugTaken", err)
	}
}
