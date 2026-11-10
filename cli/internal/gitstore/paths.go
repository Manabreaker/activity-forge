package gitstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/activityforge/activityforge/internal/forge"
)

func resolveAncestors(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	tail := make([]string, 0)
	for current := abs; ; current = filepath.Dir(current) {
		real, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				real = filepath.Join(real, tail[i])
			}
			return real, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if current == filepath.Dir(current) {
			return "", err
		}
		tail = append(tail, filepath.Base(current))
	}
}

func guardParents(path string) error {
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return fmt.Errorf("refusing history inside an existing working repository")
		}
		if parent == filepath.Dir(parent) {
			return nil
		}
	}
}

// Validate before registering anything: a rejected append must leave recovery intact.
func (s Store) validateAppend(ctx context.Context, last planRef, next forge.Plan) error {
	p, err := forge.LoadPlan(s.PlanPath(last.ID))
	if err != nil {
		return err
	}
	r, err := s.inspect(ctx, p)
	if err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("complete the previous plan before adding another; rejected plan was not registered")
	}
	stamps := flatten(next)
	if len(stamps) > 0 && len(r.Timestamps) > 0 && !stamps[0].After(r.Timestamps[len(r.Timestamps)-1]) {
		return fmt.Errorf("new plan timestamps must follow existing history chronologically across timezones")
	}
	m, err := s.ReadManifest(p.ID)
	if err != nil {
		return fmt.Errorf("recover the previous manifest by reapplying its plan before appending")
	}
	if m.PublishedHead != r.Head {
		return fmt.Errorf("publish the previous plan before appending another, or choose a new empty repository")
	}
	return nil
}
