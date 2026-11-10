package forge

import (
	"fmt"
	"maps"
	"math"
	"math/rand"
	"time"
)

// Generate is the only calendar generator. Applying a plan never calls it.
func Generate(c Config, existing Snapshot) (Plan, error) {
	if err := c.Validate(); err != nil {
		return Plan{}, err
	}
	if c.Manual != nil {
		c.Manual = &ManualSchedule{Targets: maps.Clone(c.Manual.Targets)}
	}
	_, _, loc, _ := c.Bounds()
	start, end, _ := civilRange(c.Start, c.End)
	if err := ValidateSnapshot(existing, c.Start, c.End); err != nil {
		return Plan{}, err
	}
	r := rand.New(rand.NewSource(c.Seed))
	dates := make([]time.Time, 0, int(end.Sub(start).Hours()/24)+2)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		local, err := dayStart(d, loc)
		if err != nil {
			return Plan{}, err
		}
		dates = append(dates, local)
	}
	vacation := vacationDays(c, r, len(dates))
	p := Plan{Version: Version, Config: c, Existing: existing, Days: make([]Day, 0, len(dates))}
	process := newProcess(r, c)
	for i, date := range dates {
		var target int
		if c.Manual != nil {
			target = c.Manual.Targets[date.Format(time.DateOnly)]
		} else {
			target = process.count(date, i, len(dates), vacation[i])
		}
		d := Day{Date: date.Format(time.DateOnly), Target: target, Timestamps: []time.Time{}}
		d.Existing = existing.Days[d.Date].Count
		d.New = max(0, target-d.Existing)
		d.Timestamps = timestamps(c, r, date, d.New)
		d.New = len(d.Timestamps)
		d.Projected = d.Existing + d.New
		p.Days = append(p.Days, d)
	}
	p.Stats = Summarize(p.Days)
	if p.Stats.Planned > MaxCommits {
		return Plan{}, fmt.Errorf("plan exceeds %d commits; reduce period or intensity", MaxCommits)
	}
	var err error
	p.ID, err = PlanID(p)
	return p, err
}

type process struct {
	r                                    *rand.Rand
	c                                    Config
	weekly, local, monthA, monthB, trend float64
	week                                 int
	month                                time.Month
	burst, rest                          int
	active                               bool
}

func newProcess(r *rand.Rand, c Config) *process {
	return &process{r: r, c: c, monthA: r.NormFloat64() * .24, monthB: r.NormFloat64() * .24}
}

func (p *process) advance(date time.Time) {
	_, week := date.ISOWeek()
	if p.week != week {
		p.week = week
		p.weekly = .55*p.weekly + .55*p.r.NormFloat64()
	}
	p.local = .84*p.local + .27*p.r.NormFloat64()
	p.trend = .985*p.trend + .045*p.r.NormFloat64()
	if p.month != date.Month() {
		p.month = date.Month()
		p.monthA = p.monthB
		p.monthB = p.r.NormFloat64() * .3
	}
	if p.burst > 0 {
		p.burst--
	}
	if p.rest > 0 {
		p.rest--
	}
	if p.c.Effects.WorkBursts && p.burst == 0 && p.r.Float64() < .035 {
		p.burst = 4 + p.r.Intn(11)
	}
	if p.c.Effects.RestPeriods && p.rest == 0 && p.r.Float64() < .022 {
		p.rest = 1 + p.r.Intn(4)
	}
}

func (p *process) count(date time.Time, i, n int, vacation bool) int {
	p.advance(date)
	w := weekendSettings(p.c, date.Weekday())
	probability := (1 - p.c.InactivePercent/100) * w.Probability
	if p.c.Effects.Streaks && p.active {
		probability += (1 - probability) * .35
	}
	if w.Probability == 0 || w.Multiplier == 0 || w.Max == 0 || vacation || p.rest > 0 {
		probability = 0
	}
	p.active = p.r.Float64() < probability
	if !p.active {
		return 0
	}
	mean := p.c.Mean
	if mean == 0 {
		mean = []float64{0, .9, 1.5, 2.3, 3.2, 4.4, 5.5, 6.9, 8.5, 10.5, 13}[p.c.Intensity]
	}
	a := p.c.Randomness / 100
	noise := .48*p.weekly + .58*p.local + .21*p.r.NormFloat64()
	mult := math.Exp(a*noise - .12*a*a)
	if p.c.Effects.MonthlyVariation {
		mult *= math.Exp(p.monthA + (p.monthB-p.monthA)*float64(date.Day()-1)/31)
	}
	if p.burst > 0 {
		mult *= 1.25 + .9*a
	}
	mult *= p.trendMultiplier(float64(i) / float64(max(1, n-1)))
	count := int(math.Round(mean * w.Multiplier * mult))
	limit := min(p.c.Max, w.Max)
	if limit < p.c.Min {
		p.active = false
		return 0
	}
	return min(limit, max(p.c.Min, count))
}

func (p *process) trendMultiplier(x float64) float64 {
	switch p.c.Effects.Trend {
	case "increasing":
		return .55 + .9*x
	case "decreasing":
		return 1.45 - .9*x
	case "wave":
		return 1 + .4*math.Sin(x*4*math.Pi)
	case "random":
		return math.Exp(p.trend)
	}
	return 1
}

func weekendSettings(c Config, weekday time.Weekday) WeekendDay {
	if weekday != time.Saturday && weekday != time.Sunday {
		return WeekendDay{1, 1, c.Max}
	}
	switch c.Weekends.Mode {
	case "off":
		return WeekendDay{}
	case "rare":
		return WeekendDay{.16, .28, min(c.Max, 3)}
	case "light":
		return WeekendDay{.5, .4, min(c.Max, 7)}
	case "moderate":
		return WeekendDay{.8, .72, c.Max}
	case "custom":
		if weekday == time.Saturday {
			return c.Weekends.Saturday
		}
		return c.Weekends.Sunday
	}
	return WeekendDay{1, 1, c.Max}
}

func vacationDays(c Config, r *rand.Rand, n int) []bool {
	v := make([]bool, n)
	for range c.Effects.VacationCount {
		length := min(n, c.Effects.VacationMin+r.Intn(c.Effects.VacationMax-c.Effects.VacationMin+1))
		start := r.Intn(max(1, n-length+1))
		for j := start; j < start+length; j++ {
			v[j] = true
		}
	}
	return v
}
