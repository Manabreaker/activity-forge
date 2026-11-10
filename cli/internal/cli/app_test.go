package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

func testApp(input string) (*App, *bytes.Buffer) {
	out := new(bytes.Buffer)
	return &App{In: strings.NewReader(input), Out: out, Err: out, Now: func() time.Time { return time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC) }}, out
}

func TestCLIPlanGenerateInspectAndConfigPriority(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	path := filepath.Join(root, "chosen.json")
	a, out := testApp("")
	args := []string{"plan", "--state-dir", state, "--start", "2026-01-05", "--end", "2026-01-07", "--preset", "active-developer", "--min", "2", "--max", "2", "--mean", "2", "--inactive", "0", "--randomness", "0", "--bursts=false", "--rest=false", "--vacations", "0", "--monthly=false", "--streaks=false", "--author-name", "CLI Test", "--author-email", "test@example.invalid", "--output", path}
	if err := a.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	p, err := forge.LoadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Config.Randomness != 0 || p.Config.Preset != "active-developer" || p.Stats.Planned != 6 {
		t.Fatalf("flags did not override preset: %+v", p.Config)
	}
	for _, cmd := range []string{"generate", "inspect", "status"} {
		if err = a.Run(context.Background(), []string{cmd, "--state-dir", state, "--plan", path}); err != nil {
			t.Fatalf("%s: %v\n%s", cmd, err, out)
		}
	}
	if !strings.Contains(out.String(), "VERIFIED") {
		t.Fatalf("inspection not reported: %s", out)
	}
	if _, err = os.Stat(filepath.Join(state, "config.json")); err != nil {
		t.Fatal("configuration was not persisted")
	}
	if err = a.Run(context.Background(), []string{"generate", "--state-dir", state, "--plan", path, "--seed", "7"}); err == nil {
		t.Fatal("generate silently changed a saved plan")
	}
}

func TestScriptedWizardProducesAndSavesPreview(t *testing.T) {
	t.Parallel()
	state := filepath.Join(t.TempDir(), "state")
	// Period, preset, intensity, advanced daily, randomness, weekends,
	// realism, timezone, clock, author, email; then save and cancel.
	input := "30\nactive-developer\n7\nn\n60\nlight\nn\nEurope/Moscow\nn\n\n\n9\n0\n"
	a, out := testApp(input)
	if err := a.Run(context.Background(), []string{"--state-dir", state, "--offline"}); err != nil {
		t.Fatalf("wizard: %v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "Activity Preview") || !strings.Contains(out.String(), "Saved immutable plan") {
		t.Fatalf("wizard did not complete: %s", out)
	}
	data, err := os.ReadFile(filepath.Join(state, "last-plan"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := forge.LoadPlan(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Days) != 30 {
		t.Fatalf("wizard selected %d days", len(p.Days))
	}
}

func TestJSONExportToStdoutIsMachineReadable(t *testing.T) {
	t.Parallel()
	a, out := testApp("")
	if err := a.Run(context.Background(), []string{"preview", "--state-dir", t.TempDir(), "--format", "json", "--offline"}); err != nil {
		t.Fatal(err)
	}
	var p forge.Plan
	if err := forge.Decode(out.Bytes(), &p); err != nil {
		t.Fatalf("stdout contains non-JSON output: %v", err)
	}
	if err := forge.ValidatePlan(p); err != nil {
		t.Fatal(err)
	}
}

func TestChosenSnapshotMissingIsAnError(t *testing.T) {
	t.Parallel()
	a, _ := testApp("")
	root := t.TempDir()
	if err := a.Run(context.Background(), []string{"preview", "--state-dir", root, "--existing", filepath.Join(root, "missing.json"), "--offline"}); err == nil {
		t.Fatal("missing explicit snapshot silently became an unknown baseline")
	}
}
