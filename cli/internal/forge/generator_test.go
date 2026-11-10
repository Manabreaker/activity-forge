package forge

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	c := DefaultConfig(time.Date(2026, 11, 10, 6, 0, 0, 0, time.UTC))
	c.Start, c.End = "2025-11-11", "2026-11-10"
	c.Seed = 42
	return c
}

func TestDeterministicPlanAndSeed(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	a, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same config and seed changed plan")
	}
	if len(a.Days) != 365 {
		t.Fatalf("got %d days", len(a.Days))
	}
	c.Seed++
	d, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == d.ID || reflect.DeepEqual(a.Days, d.Days) {
		t.Fatal("seed did not change calendar")
	}
}

func TestExistingContributionsAreSubtractedAndPreserved(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.Start, c.End = "2026-11-02", "2026-11-03"
	c.Mean, c.Max, c.Min, c.InactivePercent = 7, 7, 7, 0
	c.Randomness, c.Effects = 0, Effects{}
	c.Weekends.Mode = "same"
	s := Snapshot{Known: true, Start: c.Start, End: c.End, Days: map[string]Contribution{
		"2026-11-02": {Count: 4, Level: "SECOND_QUARTILE", Color: "#006d32", Weekday: 1},
		"2026-11-03": {Count: 12, Level: "FOURTH_QUARTILE", Color: "#39d353", Weekday: 2},
	}}
	p, err := Generate(c, s)
	if err != nil {
		t.Fatal(err)
	}
	if p.Days[0].New != 3 || p.Days[0].Projected != 7 || p.Days[1].New != 0 || p.Days[1].Projected != 12 {
		t.Fatalf("incorrect merge: %+v", p.Days)
	}
	if p.Stats.Existing != 16 || p.Stats.Planned != 3 || p.Stats.Total != 19 {
		t.Fatalf("incorrect stats: %+v", p.Stats)
	}
}

func TestLimitsWeekendsAndChronologicalTimestamps(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.Max, c.Min, c.Mean = 5, 2, 20
	c.Weekends.Mode = "off"
	p, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	cutoff, _ := time.Parse(time.RFC3339, c.AsOf)
	var previous time.Time
	for _, d := range p.Days {
		date, _ := time.ParseInLocation(time.DateOnly, d.Date, loc)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			if d.New != 0 {
				t.Fatalf("weekend %s has %d", d.Date, d.New)
			}
		}
		if d.New > 5 || (d.New > 0 && d.New < 2) {
			t.Fatalf("limits violated: %+v", d)
		}
		for _, stamp := range d.Timestamps {
			if stamp.In(loc).Format(time.DateOnly) != d.Date || stamp.After(cutoff) || (!previous.IsZero() && !stamp.After(previous)) {
				t.Fatalf("invalid timestamp: %s after %s", stamp, previous)
			}
			previous = stamp
		}
	}
}

func TestInvalidInputsAndFutureDates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit func(*Config)
	}{
		{"future", func(c *Config) { c.End = "2026-11-11" }},
		{"reversed", func(c *Config) { c.Start = "2026-11-10"; c.End = "2026-11-01" }},
		{"bad-zone", func(c *Config) { c.Timezone = "Mars/Nope" }},
		{"randomness", func(c *Config) { c.Randomness = 101 }},
		{"minimum", func(c *Config) { c.Min = c.Max + 1 }},
		{"evening", func(c *Config) { c.Clock.EveningProbability = 0.8; c.Clock.NightProbability = 0.8 }},
		{"negative-vacations", func(c *Config) { c.Effects.VacationCount = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := testConfig(t)
			tc.edit(&c)
			if _, err := Generate(c, Snapshot{}); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestDSTAndCurrentDayDoNotCreateFutureInstants(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.Timezone = "America/New_York"
	c.Start, c.End = "2026-10-31", "2026-11-02"
	c.AsOf = "2026-11-02T05:00:05Z"
	c.Mean, c.Max, c.Min, c.InactivePercent = 4, 4, 4, 0
	c.Randomness, c.Effects, c.Weekends.Mode = 0, Effects{}, "same"
	p, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	cutoff, _ := time.Parse(time.RFC3339, c.AsOf)
	var prev time.Time
	for _, d := range p.Days {
		for _, stamp := range d.Timestamps {
			if !prev.IsZero() && !stamp.After(prev) {
				t.Fatal("timestamps out of order at DST")
			}
			if stamp.After(cutoff) {
				t.Fatal("future timestamp")
			}
			prev = stamp
		}
	}
}

func TestPlanChecksumAndImmutableSave(t *testing.T) {
	t.Parallel()
	p, err := Generate(testConfig(t), Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/plan.json"
	if err := SavePlan(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(loaded)
	if string(a) != string(b) {
		t.Fatal("roundtrip changed plan")
	}
	if err := SavePlan(path, p); err != nil {
		t.Fatalf("same plan save should be idempotent: %v", err)
	}
	p.Days[0].Target++
	if err := ValidatePlan(p); err == nil {
		t.Fatal("tampered plan accepted")
	}
	if err := SavePlan(path, p); err == nil {
		t.Fatal("tampered overwrite accepted")
	}
}

func TestRejectsNonexistentCivilDateBeforeGeneration(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.Start, c.End, c.Timezone = "2011-12-29", "2011-12-31", "Pacific/Apia"
	if err := c.Validate(); err == nil {
		t.Fatal("nonexistent civil date accepted; generator would fail to advance")
	}
}

func TestRejectsFutureCutoffEvenWithinOneSecond(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.AsOf = time.Now().Add(2 * time.Second).Format(time.RFC3339)
	if err := c.Validate(); err == nil {
		t.Fatal("future cutoff accepted")
	}
}
