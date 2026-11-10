package quickstart

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/github"
)

func TestFriendConfigUsesCurrentAccountAndCurrentYear(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	account := github.Account{Login: "friend-user", ID: 12345}
	c := draftConfig(now, account, 2)
	if c.Author.Name != "friend-user" || c.Author.Email != "12345+friend-user@users.noreply.github.com" {
		t.Fatalf("wrong friend identity: %+v", c.Author)
	}
	if c.Start != "2025-11-11" || c.End != "2026-11-10" {
		t.Fatalf("wrong rolling dates: %s..%s", c.Start, c.End)
	}
	p, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Days) != 365 || p.Stats.Planned == 0 {
		t.Fatalf("empty or incomplete friend plan: %+v", p.Stats)
	}
	for _, day := range p.Days {
		for _, stamp := range day.Timestamps {
			if stamp.After(now) {
				t.Fatalf("future commit: %s", stamp)
			}
		}
	}
}

func TestSavedPlanIsAccountIsolated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	a := github.Account{Login: "first-user", ID: 1}
	b := github.Account{Login: "second-user", ID: 2}
	p, err := forge.Generate(draftConfig(now, a, 2), forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	state := accountState(root, a)
	path := filepath.Join(state, "plans", p.ID+".json")
	if err := forge.SavePlan(path, p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "last-plan"), []byte(path+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selected, _, err := savedPlan(root, a)
	if err != nil || selected.ID != p.ID {
		t.Fatalf("saved selection changed: %s, %v", selected.ID, err)
	}
	if _, _, err := savedPlan(root, b); err == nil {
		t.Fatal("second account inherited the first account's selected plan")
	}
}

func TestMenuEOFExitsWithoutAuthentication(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	launcher := Launcher{In: strings.NewReader(""), Out: &out, Err: &out, Root: t.TempDir()}
	if err := launcher.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
