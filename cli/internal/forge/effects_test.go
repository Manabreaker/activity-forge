package forge

import (
	"math"
	"testing"
	"time"
)

func TestPresetsRandomnessAndCorrelatedWorkload(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"balanced", "active-developer", "hardcore", "chaotic", "custom"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := ApplyPreset(testConfig(t), name)
			if _, err := Generate(c, Snapshot{}); err != nil {
				t.Fatal(err)
			}
		})
	}
	c := testConfig(t)
	c.Weekends.Mode = "same"
	c.Effects = Effects{}
	c.Mean, c.Max, c.Min, c.InactivePercent = 15, 100, 1, 0
	c.Randomness = 0
	constant, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range constant.Days {
		if d.Target != 15 {
			t.Fatalf("zero randomness unexpectedly fluctuated: %d", d.Target)
		}
	}
	c.Randomness = 100
	varied, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	mean := 0.0
	for _, d := range varied.Days {
		mean += float64(d.Target)
	}
	mean /= float64(len(varied.Days))
	variance, lag := 0.0, 0.0
	for i, d := range varied.Days {
		x := float64(d.Target) - mean
		variance += x * x
		if i > 0 {
			lag += x * (float64(varied.Days[i-1].Target) - mean)
		}
	}
	if variance < 1 || lag/variance < .3 {
		t.Fatalf("daily workload lost correlation: %.3f", lag/math.Max(variance, 1))
	}
}

func TestRestVacationAndCustomWeekendLimits(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.Mean, c.Min, c.Max, c.InactivePercent = 5, 1, 8, 0
	c.Randomness = 0
	c.Effects = Effects{}
	c.Weekends = Weekends{Mode: "custom", Saturday: WeekendDay{1, 1, 2}, Sunday: WeekendDay{0, 1, 8}}
	p, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range p.Days {
		date, _ := time.Parse(time.DateOnly, d.Date)
		if date.Weekday() == 6 && d.New > 2 {
			t.Fatal("Saturday limit exceeded")
		}
		if date.Weekday() == 0 && d.New != 0 {
			t.Fatal("Sunday was active")
		}
	}
	c.Weekends.Mode = "same"
	c.Effects = Effects{VacationCount: 1, VacationMin: 10, VacationMax: 10}
	p, err = Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Stats.InactiveDays != 10 {
		t.Fatalf("vacation should exclude exactly ten days: %d", p.Stats.InactiveDays)
	}
	c.Effects = Effects{RestPeriods: true}
	p, err = Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Stats.InactiveDays == 0 {
		t.Fatal("rest periods had no effect")
	}
}

func TestZeroWeekendMultiplierMeansNoWeekendWork(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.Weekends = Weekends{Mode: "custom", Saturday: WeekendDay{1, 0, 10}, Sunday: WeekendDay{1, 0, 10}}
	p, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range p.Days {
		date, _ := time.Parse(time.DateOnly, d.Date)
		if (date.Weekday() == 0 || date.Weekday() == 6) && d.New != 0 {
			t.Fatalf("zero multiplier generated commits on %s", d.Date)
		}
	}
}
