package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// go-git rewrites a loose ref by truncating the file and then writing the hash,
// so a reader in between sees an empty ref. The default branch must never be
// answered from such a read: not as an empty set of documents and not as the
// older hash packed-refs may still hold.

func newDocsRepo(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "org.git")
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
	st, err := NewBare(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"doc-a", "doc-b"} {
		raw := "---\ndocument_id: " + id + "\ntitle: " + id + "\nstatus: draft\n---\nbody\n"
		if _, err := st.CommitFile(filepath.Join(dir, "documents", "x", id+".md"), []byte(raw), "T", "t@example.test", "add "+id); err != nil {
			t.Fatal(err)
		}
	}
	return st, dir
}

func branchFile(t *testing.T, st *Store) string {
	t.Helper()
	ref, err := st.DefaultBranchRef()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(st.root, filepath.FromSlash(ref))
}

func TestDefaultBranchHashNeverTrustsATruncatedRef(t *testing.T) {
	st, _ := newDocsRepo(t)
	good, empty, err := st.DefaultBranchHash()
	if err != nil || empty || len(good) != 40 {
		t.Fatalf("settled ref: %q %v %v", good, empty, err)
	}
	ids, err := st.DocumentIDsAtHead()
	if err != nil || !ids["doc-a"] || !ids["doc-b"] {
		t.Fatalf("ids = %v %v", ids, err)
	}
	// Reading through go-git's own resolver during a write is the hazard.
	loose := branchFile(t, st)

	t.Run("truncated loose ref, no packed-refs", func(t *testing.T) {
		if err := os.WriteFile(loose, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(loose, []byte(good+"\n"), 0o644)
		if h, empty, err := st.DefaultBranchHash(); err == nil {
			t.Errorf("answered %q (empty %v) from a truncated ref", h, empty)
		}
		if ids, err := st.DocumentIDsAtHead(); err == nil {
			t.Errorf("DocumentIDsAtHead answered %v from a truncated ref", ids)
		}
	})

	t.Run("truncated loose ref and a stale packed-refs", func(t *testing.T) {
		parent, err := st.ParentHash(good)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := st.DefaultBranchRef()
		packed := filepath.Join(st.root, "packed-refs")
		if err := os.WriteFile(packed, []byte("# pack-refs with: peeled fully-peeled sorted\n"+parent+" "+ref+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(packed)
		if err := os.WriteFile(loose, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(loose, []byte(good+"\n"), 0o644)
		h, _, err := st.DefaultBranchHash()
		if err == nil {
			t.Errorf("answered %q from a truncated ref with a stale packed ref (the old commit is %s)", h, parent)
		}
		if ids, err := st.DocumentIDsAtHead(); err == nil {
			t.Errorf("DocumentIDsAtHead answered %v", ids)
		}
	})

	t.Run("a ref that is only packed is read", func(t *testing.T) {
		ref, _ := st.DefaultBranchRef()
		packed := filepath.Join(st.root, "packed-refs")
		if err := os.WriteFile(packed, []byte("# pack-refs with: peeled fully-peeled sorted\n"+good+" "+ref+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(packed)
		saved, _ := os.ReadFile(loose)
		if err := os.Remove(loose); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(loose, saved, 0o644)
		if h, empty, err := st.DefaultBranchHash(); err != nil || empty || h != good {
			t.Errorf("packed ref: %q %v %v", h, empty, err)
		}
	})

	t.Run("a branch that is missing while other refs exist is an error", func(t *testing.T) {
		saved, _ := os.ReadFile(loose)
		if err := os.Remove(loose); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(loose, saved, 0o644)
		other := filepath.Join(st.root, "refs", "heads", "other")
		if err := os.WriteFile(other, []byte(good+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(other)
		if h, empty, err := st.DefaultBranchHash(); err == nil {
			t.Errorf("answered %q (empty %v)", h, empty)
		}
	})
}

func TestDefaultBranchHashOfAnEmptyRepository(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty.git")
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
	st, err := NewBare(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h, empty, err := st.DefaultBranchHash(); err != nil || !empty || h != "" {
		t.Errorf("empty repository: %q %v %v", h, empty, err)
	}
	if ids, err := st.DocumentIDsAtHead(); err != nil || len(ids) != 0 {
		t.Errorf("ids = %v %v", ids, err)
	}
}

// While a ref is being rewritten (truncate, pause, write) a reader gets the real
// hash or an error, never an empty answer or another hash.
func TestDefaultBranchHashDuringRewrites(t *testing.T) {
	st, _ := newDocsRepo(t)
	good, _, err := st.DefaultBranchHash()
	if err != nil {
		t.Fatal(err)
	}
	loose := branchFile(t, st)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			f, err := os.OpenFile(loose, os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				continue
			}
			time.Sleep(200 * time.Microsecond) // the window
			f.WriteString(good + "\n")
			f.Close()
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	var okHash, okIDs int
	for i := 0; i < 400; i++ {
		h, empty, err := st.DefaultBranchHash()
		switch {
		case err != nil:
		case empty || h != good:
			t.Fatalf("read %d: %q empty=%v", i, h, empty)
		default:
			okHash++
		}
		if ids, err := st.DocumentIDsAtHead(); err == nil {
			if !(ids["doc-a"] && ids["doc-b"]) {
				t.Fatalf("read %d: a wrong set %v", i, ids)
			}
			okIDs++
		}
	}
	// Reads that fail are allowed; reads that answer must be right, and the
	// reader must not be starved into failing every time.
	if okHash == 0 || okIDs == 0 {
		t.Fatalf("no read ever succeeded (%d hashes, %d id sets)", okHash, okIDs)
	}
	if _, err := st.DocumentIDsAtCommit(strings.Repeat("0", 40)); err == nil {
		t.Error("a missing commit was read")
	}
}

// ---- unreadable objects are errors, never "absent" ---------------------------------

// looseObject is where the object h is stored when it is a loose object.
func looseObject(st *Store, h plumbing.Hash) string {
	hex := h.String()
	return filepath.Join(st.root, "objects", hex[:2], hex[2:])
}

func TestDocumentIDsAtCommitFailsOnMissingObjects(t *testing.T) {
	for _, tc := range []struct {
		name string
		pick func(st *Store, commit string) plumbing.Hash
	}{
		{"the documents tree object", func(st *Store, c string) plumbing.Hash {
			cm, _ := st.repo.CommitObject(plumbing.NewHash(c))
			root, _ := cm.Tree()
			e, _ := root.FindEntry("documents")
			return e.Hash
		}},
		{"a subtree of documents", func(st *Store, c string) plumbing.Hash {
			cm, _ := st.repo.CommitObject(plumbing.NewHash(c))
			root, _ := cm.Tree()
			e, _ := root.FindEntry("documents/x")
			return e.Hash
		}},
		{"a document blob", func(st *Store, c string) plumbing.Hash {
			cm, _ := st.repo.CommitObject(plumbing.NewHash(c))
			root, _ := cm.Tree()
			e, _ := root.FindEntry("documents/x/doc-a.md")
			return e.Hash
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, dir := newDocsRepo(t)
			head, _, err := st.DefaultBranchHash()
			if err != nil {
				t.Fatal(err)
			}
			if ids, err := st.DocumentIDsAtCommit(head); err != nil || len(ids) != 2 {
				t.Fatalf("precondition: %v %v", ids, err)
			}
			victim := looseObject(st, tc.pick(st, head))
			if err := os.Remove(victim); err != nil {
				t.Fatalf("the object is not a loose file: %v", err)
			}
			fresh, err := NewBare(dir) // no cached objects
			if err != nil {
				t.Fatal(err)
			}
			if ids, err := fresh.DocumentIDsAtCommit(head); err == nil {
				t.Errorf("answered %v with %s missing", ids, tc.name)
			}
			if ids, err := fresh.DocumentIDsAtHead(); err == nil {
				t.Errorf("DocumentIDsAtHead answered %v with %s missing", ids, tc.name)
			}
		})
	}
}

// Only a commit with no documents entry at all has no documents.
func TestDocumentIDsAtCommitWithNoDocumentsEntry(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "r.git")
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
	st, _ := NewBare(dir)
	if _, err := st.CommitFile(filepath.Join(dir, "README.md"), []byte("# r\n"), "T", "t@example.test", "init"); err != nil {
		t.Fatal(err)
	}
	head, _, _ := st.DefaultBranchHash()
	if ids, err := st.DocumentIDsAtCommit(head); err != nil || ids == nil || len(ids) != 0 {
		t.Errorf("no documents entry: %v %v", ids, err)
	}
}

// The packed-refs fallback is checked again: a loose ref that appears meanwhile
// is newer than the packed one.
func TestDefaultBranchHashPackedFallbackIsRechecked(t *testing.T) {
	st, _ := newDocsRepo(t)
	good, _, _ := st.DefaultBranchHash()
	older, _ := st.ParentHash(good)
	ref, _ := st.DefaultBranchRef()
	loose := branchFile(t, st)
	packed := filepath.Join(st.root, "packed-refs")
	if err := os.WriteFile(packed, []byte("# pack-refs with: peeled fully-peeled sorted\n"+older+" "+ref+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(packed)
	if err := os.Remove(loose); err != nil {
		t.Fatal(err)
	}
	defer os.WriteFile(loose, []byte(good+"\n"), 0o644)
	var once sync.Once
	afterPackedLookup = func() { once.Do(func() { os.WriteFile(loose, []byte(good+"\n"), 0o644) }) }
	defer func() { afterPackedLookup = nil }()
	h, _, err := st.DefaultBranchHash()
	if err != nil || h != good {
		t.Errorf("got %q (%v), want the loose %s, not the packed %s", h, err, good, older)
	}
}
