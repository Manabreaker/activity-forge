package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

type options struct {
	config                                                            forge.Config
	state, configPath, repo, plan, output, format, remote, githubUser string
	existing                                                          string
	open, serve, offline, fetch, noColor, terminal, createPrivate     bool
	noExisting                                                        bool
	port, width                                                       int
	explicit                                                          map[string]bool
}

func findArg(args []string, name, fallback string) string {
	for i, arg := range args {
		if arg == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--"+name+"=") {
			return strings.TrimPrefix(arg, "--"+name+"=")
		}
	}
	return fallback
}

func defaultState() string {
	if dir := os.Getenv("ACTIVITYFORGE_STATE_DIR"); dir != "" {
		return dir
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ".activityforge"
	}
	return filepath.Join(base, "activityforge")
}

func parseOptions(args []string, now time.Time, out io.Writer) (options, error) {
	o := options{state: findArg(args, "state-dir", defaultState()), format: "", explicit: map[string]bool{}}
	o.configPath = findArg(args, "config", filepath.Join(o.state, "config.json"))
	c := forge.DefaultConfig(now)
	if data, err := forge.ReadLimited(o.configPath, 1<<20); err == nil {
		if err = forge.Decode(data, &c); err != nil {
			return o, fmt.Errorf("read config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return o, err
	}
	o.config = c
	fs := flag.NewFlagSet("activityforge", flag.ContinueOnError)
	fs.SetOutput(out)
	commonFlags(fs, &o)
	configFlags(fs, &o.config)
	days := fs.Int("days", 0, "Last N days, inclusive of today; refreshes as_of")
	asOf := fs.String("as-of", c.AsOf, "Frozen timestamp cutoff (RFC3339 or now)")
	weekendProbability := fs.Float64("weekend-probability", -1, "Set both custom weekend activity probabilities (0–1)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() != 0 {
		return o, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	fs.Visit(func(f *flag.Flag) { o.explicit[f.Name] = true })
	if o.explicit["preset"] {
		o.config = forge.ApplyPreset(o.config, o.config.Preset)
		if err := fs.Parse(args); err != nil {
			return o, err
		}
	}
	o.config.Preset = strings.ToLower(strings.ReplaceAll(o.config.Preset, " ", "-"))
	if o.config.Preset == "active" {
		o.config.Preset = "active-developer"
	}
	o.config.Weekends.Mode = strings.ToLower(o.config.Weekends.Mode)
	if *days != 0 {
		if *days < 1 || *days > forge.MaxDays {
			return o, fmt.Errorf("days must be 1–%d", forge.MaxDays)
		}
		loc, err := time.LoadLocation(o.config.Timezone)
		if err != nil {
			return o, err
		}
		end := now.In(loc)
		o.config.End = end.Format(time.DateOnly)
		o.config.Start = end.AddDate(0, 0, 1-*days).Format(time.DateOnly)
		o.config.AsOf = now.Format(time.RFC3339)
	}
	if o.explicit["as-of"] {
		o.config.AsOf = *asOf
		if *asOf == "now" {
			o.config.AsOf = now.Format(time.RFC3339)
		}
	}
	if *weekendProbability >= 0 {
		o.config.Weekends.Mode = "custom"
		o.config.Weekends.Saturday.Probability = *weekendProbability
		o.config.Weekends.Sunday.Probability = *weekendProbability
	}
	if o.offline && o.fetch {
		return o, fmt.Errorf("--offline and --fetch cannot be combined")
	}
	if o.noExisting && (o.fetch || o.existing != "") {
		return o, fmt.Errorf("--no-existing cannot be combined with --fetch or --existing")
	}
	if o.repo == "" {
		o.repo = filepath.Join(o.state, "history")
	}
	return o, nil
}

func commonFlags(fs *flag.FlagSet, o *options) {
	fs.StringVar(&o.state, "state-dir", o.state, "Directory for config, cache, saved plans and manifests")
	fs.StringVar(&o.configPath, "config", o.configPath, "JSON configuration path")
	fs.StringVar(&o.repo, "repo", "", "Dedicated empty/owned history repository")
	fs.StringVar(&o.plan, "plan", "", "Load an immutable saved plan; generation parameters are then forbidden")
	fs.StringVar(&o.output, "output", "", "Output path (plan, or exported preview)")
	fs.StringVar(&o.format, "format", "", "Export format: html, svg, png, json or csv")
	fs.StringVar(&o.remote, "remote", "", "GitHub owner/repository; never force-pushed")
	fs.StringVar(&o.githubUser, "github-user", "", "GitHub login for contribution lookup; default authenticated account")
	fs.BoolVar(&o.open, "open", false, "Open the local browser preview and serve until Ctrl+C")
	fs.BoolVar(&o.serve, "serve", false, "Serve browser preview and print URL without opening browser")
	fs.BoolVar(&o.offline, "offline", false, "Disable GitHub network calls; use cache if available")
	fs.BoolVar(&o.fetch, "fetch", false, "Fetch and cache current GitHub contributions")
	fs.StringVar(&o.existing, "existing", "", "Load a GitHub Snapshot JSON from a chosen file")
	fs.BoolVar(&o.noExisting, "no-existing", false, "Ignore all existing contributions and cached GitHub data")
	fs.BoolVar(&o.noColor, "no-color", false, "Disable terminal ANSI colors (also NO_COLOR)")
	fs.BoolVar(&o.terminal, "terminal", false, "Use typed terminal publication approval instead of browser")
	fs.BoolVar(&o.createPrivate, "create-private", false, "Offer to create a missing private remote after approval")
	fs.IntVar(&o.port, "port", 0, "Loopback preview port; 0 chooses a free port")
	fs.IntVar(&o.width, "width", 0, "Terminal preview width; 0 detects terminal/COLUMNS")
}

func configFlags(fs *flag.FlagSet, c *forge.Config) {
	fs.StringVar(&c.Start, "start", c.Start, "Start date, YYYY-MM-DD")
	fs.StringVar(&c.End, "end", c.End, "End date, YYYY-MM-DD; no future dates")
	fs.StringVar(&c.Timezone, "timezone", c.Timezone, "IANA timezone")
	fs.StringVar(&c.Preset, "preset", c.Preset, "balanced, active-developer, hardcore, chaotic or custom")
	fs.IntVar(&c.Intensity, "intensity", c.Intensity, "Overall intensity, 1–10")
	fs.Float64Var(&c.Randomness, "randomness", c.Randomness, "Correlated variation, 0–100 percent")
	fs.Int64Var(&c.Seed, "seed", c.Seed, "Deterministic random seed")
	fs.Float64Var(&c.Mean, "mean", c.Mean, "Mean active weekday workload; 0 uses intensity")
	fs.IntVar(&c.Min, "min", c.Min, "Minimum target in an active day")
	fs.IntVar(&c.Max, "max", c.Max, "Maximum generated daily target")
	fs.Float64Var(&c.InactivePercent, "inactive", c.InactivePercent, "Base percent of inactive weekdays")
	fs.StringVar(&c.Weekends.Mode, "weekends", c.Weekends.Mode, "off, rare, light, moderate, same or custom")
	weekendFlags(fs, "saturday", &c.Weekends.Saturday)
	weekendFlags(fs, "sunday", &c.Weekends.Sunday)
	fs.BoolVar(&c.Effects.WorkBursts, "bursts", c.Effects.WorkBursts, "Enable work bursts")
	fs.BoolVar(&c.Effects.RestPeriods, "rest", c.Effects.RestPeriods, "Enable short rest periods")
	fs.BoolVar(&c.Effects.Streaks, "streaks", c.Effects.Streaks, "Enable correlated active-day streaks")
	fs.BoolVar(&c.Effects.MonthlyVariation, "monthly", c.Effects.MonthlyVariation, "Enable smooth monthly variation")
	fs.IntVar(&c.Effects.VacationCount, "vacations", c.Effects.VacationCount, "Number of vacation intervals; 0 disables")
	fs.IntVar(&c.Effects.VacationMin, "vacation-min", c.Effects.VacationMin, "Minimum vacation duration in calendar days")
	fs.IntVar(&c.Effects.VacationMax, "vacation-max", c.Effects.VacationMax, "Maximum vacation duration in calendar days")
	fs.StringVar(&c.Effects.Trend, "trend", c.Effects.Trend, "stable, increasing, decreasing, wave or random")
	fs.StringVar(&c.Clock.Start, "work-start", c.Clock.Start, "Workday start, HH:MM")
	fs.StringVar(&c.Clock.End, "work-end", c.Clock.End, "Workday end, HH:MM")
	fs.Float64Var(&c.Clock.EveningProbability, "evening", c.Clock.EveningProbability, "Evening session probability, 0–1")
	fs.Float64Var(&c.Clock.NightProbability, "night", c.Clock.NightProbability, "Night session probability, 0–1")
	fs.IntVar(&c.Clock.Spread, "time-spread", c.Clock.Spread, "Session spread in minutes")
	fs.StringVar(&c.Author.Name, "author-name", c.Author.Name, "Commit author/committer name")
	fs.StringVar(&c.Author.Email, "author-email", c.Author.Email, "GitHub verified email or account noreply address")
}

func weekendFlags(fs *flag.FlagSet, prefix string, d *forge.WeekendDay) {
	fs.Float64Var(&d.Probability, prefix+"-probability", d.Probability, "Custom "+prefix+" activity probability, 0–1")
	fs.Float64Var(&d.Multiplier, prefix+"-multiplier", d.Multiplier, "Custom "+prefix+" workload multiplier")
	fs.IntVar(&d.Max, prefix+"-max", d.Max, "Custom "+prefix+" daily maximum")
}
