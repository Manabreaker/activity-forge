package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (c Client) authToken() string {
	if c.Token != nil {
		return c.Token()
	}
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GITHUB_TOKEN")
}

// apiHTTP is an optional standard-library fallback when gh is not installed.
// Credentials remain in memory and per-process environment, never config/plans.
func (c Client) apiHTTP(ctx context.Context, args ...string) ([]byte, error) {
	if c.authToken() == "" {
		return nil, fmt.Errorf("gh is unavailable; install gh and run gh auth login, or set GH_TOKEN/GITHUB_TOKEN (never save tokens in config)")
	}
	method, path, body, err := apiRequest(args)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var input io.Reader
	if body != nil {
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com/"+path, input)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken())
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "activityforge/1.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(result) > 16<<20 {
		return nil, fmt.Errorf("GitHub response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var message struct {
			Message string `json:"message"`
		}
		json.Unmarshal(result, &message)
		return nil, fmt.Errorf("GitHub API HTTP %d: %s", resp.StatusCode, message.Message)
	}
	return result, nil
}

func apiRequest(args []string) (string, string, any, error) {
	if len(args) == 2 && args[0] == "api" && (args[1] == "user" || args[1] == "user/emails" || strings.HasPrefix(args[1], "repos/")) {
		return "GET", args[1], nil, nil
	}
	if len(args) >= 2 && args[0] == "api" && args[1] == "graphql" {
		variables := map[string]string{}
		query := ""
		for i := 2; i < len(args); i += 2 {
			if args[i] != "-f" || i+1 >= len(args) {
				return "", "", nil, fmt.Errorf("invalid GraphQL argument")
			}
			key, value, ok := strings.Cut(args[i+1], "=")
			if !ok {
				return "", "", nil, fmt.Errorf("invalid GraphQL field")
			}
			if key == "query" {
				query = value
				continue
			}
			variables[key] = value
		}
		return "POST", "graphql", map[string]any{"query": query, "variables": variables}, nil
	}
	if len(args) == 4 && args[0] == "repo" && args[1] == "create" && args[3] == "--private" {
		if err := ValidateTarget(args[2]); err != nil {
			return "", "", nil, err
		}
		_, name, _ := strings.Cut(args[2], "/")
		return "POST", "user/repos", map[string]any{"name": name, "private": true, "auto_init": false}, nil
	}
	return "", "", nil, fmt.Errorf("operation is unavailable without gh")
}
