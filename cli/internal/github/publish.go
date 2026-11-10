package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/gitstore"
)

type remote struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
}

type Preparation struct {
	Store          gitstore.Store
	Plan           forge.Plan
	Report         gitstore.Report
	Display        forge.Plan
	Target         string
	Head           string
	RemoteHead     string
	Pending        int
	CreatePrivate  bool
	RequirePrivate bool
	Notice         string
}

func (c Client) remote(ctx context.Context, target string, allowMissing bool) (remote, bool, error) {
	var r remote
	data, err := c.call(ctx, "gh", "api", "repos/"+target)
	if err != nil {
		if allowMissing && strings.Contains(err.Error(), "HTTP 404") {
			return r, false, nil
		}
		return r, false, err
	}
	if err = json.Unmarshal(data, &r); err != nil {
		return r, false, err
	}
	if !strings.EqualFold(r.FullName, target) {
		return r, false, fmt.Errorf("GitHub target does not match requested repository")
	}
	if r.Fork {
		return r, false, fmt.Errorf("forks do not qualify for ordinary commit contributions; choose a standalone repository")
	}
	if r.DefaultBranch != "main" {
		return r, false, fmt.Errorf("remote default branch is %q; this dedicated history requires main", r.DefaultBranch)
	}
	return r, true, nil
}

func remoteURL(target string) string { return "https://github.com/" + target + ".git" }

func gitArgs(s gitstore.Store, args ...string) []string {
	base := []string{"--no-replace-objects", "-C", s.Repo, "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential"}
	return append(base, args...)
}

func (c Client) remoteHead(ctx context.Context, s gitstore.Store, target string) (string, error) {
	data, err := c.call(ctx, "git", gitArgs(s, "ls-remote", "--heads", remoteURL(target), "refs/heads/main")...)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "", nil
	}
	if len(fields) != 2 || fields[1] != "refs/heads/main" {
		return "", fmt.Errorf("unexpected remote branch response")
	}
	return fields[0], nil
}

func (c Client) pending(ctx context.Context, s gitstore.Store, r gitstore.Report, remoteHead string) ([]string, error) {
	rangeSpec := "HEAD"
	if remoteHead != "" {
		if _, err := c.call(ctx, "git", gitArgs(s, "merge-base", "--is-ancestor", remoteHead, r.Head)...); err != nil {
			return nil, fmt.Errorf("remote history is not an ancestor of verified local history; ordinary push is required, never force push")
		}
		rangeSpec = remoteHead + ".." + r.Head
	}
	data, err := c.call(ctx, "git", gitArgs(s, "rev-list", "--reverse", rangeSpec)...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(data)), nil
}

// Prepare performs read-only checks. It never creates a remote or pushes.
func (c Client) Prepare(ctx context.Context, s gitstore.Store, p forge.Plan, target string, createPrivate, refresh bool) (Preparation, error) {
	prep := Preparation{Store: s, Plan: p, Target: target, RequirePrivate: createPrivate}
	if err := ValidateTarget(target); err != nil {
		return prep, err
	}
	a, err := c.VerifyEmail(ctx, p.Config.Author.Email)
	if err != nil {
		return prep, err
	}
	r, err := s.Inspect(ctx, p)
	if err != nil {
		return prep, err
	}
	if !r.OK || r.Head == "" {
		return prep, fmt.Errorf("local history is not ready for publication: %s", strings.Join(r.Issues, "; "))
	}
	prep.Head, prep.Report = r.Head, r
	remote, exists, err := c.remote(ctx, target, createPrivate)
	if err != nil {
		return prep, err
	}
	if exists && prep.RequirePrivate && !remote.Private {
		return prep, fmt.Errorf("target repository is public; choose an empty private repository")
	}
	if !exists {
		if !strings.EqualFold(strings.Split(target, "/")[0], a.Login) {
			return prep, fmt.Errorf("new private repositories must belong to authenticated @%s", a.Login)
		}
		prep.CreatePrivate = true
		prep.Notice = "Approval will create a private repository and push the verified history."
	}
	if exists {
		prep.RemoteHead, err = c.remoteHead(ctx, s, target)
		if err != nil {
			return prep, err
		}
	}
	hashes, err := c.pending(ctx, s, r, prep.RemoteHead)
	if err != nil {
		return prep, err
	}
	prep.Pending = len(hashes)
	prep.Display = gitstore.ActualPlan(p, r)
	if refresh {
		snapshot, fetchErr := c.Fetch(ctx, p.Config.Start, p.Config.End, a.Login)
		if fetchErr == nil {
			prep.Display = forge.Rebase(prep.Display, snapshot)
		}
		if fetchErr != nil {
			prep.Notice += " Current GitHub activity could not be refreshed; using the saved baseline: " + fetchErr.Error()
		}
	}
	if err = projectPending(&prep, hashes); err != nil {
		return prep, err
	}
	return prep, nil
}

func projectPending(prep *Preparation, hashes []string) error {
	selected := make(map[string]gitstore.Record, len(prep.Report.Records))
	for _, r := range prep.Report.Records {
		selected[r.Hash] = r
	}
	byDate := make(map[string][]time.Time)
	loc, _ := time.LoadLocation(prep.Plan.Config.Timezone)
	for _, hash := range hashes {
		record, ok := selected[hash]
		if !ok {
			return fmt.Errorf("pending history includes another plan; select the newest plan after publishing previous plans")
		}
		key := record.AuthorDate.In(loc).Format(time.DateOnly)
		byDate[key] = append(byDate[key], record.AuthorDate)
	}
	for i := range prep.Display.Days {
		d := &prep.Display.Days[i]
		d.Timestamps = byDate[d.Date]
		d.New = len(d.Timestamps)
		d.Projected = d.Existing + d.New
	}
	prep.Display.Stats = forge.Summarize(prep.Display.Days)
	return nil
}

// Push is called only by an explicit approval handler or typed terminal approval.
func (c Client) Push(ctx context.Context, prep Preparation) error {
	return prep.Store.WithLock(ctx, func(s gitstore.Store) error {
		if err := s.VerifyHead(ctx, prep.Plan, prep.Head); err != nil {
			return err
		}
		if _, err := c.VerifyEmail(ctx, prep.Plan.Config.Author.Email); err != nil {
			return err
		}
		remote, exists, err := c.remote(ctx, prep.Target, prep.CreatePrivate)
		if err != nil {
			return err
		}
		if exists && prep.RequirePrivate && !remote.Private {
			return fmt.Errorf("target became public after preview; private publication approval is no longer valid")
		}
		if exists {
			head, err := c.remoteHead(ctx, s, prep.Target)
			if err != nil {
				return err
			}
			if head == prep.Head {
				return s.MarkPublished(prep.Plan.ID, prep.Head)
			}
			if head != prep.RemoteHead {
				return fmt.Errorf("remote changed after preview; prepare and approve again")
			}
		}
		if !exists {
			if !prep.CreatePrivate {
				return fmt.Errorf("remote disappeared after preview")
			}
			if _, err = c.call(ctx, "gh", "repo", "create", prep.Target, "--private"); err != nil {
				return err
			}
		}
		if prep.Pending == 0 {
			return nil
		}
		if _, err = c.call(ctx, "git", gitArgs(s, "push", "--porcelain", remoteURL(prep.Target), prep.Head+":refs/heads/main")...); err != nil {
			return err
		}
		head, err := c.remoteHead(ctx, s, prep.Target)
		if err != nil {
			return err
		}
		if head != prep.Head {
			return fmt.Errorf("push returned but remote HEAD differs; inspect before retrying")
		}
		return s.MarkPublished(prep.Plan.ID, prep.Head)
	})
}
