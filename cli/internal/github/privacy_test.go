package github

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/gitstore"
)

func TestPrivateRequestRejectsPublicRemoteBeforeApproval(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := forge.DefaultConfig(time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC))
	cfg.Start, cfg.End = "2026-01-05", "2026-01-05"
	cfg.Mean, cfg.Min, cfg.Max, cfg.InactivePercent = 1, 1, 1, 0
	cfg.Effects, cfg.Randomness = forge.Effects{}, 0
	cfg.Author = forge.Identity{Name: "Test", Email: "123+forge-test@users.noreply.github.com"}
	p, err := forge.Generate(cfg, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	s := gitstore.Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	if err := s.Generate(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(root, "empty-public.git")
	if out, err := exec.Command("git", "init", "--bare", "--initial-branch=main", bare).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	private := false
	c := Client{Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if name == "gh" && joined == "api user" {
			return []byte(`{"login":"forge-test","id":123}`), nil
		}
		if name == "gh" && joined == "api repos/forge-test/activity" {
			return []byte(fmt.Sprintf(`{"full_name":"forge-test/activity","default_branch":"main","private":%t,"fork":false}`, private)), nil
		}
		if name != "git" {
			return nil, fmt.Errorf("unexpected binary %s", name)
		}
		translated := append([]string(nil), args...)
		for i, arg := range translated {
			if arg == "https://github.com/forge-test/activity.git" {
				translated[i] = bare
			}
		}
		return exec.CommandContext(ctx, "git", translated...).CombinedOutput()
	}}
	if _, err := c.Prepare(context.Background(), s, p, "forge-test/activity", true, false); err == nil {
		t.Fatal("private publication offered approval for an existing public repository")
	}
	private = true
	prep, err := c.Prepare(context.Background(), s, p, "forge-test/activity", true, false)
	if err != nil {
		t.Fatal(err)
	}
	private = false
	if err := c.Push(context.Background(), prep); err == nil {
		t.Fatal("repository becoming public after preview did not invalidate private approval")
	}
	if out, err := exec.Command("git", "-C", bare, "rev-list", "--all", "--count").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "0" {
		t.Fatalf("public repository was modified: %s %v", out, err)
	}
}
