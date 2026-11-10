package forge

import (
	"fmt"
	"time"
)

// Civil dates advance in UTC, independent of timezone jumps and skipped days.
func civilRange(start, end string) (time.Time, time.Time, error) {
	a, err := time.Parse(time.DateOnly, start)
	if err != nil {
		return a, time.Time{}, fmt.Errorf("invalid start date: %w", err)
	}
	b, err := time.Parse(time.DateOnly, end)
	if err != nil {
		return a, b, fmt.Errorf("invalid end date: %w", err)
	}
	return a, b, nil
}

// dayStart finds the first real second of a civil date, including midnight DST.
func dayStart(civil time.Time, loc *time.Location) (time.Time, error) {
	want := civil.Format(time.DateOnly)
	noon := time.Date(civil.Year(), civil.Month(), civil.Day(), 12, 0, 0, 0, loc)
	if noon.Format(time.DateOnly) != want {
		return time.Time{}, fmt.Errorf("calendar date %s does not exist in %s", want, loc)
	}
	low, high := noon.Add(-36*time.Hour).Unix(), noon.Unix()
	for low < high {
		mid := low + (high-low)/2
		if time.Unix(mid, 0).In(loc).Format(time.DateOnly) < want {
			low = mid + 1
			continue
		}
		high = mid
	}
	return time.Unix(low, 0).In(loc), nil
}

func (c Config) validateCalendar(loc *time.Location) error {
	a, b, err := civilRange(c.Start, c.End)
	if err != nil {
		return err
	}
	if b.Before(a) || int(b.Sub(a).Hours()/24)+1 > MaxDays {
		return fmt.Errorf("date range must be ordered and at most %d days", MaxDays)
	}
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		if _, err := dayStart(d, loc); err != nil {
			return err
		}
	}
	return nil
}
