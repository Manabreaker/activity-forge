package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Runner func(context.Context, string, ...string) ([]byte, error)
type Client struct {
	Run        Runner
	APIOnly    bool
	Token      func() string
	HTTPClient *http.Client
}
type Account struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

func (c Client) call(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if c.Run != nil {
		return c.Run(ctx, name, args...)
	}
	if name == "gh" {
		if _, err := exec.LookPath("gh"); err != nil || c.APIOnly {
			return c.apiHTTP(ctx, args...)
		}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	env := make([]string, 0, len(os.Environ())+6)
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GIT_") || strings.HasPrefix(v, "GH_HOST=") {
			continue
		}
		env = append(env, v)
	}
	cmd.Env = append(env, "GH_HOST=github.com", "GH_PROMPT_DISABLED=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
	if token := c.authToken(); name == "git" && token != "" {
		header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
		cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_0="+header)
	}
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return out.Bytes(), nil
}

func (c Client) Account(ctx context.Context) (Account, error) {
	var account Account
	data, err := c.call(ctx, "gh", "api", "user")
	if err != nil {
		return account, fmt.Errorf("GitHub authentication required; run gh auth login: %w", err)
	}
	if err = json.Unmarshal(data, &account); err != nil {
		return account, err
	}
	if account.Login == "" || account.ID <= 0 {
		return account, fmt.Errorf("GitHub returned an invalid account")
	}
	return account, nil
}

func (c Client) VerifyEmail(ctx context.Context, email string) (Account, error) {
	a, err := c.Account(ctx)
	if err != nil {
		return a, err
	}
	for _, allowed := range []string{a.Login + "@users.noreply.github.com", fmt.Sprintf("%d+%s@users.noreply.github.com", a.ID, a.Login)} {
		if strings.EqualFold(email, allowed) {
			return a, nil
		}
	}
	if strings.HasSuffix(strings.ToLower(email), "@users.noreply.github.com") {
		return a, fmt.Errorf("noreply email does not belong to authenticated @%s", a.Login)
	}
	data, err := c.call(ctx, "gh", "api", "user/emails")
	if err != nil {
		return a, fmt.Errorf("cannot verify email (gh may need user:email scope); use %d+%s@users.noreply.github.com: %w", a.ID, a.Login, err)
	}
	var emails []struct {
		Email    string `json:"email"`
		Verified bool   `json:"verified"`
	}
	if err = json.Unmarshal(data, &emails); err != nil {
		return a, err
	}
	for _, entry := range emails {
		if entry.Verified && strings.EqualFold(entry.Email, email) {
			return a, nil
		}
	}
	return a, fmt.Errorf("author email is not a verified address for @%s", a.Login)
}

var targetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9_][A-Za-z0-9_.-]{0,99}$`)

func ValidateTarget(target string) error {
	if !targetPattern.MatchString(target) || strings.HasSuffix(target, ".git") {
		return fmt.Errorf("remote must be owner/repository, without a URL or .git suffix")
	}
	return nil
}
