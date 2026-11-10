package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/github"
	"github.com/activityforge/activityforge/internal/gitstore"
	"github.com/activityforge/activityforge/internal/render"
	"github.com/activityforge/activityforge/internal/web"
)

type App struct {
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	Now    func() time.Time
	GitHub github.Client
}

func (a *App) Run(ctx context.Context, args []string) error {
	if a.In == nil {
		a.In = os.Stdin
	}
	if a.Out == nil {
		a.Out = os.Stdout
	}
	if a.Err == nil {
		a.Err = os.Stderr
	}
	if a.Now == nil {
		a.Now = time.Now
	}
	command := "wizard"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if command == "help" || contains(args, "--help") || contains(args, "-h") {
		a.help()
		return nil
	}
	if command == "version" || contains(args, "--version") {
		fmt.Fprintln(a.Out, "activityforge 1.0.0")
		return nil
	}
	o, err := parseOptions(args, a.Now(), a.Err)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	switch command {
	case "init":
		return a.initialize(o)
	case "wizard", "configure":
		return a.wizard(ctx, o, command == "configure")
	case "preview", "plan":
		return a.preview(ctx, o, command == "plan")
	case "generate":
		return a.generate(ctx, o)
	case "inspect", "status":
		return a.inspect(ctx, o, command == "status")
	case "publish":
		return a.publish(ctx, o)
	}
	return fmt.Errorf("unknown command %q; run activityforge help", command)
}

func contains(args []string, s string) bool {
	for _, v := range args {
		if v == s {
			return true
		}
	}
	return false
}

func (a *App) initialize(o options) error {
	if err := o.config.Validate(); err != nil {
		return err
	}
	if err := forge.WriteJSON(o.configPath, o.config); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Initialized Activity Forge\nConfig: %s\nState: %s\nHistory repository (created only by generate): %s\nRun activityforge to open the wizard.\n", o.configPath, o.state, o.repo)
	return nil
}

func (a *App) snapshot(ctx context.Context, o options) (forge.Snapshot, error) {
	if o.noExisting {
		return forge.Snapshot{}, nil
	}
	path := filepath.Join(o.state, "github-cache.json")
	if o.fetch {
		s, err := a.GitHub.Fetch(ctx, o.config.Start, o.config.End, o.githubUser)
		if err != nil {
			return s, err
		}
		return s, forge.WriteJSON(path, s)
	}
	if o.existing != "" {
		path = o.existing
	}
	var s forge.Snapshot
	data, err := forge.ReadLimited(path, 16<<20)
	if os.IsNotExist(err) && o.existing == "" {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = forge.Decode(data, &s); err != nil {
		return s, fmt.Errorf("invalid GitHub cache: %w", err)
	}
	if err = forge.ValidateSnapshot(s, o.config.Start, o.config.End); err != nil {
		if o.existing != "" {
			return s, err
		}
		fmt.Fprintf(a.Err, "Cached GitHub activity does not cover this period; existing activity is unknown. Use --fetch to refresh.\n")
		return forge.Snapshot{}, nil
	}
	return s, nil
}

func (a *App) buildPlan(ctx context.Context, o options) (forge.Plan, error) {
	if o.plan != "" {
		for name := range o.explicit {
			if generationFlag(name) {
				return forge.Plan{}, fmt.Errorf("--%s cannot modify an immutable --plan; create a new plan instead", name)
			}
		}
		return forge.LoadPlan(o.plan)
	}
	s, err := a.snapshot(ctx, o)
	if err != nil {
		return forge.Plan{}, err
	}
	p, err := forge.Generate(o.config, s)
	if err != nil {
		return p, err
	}
	return p, forge.WriteJSON(o.configPath, o.config)
}

func generationFlag(name string) bool {
	return contains([]string{"start", "end", "timezone", "preset", "intensity", "randomness", "seed", "mean", "min", "max", "inactive", "weekends", "weekend-probability", "saturday-probability", "sunday-probability", "saturday-multiplier", "sunday-multiplier", "saturday-max", "sunday-max", "bursts", "rest", "streaks", "monthly", "vacations", "vacation-min", "vacation-max", "trend", "work-start", "work-end", "evening", "night", "time-spread", "author-name", "author-email", "as-of", "days", "fetch", "github-user", "existing", "no-existing"}, name)
}

func (a *App) save(o options, p forge.Plan, custom string) (string, error) {
	path := filepath.Join(o.state, "plans", p.ID+".json")
	if custom != "" {
		path = custom
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err = forge.SavePlan(abs, p); err != nil {
		return "", err
	}
	if err = forge.WriteJSON(o.configPath, p.Config); err != nil {
		return "", err
	}
	if err = forge.AtomicWrite(filepath.Join(o.state, "last-plan"), []byte(abs+"\n")); err != nil {
		return "", err
	}
	return abs, nil
}

func (a *App) loadSelected(o options) (forge.Plan, error) {
	if o.plan != "" {
		return a.buildPlan(context.Background(), o)
	}
	for name := range o.explicit {
		if generationFlag(name) {
			return forge.Plan{}, fmt.Errorf("--%s cannot modify stored history; create a new plan", name)
		}
	}
	data, err := forge.ReadLimited(filepath.Join(o.state, "last-plan"), 8192)
	if err != nil {
		return forge.Plan{}, fmt.Errorf("no selected plan; use --plan or run activityforge plan first: %w", err)
	}
	return forge.LoadPlan(strings.TrimSpace(string(data)))
}

func (a *App) preview(ctx context.Context, o options, save bool) error {
	p, err := a.buildPlan(ctx, o)
	if err != nil {
		return err
	}
	v := render.View{Plan: p, Kind: "preview", Frozen: o.plan != ""}
	if save {
		path, err := a.save(o, p, o.output)
		if err != nil {
			return err
		}
		v.Frozen = true
		v.SavedPath = path
		fmt.Fprintf(a.Out, "Saved immutable plan: %s\n", path)
	}
	if o.format == "" || o.output != "" || save {
		if err = a.terminal(o, p); err != nil {
			return err
		}
	}
	if !save {
		if err = a.export(o, v); err != nil {
			return err
		}
	}
	if o.open || o.serve {
		return a.serve(ctx, o, v)
	}
	return nil
}

func (a *App) terminal(o options, p forge.Plan) error {
	return render.Terminal(a.Out, p, terminalWidth(o.width), terminalColor(a.Out, o.noColor))
}

func (a *App) export(o options, v render.View) error {
	if o.output == "" && o.format == "" {
		return nil
	}
	format := o.format
	if format == "" {
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(o.output)), ".")
	}
	if format == "" {
		format = "html"
	}
	if o.output == "" {
		return render.Export(a.Out, v, format)
	}
	var b bytes.Buffer
	if err := render.Export(&b, v, format); err != nil {
		return err
	}
	if err := forge.AtomicWrite(o.output, b.Bytes()); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Exported %s: %s\n", format, o.output)
	return nil
}

func (a *App) browserOptions(o options, v render.View) web.Options {
	opts := web.Options{View: v, Save: func(_ context.Context, p forge.Plan) (string, error) { return a.save(o, p, "") }}
	if !o.offline {
		opts.Fetch = func(ctx context.Context, c forge.Config) (forge.Snapshot, error) {
			s, err := a.GitHub.Fetch(ctx, c.Start, c.End, o.githubUser)
			if err != nil {
				return s, err
			}
			return s, forge.WriteJSON(filepath.Join(o.state, "github-cache.json"), s)
		}
	}
	return opts
}

func (a *App) serve(ctx context.Context, o options, v render.View) error {
	return web.Serve(ctx, a.browserOptions(o, v), o.port, o.open, a.Out)
}

func (a *App) generate(ctx context.Context, o options) error {
	p, err := a.loadSelected(o)
	if err != nil {
		return err
	}
	s := gitstore.Store{Repo: o.repo, State: o.state}
	fmt.Fprintf(a.Out, "Applying exact saved plan %s\nRepository: %s\n", p.ID, o.repo)
	if err = s.Generate(ctx, p); err != nil {
		return err
	}
	r, err := s.Inspect(ctx, p)
	if err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("post-generation verification failed: %s", strings.Join(r.Issues, "; "))
	}
	if _, err = a.save(o, p, ""); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "VERIFIED: %d / %d commits; HEAD %s\nManifest: %s\nRun activityforge inspect --open before publication.\n", r.Actual, r.Expected, r.Head, s.ManifestPath(p.ID))
	return nil
}

func (a *App) inspect(ctx context.Context, o options, status bool) error {
	p, err := a.loadSelected(o)
	if err != nil {
		return err
	}
	s := gitstore.Store{Repo: o.repo, State: o.state}
	r, err := s.Inspect(ctx, p)
	if err != nil {
		return err
	}
	v := render.View{Plan: gitstore.ActualPlan(p, r), Kind: "inspect", Frozen: true, Verification: &r}
	if o.format != "" && o.output == "" {
		return a.export(o, v)
	}
	if r.OK {
		fmt.Fprintf(a.Out, "VERIFIED: %d / %d planned commits match real Git log.\nHEAD: %s\n", r.Actual, r.Expected, r.Head)
	}
	if !r.OK {
		fmt.Fprintf(a.Out, "MISMATCH: %s\n", strings.Join(r.Issues, "; "))
	}
	if !status {
		if err = a.terminal(o, v.Plan); err != nil {
			return err
		}
	}
	if m, e := s.ReadManifest(p.ID); e == nil {
		fmt.Fprintf(a.Out, "Manifest complete: %t · Published HEAD: %s\n", m.Completed, m.PublishedHead)
	}
	if err = a.export(o, v); err != nil {
		return err
	}
	if o.open || o.serve {
		return a.serve(ctx, o, v)
	}
	if !r.OK {
		return fmt.Errorf("local history differs from saved plan")
	}
	return nil
}
