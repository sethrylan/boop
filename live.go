package main

import (
	"context"
	"fmt"
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
	points     []timeSeriesPoint
	recordCh   <-chan record
	count      int
	lastTime   time.Time
	startTime  time.Time
	windowSize time.Duration
}

func newLiveMetrics(windowSize time.Duration, recordCh <-chan record) *liveMetrics {
	now := time.Now()
	return &liveMetrics{
		points:     make([]timeSeriesPoint, 0, 100),
		recordCh:   recordCh,
		lastTime:   now,
		startTime:  now,
		windowSize: windowSize,
	}
}

// processSamples collects samples from channel and updates metrics
func (lm *liveMetrics) processSamples(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var samples []record
	lastProcessTime := time.Now()

	for {
		select {
		case rec, ok := <-lm.recordCh:
			if !ok { // Channel closed
				return
			}
			samples = append(samples, rec)
		case <-ticker.C:
			// Process accumulated samples on tick
			if len(samples) > 0 {
				lm.updateMetrics(samples, lastProcessTime)
				samples = samples[:0] // Clear buffer
				lastProcessTime = time.Now()
			}
			fmt.Print(lm.renderGraphs())

		case <-ctx.Done():
			return
		}
	}
}

func (lm *liveMetrics) updateMetrics(samples []record, lastTime time.Time) {
	lm.Lock()
	defer lm.Unlock()

	now := time.Now()
	timeDiff := now.Sub(lastTime).Seconds()

	// Calculate RPS
	rps := 0.0
	if timeDiff > 0 {
		rps = float64(len(samples)) / timeDiff
	}

	// Calculate average latency
	var totalLatency time.Duration
	successCount := 0
	for _, sample := range samples {
		if !sample.failed {
			totalLatency += sample.latency
			successCount++
		}
	}

	avgLatency := 0.0
	if successCount > 0 {
		avgLatency = totalLatency.Seconds() / float64(successCount)
	}

	// Update total count
	lm.count += len(samples)

	// Add the data point
	lm.points = append(lm.points, timeSeriesPoint{
		timestamp: now,
		latency:   avgLatency,
		rps:       rps,
	})

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

	// Combine graphs with headers
	return fmt.Sprintf("\033[H\033[2J(running for %s, showing %s)\n\n%s\n\n%s", elapsedTime, min(lm.windowSize, elapsedTime), latencyGraph, rpsGraph)
}

func startLiveMonitor(ctx context.Context, recordCh chan record) {
	// Create and start metrics processor directly with the record channel
	metrics := newLiveMetrics(30*time.Second, recordCh)
	metrics.processSamples(ctx)
}
