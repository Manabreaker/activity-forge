package gitstore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

type Record struct {
	Hash           string    `json:"hash"`
	Parent         string    `json:"parent"`
	Tree           string    `json:"tree"`
	AuthorName     string    `json:"author_name"`
	AuthorEmail    string    `json:"author_email"`
	AuthorDate     time.Time `json:"author_date"`
	CommitterName  string    `json:"committer_name"`
	CommitterEmail string    `json:"committer_email"`
	CommitterDate  time.Time `json:"committer_date"`
	PlanID         string    `json:"plan_id"`
	Index          int       `json:"index"`
}

type Report struct {
	OK           bool        `json:"ok"`
	HistoryValid bool        `json:"history_valid"`
	Expected     int         `json:"expected"`
	Actual       int         `json:"actual"`
	Head         string      `json:"head"`
	Issues       []string    `json:"issues"`
	Timestamps   []time.Time `json:"timestamps"`
	Records      []Record    `json:"records"`
}

func (s Store) readLog(ctx context.Context) ([]Record, error) {
	if _, err := s.Head(ctx); err != nil {
		count, e := s.command(ctx, "", nil, "rev-list", "--all", "--count")
		if e == nil && count == "0" {
			return []Record{}, nil
		}
		return nil, fmt.Errorf("cannot resolve repository head: %w", err)
	}
	out, err := s.command(ctx, "", nil, "log", "--reverse", "--format=%H%x00%P%x00%T%x00%an%x00%ae%x00%aI%x00%cn%x00%ce%x00%cI%x00%B%x1e", "HEAD")
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0)
	for _, raw := range strings.Split(out, "\x1e") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		fields := strings.SplitN(strings.TrimSpace(raw), "\x00", 10)
		if len(fields) != 10 {
			return nil, fmt.Errorf("invalid git log record")
		}
		a, err := time.Parse(time.RFC3339, fields[5])
		if err != nil {
			return nil, err
		}
		c, err := time.Parse(time.RFC3339, fields[8])
		if err != nil {
			return nil, err
		}
		id, index := parseMessage(fields[9])
		records = append(records, Record{Hash: fields[0], Parent: fields[1], Tree: fields[2], AuthorName: fields[3], AuthorEmail: fields[4],
			AuthorDate: a, CommitterName: fields[6], CommitterEmail: fields[7], CommitterDate: c, PlanID: id, Index: index})
	}
	return records, nil
}

func parseMessage(body string) (string, int) {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 4 || lines[0] != "Decorative activity" || lines[1] != "" || !strings.HasPrefix(lines[2], "ActivityForge-Plan: ") || !strings.HasPrefix(lines[3], "ActivityForge-Index: ") {
		return "", -1
	}
	i, err := strconv.Atoi(strings.TrimPrefix(lines[3], "ActivityForge-Index: "))
	if err != nil || i < 0 {
		return "", -1
	}
	return strings.TrimPrefix(lines[2], "ActivityForge-Plan: "), i
}

func (s Store) Inspect(ctx context.Context, p forge.Plan) (Report, error) {
	var r Report
	if err := forge.ValidatePlan(p); err != nil {
		return r, err
	}
	err := s.WithLock(ctx, func(s Store) error { var err error; r, err = s.inspect(ctx, p); return err })
	return r, err
}

func (s Store) inspect(ctx context.Context, p forge.Plan) (Report, error) {
	r := Report{HistoryValid: true, Expected: p.Stats.Planned, Issues: []string{}, Timestamps: []time.Time{}, Records: []Record{}}
	o, err := s.readOwner()
	if err != nil {
		return r, err
	}
	branch, err := s.Branch(ctx)
	if err != nil {
		return r, err
	}
	if branch != "main" {
		r.problem("generated repository must remain on main")
	}
	status, err := s.command(ctx, "", nil, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return r, err
	}
	if status != "" {
		r.problem("generated repository has foreign working-tree changes")
	}
	plans := make(map[string]forge.Plan, len(o.Plans))
	expected := make(map[string][]time.Time, len(o.Plans))
	for _, ref := range o.Plans {
		plan, err := forge.LoadPlan(s.PlanPath(ref.ID))
		if err != nil {
			return r, fmt.Errorf("load registered plan: %w", err)
		}
		plans[ref.ID] = plan
		expected[ref.ID] = flatten(plan)
	}
	if _, ok := plans[p.ID]; !ok {
		return r, fmt.Errorf("selected plan does not belong to this repository")
	}
	records, err := s.readLog(ctx)
	if err != nil {
		return r, err
	}
	emptyTree, err := s.command(ctx, "", nil, "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		return r, err
	}
	seen := make(map[string]int, len(o.Plans))
	parent := ""
	previous := time.Time{}
	for _, record := range records {
		if record.Tree != emptyTree {
			r.problem("foreign non-empty tree in commit " + record.Hash)
		}
		if record.Parent != parent {
			r.problem("foreign parent chain or merge in local history")
		}
		if !previous.IsZero() && !record.AuthorDate.After(previous) {
			r.problem("local history timestamps are not chronological")
		}
		plan, ok := plans[record.PlanID]
		index := seen[record.PlanID]
		if !ok {
			r.problem("foreign commit " + record.Hash)
		}
		if ok && (record.Index != index || index >= len(expected[record.PlanID])) {
			r.problem("duplicate or unexpected commit index")
		}
		if ok && index < len(expected[record.PlanID]) {
			r.verifyRecord(record, plan, expected[record.PlanID][index])
		}
		seen[record.PlanID]++
		if record.PlanID == p.ID {
			r.Actual++
			r.Timestamps = append(r.Timestamps, record.AuthorDate)
			r.Records = append(r.Records, record)
		}
		parent = record.Hash
		previous = record.AuthorDate
	}
	r.Head = parent
	for id, stamps := range expected {
		if id != p.ID && seen[id] != len(stamps) {
			r.problem("another registered plan is incomplete")
		}
	}
	if r.Actual != r.Expected {
		r.Issues = append(r.Issues, fmt.Sprintf("missing commits: expected %d, found %d", r.Expected, r.Actual))
	}
	r.OK = r.HistoryValid && r.Actual == r.Expected
	return r, nil
}

func (r *Report) problem(issue string) { r.HistoryValid = false; r.Issues = append(r.Issues, issue) }

func (r *Report) verifyRecord(rec Record, p forge.Plan, stamp time.Time) {
	if rec.AuthorName != p.Config.Author.Name || rec.AuthorEmail != p.Config.Author.Email || rec.CommitterName != p.Config.Author.Name || rec.CommitterEmail != p.Config.Author.Email {
		r.problem("author or committer identity mismatch at " + rec.Hash)
	}
	if rec.AuthorDate.Format(time.RFC3339) != stamp.Format(time.RFC3339) || rec.CommitterDate.Format(time.RFC3339) != stamp.Format(time.RFC3339) {
		r.problem("author or committer timestamp mismatch at " + rec.Hash)
	}
}

func ActualPlan(p forge.Plan, r Report) forge.Plan {
	p.Days = append([]forge.Day(nil), p.Days...)
	byDate := make(map[string][]time.Time)
	loc, _ := time.LoadLocation(p.Config.Timezone)
	for _, stamp := range r.Timestamps {
		key := stamp.In(loc).Format(time.DateOnly)
		byDate[key] = append(byDate[key], stamp)
	}
	for i := range p.Days {
		d := &p.Days[i]
		d.Timestamps = byDate[d.Date]
		d.New = len(d.Timestamps)
		d.Projected = d.Existing + d.New
	}
	p.Stats = forge.Summarize(p.Days)
	return p
}

// VerifyHead must be called under WithLock immediately before a remote mutation.
func (s Store) VerifyHead(ctx context.Context, p forge.Plan, approvedHead string) error {
	r, err := s.inspect(ctx, p)
	if err != nil {
		return err
	}
	if !r.OK || r.Head != approvedHead {
		return fmt.Errorf("local history changed after preview; inspect and approve again")
	}
	return nil
}
