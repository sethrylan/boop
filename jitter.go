package main

import (
	"math/rand"
	"time"
)

type JitterTicker struct {
	C    chan time.Time
	stop chan struct{}
	min  time.Duration
	max  time.Duration
}

func (jt *JitterTicker) loop() {
	t := time.NewTimer(jt.nextInterval())
	for {
		select {
		case <-jt.stop:
			t.Stop()
			return
		case <-t.C:
			select {
			case jt.C <- time.Now():
				t.Stop()
				t = time.NewTimer(jt.nextInterval())
			default:
				// skip if there is no receiver
			}
		}
	}
}

func (jt *JitterTicker) nextInterval() time.Duration {
	delta := jt.max - jt.min
	return jt.min + time.Duration(rand.Int63n(int64(delta)))
}

func (jt *JitterTicker) Stop() {
	close(jt.stop)
	// Don't close jt.C immediately - can cause race condition
	// and allows values to be read that were already sent
}

func NewJitterTicker(d time.Duration, jitter time.Duration) *JitterTicker {
	min := d - jitter
	max := d + jitter
	jt := &JitterTicker{
		C:    make(chan time.Time),
		stop: make(chan struct{}),
		min:  min,
		max:  max,
	}
	go jt.loop()
	return jt
}

func JitterTick(d time.Duration, jitter time.Duration) <-chan time.Time {
	return NewJitterTicker(d, jitter).C
}
