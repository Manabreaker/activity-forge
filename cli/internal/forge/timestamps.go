package forge

import (
	"math/rand"
	"slices"
	"time"
)

// Sessions are clustered, then sorted by instant. AddDate preserves DST days.
func timestamps(c Config, r *rand.Rand, date time.Time, count int) []time.Time {
	out := make([]time.Time, 0, count)
	if count == 0 {
		return out
	}
	cutoff, _ := time.Parse(time.RFC3339, c.AsOf)
	civil := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	var next time.Time
	for range 7 {
		candidate, err := dayStart(civil, date.Location())
		if err == nil {
			next = candidate
			break
		}
		civil = civil.AddDate(0, 0, 1)
	}
	if next.IsZero() {
		return out
	}
	last := next.Add(-time.Second)
	if cutoff.Before(last) {
		last = cutoff.Truncate(time.Second)
	}
	available := int(last.Sub(date).Seconds()) + 1
	if available <= 0 {
		return out
	}
	count = min(count, available)
	a, _ := parseMinute(c.Clock.Start)
	b, _ := parseMinute(c.Clock.End)
	centers := make([]int, 1+r.Intn(3))
	for i := range centers {
		centers[i] = a + r.Intn(b-a)
	}
	used := make(map[int64]bool, count)
	for range count {
		minute := float64(centers[r.Intn(len(centers))]) + r.NormFloat64()*float64(c.Clock.Spread)
		u := r.Float64()
		if u < c.Clock.NightProbability {
			minute = r.Float64() * 5 * 60
		}
		if u >= c.Clock.NightProbability && u < c.Clock.NightProbability+c.Clock.EveningProbability {
			minute = float64(max(b, 18*60)) + r.Float64()*float64(1440-max(b, 18*60))
		}
		m := max(0, min(1439, int(minute)))
		stamp := time.Date(date.Year(), date.Month(), date.Day(), m/60, m%60, r.Intn(60), 0, date.Location())
		if stamp.Before(date) {
			stamp = date
		}
		if stamp.After(last) {
			stamp = date.Add(time.Duration(r.Intn(available)) * time.Second)
		}
		key := stamp.Unix()
		for used[key] {
			key++
			if key > last.Unix() {
				key = date.Unix()
			}
		}
		used[key] = true
		out = append(out, time.Unix(key, 0).In(date.Location()))
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	return out
}
