package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

type Store struct {
	Repo     string
	State    string
	Progress func(done, total int)
}

type planRef struct {
	ID    string `json:"id"`
	Start string `json:"start"`
	End   string `json:"end"`
}
type owner struct {
	Version int       `json:"version"`
	Repo    string    `json:"repo"`
	Plans   []planRef `json:"plans"`
}

type Manifest struct {
	PlanID        string `json:"plan_id"`
	Repo          string `json:"repo"`
	Generated     int    `json:"generated"`
	Completed     bool   `json:"completed"`
	Head          string `json:"head"`
	PublishedHead string `json:"published_head,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

func (s Store) ManifestPath(id string) string { return filepath.Join(s.State, "manifests", id+".json") }
func (s Store) PlanPath(id string) string     { return filepath.Join(s.State, "plans", id+".json") }
func (s Store) ownerPath() string             { return filepath.Join(s.Repo, ".git", "activityforge.json") }

func (s Store) canonical() (Store, error) {
	if s.Repo == "" || s.State == "" {
		return s, fmt.Errorf("repository and state paths are required")
	}
	var err error
	s.Repo, err = filepath.Abs(s.Repo)
	if err != nil {
		return s, err
	}
	if info, err := os.Lstat(s.Repo); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return s, fmt.Errorf("repository path cannot be a symbolic link")
	}
	s.Repo, err = resolveAncestors(s.Repo)
	if err != nil {
		return s, err
	}
	s.State, err = resolveAncestors(s.State)
	if err != nil {
		return s, err
	}
	if within(s.Repo, s.State) {
		return s, fmt.Errorf("state must be outside the generated repository")
	}
	return s, guardParents(s.Repo)
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// WithLock serializes initialization, generation and approval-bound publication.
func (s Store) WithLock(ctx context.Context, fn func(Store) error) error {
	s, err := s.canonical()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.Repo), 0700); err != nil {
		return err
	}
	f, err := lockRepository(s.Repo + ".activityforge.lock")
	if err != nil {
		return err
	}
	defer f.Close()
	if err = ctx.Err(); err != nil {
		return err
	}
	return fn(s)
}

func (s Store) readOwner() (owner, error) {
	var o owner
	data, err := forge.ReadLimited(s.ownerPath(), 1<<20)
	if err != nil {
		return o, fmt.Errorf("repository is not owned by Activity Forge: %w", err)
	}
	if err = forge.Decode(data, &o); err != nil {
		return o, err
	}
	if o.Version != forge.Version {
		return o, fmt.Errorf("unsupported repository ownership version")
	}
	real, err := filepath.EvalSymlinks(s.Repo)
	if err != nil {
		return o, err
	}
	if o.Repo != real {
		return o, fmt.Errorf("repository ownership path differs; do not copy or move generated repositories")
	}
	return o, nil
}

func (s Store) initialize(ctx context.Context) (owner, error) {
	if _, err := os.Lstat(filepath.Join(s.Repo, ".git")); err == nil {
		return s.readOwner()
	}
	for parent := filepath.Dir(s.Repo); ; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return owner{}, fmt.Errorf("refusing to create history inside an existing working repository")
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	entries, err := os.ReadDir(s.Repo)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return owner{}, err
	}
	if len(entries) > 0 {
		return owner{}, fmt.Errorf("history directory must be empty; existing files are never modified")
	}
	if err = os.MkdirAll(s.Repo, 0700); err != nil {
		return owner{}, err
	}
	if _, err = s.command(ctx, "", nil, "init", "--initial-branch=main", "--template="); err != nil {
		return owner{}, err
	}
	real, err := filepath.EvalSymlinks(s.Repo)
	if err != nil {
		return owner{}, err
	}
	o := owner{Version: forge.Version, Repo: real, Plans: []planRef{}}
	return o, forge.WriteJSON(s.ownerPath(), o)
}

func (s Store) register(ctx context.Context, o owner, p forge.Plan) error {
	for _, ref := range o.Plans {
		if ref.ID == p.ID {
			return nil
		}
		if p.Config.Start <= ref.End && p.Config.End >= ref.Start {
			return fmt.Errorf("plan overlaps existing generated dates; resume the original plan or use a new empty repository")
		}
	}
	if len(o.Plans) > 0 {
		last := o.Plans[len(o.Plans)-1]
		if p.Config.Start <= last.End {
			return fmt.Errorf("new plans must follow existing history chronologically")
		}
		if err := s.validateAppend(ctx, last, p); err != nil {
			return err
		}
	}
	if err := forge.SavePlan(s.PlanPath(p.ID), p); err != nil {
		return err
	}
	o.Plans = append(o.Plans, planRef{ID: p.ID, Start: p.Config.Start, End: p.Config.End})
	return forge.WriteJSON(s.ownerPath(), o)
}

func (s Store) Generate(ctx context.Context, p forge.Plan) error {
	if err := forge.ValidatePlan(p); err != nil {
		return err
	}
	if err := ValidIdentity(p.Config.Author.Name, p.Config.Author.Email); err != nil {
		return err
	}
	return s.WithLock(ctx, func(s Store) error {
		o, err := s.initialize(ctx)
		if err != nil {
			return err
		}
		if err = s.register(ctx, o, p); err != nil {
			return err
		}
		r, err := s.inspect(ctx, p)
		if err != nil {
			return err
		}
		if !r.HistoryValid {
			return fmt.Errorf("history verification failed: %s", strings.Join(r.Issues, "; "))
		}
		return s.appendCommits(ctx, p, r)
	})
}

func flatten(p forge.Plan) []time.Time {
	out := make([]time.Time, 0, p.Stats.Planned)
	for _, d := range p.Days {
		out = append(out, d.Timestamps...)
	}
	return out
}

func (s Store) appendCommits(ctx context.Context, p forge.Plan, r Report) error {
	tree, err := s.command(ctx, "", nil, "mktree")
	if err != nil {
		return err
	}
	head := r.Head
	stamps := flatten(p)
	for i := r.Actual; i < len(stamps); i++ {
		stamp := stamps[i].Format(time.RFC3339)
		env := []string{"GIT_AUTHOR_NAME=" + p.Config.Author.Name, "GIT_AUTHOR_EMAIL=" + p.Config.Author.Email,
			"GIT_COMMITTER_NAME=" + p.Config.Author.Name, "GIT_COMMITTER_EMAIL=" + p.Config.Author.Email,
			"GIT_AUTHOR_DATE=" + stamp, "GIT_COMMITTER_DATE=" + stamp}
		args := []string{"commit-tree", tree}
		if head != "" {
			args = append(args, "-p", head)
		}
		message := fmt.Sprintf("Decorative activity\n\nActivityForge-Plan: %s\nActivityForge-Index: %d\n", p.ID, i)
		next, err := s.command(ctx, message, env, args...)
		if err != nil {
			return err
		}
		old := head
		if old == "" {
			old = strings.Repeat("0", len(next))
		}
		if _, err = s.command(ctx, "", nil, "update-ref", "refs/heads/main", next, old); err != nil {
			return err
		}
		head = next
		if s.Progress != nil {
			s.Progress(i+1, len(stamps))
		}
		if (i+1)%25 == 0 {
			if err = s.checkpoint(p, i+1, head); err != nil {
				return err
			}
		}
	}
	return s.checkpoint(p, len(stamps), head)
}

func (s Store) checkpoint(p forge.Plan, count int, head string) error {
	m := Manifest{PlanID: p.ID, Repo: s.Repo, Generated: count, Completed: count == p.Stats.Planned, Head: head, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if old, err := s.ReadManifest(p.ID); err == nil {
		m.PublishedHead = old.PublishedHead
	}
	return forge.WriteJSON(s.ManifestPath(p.ID), m)
}

func (s Store) ReadManifest(id string) (Manifest, error) {
	var m Manifest
	data, err := forge.ReadLimited(s.ManifestPath(id), 1<<20)
	if err != nil {
		return m, err
	}
	return m, forge.Decode(data, &m)
}

func (s Store) MarkPublished(id, head string) error {
	m, err := s.ReadManifest(id)
	if err != nil {
		return err
	}
	m.PublishedHead = head
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return forge.WriteJSON(s.ManifestPath(id), m)
}
