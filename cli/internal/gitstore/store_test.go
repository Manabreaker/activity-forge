package gitstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

func smallPlan(t *testing.T) forge.Plan {
	t.Helper()
	c := forge.DefaultConfig(time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC))
	c.Start, c.End, c.Seed = "2026-01-05", "2026-01-07", 8
	c.Mean, c.Min, c.Max, c.InactivePercent = 3, 3, 3, 0
	c.Effects, c.Randomness, c.Weekends.Mode = forge.Effects{}, 0, "same"
	c.Author = forge.Identity{Name: "Forge Test", Email: "forge@example.invalid"}
	p, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGenerateRealLogRepeatAndCrashRecovery(t *testing.T) {
	t.Parallel()
	p := smallPlan(t)
	root := t.TempDir()
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	ctx := context.Background()
	if err := s.Generate(ctx, p); err != nil {
		t.Fatal(err)
	}
	r, err := s.Inspect(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || r.Actual != 9 || len(r.Issues) != 0 {
		t.Fatalf("verification failed: %+v", r)
	}
	if err := s.Generate(ctx, p); err != nil {
		t.Fatal(err)
	}
	after, err := s.Inspect(ctx, p)
	if err != nil || after.Head != r.Head {
		t.Fatalf("repeat added commits: %+v %v", after, err)
	}
	// A crash can lose the manifest after advancing the Git ref.
	if err := os.Remove(s.ManifestPath(p.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.Generate(ctx, p); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	after, err = s.Inspect(ctx, p)
	if err != nil || after.Head != r.Head || after.Actual != 9 {
		t.Fatalf("recovery duplicated history: %+v %v", after, err)
	}
	for i, stamp := range r.Timestamps {
		if !stamp.Equal(p.Days[i/3].Timestamps[i%3]) {
			t.Fatalf("timestamp %d differs", i)
		}
	}
}

func TestRefusesExistingRepositoryAndOverlap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	foreign := filepath.Join(root, "foreign")
	if out, err := exec.Command("git", "init", foreign).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	s := Store{Repo: foreign, State: filepath.Join(root, "state")}
	if err := s.Generate(context.Background(), smallPlan(t)); err == nil {
		t.Fatal("foreign repository accepted")
	}
	s.Repo = filepath.Join(root, "dedicated")
	p := smallPlan(t)
	if err := s.Generate(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p.Config.Seed++
	other, err := forge.Generate(p.Config, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Generate(context.Background(), other); err == nil {
		t.Fatal("overlapping plan accepted")
	}
}

func TestInspectDetectsForeignCommitAndIdentityTampering(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := smallPlan(t)
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	if err := s.Generate(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", s.Repo, "-c", "user.name=Intruder", "-c", "user.email=intruder@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "foreign")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	r, err := s.Inspect(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if r.OK || r.Actual != 9 || !strings.Contains(strings.Join(r.Issues, " "), "foreign") {
		t.Fatalf("foreign commit not detected: %+v", r)
	}
}

func TestCancellationCanResumeExactRemainingCommits(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := smallPlan(t)
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Generate(ctx, p); err == nil {
		t.Fatal("cancellation ignored")
	}
	if err := s.Generate(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	r, err := s.Inspect(context.Background(), p)
	if err != nil || !r.OK || r.Actual != 9 {
		t.Fatalf("resume failed: %+v %v", r, err)
	}
}

func TestPartialCommitSequenceResumesAfterCancellation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := smallPlan(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state"), Progress: func(done, total int) {
		if done == 3 {
			cancel()
		}
	}}
	if err := s.Generate(ctx, p); err == nil {
		t.Fatal("expected interrupted generation")
	}
	s.Progress = nil
	r, err := s.Inspect(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Actual != 3 || !r.HistoryValid || r.OK {
		t.Fatalf("wrong partial state: %+v", r)
	}
	if err = s.Generate(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	r, err = s.Inspect(context.Background(), p)
	if err != nil || !r.OK || r.Actual != 9 {
		t.Fatalf("resume: %+v %v", r, err)
	}
}

func TestInspectRejectsUnexpectedTreeEvenWithMatchingCommitMetadata(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := smallPlan(t)
	ctx := context.Background()
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	if err := s.Generate(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Repo, "unexpected.txt"), []byte("unrelated content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.command(ctx, "", nil, "add", "unexpected.txt"); err != nil {
		t.Fatal(err)
	}
	stamp := p.Days[2].Timestamps[2].Format(time.RFC3339)
	env := []string{"GIT_COMMITTER_NAME=" + p.Config.Author.Name, "GIT_COMMITTER_EMAIL=" + p.Config.Author.Email, "GIT_COMMITTER_DATE=" + stamp}
	if _, err := s.command(ctx, "", env, "commit", "--amend", "--no-edit"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Inspect(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.OK || !strings.Contains(strings.Join(r.Issues, " "), "tree") {
		t.Fatalf("unexpected files accepted: %+v", r)
	}
}

func TestRejectedLaterPlanCannotPoisonInterruptedPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := smallPlan(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state"), Progress: func(done, total int) {
		if done == 3 {
			cancel()
		}
	}}
	if err := s.Generate(ctx, p); err == nil {
		t.Fatal("interruption expected")
	}
	s.Progress = nil
	c := p.Config
	c.Start, c.End = "2026-01-08", "2026-01-08"
	later, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Generate(context.Background(), later); err == nil {
		t.Fatal("later plan accepted before interrupted plan completed")
	}
	if err = s.Generate(context.Background(), p); err != nil {
		t.Fatalf("rejected operation poisoned original recovery: %v", err)
	}
}

func TestRejectsBackwardsInstantsAcrossDifferentTimezones(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := smallPlan(t)
	c := p.Config
	c.Start, c.End = "2026-01-05", "2026-01-05"
	c.Timezone = "Pacific/Honolulu"
	c.Clock.Start, c.Clock.End = "23:00", "23:01"
	c.Clock.EveningProbability, c.Clock.NightProbability = 0, 0
	a, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	s := Store{Repo: filepath.Join(root, "history"), State: filepath.Join(root, "state")}
	ctx := context.Background()
	if err = s.Generate(ctx, a); err != nil {
		t.Fatal(err)
	}
	old, err := s.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c.Start, c.End, c.Timezone = "2026-01-06", "2026-01-06", "Pacific/Kiritimati"
	c.Clock.Start, c.Clock.End = "00:00", "00:01"
	b, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Generate(ctx, b); err == nil {
		t.Fatal("backwards timestamps appended")
	}
	head, err := s.Head(ctx)
	if err != nil || head != old {
		t.Fatal("rejected generation changed HEAD")
	}
	r, err := s.Inspect(ctx, a)
	if err != nil || !r.OK {
		t.Fatalf("original history poisoned: %+v %v", r, err)
	}
}

func TestSymlinkAncestorCannotBypassForeignWorktreeGuard(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	foreign := filepath.Join(root, "foreign")
	sub := filepath.Join(foreign, "subdir")
	if out, err := exec.Command("git", "init", foreign).CombinedOutput(); err != nil {
		t.Fatalf("%s %v", out, err)
	}
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(sub, alias); err != nil {
		t.Fatal(err)
	}
	s := Store{Repo: filepath.Join(alias, "history"), State: filepath.Join(root, "state")}
	if err := s.Generate(context.Background(), smallPlan(t)); err == nil {
		t.Fatal("foreign worktree mutated through symlink ancestor")
	}
	if _, err := os.Stat(filepath.Join(sub, "history")); !os.IsNotExist(err) {
		t.Fatal("rejected operation created history inside foreign worktree")
	}
}
