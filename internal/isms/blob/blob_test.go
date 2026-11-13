package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func TestLocalStoreOpenStreamsAndReportsSize(t *testing.T) {
	ctx := context.Background()
	l := NewLocal(t.TempDir())
	want := []byte("hello streaming world")
	if err := l.PutStream(ctx, "org-1", "trust/certifications/a.pdf", "application/pdf", bytes.NewReader(want)); err != nil {
		t.Fatal(err)
	}
	r, size, err := l.Open(ctx, "org-1", "trust/certifications/a.pdf")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != string(want) || size != int64(len(want)) {
		t.Errorf("got %q size %d, want %q size %d", got, size, want, len(want))
	}
	// A missing file is an error the caller can tell from success, and other
	// organizations' keys do not collide.
	if _, _, err := l.Open(ctx, "org-2", "trust/certifications/a.pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("other org Open err = %v, want not exist", err)
	}
}
