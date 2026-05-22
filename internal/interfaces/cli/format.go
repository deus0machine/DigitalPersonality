package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/digital-personality/internal/domain/entity"
)

const (
	maxSnippetLen = 120
	barWidth      = 20
	sepWidth      = 72
)

func printSeparator() {
	fmt.Println(strings.Repeat("─", sepWidth))
}

func printHeader(title string) {
	fmt.Println()
	fmt.Println(title)
	printSeparator()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

func formatDirection(outgoing bool) string {
	if outgoing {
		return "→ outgoing"
	}
	return "← incoming"
}

func formatMatchType(mt string) string {
	switch mt {
	case "fts":
		return "[fts    ]"
	case "trigram":
		return "[trigram]"
	default:
		return "[filter ]"
	}
}

func formatSurface(s entity.PersonalitySurface) string {
	if s == "" {
		return "unknown"
	}
	return string(s)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// bar renders a fixed-width ASCII bar for histogram display.
func bar(value, max, width int) string {
	if max == 0 || value == 0 {
		return ""
	}
	filled := int(float64(value) / float64(max) * float64(width))
	if filled == 0 {
		filled = 1
	}
	return strings.Repeat("█", filled)
}

func pct(value, total int) string {
	if total == 0 {
		return "  0%"
	}
	return fmt.Sprintf("%3d%%", value*100/total)
}

func peakHour(dist map[int]int) string {
	maxH, maxV := 0, 0
	for h, v := range dist {
		if v > maxV {
			maxH, maxV = h, v
		}
	}
	if maxV == 0 {
		return "—"
	}
	return fmt.Sprintf("%02d:00", maxH)
}

func dominantLengthClass(dist map[string]int) string {
	best, bestV := "—", 0
	for cls, v := range dist {
		if v > bestV {
			best, bestV = cls, v
		}
	}
	return best
}
