package render

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

var Dark = []string{"#161b22", "#0e4429", "#006d32", "#26a641", "#39d353"}
var Light = []string{"#eff2f5", "#aceebb", "#4ac26b", "#2da44e", "#116329"}

type Cell struct {
	Day    forge.Day
	Row    int
	Column int
}

func Layout(days []forge.Day) ([]Cell, int) {
	if len(days) == 0 {
		return nil, 0
	}
	start, _ := time.Parse(time.DateOnly, days[0].Date)
	offset := int(start.Weekday())
	cells := make([]Cell, 0, len(days))
	for i, d := range days {
		cells = append(cells, Cell{Day: d, Row: (i + offset) % 7, Column: (i + offset) / 7})
	}
	return cells, (len(days) + offset + 6) / 7
}

func monthColumns(cells []Cell, perBlock int) map[int]string {
	labels := make(map[int]string)
	for _, cell := range cells {
		date, _ := time.Parse(time.DateOnly, cell.Day.Date)
		if cell.Column%perBlock == 0 && labels[cell.Column] == "" {
			labels[cell.Column] = date.Format("Jan")
		}
		if date.Day() == 1 {
			labels[cell.Column] = date.Format("Jan")
			if cell.Column%perBlock == 1 {
				delete(labels, cell.Column-1)
			}
		}
	}
	return labels
}

// Levels uses positive-count quartiles, not fixed arbitrary thresholds.
func Levels(days []forge.Day) map[int]int {
	positive := make([]int, 0, len(days))
	for _, d := range days {
		if d.Projected > 0 {
			positive = append(positive, d.Projected)
		}
	}
	slices.Sort(positive)
	out := map[int]int{0: 0}
	if len(positive) == 0 {
		return out
	}
	q1, q2, q3 := positive[(len(positive)-1)/4], positive[(len(positive)-1)/2], positive[3*(len(positive)-1)/4]
	for _, v := range positive {
		level := 1
		if v > q1 {
			level = 2
		}
		if v > q2 {
			level = 3
		}
		if v > q3 {
			level = 4
		}
		out[v] = level
	}
	return out
}

func Terminal(w io.Writer, p forge.Plan, width int, color bool) error {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  Activity Forge · Activity Preview\n\n  Period: %s → %s  |  %s\n  Seed: %d  ·  Randomness: %.0f%%  ·  Weekends: %s\n\n", p.Config.Start, p.Config.End, p.Config.Preset, p.Config.Seed, p.Config.Randomness, strings.ToUpper(p.Config.Weekends.Mode))
	cells, weeks := Layout(p.Days)
	levels := Levels(p.Days)
	perBlock := max(4, (max(20, width)-7)/2)
	monthLabels := monthColumns(cells, perBlock)
	labels := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	for block := 0; block < weeks; block += perBlock {
		n := min(perBlock, weeks-block)
		grid := make([][]string, 7)
		for row := range grid {
			grid[row] = make([]string, n)
			for col := range grid[row] {
				grid[row][col] = "  "
			}
		}
		for _, cell := range cells {
			col := cell.Column - block
			if col < 0 || col >= n {
				continue
			}
			level := levels[cell.Day.Projected]
			mark := []string{"· ", "░ ", "▒ ", "▓ ", "█ "}[level]
			if color {
				hex := Dark[level]
				var r, g, bl int
				fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &bl)
				mark = fmt.Sprintf("\x1b[48;2;%d;%d;%dm  \x1b[0m", r, g, bl)
			}
			grid[cell.Row][col] = mark
		}
		line := []rune(strings.Repeat(" ", 2*n+1))
		for col := 0; col < n; col++ {
			if label := monthLabels[block+col]; label != "" {
				copy(line[col*2:], []rune(label))
			}
		}
		b.WriteString("      " + strings.TrimRight(string(line), " "))
		b.WriteByte('\n')
		for row := range grid {
			fmt.Fprintf(&b, "  %s %s\n", labels[row], strings.Join(grid[row], ""))
		}
		b.WriteByte('\n')
	}
	s := p.Stats
	existing := fmt.Sprint(s.Existing)
	if !p.Existing.Known {
		existing = "unknown (offline baseline)"
	}
	fmt.Fprintf(&b, "  Existing contributions: %s\n  Planned new commits: %d\n  Projected total: %d\n\n  Active days: %d   Inactive days: %d   Longest streak: %d days\n  Average / active day: %.1f   Maximum daily activity: %d\n  Weekday activity: %.1f%%   Weekend activity: %.1f%%\n\n  Plan: %s\n", existing, s.Planned, s.Total, s.ActiveDays, s.InactiveDays, s.LongestStreak, s.AverageActive, s.MaxDaily, s.WeekdayPercent, s.WeekendPercent, p.ID)
	_, err := io.WriteString(w, b.String())
	return err
}
