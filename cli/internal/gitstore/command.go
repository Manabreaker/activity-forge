package gitstore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func cleanEnv(extra ...string) []string {
	env := make([]string, 0, len(os.Environ())+len(extra)+5)
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GIT_") {
			continue
		}
		env = append(env, v)
	}
	return append(env, append([]string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, extra...)...)
}

func (s Store) command(ctx context.Context, input string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	base := []string{"--no-replace-objects", "-C", s.Repo, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "core.fsmonitor=false"}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = cleanEnv(env...)
	cmd.Stdin = strings.NewReader(input)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func (s Store) Head(ctx context.Context) (string, error) {
	return s.command(ctx, "", nil, "rev-parse", "--verify", "HEAD")
}

func (s Store) Branch(ctx context.Context) (string, error) {
	return s.command(ctx, "", nil, "symbolic-ref", "--short", "HEAD")
}

func ValidIdentity(idName, idEmail string) error {
	if strings.TrimSpace(idName) == "" || strings.ContainsAny(idName, "\n\r\x00<>") {
		return fmt.Errorf("author name is required and cannot contain newlines or angle brackets")
	}
	if strings.TrimSpace(idEmail) != idEmail || !strings.Contains(idEmail, "@") || strings.ContainsAny(idEmail, "\n\r\x00 <>\t") {
		return fmt.Errorf("a valid author email is required; configure it before saving the plan")
	}
	return nil
}
