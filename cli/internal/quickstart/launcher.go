package quickstart

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/github"
)

type Launcher struct {
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	Root   string
	Now    func() time.Time
	GitHub github.Client
	reader *bufio.Reader
}

func (l *Launcher) Run(ctx context.Context) error {
	if l.In == nil {
		l.In = os.Stdin
	}
	if l.Out == nil {
		l.Out = os.Stdout
	}
	if l.Err == nil {
		l.Err = os.Stderr
	}
	if l.Now == nil {
		l.Now = time.Now
	}
	if l.Root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		l.Root = filepath.Join(base, "activityforge-easy")
	}
	l.reader = bufio.NewReader(l.In)
	fmt.Fprintln(l.Out, "\nACTIVITY FORGE — красивый декоративный календарь GitHub")
	fmt.Fprintln(l.Out, "Сначала выбираешь вид в браузере. Отправка — только после Approve & Push.")
	for ctx.Err() == nil {
		fmt.Fprint(l.Out, "\n1. Войти в свой GitHub\n2. Выбрать календарь в браузере\n3. Создать коммиты и открыть финальное подтверждение\n4. Проверить сохранённую историю\n0. Выйти\n\nНомер: ")
		choice, err := l.read()
		if errors.Is(err, io.EOF) || choice == "0" {
			return nil
		}
		if err != nil {
			return err
		}
		if err := l.action(ctx, choice); err != nil {
			fmt.Fprintf(l.Err, "\nНе получилось: %v\n", err)
		}
	}
	return ctx.Err()
}

func (l *Launcher) read() (string, error) {
	line, err := l.reader.ReadString('\n')
	if len(line) > 0 {
		return strings.TrimSpace(line), nil
	}
	return "", err
}

func (l *Launcher) action(ctx context.Context, choice string) error {
	if choice != "1" && choice != "2" && choice != "3" && choice != "4" {
		fmt.Fprintln(l.Out, "Выбери 1, 2, 3, 4 или 0.")
		return nil
	}
	if err := l.dependencies(); err != nil {
		return err
	}
	if choice == "1" {
		return l.login(ctx)
	}
	account, err := l.GitHub.Account(ctx)
	if err != nil {
		return fmt.Errorf("choose 1 to log in to your GitHub account: %w", err)
	}
	if err := github.ValidateTarget(account.Login + "/activity-calendar"); err != nil {
		return err
	}
	fmt.Fprintf(l.Out, "\nАккаунт: @%s\n", account.Login)
	if choice == "2" {
		return l.preview(ctx, account)
	}
	if choice == "3" {
		return l.publish(ctx, account)
	}
	return l.status(ctx, account)
}

func (l *Launcher) dependencies() error {
	missing := []string{}
	for _, name := range []string{"git", "gh"} {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	fmt.Fprintln(l.Out, "\nОдин раз установи Git и GitHub CLI, затем перезапусти программу:")
	if runtime.GOOS == "windows" {
		fmt.Fprintln(l.Out, "PowerShell:\n  winget install --id Git.Git -e --source winget\n  winget install --id GitHub.cli -e --source winget")
	}
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(l.Out, "Terminal:\n  brew install git gh\nЕсли Homebrew ещё нет: https://brew.sh")
	}
	if runtime.GOOS == "linux" {
		fmt.Fprintln(l.Out, "Ubuntu/Debian:\n  sudo apt update\n  sudo apt install git gh xdg-utils")
	}
	return fmt.Errorf("missing programs: %s", strings.Join(missing, ", "))
}

func (l *Launcher) login(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	fmt.Fprintln(l.Out, "Войди через браузер именно в СВОЙ аккаунт GitHub.")
	cmd := exec.CommandContext(ctx, "gh", "auth", "login", "--hostname", "github.com", "--git-protocol", "https", "--web")
	// Preserve the actual console descriptor: gh needs a terminal, and wrapping
	// stdin in a reader makes os/exec wait for a background copy until EOF.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = l.In, l.Out, l.Err
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("github login failed: %w", err)
	}
	return nil
}
