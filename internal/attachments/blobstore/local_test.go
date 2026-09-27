package blobstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStoreContract(t *testing.T) {
	runContract(t, NewLocal(t.TempDir()))
}

// A symlink planted at the first path segment a key resolves to must not let
// Put escape root, even though the deeper segments under it do not exist yet
// and so cannot be caught by a plain "does this path exist" check.
func TestLocalPutRejectsSymlinkedFirstSegment(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	if err := os.Symlink(outside, filepath.Join(root, "ws")); err != nil {
		t.Fatal(err)
	}

	s := NewLocal(root)
	body := []byte("attacker-controlled")
	err := s.Put(context.Background(), "ws/db/sha256/ab/abcdef0123", bytes.NewReader(body), int64(len(body)), "text/plain")
	if err == nil {
		t.Fatal("Put through a symlinked first segment succeeded, want error")
	}

	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("Put wrote into the symlink target: %v", entries)
	}
}

// A dangling symlink at a blob's own path is not "not yet created": Put must
// refuse it rather than silently replacing it, and Open must refuse it
// rather than reporting ErrNotFound as if nothing were there.
func TestLocalRejectsDanglingSymlinkAtBlobPath(t *testing.T) {
	root := t.TempDir()
	key := "ws/db/sha256/ab/abcdef0123"
	dir := filepath.Join(root, "ws", "db", "sha256", "ab")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "abcdef0123")
	if err := os.Symlink(filepath.Join(dir, "does-not-exist"), link); err != nil {
		t.Fatal(err)
	}

	s := NewLocal(root)
	body := []byte("hello")
	if err := s.Put(context.Background(), key, bytes.NewReader(body), int64(len(body)), "text/plain"); err == nil {
		t.Fatal("Put through a dangling symlink succeeded, want error")
	}
	if _, err := s.Open(context.Background(), key); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Open on a dangling symlink: err = %v, want a non-ErrNotFound error", err)
	}

	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the dangling symlink was replaced instead of rejected: %v, %v", fi, err)
	}
}
