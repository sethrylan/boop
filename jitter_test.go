package main

import (
	"testing"
	"time"
)

func TestJitterTickerRange(t *testing.T) {
	interval := 100 * time.Millisecond
	jitter := 50 * time.Millisecond
	jt := NewJitterTicker(interval, jitter)
	defer jt.Stop()

	min := interval - jitter
	max := interval + jitter

	// Collect a few ticks and check intervals
	prev := time.Now()
	for range 5 {
		select {
		case tick := <-jt.C:
			elapsed := tick.Sub(prev)
			if elapsed < min || elapsed > max {
				t.Errorf("tick interval out of range: got %v, want [%v, %v]", elapsed, min, max)
			}
			prev = tick
		case <-time.After(max + 100*time.Millisecond):
			t.Fatal("timeout waiting for jitter tick")
		}
	}
}

func TestJitterTickerStop(t *testing.T) {
	interval := 50 * time.Millisecond
	jitter := 10 * time.Millisecond
	jt := NewJitterTicker(interval, jitter)
	jt.Stop()
	// After stopping, channel should not emit further ticks
	select {
	case <-jt.C:
		t.Error("received tick after Stop() called")
	case <-time.After(2 * interval):
		// Success: no tick received
	}
}
