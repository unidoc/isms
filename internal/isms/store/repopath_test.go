package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func initBare(t *testing.T, path string) {
	t.Helper()
	if _, err := git.PlainInit(path, true); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRepoPath(t *testing.T) {
	tmp := t.TempDir()
	stored := filepath.Join(tmp, "custom", "a.git")
	missing := filepath.Join(tmp, "old", "repos", "a.git")
	dataDir := filepath.Join(tmp, "data")
	fallback := filepath.Join(dataDir, "repos", "a.git")

	t.Run("stored ok", func(t *testing.T) {
		initBare(t, stored)
		initBare(t, fallback)
		got, fell, err := ResolveRepoPath(stored, dataDir, "a")
		if err != nil || fell || got != stored {
			t.Errorf("got %q fell=%v err=%v, want stored", got, fell, err)
		}
	})
	t.Run("stored missing fallback ok", func(t *testing.T) {
		got, fell, err := ResolveRepoPath(missing, dataDir, "a")
		if err != nil || !fell || got != fallback {
			t.Errorf("got %q fell=%v err=%v, want %q", got, fell, err, fallback)
		}
	})
	t.Run("both missing names both", func(t *testing.T) {
		_, _, err := ResolveRepoPath(missing, filepath.Join(tmp, "empty"), "a")
		if err == nil {
			t.Fatal("want error")
		}
		for _, p := range []string{missing, filepath.Join(tmp, "empty", "repos", "a.git")} {
			if !strings.Contains(err.Error(), p) {
				t.Errorf("error %q does not name %q", err, p)
			}
		}
	})
	t.Run("relative data dir is absolutised", func(t *testing.T) {
		t.Chdir(tmp)
		got, fell, err := ResolveRepoPath(missing, "data", "a")
		if err != nil || !fell || got != fallback {
			t.Errorf("got %q fell=%v err=%v, want %q", got, fell, err, fallback)
		}
	})
	t.Run("empty data dir does not fall back", func(t *testing.T) {
		t.Chdir(tmp)
		if err := os.MkdirAll(filepath.Join(tmp, "repos"), 0o755); err != nil {
			t.Fatal(err)
		}
		initBare(t, filepath.Join(tmp, "repos", "a.git"))
		_, fell, err := ResolveRepoPath(missing, "", "a")
		if err == nil || fell {
			t.Errorf("fell=%v err=%v, want error and no fallback", fell, err)
		}
	})
}
