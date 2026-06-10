package main

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TestRequireMain verifies sync refuses to run off main unless --force is given,
// so it never silently discards work on a detached HEAD or another branch.
func TestRequireMain(t *testing.T) {
	main := plumbing.NewBranchReferenceName("main")
	other := plumbing.NewBranchReferenceName("feature")

	if err := requireMain(main, false); err != nil {
		t.Fatalf("on main should pass: %v", err)
	}
	if err := requireMain(plumbing.HEAD, false); err == nil {
		t.Fatal("detached HEAD without --force should fail")
	}
	if err := requireMain(other, false); err == nil {
		t.Fatal("other branch without --force should fail")
	}
	if err := requireMain(plumbing.HEAD, true); err != nil {
		t.Fatalf("--force should allow a detached HEAD: %v", err)
	}
	if err := requireMain(other, true); err != nil {
		t.Fatalf("--force should allow another branch: %v", err)
	}
}

// TestFastForwardKeepsHeadAttached is a regression for the data-loss path where
// a sync fast-forward checked out the bare remote hash, leaving HEAD detached.
// A later sync would then "recover" by force-resetting to the remote tip,
// silently discarding any commits made on the detached HEAD. HEAD must stay
// attached to the branch after a fast-forward.
func TestFastForwardKeepsHeadAttached(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	main := plumbing.NewBranchReferenceName("main")
	// PlainInit points HEAD at master; ISMS content lives on main.
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, main)); err != nil {
		t.Fatalf("set HEAD->main: %v", err)
	}

	commit := func(name string) plumbing.Hash {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
		h, err := wt.Commit("add "+name, &git.CommitOptions{
			Author: &object.Signature{Name: "t", Email: "t@example.test"},
		})
		if err != nil {
			t.Fatalf("commit %s: %v", name, err)
		}
		return h
	}

	c1 := commit("a.txt") // main @ c1
	c2 := commit("b.txt") // remote tip we will fast-forward to

	// Simulate "local behind remote": rewind main to c1, HEAD still on main.
	if err := repo.Storer.SetReference(plumbing.NewHashReference(main, c1)); err != nil {
		t.Fatalf("rewind main: %v", err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: main, Force: true}); err != nil {
		t.Fatalf("checkout c1: %v", err)
	}

	// Fast-forward to the remote tip.
	if err := fastForwardToRemote(repo, c2); err != nil {
		t.Fatalf("fastForwardToRemote: %v", err)
	}

	// HEAD must be a symbolic ref to main — NOT detached.
	headRaw, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		t.Fatalf("read raw HEAD: %v", err)
	}
	if headRaw.Type() != plumbing.SymbolicReference || headRaw.Target() != main {
		t.Fatalf("HEAD detached after fast-forward: type=%v target=%v", headRaw.Type(), headRaw.Target())
	}

	// Branch must have advanced to the remote tip, and the worktree with it.
	mainRef, err := repo.Reference(main, false)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	if mainRef.Hash() != c2 {
		t.Fatalf("main not advanced: got %s want %s", mainRef.Hash(), c2)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatalf("worktree not updated to remote tip: %v", err)
	}
}

// TestFastForwardRefusesDirtyWorktree verifies a fast-forward refuses (and does
// NOT silently discard) uncommitted edits to tracked files — a plain sync must
// never wipe local working-tree changes.
func TestFastForwardRefusesDirtyWorktree(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	main := plumbing.NewBranchReferenceName("main")
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, main)); err != nil {
		t.Fatalf("set HEAD->main: %v", err)
	}
	commit := func(name string) plumbing.Hash {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
		h, err := wt.Commit("add "+name, &git.CommitOptions{
			Author: &object.Signature{Name: "t", Email: "t@example.test"},
		})
		if err != nil {
			t.Fatalf("commit %s: %v", name, err)
		}
		return h
	}
	c1 := commit("a.txt")
	c2 := commit("b.txt")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(main, c1)); err != nil {
		t.Fatalf("rewind main: %v", err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: main, Force: true}); err != nil {
		t.Fatalf("checkout c1: %v", err)
	}

	// Uncommitted edit to a tracked file.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("LOCAL EDIT"), 0o644); err != nil {
		t.Fatalf("dirty edit: %v", err)
	}

	if err := fastForwardToRemote(repo, c2); err == nil {
		t.Fatal("expected fast-forward to refuse a dirty working tree, got nil")
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(got) != "LOCAL EDIT" {
		t.Fatalf("uncommitted edit was discarded: a.txt = %q", got)
	}
}

// TestCloneLandsOnMain verifies clonePinnedToMain leaves HEAD attached to main,
// independent of the server's HEAD advertisement.
func TestCloneLandsOnMain(t *testing.T) {
	src := t.TempDir()
	srcRepo, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatalf("init src: %v", err)
	}
	swt, err := srcRepo.Worktree()
	if err != nil {
		t.Fatalf("src worktree: %v", err)
	}
	main := plumbing.NewBranchReferenceName("main")
	if err := srcRepo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, main)); err != nil {
		t.Fatalf("src HEAD->main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := swt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := swt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.test"},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	target := filepath.Join(t.TempDir(), "clone")
	repo, err := clonePinnedToMain(target, src, nil)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	headRaw, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		t.Fatalf("read HEAD: %v", err)
	}
	if headRaw.Type() != plumbing.SymbolicReference || headRaw.Target() != main {
		t.Fatalf("clone left HEAD detached/off-main: type=%v target=%v", headRaw.Type(), headRaw.Target())
	}
}
