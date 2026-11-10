package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/gitstore"
	"github.com/activityforge/activityforge/internal/render"
)

func webPlan(t *testing.T) forge.Plan {
	t.Helper()
	c := forge.DefaultConfig(time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC))
	c.Start, c.End = "2026-01-01", "2026-01-10"
	p, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func doRequest(t *testing.T, s *Server, method, path, body, token, origin, host string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	r.Host = host
	r.Header.Set("X-ActivityForge-Token", token)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.Handler("127.0.0.1:7010").ServeHTTP(w, r)
	return w
}

func TestLoopbackSecurityAndFrozenPlans(t *testing.T) {
	t.Parallel()
	p := webPlan(t)
	s, err := New(Options{View: render.View{Plan: p, Kind: "preview"}, Save: func(_ context.Context, p forge.Plan) (string, error) { return "/tmp/" + p.ID + ".json", nil }})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, token, origin, host string }{
		{"no token", "", "", "127.0.0.1:7010"}, {"foreign origin", s.Token(), "https://evil.example", "127.0.0.1:7010"}, {"dns rebinding", s.Token(), "", "evil.example:7010"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if w := doRequest(t, s, "POST", "/api/freeze", `{"id":"`+p.ID+`"}`, tc.token, tc.origin, tc.host); w.Code != http.StatusForbidden {
				t.Fatalf("unauthorized request: %d %s", w.Code, w.Body)
			}
		})
	}
	w := doRequest(t, s, "POST", "/api/freeze", `{"id":"`+p.ID+`"}`, s.Token(), "http://127.0.0.1:7010", "127.0.0.1:7010")
	if w.Code != 200 {
		t.Fatalf("freeze failed: %d %s", w.Code, w.Body)
	}
	body, _ := json.Marshal(map[string]any{"config": p.Config, "revision": 1})
	if w = doRequest(t, s, "POST", "/api/preview", string(body), s.Token(), "", "127.0.0.1:7010"); w.Code != 409 {
		t.Fatalf("frozen plan was edited: %d %s", w.Code, w.Body)
	}
}

func TestGoBackendRecalculatesAndRejectsStaleRequests(t *testing.T) {
	t.Parallel()
	p := webPlan(t)
	s, err := New(Options{View: render.View{Plan: p, Kind: "preview"}})
	if err != nil {
		t.Fatal(err)
	}
	c := p.Config
	c.Seed++
	body, _ := json.Marshal(map[string]any{"config": c, "revision": 2})
	w := doRequest(t, s, "POST", "/api/preview", string(body), s.Token(), "", "127.0.0.1:7010")
	if w.Code != 200 {
		t.Fatalf("recalculation failed: %s", w.Body)
	}
	var v render.View
	if err = json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Plan.ID == p.ID {
		t.Fatal("backend did not update plan")
	}
	body, _ = json.Marshal(map[string]any{"config": p.Config, "revision": 1})
	if w = doRequest(t, s, "POST", "/api/preview", string(body), s.Token(), "", "127.0.0.1:7010"); w.Code != 409 {
		t.Fatal("stale update accepted")
	}
	if w = doRequest(t, s, "GET", "/api/push", "", s.Token(), "", "127.0.0.1:7010"); w.Code != 405 {
		t.Fatal("GET can mutate state")
	}
}

func TestApprovalIsBoundToVerifiedPlanAndOnlyRunsOnce(t *testing.T) {
	t.Parallel()
	p := webPlan(t)
	calls := 0
	s, err := New(Options{View: render.View{Plan: p, Kind: "publish", Frozen: true, CanPush: true, Verification: &gitstore.Report{OK: true, Head: "verified-head"}}, Push: func(context.Context) error { calls++; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"id":"wrong","head":"verified-head"}`, `{"id":"` + p.ID + `","head":"stale-head"}`} {
		if w := doRequest(t, s, "POST", "/api/push", body, s.Token(), "", "127.0.0.1:7010"); w.Code != 409 {
			t.Fatal("stale approval accepted")
		}
	}
	if calls != 0 {
		t.Fatal("push ran without valid approval")
	}
	good := `{"id":"` + p.ID + `","head":"verified-head"}`
	if w := doRequest(t, s, "POST", "/api/push", good, s.Token(), "", "127.0.0.1:7010"); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := doRequest(t, s, "POST", "/api/push", good, s.Token(), "", "127.0.0.1:7010"); w.Code != 409 {
		t.Fatal("duplicate approval accepted")
	}
	if calls != 1 {
		t.Fatalf("push calls = %d", calls)
	}
}

func TestReloadBootstrapsCurrentPreviewRevision(t *testing.T) {
	t.Parallel()
	p := webPlan(t)
	s, err := New(Options{View: render.View{Plan: p, Kind: "preview"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"config": p.Config, "revision": 5})
	if w := doRequest(t, s, "POST", "/api/preview", string(body), s.Token(), "", "127.0.0.1:7010"); w.Code != 200 {
		t.Fatal(w.Body)
	}
	w := doRequest(t, s, "GET", "/", "", s.Token(), "", "127.0.0.1:7010")
	_, script, ok := strings.Cut(w.Body.String(), "window.BOOTSTRAP=")
	if !ok {
		t.Fatal("missing bootstrap")
	}
	data, _, ok := strings.Cut(script, ";</script>")
	if !ok {
		t.Fatal("invalid bootstrap")
	}
	var boot struct {
		Revision int64 `json:"revision"`
	}
	if err = json.Unmarshal([]byte(data), &boot); err != nil {
		t.Fatal(err)
	}
	if boot.Revision != 5 {
		t.Fatalf("browser reload reset revision: %d", boot.Revision)
	}
}

func TestReturnFromPublicationToSettingsDisarmsOldApproval(t *testing.T) {
	t.Parallel()
	p := webPlan(t)
	pushes := 0
	s, err := New(Options{View: render.View{Plan: p, Kind: "publish", Frozen: true, CanPush: true, Verification: &gitstore.Report{OK: true, Head: "approved-head"}}, Push: func(context.Context) error { pushes++; return nil }, Save: func(_ context.Context, p forge.Plan) (string, error) { return "saved.json", nil }})
	if err != nil {
		t.Fatal(err)
	}
	w := doRequest(t, s, "POST", "/api/draft", "{}", s.Token(), "", "127.0.0.1:7010")
	if w.Code != 200 {
		t.Fatalf("cannot return to settings: %d %s", w.Code, w.Body)
	}
	var view render.View
	if err = json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Kind != "preview" || view.Frozen || view.CanPush || view.Verification != nil {
		t.Fatalf("publication remains armed: %+v", view)
	}
	if err = forge.ValidatePlan(view.Plan); err != nil {
		t.Fatal(err)
	}
	approval := `{"id":"` + p.ID + `","head":"approved-head"}`
	if w = doRequest(t, s, "POST", "/api/push", approval, s.Token(), "", "127.0.0.1:7010"); w.Code != 409 || pushes != 0 {
		t.Fatal("old publication callback remained usable")
	}
	if w = doRequest(t, s, "POST", "/api/freeze", `{"id":"`+view.Plan.ID+`"}`, s.Token(), "", "127.0.0.1:7010"); w.Code != 200 {
		t.Fatal("new draft cannot be saved")
	}
}
