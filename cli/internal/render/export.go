package render

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"strconv"

	"github.com/activityforge/activityforge/internal/forge"
	"github.com/activityforge/activityforge/internal/gitstore"
)

type View struct {
	Revision     int64            `json:"revision"`
	CanFetch     bool             `json:"can_fetch"`
	Plan         forge.Plan       `json:"plan"`
	Kind         string           `json:"kind"`
	Frozen       bool             `json:"frozen"`
	SavedPath    string           `json:"saved_path,omitempty"`
	Verification *gitstore.Report `json:"verification,omitempty"`
	Target       string           `json:"target,omitempty"`
	Pending      int              `json:"pending"`
	CanPush      bool             `json:"can_push"`
	Published    bool             `json:"published"`
	Notice       string           `json:"notice,omitempty"`
}

func Export(w io.Writer, v View, format string) error {
	switch format {
	case "json":
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		if v.Kind == "inspect" || v.Kind == "publish" {
			return e.Encode(v)
		}
		return e.Encode(v.Plan)
	case "csv":
		return exportCSV(w, v.Plan)
	case "svg":
		return exportSVG(w, v.Plan)
	case "png":
		return exportPNG(w, v.Plan)
	case "html":
		return Page(w, v, true, "")
	}
	return fmt.Errorf("unsupported export %q; choose json, csv, svg, html or png", format)
}

func exportCSV(w io.Writer, p forge.Plan) error {
	c := csv.NewWriter(w)
	if err := c.Write([]string{"date", "target", "existing", "new", "projected", "existing_known", "timestamps"}); err != nil {
		return err
	}
	for _, d := range p.Days {
		stamps, _ := json.Marshal(d.Timestamps)
		if err := c.Write([]string{d.Date, strconv.Itoa(d.Target), strconv.Itoa(d.Existing), strconv.Itoa(d.New), strconv.Itoa(d.Projected), strconv.FormatBool(p.Existing.Known), string(stamps)}); err != nil {
			return err
		}
	}
	c.Flush()
	return c.Error()
}

func dimensions(weeks int) (int, int) {
	return max(800, min(53, weeks)*16+100), ((max(1, weeks)+52)/53)*155 + 140
}

func exportSVG(w io.Writer, p forge.Plan) error {
	cells, weeks := Layout(p.Days)
	width, height := dimensions(weeks)
	levels := Levels(p.Days)
	months := monthColumns(cells, 53)
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d"><rect width="100%%" height="100%%" fill="#0d1117"/><g font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" fill="#e6edf3"><text x="28" y="32" font-size="19">%d projected contributions</text><text x="28" y="55" font-size="12" fill="#9198a1">%s to %s · Activity Forge</text>`, width, height, width, height, p.Stats.Total, html.EscapeString(p.Config.Start), html.EscapeString(p.Config.End))
	for block := 0; block < (weeks+52)/53; block++ {
		for row, label := range []string{"", "Mon", "", "Wed", "", "Fri", ""} {
			if label != "" {
				fmt.Fprintf(w, `<text x="28" y="%d" font-size="11" fill="#9198a1">%s</text>`, 91+block*155+row*16, label)
			}
		}
	}
	for col, label := range months {
		fmt.Fprintf(w, `<text x="%d" y="%d" font-size="11" fill="#9198a1">%s</text>`, 70+(col%53)*16, 70+(col/53)*155, label)
	}
	for _, cell := range cells {
		x, y := 70+(cell.Column%53)*16, 80+(cell.Column/53)*155+cell.Row*16
		fmt.Fprintf(w, `<rect x="%d" y="%d" width="12" height="12" rx="2" fill="%s" stroke="#ffffff12"><title>%s: %d existing, %d new, %d projected</title></rect>`, x, y, Dark[levels[cell.Day.Projected]], cell.Day.Date, cell.Day.Existing, cell.Day.New, cell.Day.Projected)
	}
	fmt.Fprintf(w, `<text x="28" y="%d" font-size="12" fill="#9198a1">%d existing · %d new · %d active days · longest streak %d days</text><text x="28" y="%d" font-size="10" fill="#9198a1">Projected colors are estimates. Existing activity %s.</text></g></svg>`, height-32, p.Stats.Existing, p.Stats.Planned, p.Stats.ActiveDays, p.Stats.LongestStreak, height-12, map[bool]string{true: "loaded from GitHub", false: "unknown"}[p.Existing.Known])
	return nil
}

func rgb(hex string) color.RGBA {
	var r, g, b uint8
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}
}

func exportPNG(w io.Writer, p forge.Plan) error {
	cells, weeks := Layout(p.Days)
	width, height := dimensions(weeks)
	levels := Levels(p.Days)
	months := monthColumns(cells, 53)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{rgb("#0d1117")}, image.Point{}, draw.Src)
	ink, muted := rgb("#e6edf3"), rgb("#9198a1")
	drawText(img, 28, 20, fmt.Sprintf("%d PROJECTED CONTRIBUTIONS", p.Stats.Total), 2, ink)
	drawText(img, 28, 47, p.Config.Start+" TO "+p.Config.End+" / ACTIVITY FORGE", 1, muted)
	for block := 0; block < (weeks+52)/53; block++ {
		for row, label := range []string{"", "MON", "", "WED", "", "FRI", ""} {
			drawText(img, 30, 82+block*155+row*16, label, 1, muted)
		}
	}
	for col, label := range months {
		drawText(img, 70+(col%53)*16, 65+(col/53)*155, label, 1, muted)
	}
	for _, cell := range cells {
		x, y := 70+(cell.Column%53)*16, 80+(cell.Column/53)*155+cell.Row*16
		draw.Draw(img, image.Rect(x, y, x+12, y+12), &image.Uniform{rgb(Dark[levels[cell.Day.Projected]])}, image.Point{}, draw.Src)
	}
	drawText(img, 28, height-37, fmt.Sprintf("%d EXISTING / %d NEW / %d ACTIVE DAYS / %d DAY STREAK", p.Stats.Existing, p.Stats.Planned, p.Stats.ActiveDays, p.Stats.LongestStreak), 1, ink)
	drawText(img, 28, height-17, "PROJECTED COLORS ARE ESTIMATES. EXISTING ACTIVITY "+map[bool]string{true: "LOADED", false: "UNKNOWN"}[p.Existing.Known], 1, muted)
	return png.Encode(w, img)
}
