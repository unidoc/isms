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

func TestSetOrganizationRepoPath(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	slug := fmt.Sprintf("set-repo-%d", time.Now().UnixNano())

	org := &Organization{Name: "A", Slug: slug, RepoPath: "/tmp/old.git"}
	if err := d.CreateOrganization(ctx, org); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, org.ID)
	})
	before, err := d.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := d.SetOrganizationRepoPath(ctx, org.ID, "/tmp/new.git"); err != nil {
		t.Fatalf("set: %v", err)
	}
	after, err := d.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.RepoPath != "/tmp/new.git" {
		t.Errorf("repo_path = %q, want /tmp/new.git", after.RepoPath)
	}
	if !after.UpdatedAt.Time.After(before.UpdatedAt.Time) {
		t.Errorf("updated_at did not move: %v -> %v", before.UpdatedAt, after.UpdatedAt)
	}

	if err := d.DeleteOrganization(ctx, org.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.SetOrganizationRepoPath(ctx, org.ID, "/tmp/third.git"); err == nil {
		t.Error("set on a soft-deleted org succeeded, want error")
	}
	var got string
	if err := d.pool.QueryRow(ctx, `SELECT repo_path FROM organizations WHERE id = $1`, org.ID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/new.git" {
		t.Errorf("soft-deleted org repo_path = %q, want unchanged /tmp/new.git", got)
	}
}
