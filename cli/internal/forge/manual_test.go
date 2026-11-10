package forge

import (
	"encoding/json"
	"reflect"
	"testing"
)

func manualConfig(t *testing.T, targets any) Config {
	t.Helper()
	c := testConfig(t)
	c.Start, c.End = "2025-06-01", "2025-06-04"
	c.Max = 20
	c.Weekends.Mode = "off"
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	fields["manual"] = map[string]any{"targets": targets}
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := Decode(data, &c); err != nil {
		t.Fatalf("decode manual drawing: %v", err)
	}
	return c
}

func TestManualCalendarUsesExactTargetsAndPreservesExisting(t *testing.T) {
	t.Parallel()
	c := manualConfig(t, map[string]int{"2025-06-01": 7, "2025-06-02": 0, "2025-06-03": 10})
	s := Snapshot{Known: true, Start: c.Start, End: c.End, Days: map[string]Contribution{
		"2025-06-01": {Count: 4, Weekday: 0}, "2025-06-02": {Count: 12, Weekday: 1},
		"2025-06-03": {Count: 0, Weekday: 2}, "2025-06-04": {Count: 0, Weekday: 3},
	}}
	p, err := Generate(c, s)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{3, 0, 10, 0} {
		if p.Days[i].New != want {
			t.Fatalf("%s: want %d new, got %d", p.Days[i].Date, want, p.Days[i].New)
		}
	}
	if p.Days[1].Projected != 12 || p.Stats.Planned != 13 {
		t.Fatalf("incorrect merge: %+v", p.Days)
	}
	if err := ValidatePlan(p); err != nil {
		t.Fatal(err)
	}
	again, err := Generate(c, s)
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatal("manual generation is not deterministic", err)
	}
	p.Days[0].Target++
	p.ID, err = PlanID(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePlan(p); err == nil {
		t.Fatal("accepted target inconsistent with manual drawing")
	}
}

func TestEmptyManualDrawingRemainsBlank(t *testing.T) {
	t.Parallel()
	c := manualConfig(t, map[string]int{})
	p, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Stats.Planned != 0 {
		t.Fatalf("blank drawing generated %d commits", p.Stats.Planned)
	}
	data, err := json.Marshal(p.Config)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["manual"] == nil {
		t.Fatal("empty drawing serialized as automatic mode")
	}
}

func TestManualCalendarRejectsInvalidTargets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		targets any
	}{
		{"negative", map[string]int{"2025-06-01": -1}},
		{"over-limit", map[string]int{"2025-06-01": 21}},
		{"outside-period", map[string]int{"2025-05-31": 1}},
		{"invalid-date", map[string]int{"2025-06-99": 1}},
		{"missing-targets", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := manualConfig(t, tc.targets)
			if _, err := Generate(c, Snapshot{}); err == nil {
				t.Fatal("invalid manual schedule accepted")
			}
		})
	}
}
