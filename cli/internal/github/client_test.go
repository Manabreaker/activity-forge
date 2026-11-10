package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/gitstore"
)

func TestFetchCalendarAndVerifiedIdentity(t *testing.T) {
	t.Parallel()
	c := Client{Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if name != "gh" {
			return nil, fmt.Errorf("unexpected binary %s", name)
		}
		switch {
		case joined == "api user":
			return []byte(`{"login":"forge-test","id":123}`), nil
		case joined == "api user/emails":
			return []byte(`[{"email":"verified@example.invalid","verified":true}]`), nil
		case strings.HasPrefix(joined, "api graphql "):
			return []byte(`{"data":{"user":{"contributionsCollection":{"contributionCalendar":{"totalContributions":7,"colors":["#0e4429","#006d32","#26a641","#39d353"],"weeks":[{"contributionDays":[{"date":"2026-01-05","weekday":1,"contributionCount":3,"contributionLevel":"SECOND_QUARTILE","color":"#006d32"},{"date":"2026-01-06","weekday":2,"contributionCount":4,"contributionLevel":"THIRD_QUARTILE","color":"#26a641"}]}]}}}}}`), nil
		}
		return nil, fmt.Errorf("unexpected command %s", joined)
	}}
	s, err := c.Fetch(context.Background(), "2026-01-05", "2026-01-06", "")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Known || s.TotalContributions != 7 || s.Days["2026-01-06"].Color != "#26a641" {
		t.Fatalf("calendar lost API data: %+v", s)
	}
	for _, email := range []string{"verified@example.invalid", "123+forge-test@users.noreply.github.com", "forge-test@users.noreply.github.com"} {
		if _, err = c.VerifyEmail(context.Background(), email); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = c.VerifyEmail(context.Background(), "someone-else@users.noreply.github.com"); err == nil {
		t.Fatal("foreign noreply identity accepted")
	}
}

func TestPublishRequiresVerifiedHeadAndDoesNotPushTwice(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	if out, err := exec.Command("git", "init", "--bare", "--initial-branch=main", bare).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	cfg := forge.DefaultConfig(time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC))
	cfg.Start, cfg.End = "2026-01-05", "2026-01-05"
	cfg.Mean, cfg.Min, cfg.Max, cfg.InactivePercent = 2, 2, 2, 0
	cfg.Effects, cfg.Randomness = forge.Effects{}, 0
	cfg.Author = forge.Identity{Name: "Test", Email: "123+forge-test@users.noreply.github.com"}
	p, err := forge.Generate(cfg, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	s := gitstore.Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	if err = s.Generate(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	pushes := 0
	c := Client{Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			if strings.Join(args, " ") == "api user" {
				return []byte(`{"login":"forge-test","id":123}`), nil
			}
			if len(args) > 1 && args[1] == "repos/forge-test/activity" {
				return []byte(`{"full_name":"forge-test/activity","default_branch":"main","private":true,"fork":false}`), nil
			}
			return nil, fmt.Errorf("unexpected gh command %v", args)
		}
		if name != "git" {
			return nil, fmt.Errorf("unexpected binary")
		}
		translated := append([]string(nil), args...)
		for i, v := range translated {
			if v == "https://github.com/forge-test/activity.git" {
				translated[i] = bare
			}
			if v == "push" {
				mu.Lock()
				pushes++
				mu.Unlock()
			}
		}
		return exec.CommandContext(ctx, "git", translated...).CombinedOutput()
	}}
	prep, err := c.Prepare(context.Background(), s, p, "forge-test/activity", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if pushes != 0 || prep.Pending != 2 {
		t.Fatalf("preparation mutated remote or wrong count: %+v", prep)
	}
	bad := prep
	bad.Head = strings.Repeat("0", 40)
	if err = c.Push(context.Background(), bad); err == nil {
		t.Fatal("stale head approval accepted")
	}
	if err = c.Push(context.Background(), prep); err != nil {
		t.Fatal(err)
	}
	second, err := c.Prepare(context.Background(), s, p, "forge-test/activity", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Pending != 0 {
		t.Fatalf("published commits counted twice: %+v", second)
	}
	if err = c.Push(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if pushes != 1 {
		t.Fatalf("expected one actual push, got %d", pushes)
	}
	data, _ := json.Marshal(second.Display)
	if !strings.Contains(string(data), "\"new\":0") {
		t.Fatal("published commits still projected as new")
	}
}
