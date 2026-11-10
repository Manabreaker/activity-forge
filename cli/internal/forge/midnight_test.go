package forge

import (
	"testing"
	"time"
)

func TestMidnightDSTStillGeneratesEveryCivilDay(t *testing.T) {
	t.Parallel()
	c := testConfig(t)
	c.Start, c.End, c.Timezone = "2018-11-03", "2018-11-05", "America/Sao_Paulo"
	c.Effects, c.Randomness, c.Weekends.Mode = Effects{}, 0, "same"
	c.Min, c.Max, c.Mean, c.InactivePercent = 3, 3, 3, 0
	p, err := Generate(c, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Days) != 3 || p.Stats.Planned != 9 {
		t.Fatalf("midnight DST lost days: %+v", p.Stats)
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range p.Days {
		for _, stamp := range d.Timestamps {
			if stamp.In(loc).Format(time.DateOnly) != d.Date {
				t.Fatalf("wrong civil date: %s", stamp)
			}
		}
	}
	if err := ValidatePlan(p); err != nil {
		t.Fatal(err)
	}
}
