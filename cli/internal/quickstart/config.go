package quickstart

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/github"
)

func draftConfig(now time.Time, account github.Account, profile int) forge.Config {
	c := forge.DefaultConfig(now)
	c.Preset, c.Intensity, c.Mean = "custom", 3, 3
	c.Max, c.Randomness, c.InactivePercent = 14, 100, 41
	c.Effects.Streaks, c.Effects.Trend = false, "random"
	c.Seed = now.UnixNano()
	c.Author = forge.Identity{Name: account.Login, Email: fmt.Sprintf("%d+%s@users.noreply.github.com", account.ID, account.Login)}
	if profile == 1 {
		c.Max, c.Randomness, c.InactivePercent = 12, 85, 32
		c.Effects.Streaks, c.Effects.Trend = true, "wave"
	}
	if profile == 3 {
		c.Mean, c.InactivePercent = 3.6, 47
		c.Effects.Trend = "wave"
		c.Effects.VacationCount, c.Effects.VacationMin, c.Effects.VacationMax = 3, 7, 16
	}
	return c
}

func accountState(root string, account github.Account) string {
	return filepath.Join(root, account.Login)
}

func savedPlan(root string, account github.Account) (forge.Plan, string, error) {
	data, err := forge.ReadLimited(filepath.Join(accountState(root, account), "last-plan"), 8192)
	if err != nil {
		return forge.Plan{}, "", fmt.Errorf("first open the preview and click Save this plan: %w", err)
	}
	path := strings.TrimSpace(string(data))
	p, err := forge.LoadPlan(path)
	return p, path, err
}
