package timing

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type Step struct {
	Label    string
	Start    time.Time
	End      time.Time
	Duration time.Duration
}

type Tracker struct {
	steps   []Step
	current *Step
}

func New() *Tracker {
	return &Tracker{}
}

func (t *Tracker) Start(label string) {
	if t.current != nil {
		t.Stop()
	}
	t.current = &Step{
		Label: label,
		Start: time.Now(),
	}
}

func (t *Tracker) Stop() time.Duration {
	if t.current == nil {
		return 0
	}
	t.current.End = time.Now()
	t.current.Duration = t.current.End.Sub(t.current.Start)
	t.steps = append(t.steps, *t.current)
	d := t.current.Duration
	t.current = nil
	return d
}

func (t *Tracker) Steps() []Step {
	return t.steps
}

func (t *Tracker) Total() time.Duration {
	var total time.Duration
	for _, s := range t.steps {
		total += s.Duration
	}
	return total
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", m, s)
}

func (t *Tracker) PrintTable(runLabel string) {
	maxLabel := 5
	for _, s := range t.steps {
		if len(s.Label) > maxLabel {
			maxLabel = len(s.Label)
		}
	}
	maxLabel += 2

	border := strings.Repeat("─", maxLabel) + "┬" + strings.Repeat("─", 12)
	fmt.Printf("\n%s:\n", runLabel)
	fmt.Printf("  ┌%s┐\n", border)
	fmt.Printf("  │%-*s│%12s│\n", maxLabel, " Step", "Latency ")

	mid := strings.Repeat("─", maxLabel) + "┼" + strings.Repeat("─", 12)
	fmt.Printf("  ├%s┤\n", mid)

	for _, s := range t.steps {
		fmt.Printf("  │ %-*s│%11s │\n", maxLabel-1, s.Label, formatDuration(s.Duration))
	}

	fmt.Printf("  ├%s┤\n", mid)
	fmt.Printf("  │ %-*s│%11s │\n", maxLabel-1, "TOTAL", formatDuration(t.Total()))

	bottom := strings.Repeat("─", maxLabel) + "┴" + strings.Repeat("─", 12)
	fmt.Printf("  └%s┘\n", bottom)
}

// Summary computes aggregate stats across multiple trackers.
type Summary struct {
	trackers []*Tracker
}

func NewSummary() *Summary {
	return &Summary{}
}

func (s *Summary) Add(t *Tracker) {
	s.trackers = append(s.trackers, t)
}

func (s *Summary) Print() {
	if len(s.trackers) == 0 {
		return
	}

	// Collect all unique labels in order.
	labelOrder := []string{}
	seen := map[string]bool{}
	for _, t := range s.trackers {
		for _, step := range t.Steps() {
			if !seen[step.Label] {
				seen[step.Label] = true
				labelOrder = append(labelOrder, step.Label)
			}
		}
	}
	labelOrder = append(labelOrder, "TOTAL")

	// Gather durations per label.
	durations := map[string][]time.Duration{}
	for _, t := range s.trackers {
		for _, step := range t.Steps() {
			durations[step.Label] = append(durations[step.Label], step.Duration)
		}
		durations["TOTAL"] = append(durations["TOTAL"], t.Total())
	}

	maxLabel := 5
	for _, l := range labelOrder {
		if len(l) > maxLabel {
			maxLabel = len(l)
		}
	}
	maxLabel += 2

	fmt.Printf("\nSummary (%d runs):\n", len(s.trackers))
	header := fmt.Sprintf("  │%-*s│%9s│%9s│%9s│%9s│%9s│", maxLabel, " Step", "  Min ", "  Avg ", "  P50 ", "  P95 ", "  Max ")
	w := len(header) - 4
	border := strings.Repeat("─", w)
	fmt.Printf("  ┌%s┐\n", border)
	fmt.Println(header)
	fmt.Printf("  ├%s┤\n", border)

	for _, label := range labelOrder {
		ds := durations[label]
		if len(ds) == 0 {
			continue
		}
		if label == "TOTAL" {
			fmt.Printf("  ├%s┤\n", border)
		}
		stats := calcStats(ds)
		fmt.Printf("  │ %-*s│%8s │%8s │%8s │%8s │%8s │\n",
			maxLabel-1, label,
			formatDuration(stats.Min),
			formatDuration(stats.Avg),
			formatDuration(stats.P50),
			formatDuration(stats.P95),
			formatDuration(stats.Max),
		)
	}
	fmt.Printf("  └%s┘\n", border)
}

type stats struct {
	Min, Max, Avg, P50, P95 time.Duration
}

func calcStats(ds []time.Duration) stats {
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	n := len(ds)
	return stats{
		Min: ds[0],
		Max: ds[n-1],
		Avg: time.Duration(int64(sum) / int64(n)),
		P50: ds[percentileIdx(n, 50)],
		P95: ds[percentileIdx(n, 95)],
	}
}

func percentileIdx(n, p int) int {
	idx := int(math.Ceil(float64(p)/100.0*float64(n))) - 1
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}
