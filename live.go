package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guptarohit/asciigraph"
)

type timeSeriesPoint struct {
	timestamp time.Time
	latency   float64
	rps       float64
}

type liveMetrics struct {
	sync.Mutex
	points      []timeSeriesPoint
	lastCount   int
	lastTime    time.Time
	startTime   time.Time
	windowSize  time.Duration
	statusCount map[int]int
}

func newLiveMetrics(windowSize time.Duration) *liveMetrics {
	now := time.Now()
	return &liveMetrics{
		points:     make([]timeSeriesPoint, 0, 100),
		lastTime:   now,
		startTime:  now,
		windowSize: windowSize,
	}
}

func (lm *liveMetrics) sample(results *resultSet) {
	lm.Lock()
	defer lm.Unlock()

	results.mu.Lock()
	records := results.records
	currentCount := len(records)
	lm.statusCount = map[int]int{}
	for _, rec := range results.records {
		lm.statusCount[rec.status]++
	}
	results.mu.Unlock()

	now := time.Now()

	// Calculate RPS
	timeDiff := now.Sub(lm.lastTime).Seconds()
	countDiff := currentCount - lm.lastCount
	rps := 0.0
	if timeDiff > 0 {
		rps = float64(countDiff) / timeDiff
	}

	// Calculate average latency for new records
	var totalLatency time.Duration
	newRecords := 0
	for i := lm.lastCount; i < currentCount; i++ {
		if !records[i].failed {
			totalLatency += records[i].latency
			newRecords++
		}
	}

	avgLatency := 0.0
	if newRecords > 0 {
		avgLatency = totalLatency.Seconds() / float64(newRecords)
	}

	// Add the data point
	lm.points = append(lm.points, timeSeriesPoint{
		timestamp: now,
		latency:   avgLatency,
		rps:       rps,
	})

	// Update tracking values
	lm.lastCount = currentCount
	lm.lastTime = now

	// Trim old points outside window
	cutoff := now.Add(-lm.windowSize)
	i := 0
	for ; i < len(lm.points); i++ {
		if lm.points[i].timestamp.After(cutoff) {
			break
		}
	}
	if i > 0 {
		lm.points = lm.points[i:]
	}
}

func (lm *liveMetrics) renderGraphs() string {
	lm.Lock()
	defer lm.Unlock()

	if len(lm.points) < 2 {
		return "Collecting data..."
	}

	// Extract data series
	latencies := make([]float64, len(lm.points))
	rps := make([]float64, len(lm.points))

	for i, p := range lm.points {
		latencies[i] = p.latency
		rps[i] = p.rps
	}

	// Configure graph options
	width := 40
	height := 10

	// Create graphs
	latencyGraph := asciigraph.Plot(
		latencies,
		asciigraph.Height(height),
		asciigraph.Width(width),
		asciigraph.Caption("Latency (seconds)"),
		asciigraph.SeriesColors(asciigraph.Green),
	)

	rpsGraph := asciigraph.Plot(
		rps,
		asciigraph.Height(height),
		asciigraph.Width(width),
		asciigraph.Caption("RPS"),
		asciigraph.SeriesColors(asciigraph.Blue),
	)

	elapsedTime := time.Since(lm.startTime).Round(time.Second)

	sb := &strings.Builder{}
	sb.WriteString("\nStatus code distribution:\n")
	for code, cnt := range lm.statusCount {
		sb.WriteString(fmt.Sprintf("  [%d] %d responses\n", code, cnt))
	}

	// Combine graphs with headers
	return fmt.Sprintf("\033[H\033[2J(running for %s, showing %s)\n\n%s\n\n%s\n\n%s", elapsedTime, min(lm.windowSize, elapsedTime), latencyGraph, rpsGraph, sb.String())
}

func startLiveMonitor(ctx context.Context, results *resultSet) {
	metrics := newLiveMetrics(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics.sample(results)
			fmt.Print(metrics.renderGraphs())
		case <-ctx.Done():
			return
		}
	}
}
