package browser

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

func TestBrowserUsesOriginalGenerator(t *testing.T) {
	t.Parallel()
	now := time.Date(2025, 6, 20, 18, 0, 0, 0, time.UTC)
	c := forge.DefaultConfig(now)
	c.End, c.Start = "2025-06-19", "2025-05-20"
	input, err := json.Marshal(Request{Action: "generate", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	output := Evaluate(string(input))
	var result Response
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != "" || result.Plan == nil {
		t.Fatalf("missing plan: %s", output)
	}
	want, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.ID != want.ID {
		t.Fatalf("browser %s != original %s", result.Plan.ID, want.ID)
	}
	var p forge.Plan
	if err := json.Unmarshal([]byte(result.Payload), &p); err != nil {
		t.Fatal(err)
	}
	p.ID = result.Plan.ID
	if err := forge.ValidatePlan(p); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserRejectsUnboundedInput(t *testing.T) {
	t.Parallel()
	c := forge.DefaultConfig(time.Now().Add(-time.Hour))
	c.Start = "2000-01-01"
	input, err := json.Marshal(Request{Action: "generate", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	var result Response
	if err := json.Unmarshal([]byte(Evaluate(string(input))), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Fatal("unbounded browser generation accepted")
	}
}
