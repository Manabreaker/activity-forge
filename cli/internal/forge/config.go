package forge

import (
	"fmt"
	"math"
	"strings"
	"time"
	_ "time/tzdata"
)

const Version = 1
const MaxDays = 36600
const MaxCommits = 200000

type Identity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type WeekendDay struct {
	Probability float64 `json:"probability"`
	Multiplier  float64 `json:"multiplier"`
	Max         int     `json:"max"`
}

type Weekends struct {
	Mode     string     `json:"mode"`
	Saturday WeekendDay `json:"saturday"`
	Sunday   WeekendDay `json:"sunday"`
}

type Effects struct {
	WorkBursts       bool   `json:"work_bursts"`
	RestPeriods      bool   `json:"rest_periods"`
	VacationCount    int    `json:"vacation_count"`
	VacationMin      int    `json:"vacation_min"`
	VacationMax      int    `json:"vacation_max"`
	Streaks          bool   `json:"streaks"`
	MonthlyVariation bool   `json:"monthly_variation"`
	Trend            string `json:"trend"`
}

type Clock struct {
	Start              string  `json:"start"`
	End                string  `json:"end"`
	EveningProbability float64 `json:"evening_probability"`
	NightProbability   float64 `json:"night_probability"`
	Spread             int     `json:"spread_minutes"`
}

type Config struct {
	Start           string   `json:"start"`
	End             string   `json:"end"`
	Timezone        string   `json:"timezone"`
	AsOf            string   `json:"as_of"`
	Preset          string   `json:"preset"`
	Intensity       int      `json:"intensity"`
	Mean            float64  `json:"mean"`
	Min             int      `json:"min"`
	Max             int      `json:"max"`
	InactivePercent float64  `json:"inactive_percent"`
	Randomness      float64  `json:"randomness"`
	Seed            int64    `json:"seed"`
	Weekends        Weekends `json:"weekends"`
	Effects         Effects  `json:"effects"`
	Clock           Clock    `json:"clock"`
	Author          Identity `json:"author"`
}

func DefaultConfig(now time.Time) Config {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("Europe/Moscow", 3*60*60)
	}
	now = now.In(loc)
	c := Config{Start: now.AddDate(0, 0, -364).Format(time.DateOnly), End: now.Format(time.DateOnly),
		Timezone: "Europe/Moscow", AsOf: now.Format(time.RFC3339), Seed: 42,
		Clock: Clock{Start: "09:00", End: "18:00", EveningProbability: .12, NightProbability: .025, Spread: 35}}
	return ApplyPreset(c, "balanced")
}

func ApplyPreset(c Config, name string) Config {
	c.Preset = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
	if c.Preset == "active" {
		c.Preset = "active-developer"
	}
	if c.Preset == "custom" {
		return c
	}
	c.Mean, c.Min, c.Max = 0, 1, 18
	c.Intensity, c.Randomness, c.InactivePercent = 5, 45, 18
	c.Weekends = Weekends{Mode: "rare", Saturday: WeekendDay{.45, .4, 6}, Sunday: WeekendDay{.25, .25, 4}}
	c.Effects = Effects{WorkBursts: true, RestPeriods: true, VacationCount: 1, VacationMin: 5,
		VacationMax: 12, Streaks: true, MonthlyVariation: true, Trend: "stable"}
	switch c.Preset {
	case "active-developer":
		c.Intensity, c.Randomness, c.InactivePercent, c.Weekends.Mode = 7, 60, 7, "light"
	case "hardcore":
		c.Intensity, c.Randomness, c.InactivePercent, c.Max, c.Weekends.Mode = 10, 55, 3, 35, "moderate"
	case "chaotic":
		c.Intensity, c.Randomness, c.InactivePercent, c.Max, c.Weekends.Mode = 7, 95, 28, 30, "light"
	}
	return c
}

func (c Config) Bounds() (time.Time, time.Time, *time.Location, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, nil, fmt.Errorf("invalid timezone: %w", err)
	}
	a, b, err := civilRange(c.Start, c.End)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	start, err := dayStart(a, loc)
	if err != nil {
		return start, time.Time{}, nil, err
	}
	end, err := dayStart(b, loc)
	return start, end, loc, err
}

func (c Config) Validate() error {
	start, end, loc, err := c.Bounds()
	if err != nil {
		return err
	}
	cutoff, err := time.Parse(time.RFC3339, c.AsOf)
	if err != nil {
		return fmt.Errorf("invalid as_of: %w", err)
	}
	if cutoff.After(time.Now()) {
		return fmt.Errorf("as_of cannot be in the future")
	}
	if start.Year() < 1970 || end.Before(start) || end.Format(time.DateOnly) > cutoff.In(loc).Format(time.DateOnly) {
		return fmt.Errorf("dates must be ordered, at or after 1970, and no later than as_of in %s", c.Timezone)
	}
	if end.Sub(start).Hours()/24 > MaxDays-1 {
		return fmt.Errorf("period exceeds %d days", MaxDays)
	}
	if err := c.validateCalendar(loc); err != nil {
		return err
	}
	if c.Intensity < 1 || c.Intensity > 10 || c.Min < 1 || c.Max < c.Min || c.Max > 500 {
		return fmt.Errorf("intensity must be 1–10 and 1 <= min <= max <= 500")
	}
	if !finiteRange(c.Randomness, 0, 100) || !finiteRange(c.InactivePercent, 0, 100) || !finiteRange(c.Mean, 0, 500) {
		return fmt.Errorf("randomness/inactive percent must be 0–100 and mean 0–500")
	}
	if !oneOf(c.Preset, "balanced", "active-developer", "hardcore", "chaotic", "custom") {
		return fmt.Errorf("unknown preset %q", c.Preset)
	}
	if err := c.validateEffects(); err != nil {
		return err
	}
	if err := c.validateClock(); err != nil {
		return err
	}
	return c.validateWeekends()
}

func (c Config) validateEffects() error {
	e := c.Effects
	if e.VacationCount < 0 || e.VacationCount > 50 {
		return fmt.Errorf("vacation_count must be 0–50")
	}
	if e.VacationCount > 0 && (e.VacationMin < 1 || e.VacationMax < e.VacationMin || e.VacationMax > 365) {
		return fmt.Errorf("vacation duration must satisfy 1 <= min <= max <= 365")
	}
	if !oneOf(e.Trend, "", "stable", "increasing", "decreasing", "wave", "random") {
		return fmt.Errorf("unknown trend %q", e.Trend)
	}
	return nil
}

func (c Config) validateWeekends() error {
	if !oneOf(c.Weekends.Mode, "off", "rare", "light", "moderate", "same", "custom") {
		return fmt.Errorf("unknown weekend mode %q", c.Weekends.Mode)
	}
	for _, d := range []WeekendDay{c.Weekends.Saturday, c.Weekends.Sunday} {
		if !finiteRange(d.Probability, 0, 1) || !finiteRange(d.Multiplier, 0, 5) || d.Max < 0 || d.Max > 500 {
			return fmt.Errorf("weekend probability must be 0–1, multiplier 0–5, max 0–500")
		}
	}
	return nil
}

func (c Config) validateClock() error {
	a, err := parseMinute(c.Clock.Start)
	if err != nil {
		return err
	}
	b, err := parseMinute(c.Clock.End)
	if err != nil {
		return err
	}
	if a >= b || !finiteRange(c.Clock.EveningProbability, 0, 1) || !finiteRange(c.Clock.NightProbability, 0, 1) || c.Clock.EveningProbability+c.Clock.NightProbability > 1 {
		return fmt.Errorf("clock needs start < end and evening + night probabilities <= 1")
	}
	if c.Clock.Spread < 1 || c.Clock.Spread > 240 {
		return fmt.Errorf("spread_minutes must be 1–240")
	}
	return nil
}

func parseMinute(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("invalid clock time %q; use HH:MM", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func finiteRange(v, lo, hi float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo && v <= hi
}
func oneOf(v string, choices ...string) bool {
	for _, s := range choices {
		if v == s {
			return true
		}
	}
	return false
}
