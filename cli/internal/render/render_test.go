package render

import (
	"bytes"
	"encoding/csv"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

func sample(t *testing.T) forge.Plan {
	t.Helper()
	c := forge.DefaultConfig(time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC))
	c.Start, c.End = "2026-01-04", "2026-01-06"
	c.Mean, c.Min, c.Max, c.InactivePercent = 2, 2, 2, 0
	c.Weekends.Mode, c.Effects, c.Randomness = "same", forge.Effects{}, 0
	p, err := forge.Generate(c, forge.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSundayAlignmentAndTerminalWrapping(t *testing.T) {
	t.Parallel()
	p := sample(t)
	cells, weeks := Layout(p.Days)
	if weeks != 1 || len(cells) != 3 || cells[0].Row != 0 || cells[2].Row != 2 || cells[2].Column != 0 {
		t.Fatalf("wrong layout: %+v", cells)
	}
	var out bytes.Buffer
	if err := Terminal(&out, p, 25, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b[") || !strings.Contains(out.String(), "Mon") || !strings.Contains(out.String(), "Planned new commits: 6") {
		t.Fatalf("bad terminal output: %s", out.String())
	}
}

func TestExportsContainActualDailyPlan(t *testing.T) {
	t.Parallel()
	p := sample(t)
	for _, format := range []string{"json", "csv", "svg", "html", "png"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			var b bytes.Buffer
			if err := Export(&b, View{Plan: p, Kind: "preview"}, format); err != nil {
				t.Fatal(err)
			}
			switch format {
			case "png":
				img, err := png.Decode(bytes.NewReader(b.Bytes()))
				if err != nil || img.Bounds().Dx() < 200 {
					t.Fatalf("invalid PNG: %v", err)
				}
			case "csv":
				rows, err := csv.NewReader(strings.NewReader(b.String())).ReadAll()
				if err != nil || len(rows) != 4 || rows[1][0] != "2026-01-04" || rows[1][3] != "2" {
					t.Fatalf("bad CSV: %v %v", rows, err)
				}
			case "html":
				if !strings.Contains(b.String(), p.ID) || !strings.Contains(b.String(), "\"offline\":true") || !strings.Contains(b.String(), "CURRENT") {
					t.Fatal("HTML is not a standalone data-backed comparison")
				}
			case "svg":
				if !strings.Contains(b.String(), "2026-01-04") || !strings.Contains(b.String(), "2 new") {
					t.Fatal("SVG does not reflect plan")
				}
			case "json":
				got, err := forge.PlanID(p)
				if err != nil || !strings.Contains(b.String(), got) {
					t.Fatal("JSON lost plan")
				}
			}
		})
	}
}
