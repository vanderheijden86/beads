package blobstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// runContract checks the behaviour bd attachment relies on. Each backend's test
// calls it with a fresh, empty store.
func runContract(t *testing.T, s Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	key := "ws/db/sha256/ab/abcdef0123"
	body := []byte("hello attachment")

	if _, err := s.Stat(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat on empty store: err = %v, want ErrNotFound", err)
	}
	for i := 0; i < 2; i++ { // second Put is the idempotent retry
		if err := s.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
			t.Fatalf("Put #%d: %v", i+1, err)
		}
	}
	info, err := s.Stat(ctx, key)
	if err != nil || info.Size != int64(len(body)) {
		t.Fatalf("Stat = %+v, %v", info, err)
	}
	rc, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("Open read %q", got)
	}
	var listed []string
	if err := s.List(ctx, "ws/db/", func(i Info) error { listed = append(listed, i.Key); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0] != key {
		t.Fatalf("List = %v", listed)
	}
	if u, err := s.URL(ctx, key, time.Minute, "hello.txt"); err != nil || u == "" {
		t.Fatalf("URL = %q, %v", u, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after Delete: err = %v, want ErrNotFound", err)
	}
}
