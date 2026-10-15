package main

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func TestValidateRepoPath(t *testing.T) {
	tmp := t.TempDir()
	if _, err := git.PlainInit(filepath.Join(tmp, "r.git"), true); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("relative path is absolutised", func(t *testing.T) {
		t.Chdir(tmp)
		got, err := validateRepoPath("r.git")
		if err != nil {
			t.Fatal(err)
		}
		want, _ := filepath.Abs("r.git")
		if got != want || !filepath.IsAbs(got) {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("non-repo dir rejected", func(t *testing.T) {
		if _, err := validateRepoPath(filepath.Join(tmp, "plain")); err == nil {
			t.Error("want error")
		}
	})
	t.Run("missing path rejected", func(t *testing.T) {
		if _, err := validateRepoPath(filepath.Join(tmp, "nope.git")); err == nil {
			t.Error("want error")
		}
	})
}
