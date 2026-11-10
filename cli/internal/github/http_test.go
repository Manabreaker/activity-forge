package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTokenAPIFallbackDoesNotRequireGH(t *testing.T) {
	t.Parallel()
	c := Client{APIOnly: true, Token: func() string { return "non-secret-test-placeholder" }, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") == "" {
			t.Fatal("missing scoped API authorization")
		}
		if r.Method != "GET" || r.URL.Path != "/user" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"login":"offline-fixture","id":42}`)), Header: make(http.Header)}, nil
	})}}
	a, err := c.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Login != "offline-fixture" || a.ID != 42 {
		t.Fatalf("fallback account: %+v", a)
	}
}
