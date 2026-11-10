package forge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

func PlanID(p Plan) (string, error) {
	p.ID = ""
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func ValidateSnapshot(s Snapshot, start, end string) error {
	if !s.Known {
		if len(s.Days) > 0 {
			return fmt.Errorf("unknown snapshot cannot contain daily counts")
		}
		return nil
	}
	if s.Start > start || s.End < end {
		return fmt.Errorf("cached GitHub calendar does not cover %s through %s; fetch again or disable existing activity", start, end)
	}
	a, err := time.Parse(time.DateOnly, start)
	if err != nil {
		return err
	}
	b, err := time.Parse(time.DateOnly, end)
	if err != nil {
		return err
	}
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		entry, ok := s.Days[d.Format(time.DateOnly)]
		if !ok || entry.Count < 0 || entry.Count > 10000000 || entry.Weekday != int(d.Weekday()) {
			return fmt.Errorf("invalid or missing existing activity for %s", d.Format(time.DateOnly))
		}
	}
	return nil
}

func ValidatePlan(p Plan) error {
	if p.Version != Version {
		return fmt.Errorf("unsupported plan version %d", p.Version)
	}
	if err := p.Config.Validate(); err != nil {
		return err
	}
	if err := ValidateSnapshot(p.Existing, p.Config.Start, p.Config.End); err != nil {
		return err
	}
	_, _, loc, _ := p.Config.Bounds()
	start, end, _ := civilRange(p.Config.Start, p.Config.End)
	cutoff, _ := time.Parse(time.RFC3339, p.Config.AsOf)
	next, previous := start, time.Time{}
	for _, d := range p.Days {
		if next.After(end) || d.Date != next.Format(time.DateOnly) {
			return fmt.Errorf("plan has missing, duplicate or unordered dates")
		}
		if d.Target < 0 || d.Target > p.Config.Max || d.Existing != p.Existing.Days[d.Date].Count || d.New < 0 || d.New != len(d.Timestamps) || d.New > max(0, d.Target-d.Existing) || d.Projected != d.Existing+d.New {
			return fmt.Errorf("invalid plan counts on %s", d.Date)
		}
		if p.Config.Manual != nil && d.Target != p.Config.Manual.Targets[d.Date] {
			return fmt.Errorf("manual target does not match plan on %s", d.Date)
		}
		for _, stamp := range d.Timestamps {
			if stamp.In(loc).Format(time.DateOnly) != d.Date || stamp.After(cutoff) || stamp.Nanosecond() != 0 || (!previous.IsZero() && !stamp.After(previous)) {
				return fmt.Errorf("invalid timestamp on %s", d.Date)
			}
			previous = stamp
		}
		next = next.AddDate(0, 0, 1)
	}
	if !next.After(end) || !reflect.DeepEqual(p.Stats, Summarize(p.Days)) || p.Stats.Planned > MaxCommits {
		return fmt.Errorf("plan days or statistics do not match")
	}
	id, err := PlanID(p)
	if err != nil {
		return err
	}
	if p.ID != id {
		return fmt.Errorf("plan checksum mismatch; saved plans are immutable")
	}
	return nil
}

func SavePlan(path string, p Plan) error {
	if err := ValidatePlan(p); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil {
		var prior Plan
		if Decode(old, &prior) == nil && prior.ID == p.ID && ValidatePlan(prior) == nil {
			return nil
		}
		return fmt.Errorf("refusing to overwrite existing plan %s", path)
	}
	return writeImmutable(path, append(data, '\n'))
}

func LoadPlan(path string) (Plan, error) {
	var p Plan
	data, err := ReadLimited(path, 64<<20)
	if err != nil {
		return p, err
	}
	if err = Decode(data, &p); err != nil {
		return p, fmt.Errorf("invalid plan JSON: %w", err)
	}
	return p, ValidatePlan(p)
}

func Decode(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

func ReadLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return data, err
}

func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(data, '\n'))
}

func AtomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".activityforge-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func writeImmutable(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".activityforge-plan-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Link(f.Name(), path); err != nil {
		return fmt.Errorf("save immutable plan: %w", err)
	}
	return nil
}
