package gist

import (
	"time"

	"github.com/qingyu-org/Gist/internal/db"
)

// HeatmapCell is one day in a contribution heatmap.
type HeatmapCell struct {
	Date     string
	Label    string
	Count    int
	Level    int
	InRange  bool
	Weekday  int
	IsFuture bool
	IsToday  bool
}

// HeatmapWeek is a column of seven days, Monday first.
type HeatmapWeek struct {
	Cells []HeatmapCell
}

// HeatmapData is everything the heatmap partial needs to render.
type HeatmapData struct {
	Weeks      []HeatmapWeek
	Months     []HeatmapMonth
	Bands      []HeatmapBand
	Total      int
	Max        int
	ActiveDays int
	BestCount  int
	BestLabel  string
	FromLabel  string
	ToLabel    string
	Days       int
}

// HeatmapBand is a horizontal slice of the calendar used on narrow screens,
// where showing a full year in one row would make every cell unreadably small.
type HeatmapBand struct {
	Weeks  []HeatmapWeek
	Months []HeatmapMonth
}

// HeatmapMonth marks the first week of a month so the template can render a
// month label above the grid without JavaScript. Column is one-based so it can
// be used directly in a CSS grid-column declaration.
type HeatmapMonth struct {
	Week   int
	Column int
	Span   int
	Month  int
}

const heatmapDays = 371 // 53 weeks, enough to always cover a full year view

// buildHeatmap aggregates activity for a user (userID != 0) or the whole
// instance (userID == 0) into Monday-first weeks ending today.
func buildHeatmap(userID uint) *HeatmapData {
	today := time.Now()
	rangeStart := today.AddDate(0, 0, -(heatmapDays - 1))
	start := rangeStart
	// Rewind to the Monday of the first week so every column has seven cells.
	for start.Weekday() != time.Monday {
		start = start.AddDate(0, 0, -1)
	}
	end := today.AddDate(0, 0, 6-int(today.Weekday())) // Sunday of the current week

	rows, err := db.GetUserActivity(userID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		rows = nil
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Day] = row.Count
	}

	data := &HeatmapData{
		FromLabel: start.Format("2006-01-02"),
		ToLabel:   today.Format("2006-01-02"),
		Days:      heatmapDays,
	}

	day := start
	var week HeatmapWeek
	for !day.After(end) {
		date := day.Format("2006-01-02")
		inRange := !day.Before(rangeStart) && !day.After(today)
		count := counts[date]
		cell := HeatmapCell{
			Date:     date,
			Label:    day.Format("2006-01-02"),
			Count:    count,
			Level:    heatmapLevel(count),
			InRange:  inRange,
			Weekday:  int(day.Weekday()),
			IsFuture: day.After(today),
			IsToday:  date == today.Format("2006-01-02"),
		}
		if !inRange {
			cell.Count = 0
			cell.Level = 0
		} else {
			data.Total += count
			if count > 0 {
				data.ActiveDays++
			}
			if count > data.BestCount {
				data.BestCount = count
				data.BestLabel = day.Format("2006-01-02")
			}
			if count > data.Max {
				data.Max = count
			}
		}
		week.Cells = append(week.Cells, cell)

		if day.Weekday() == time.Sunday {
			data.Weeks = append(data.Weeks, week)
			week = HeatmapWeek{}
		}
		day = day.AddDate(0, 0, 1)
	}
	if len(week.Cells) > 0 {
		data.Weeks = append(data.Weeks, week)
	}

	// Month labels: mark the week that contains the first Monday of a month.
	seen := ""
	for i, w := range data.Weeks {
		if len(w.Cells) == 0 {
			continue
		}
		month := w.Cells[0].Date[:7]
		if month != seen {
			seen = month
			if i > 0 {
				t, _ := time.Parse("2006-01-02", w.Cells[0].Date)
				data.Months = append(data.Months, HeatmapMonth{Week: i, Column: i + 1, Month: int(t.Month())})
			}
		}
	}
	// A label spans up to the next month; cap it so a stray label can never
	// cover the whole row.
	for i := range data.Months {
		end := len(data.Weeks)
		if i+1 < len(data.Months) {
			end = data.Months[i+1].Week
		}
		span := end - data.Months[i].Week
		if span < 1 {
			span = 1
		}
		if span > 5 {
			span = 5
		}
		data.Months[i].Span = span
	}

	// Split the year into two bands for narrow screens. Months are re-based on
	// each band so a label never points outside its own grid.
	data.Bands = splitHeatmapBands(data.Weeks, data.Months)

	return data
}

// splitHeatmapBands divides the calendar into two horizontal bands and rebases
// each month label on its band. It is pure so the responsive layout can be
// tested without a database.
func splitHeatmapBands(weeks []HeatmapWeek, months []HeatmapMonth) []HeatmapBand {
	if len(weeks) == 0 {
		return nil
	}
	size := (len(weeks) + 1) / 2
	bands := make([]HeatmapBand, 0, 2)
	for start := 0; start < len(weeks); start += size {
		end := start + size
		if end > len(weeks) {
			end = len(weeks)
		}
		band := HeatmapBand{Weeks: weeks[start:end]}
		for _, month := range months {
			if month.Week < start || month.Week >= end {
				continue
			}
			month.Column = month.Week - start + 1
			if weekSpan := end - month.Week; month.Span > weekSpan {
				month.Span = weekSpan
			}
			band.Months = append(band.Months, month)
		}
		bands = append(bands, band)
	}
	return bands
}

// heatmapLevel maps a daily count to one of five visual intensity levels.
func heatmapLevel(count int) int {
	switch {
	case count <= 0:
		return 0
	case count == 1:
		return 1
	case count <= 3:
		return 2
	case count <= 6:
		return 3
	default:
		return 4
	}
}
