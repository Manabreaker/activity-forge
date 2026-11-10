package gitstore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRepositoryLockRejectsConcurrentOwnerAndReleases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	ctx := context.Background()
	if err := s.WithLock(ctx, func(locked Store) error {
		called := false
		err := locked.WithLock(ctx, func(Store) error { called = true; return nil })
		if err == nil || called {
			t.Fatal("concurrent operation acquired the same repository")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.WithLock(ctx, func(Store) error { return nil }); err != nil {
		t.Fatalf("lock remained held after operation: %v", err)
	}
}
