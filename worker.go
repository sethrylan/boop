package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

func worker(
	ctx context.Context,
	id int,
	client *http.Client,
	reqTpl *http.Request,
	jobCh <-chan int,
	out *resultSet,
	wg *sync.WaitGroup,
	limiter <-chan time.Time,
	withTrace bool,
) {
	defer wg.Done()

	for range jobCh { // each value from jobCh is a placeholder index
		// Check if context is done before processing
		select {
		case <-ctx.Done():
			return // Exit if context was canceled
		default:
			// Continue processing
		}

		if limiter != nil {
			select {
			case <-limiter:
				// Rate limiting
			case <-ctx.Done():
				return // Exit if context was canceled while waiting
			}
		}

		// Clone request (cheap shallow copy, new body)
		req := reqTpl.Clone(reqTpl.Context())
		if req.Body != nil {
			_ = req.Body.Close() // close old (no‑op for NopCloser)
			req.Body, _ = reqTpl.GetBody()
		}

		start := time.Now()
		var rec record

		if withTrace {
			trace := &httptrace.ClientTrace{
				GotConn: func(ci httptrace.GotConnInfo) {
					fmt.Printf("worker %d got conn: reused=%v idle=%v\n", id, ci.Reused, ci.WasIdle)
				},
			}
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
		}

		resp, err := client.Do(req)
		if err != nil {
			rec.failed = true
			rec.errMsg = err.Error()
			out.add(rec)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body) // drain body
		_ = resp.Body.Close()

		rec.latency = time.Since(start)
		rec.status = resp.StatusCode
		if resp.ContentLength > 0 {
			rec.size = resp.ContentLength
		}
		out.add(rec)
	}
}
