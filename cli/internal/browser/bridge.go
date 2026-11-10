// Package browser exposes the original generator without filesystem operations.
package browser

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

type Request struct {
	Action   string         `json:"action"`
	Profile  string         `json:"profile"`
	Config   forge.Config   `json:"config"`
	Existing forge.Snapshot `json:"existing"`
}

type Response struct {
	Error   string        `json:"error,omitempty"`
	Config  *forge.Config `json:"config,omitempty"`
	Plan    *forge.Plan   `json:"plan,omitempty"`
	Payload string        `json:"payload,omitempty"`
}

func Evaluate(input string) string {
	response, err := evaluate(input)
	if err != nil {
		response = Response{Error: err.Error()}
	}
	data, err := json.Marshal(response)
	if err != nil {
		return `{"error":"failed to encode generator response"}`
	}
	return string(data)
}

func evaluate(input string) (Response, error) {
	if len(input) > 2<<20 {
		return Response{}, fmt.Errorf("request exceeds browser limit")
	}
	var r Request
	if err := forge.Decode([]byte(input), &r); err != nil {
		return Response{}, err
	}
	if r.Action == "defaults" {
		c := forge.DefaultConfig(time.Now())
		c.Preset, c.Intensity, c.Mean, c.Max = "custom", 3, 1.6, 9
		c.Randomness, c.InactivePercent, c.Weekends.Mode = 80, 26, "rare"
		c.Effects.Trend = "wave"
		if r.Profile == "waves" {
			c.Mean, c.Randomness, c.InactivePercent = 2.1, 52, 15
		}
		if r.Profile == "islands" {
			c.Mean, c.Randomness, c.InactivePercent = 2.7, 95, 42
			c.Weekends.Mode = "off"
		}
		return Response{Config: &c}, nil
	}
	if r.Action != "generate" {
		return Response{}, fmt.Errorf("unknown browser action")
	}
	a, err := time.Parse(time.DateOnly, r.Config.Start)
	if err != nil {
		return Response{}, err
	}
	b, err := time.Parse(time.DateOnly, r.Config.End)
	if err != nil || b.Sub(a) > 365*24*time.Hour {
		return Response{}, fmt.Errorf("browser period must not exceed 366 days")
	}
	p, err := forge.Generate(r.Config, r.Existing)
	if err != nil {
		return Response{}, err
	}
	if p.Stats.Planned > 4000 {
		return Response{}, fmt.Errorf("web publication is limited to 4000 commits; reduce intensity")
	}
	canonical := p
	canonical.ID = ""
	payload, err := json.Marshal(canonical)
	return Response{Plan: &p, Payload: string(payload)}, err
}
