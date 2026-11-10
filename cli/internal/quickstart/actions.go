package quickstart

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"

	"github.com/activityforge/activityforge/internal/cli"
	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/github"
)

func (l *Launcher) preview(ctx context.Context, account github.Account) error {
	fmt.Fprint(l.Out, "\n1. Мягкие волны\n2. Короткие всплески — рекомендую\n3. Островки активности\n\nВариант [2]: ")
	line, err := l.read()
	if err != nil {
		return err
	}
	if line == "" {
		line = "2"
	}
	profile, err := strconv.Atoi(line)
	if err != nil || profile < 1 || profile > 3 {
		return fmt.Errorf("choose a profile from 1 to 3")
	}
	state := accountState(l.Root, account)
	path := filepath.Join(state, "draft.json")
	if err := forge.WriteJSON(path, draftConfig(l.Now(), account, profile)); err != nil {
		return err
	}
	fmt.Fprintln(l.Out, "\nВ браузере настрой вид и нажми Save this plan.")
	fmt.Fprintln(l.Out, "Затем вернись в это окно и нажми Ctrl+C: вернёшься в меню.")
	fmt.Fprintln(l.Out, "Если браузер не открылся, скопируй ссылку Visual Preview ниже.")
	return l.runCLI(ctx, "preview", "--state-dir", state, "--config", path, "--fetch", "--open")
}

func (l *Launcher) publish(ctx context.Context, account github.Account) error {
	p, path, err := savedPlan(l.Root, account)
	if err != nil {
		return err
	}
	if _, err := l.GitHub.VerifyEmail(ctx, p.Config.Author.Email); err != nil {
		return err
	}
	state := accountState(l.Root, account)
	repo := filepath.Join(state, "history", p.ID)
	fmt.Fprintf(l.Out, "\nВыбранный календарь: %d новых коммитов, прогноз %d contributions.\n", p.Stats.Planned, p.Stats.Total)
	fmt.Fprintf(l.Out, "Цель: приватный https://github.com/%s/activity-calendar\n", account.Login)
	fmt.Fprintln(l.Out, "Сейчас создаются локальные коммиты. В браузере появится финальный Approve & Push.")
	args := []string{"--state-dir", state, "--repo", repo, "--plan", path}
	if err := l.runCLI(ctx, append([]string{"generate"}, args...)...); err != nil {
		return err
	}
	pub := append([]string{"publish"}, args...)
	pub = append(pub, "--remote", account.Login+"/activity-calendar", "--create-private")
	return l.runCLI(ctx, pub...)
}

func (l *Launcher) status(ctx context.Context, account github.Account) error {
	p, path, err := savedPlan(l.Root, account)
	if err != nil {
		return err
	}
	state := accountState(l.Root, account)
	repo := filepath.Join(state, "history", p.ID)
	fmt.Fprintf(l.Out, "Локальные данные: %s\n", state)
	return l.runCLI(ctx, "status", "--state-dir", state, "--repo", repo, "--plan", path, "--offline")
}

func (l *Launcher) runCLI(ctx context.Context, args ...string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	a := cli.App{In: l.reader, Out: l.Out, Err: l.Err, Now: l.Now, GitHub: l.GitHub}
	return a.Run(ctx, args)
}
