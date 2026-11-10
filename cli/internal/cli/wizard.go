package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/gitstore"
	"github.com/activityforge/activityforge/internal/render"
)

type prompts struct {
	r     *bufio.Reader
	out   io.Writer
	err   error
	color bool
}

func (p *prompts) heading(step int, title string) {
	if p.color {
		fmt.Fprintf(p.out, "\n\x1b[1;32m  %d / 7  %s\x1b[0m\n", step, title)
		return
	}
	fmt.Fprintf(p.out, "\n  %d / 7  %s\n", step, title)
}

func (p *prompts) ask(label, def string) string {
	if p.err != nil {
		return def
	}
	fmt.Fprintf(p.out, "  %s", label)
	if def != "" {
		fmt.Fprintf(p.out, " [%s]", def)
	}
	fmt.Fprint(p.out, ": ")
	line, err := p.r.ReadString('\n')
	if err != nil && len(line) == 0 {
		p.err = err
		return def
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func (p *prompts) number(label string, def, low, high float64) float64 {
	for p.err == nil {
		v, err := strconv.ParseFloat(p.ask(label, strconv.FormatFloat(def, 'f', -1, 64)), 64)
		if err == nil && v >= low && v <= high {
			return v
		}
		fmt.Fprintf(p.out, "  Enter a number from %g to %g.\n", low, high)
	}
	return def
}

func (p *prompts) integer(label string, def, low, high int) int {
	for p.err == nil {
		v, err := strconv.Atoi(p.ask(label, strconv.Itoa(def)))
		if err == nil && v >= low && v <= high {
			return v
		}
		fmt.Fprintf(p.out, "  Enter an integer from %d to %d.\n", low, high)
	}
	return def
}

func (p *prompts) yes(label string, def bool) bool {
	d := "n"
	if def {
		d = "y"
	}
	for p.err == nil {
		s := strings.ToLower(p.ask(label+" (y/n)", d))
		if s == "y" || s == "yes" {
			return true
		}
		if s == "n" || s == "no" {
			return false
		}
	}
	return def
}

func (p *prompts) choice(label, def string, values ...string) string {
	for p.err == nil {
		s := strings.ToLower(p.ask(label, def))
		for _, v := range values {
			if s == v {
				return s
			}
		}
		fmt.Fprintf(p.out, "  Choose %s.\n", strings.Join(values, ", "))
	}
	return def
}

func (a *App) wizard(ctx context.Context, o options, configureOnly bool) error {
	p := &prompts{r: bufio.NewReader(a.In), out: a.Out, color: terminalColor(a.Out, o.noColor)}
	fmt.Fprint(a.Out, "\n  ◆ ACTIVITY FORGE\n  Shape a decorative calendar. Preview it before creating history.\n")
	c := o.config
	p.heading(1, "Activity period")
	a.wizardDates(p, &c)
	p.heading(2, "Activity profile")
	c = forge.ApplyPreset(c, p.choice("Preset: balanced / active-developer / hardcore / chaotic / custom", c.Preset, "balanced", "active-developer", "hardcore", "chaotic", "custom"))
	c.Intensity = p.integer("Intensity", c.Intensity, 1, 10)
	if p.yes("Adjust mean, min, max and inactive days", false) {
		wizardDaily(p, &c)
	}
	p.heading(3, "Randomness")
	c.Randomness = p.number("Randomness %", c.Randomness, 0, 100)
	p.heading(4, "Weekends")
	wizardWeekends(p, &c)
	p.heading(5, "Natural variation")
	fmt.Fprintf(a.Out, "  Bursts: %t · Rest: %t · Vacations: %d · Streaks: %t · Trend: %s\n", c.Effects.WorkBursts, c.Effects.RestPeriods, c.Effects.VacationCount, c.Effects.Streaks, c.Effects.Trend)
	if p.yes("Adjust natural effects", false) {
		wizardEffects(p, &c)
	}
	p.heading(6, "Commit times")
	c.Timezone = p.ask("Timezone", c.Timezone)
	if p.yes("Adjust workday and session timing", false) {
		wizardClock(p, &c)
	}
	p.heading(7, "Identity & existing activity")
	c.Author.Name = p.ask("Author name (blank is fine for preview)", c.Author.Name)
	c.Author.Email = p.ask("GitHub verified/noreply email (blank for preview)", c.Author.Email)
	if p.err != nil {
		return fmt.Errorf("wizard input ended: %w", p.err)
	}
	o.config = c
	if !o.offline && githubAuthenticated(ctx) {
		o.fetch = p.yes("Fetch existing GitHub contributions", true)
	}
	if p.err != nil {
		return p.err
	}
	plan, err := a.buildPlan(ctx, o)
	if err != nil {
		return err
	}
	if configureOnly {
		fmt.Fprintf(a.Out, "Saved configuration: %s\n", o.configPath)
		return nil
	}
	return a.wizardMenu(ctx, o, p, plan)
}

func (a *App) wizardDates(p *prompts, c *forge.Config) {
	choice := p.choice("Period: 30 / 90 / 180 / 365 / custom", "365", "30", "90", "180", "365", "custom")
	c.AsOf = a.Now().Format(time.RFC3339)
	if choice == "custom" {
		c.Start = p.ask("Start date YYYY-MM-DD", c.Start)
		c.End = p.ask("End date YYYY-MM-DD", c.End)
		return
	}
	n, _ := strconv.Atoi(choice)
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		p.err = err
		return
	}
	end := a.Now().In(loc)
	c.Start = end.AddDate(0, 0, 1-n).Format(time.DateOnly)
	c.End = end.Format(time.DateOnly)
}

func wizardDaily(p *prompts, c *forge.Config) {
	c.Mean = p.number("Mean active weekday commits (0 = intensity)", c.Mean, 0, 500)
	c.Min = p.integer("Minimum active day target", c.Min, 1, 500)
	c.Max = p.integer("Maximum day target", max(c.Max, c.Min), c.Min, 500)
	c.InactivePercent = p.number("Inactive weekdays %", c.InactivePercent, 0, 100)
}

func wizardWeekends(p *prompts, c *forge.Config) {
	c.Weekends.Mode = p.choice("Weekend mode", c.Weekends.Mode, "off", "rare", "light", "moderate", "same", "custom")
	if c.Weekends.Mode != "custom" {
		return
	}
	for i, d := range []*forge.WeekendDay{&c.Weekends.Saturday, &c.Weekends.Sunday} {
		label := []string{"Saturday", "Sunday"}[i]
		d.Probability = p.number(label+" probability (0–1)", d.Probability, 0, 1)
		d.Multiplier = p.number(label+" workload multiplier", d.Multiplier, 0, 5)
		d.Max = p.integer(label+" maximum", d.Max, 0, 500)
	}
}

func wizardEffects(p *prompts, c *forge.Config) {
	e := &c.Effects
	e.WorkBursts = p.yes("Work bursts", e.WorkBursts)
	e.RestPeriods = p.yes("Rest periods", e.RestPeriods)
	e.Streaks = p.yes("Streaks", e.Streaks)
	e.MonthlyVariation = p.yes("Smooth monthly variation", e.MonthlyVariation)
	e.VacationCount = p.integer("Vacation count (0 = off)", e.VacationCount, 0, 50)
	if e.VacationCount > 0 {
		e.VacationMin = p.integer("Min vacation days", max(1, e.VacationMin), 1, 365)
		e.VacationMax = p.integer("Max vacation days", max(e.VacationMin, e.VacationMax), e.VacationMin, 365)
	}
	e.Trend = p.choice("Trend", e.Trend, "stable", "increasing", "decreasing", "wave", "random")
}

func wizardClock(p *prompts, c *forge.Config) {
	c.Clock.Start = p.ask("Workday start HH:MM", c.Clock.Start)
	c.Clock.End = p.ask("Workday end HH:MM", c.Clock.End)
	c.Clock.EveningProbability = p.number("Evening probability (0–1)", c.Clock.EveningProbability, 0, 1)
	c.Clock.NightProbability = p.number("Night probability (0–1)", c.Clock.NightProbability, 0, 1-c.Clock.EveningProbability)
	c.Clock.Spread = p.integer("Session spread in minutes", c.Clock.Spread, 1, 240)
}

func githubAuthenticated(ctx context.Context) bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "gh", "auth", "status", "--active").Run() == nil
}

func (a *App) wizardMenu(ctx context.Context, o options, p *prompts, plan forge.Plan) error {
	for ctx.Err() == nil {
		if err := a.terminal(o, plan); err != nil {
			return err
		}
		fmt.Fprint(a.Out, "\n  1 Generate commits     2 Adjust intensity     3 Adjust randomness\n  4 Configure weekends   5 New seed             6 Change dates\n  7 Export preview       8 Browser preview      9 Save plan\n  0 Cancel\n")
		choice := p.choice("What would you like to do", "0", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9")
		if p.err != nil {
			return p.err
		}
		if choice == "0" {
			fmt.Fprintln(a.Out, "  Configuration saved. No remote changes were made.")
			return nil
		}
		if choice == "1" {
			return a.wizardGenerate(ctx, o, p, plan)
		}
		if choice == "7" {
			if err := a.wizardExport(o, p, plan); err != nil {
				return err
			}
			continue
		}
		if choice == "8" {
			o.open = true
			return a.serve(ctx, o, render.View{Plan: plan, Kind: "preview"})
		}
		if choice == "9" {
			path, err := a.save(o, plan, "")
			if err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "  Saved immutable plan: %s\n", path)
			continue
		}
		c := plan.Config
		switch choice {
		case "2":
			c.Intensity = p.integer("Intensity", c.Intensity, 1, 10)
		case "3":
			c.Randomness = p.number("Randomness %", c.Randomness, 0, 100)
		case "4":
			wizardWeekends(p, &c)
		case "5":
			c.Seed = rand.New(rand.NewSource(a.Now().UnixNano())).Int63n(1 << 52)
			fmt.Fprintf(a.Out, "  New seed: %d\n", c.Seed)
		case "6":
			a.wizardDates(p, &c)
		}
		if p.err != nil {
			return p.err
		}
		o.config = c
		var err error
		plan, err = a.buildPlan(ctx, o)
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (a *App) wizardGenerate(ctx context.Context, o options, p *prompts, plan forge.Plan) error {
	c := plan.Config
	if c.Author.Name == "" {
		c.Author.Name = p.ask("Author name (required)", "")
	}
	if c.Author.Email == "" {
		c.Author.Email = p.ask("GitHub verified/noreply author email (required)", "")
	}
	if p.err != nil {
		return p.err
	}
	if err := gitstore.ValidIdentity(c.Author.Name, c.Author.Email); err != nil {
		return err
	}
	var err error
	plan, err = forge.Generate(c, plan.Existing)
	if err != nil {
		return err
	}
	path, err := a.save(o, plan, "")
	if err != nil {
		return err
	}
	o.plan = path
	o.explicit = map[string]bool{}
	return a.generate(ctx, o)
}

func (a *App) wizardExport(o options, p *prompts, plan forge.Plan) error {
	o.format = p.choice("Format", "html", "html", "svg", "png", "json", "csv")
	o.output = p.ask("Output path", "activity-preview."+o.format)
	if p.err != nil {
		return p.err
	}
	return a.export(o, render.View{Plan: plan, Kind: "preview"})
}
