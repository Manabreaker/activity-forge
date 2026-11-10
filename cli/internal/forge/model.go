package forge

import "time"

type Contribution struct {
	Count   int    `json:"contributionCount"`
	Level   string `json:"contributionLevel"`
	Color   string `json:"color"`
	Weekday int    `json:"weekday"`
}

type Snapshot struct {
	Known              bool                    `json:"known"`
	Login              string                  `json:"login,omitempty"`
	FetchedAt          string                  `json:"fetched_at,omitempty"`
	Start              string                  `json:"start,omitempty"`
	End                string                  `json:"end,omitempty"`
	TotalContributions int                     `json:"totalContributions"`
	Colors             []string                `json:"colors,omitempty"`
	Days               map[string]Contribution `json:"days,omitempty"`
}

type Day struct {
	Date       string      `json:"date"`
	Target     int         `json:"target"`
	Existing   int         `json:"existing"`
	New        int         `json:"new"`
	Projected  int         `json:"projected"`
	Timestamps []time.Time `json:"timestamps"`
}

type Monthly struct {
	Month    string `json:"month"`
	Existing int    `json:"existing"`
	Planned  int    `json:"planned"`
	Total    int    `json:"total"`
}

type Statistics struct {
	Existing       int       `json:"existing"`
	Planned        int       `json:"planned"`
	Total          int       `json:"total"`
	ActiveDays     int       `json:"active_days"`
	InactiveDays   int       `json:"inactive_days"`
	LongestStreak  int       `json:"longest_streak"`
	AverageActive  float64   `json:"average_active"`
	MaxDaily       int       `json:"max_daily"`
	WeekdayPercent float64   `json:"weekday_percent"`
	WeekendPercent float64   `json:"weekend_percent"`
	Monthly        []Monthly `json:"monthly"`
}

type Plan struct {
	Version  int        `json:"version"`
	ID       string     `json:"id"`
	Config   Config     `json:"config"`
	Existing Snapshot   `json:"existing"`
	Days     []Day      `json:"days"`
	Stats    Statistics `json:"statistics"`
}

func Summarize(days []Day) Statistics {
	s := Statistics{Monthly: []Monthly{}}
	weekdays, weekends, activeWeekdays, activeWeekends, streak := 0, 0, 0, 0, 0
	for _, d := range days {
		s.Existing += d.Existing
		s.Planned += d.New
		s.Total += d.Projected
		s.MaxDaily = max(s.MaxDaily, d.Projected)
		date, _ := time.Parse(time.DateOnly, d.Date)
		weekend := date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
		if weekend {
			weekends++
		}
		if !weekend {
			weekdays++
		}
		if d.Projected > 0 {
			s.ActiveDays++
			streak++
			s.LongestStreak = max(s.LongestStreak, streak)
			if weekend {
				activeWeekends++
			}
			if !weekend {
				activeWeekdays++
			}
		}
		if d.Projected == 0 {
			s.InactiveDays++
			streak = 0
		}
		month := d.Date[:7]
		if len(s.Monthly) == 0 || s.Monthly[len(s.Monthly)-1].Month != month {
			s.Monthly = append(s.Monthly, Monthly{Month: month})
		}
		m := &s.Monthly[len(s.Monthly)-1]
		m.Existing += d.Existing
		m.Planned += d.New
		m.Total += d.Projected
	}
	if s.ActiveDays > 0 {
		s.AverageActive = float64(s.Total) / float64(s.ActiveDays)
	}
	if weekdays > 0 {
		s.WeekdayPercent = 100 * float64(activeWeekdays) / float64(weekdays)
	}
	if weekends > 0 {
		s.WeekendPercent = 100 * float64(activeWeekends) / float64(weekends)
	}
	return s
}

func Rebase(p Plan, s Snapshot) Plan {
	p.Existing = s
	p.Days = append([]Day(nil), p.Days...)
	for i := range p.Days {
		p.Days[i].Existing = s.Days[p.Days[i].Date].Count
		p.Days[i].Projected = p.Days[i].Existing + p.Days[i].New
	}
	p.Stats = Summarize(p.Days)
	return p
}
