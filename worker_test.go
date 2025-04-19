package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// dummyRoundTripper simulates HTTP responses.
type dummyRoundTripper struct {
	fail bool
}

func (d dummyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if d.fail {
		return nil, errors.New("dummy error")
	}
	return &http.Response{
		StatusCode:    200,
		Body:          io.NopCloser(strings.NewReader("OK")),
		ContentLength: 2,
		Header:        make(http.Header),
	}, nil
}

// bodyCheckTransport is a test transport that verifies request body
type bodyCheckTransport struct {
	t            *testing.T
	expectedBody string
}

func (b *bodyCheckTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			b.t.Errorf("failed to read request body: %v", err)
		} else if string(body) != b.expectedBody {
			b.t.Errorf("expected body %q, got %q", b.expectedBody, string(body))
		}
		_ = req.Body.Close()
	}

	return &http.Response{
		StatusCode:    200,
		Body:          io.NopCloser(strings.NewReader("OK")),
		ContentLength: 2,
		Header:        make(http.Header),
	}, nil
}

func TestWorkerSuccess(t *testing.T) {
	// Set up a dummy client that always succeeds.
	client := &http.Client{
		Transport: dummyRoundTripper{fail: false},
		Timeout:   5 * time.Second,
	}
	reqTpl, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request template: %v", err)
	}

	// Set up channels and result set.
	jobCh := make(chan int, 3)
	var wg sync.WaitGroup
	results := &resultSet{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start one worker.
	wg.Add(1)
	go worker(ctx, 1, client, reqTpl, jobCh, results, &wg, nil, false)

	// Send 3 jobs.
	for i := 0; i < 3; i++ {
		jobCh <- i
	}
	close(jobCh)
	wg.Wait()

	// Validate results.
	if len(results.records) != 3 {
		t.Errorf("expected 3 records, got %d", len(results.records))
	}
	for _, rec := range results.records {
		if rec.failed {
			t.Error("expected success but got failure")
		}
		if rec.status != 200 {
			t.Errorf("expected status 200, got %d", rec.status)
		}
	}
}

func TestWorkerFailure(t *testing.T) {
	// Set up a dummy client that always fails.
	client := &http.Client{
		Transport: dummyRoundTripper{fail: true},
		Timeout:   5 * time.Second,
	}
	reqTpl, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request template: %v", err)
	}

	// Set up channels and result set.
	jobCh := make(chan int, 2)
	var wg sync.WaitGroup
	results := &resultSet{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start one worker.
	wg.Add(1)
	go worker(ctx, 2, client, reqTpl, jobCh, results, &wg, nil, false)

	// Send 2 jobs.
	for i := 0; i < 2; i++ {
		jobCh <- i
	}
	close(jobCh)
	wg.Wait()

	// Validate results.
	if len(results.records) != 2 {
		t.Errorf("expected 2 records, got %d", len(results.records))
	}
	for _, rec := range results.records {
		if !rec.failed {
			t.Error("expected failure but got success")
		}
		if rec.errMsg == "" {
			t.Error("expected an error message but got empty")
		}
	}
}

// TestWorkerContextCancellation tests that worker stops when context is canceled
func TestWorkerContextCancellation(t *testing.T) {
	client := &http.Client{
		Transport: dummyRoundTripper{fail: false},
		Timeout:   5 * time.Second,
	}
	reqTpl, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request template: %v", err)
	}

	// Create a buffered channel that won't block
	jobCh := make(chan int, 100)
	var wg sync.WaitGroup
	results := &resultSet{}

	// Create a context we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start the worker
	wg.Add(1)
	go worker(ctx, 1, client, reqTpl, jobCh, results, &wg, nil, false)

	// Send a few jobs to ensure worker is running
	for i := 0; i < 3; i++ {
		jobCh <- i
	}

	// Small delay to ensure worker processes some jobs
	time.Sleep(50 * time.Millisecond)

	// Cancel the context to signal the worker to stop
	cancel()

	// Send more jobs that should be ignored
	for i := 3; i < 10; i++ {
		jobCh <- i
	}

	// Close job channel and wait for worker to exit
	close(jobCh)
	wg.Wait()

	// Only the first few jobs should be processed
	if len(results.records) > 5 {
		t.Errorf("expected fewer than 5 records after cancellation, got %d", len(results.records))
	}
}

// TestWorkerRateLimiting tests that worker respects rate limiting
func TestWorkerRateLimiting(t *testing.T) {
	client := &http.Client{
		Transport: dummyRoundTripper{fail: false},
		Timeout:   5 * time.Second,
	}
	reqTpl, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request template: %v", err)
	}

	jobCh := make(chan int, 5)
	var wg sync.WaitGroup
	results := &resultSet{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a controlled rate limiter channel
	limiterCh := make(chan time.Time, 5)

	// Start the worker with our controlled limiter
	wg.Add(1)
	go worker(ctx, 1, client, reqTpl, jobCh, results, &wg, limiterCh, false)

	// Send jobs but don't release the limiter yet
	for i := 0; i < 5; i++ {
		jobCh <- i
	}
	close(jobCh)

	// Worker shouldn't process any jobs until limiter releases
	time.Sleep(50 * time.Millisecond)
	if len(results.records) > 0 {
		t.Errorf("expected 0 records before rate limit released, got %d", len(results.records))
	}

	// Release the rate limiter for each job
	for i := 0; i < 5; i++ {
		limiterCh <- time.Now()
		time.Sleep(10 * time.Millisecond) // Give worker time to process
	}

	wg.Wait()

	if len(results.records) != 5 {
		t.Errorf("expected 5 records after rate limiting, got %d", len(results.records))
	}
}

// // captureOutput is a helper to capture stdout during tests
// func captureOutput(t *testing.T, f func()) string {
// 	old := os.Stdout
// 	r, w, _ := os.Pipe()
// 	os.Stdout = w

// 	f()

// 	err := w.Close()
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	os.Stdout = old

// 	var buf strings.Builder
// 	_, _ = io.Copy(&buf, r)
// 	return buf.String()
// }

// TestWorkerWithBody ensures worker correctly clones request bodies
func TestWorkerWithBody(t *testing.T) {
	client := &http.Client{
		Transport: &bodyCheckTransport{t: t, expectedBody: "test-body"},
		Timeout:   5 * time.Second,
	}

	// Create request with a body
	reqBody := "test-body"
	reqBodyReader := strings.NewReader(reqBody)
	reqTpl, err := http.NewRequest("POST", "http://example.com", reqBodyReader)

	// Set up GetBody function to recreate the body when needed
	reqTpl.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(reqBody)), nil
	}

	if err != nil {
		t.Fatalf("failed to create request template: %v", err)
	}

	jobCh := make(chan int, 2)
	var wg sync.WaitGroup
	results := &resultSet{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker
	wg.Add(1)
	go worker(ctx, 1, client, reqTpl, jobCh, results, &wg, nil, false)

	// Send two jobs to test body reuse
	jobCh <- 1
	jobCh <- 2

	close(jobCh)
	wg.Wait()

	// Both requests should succeed
	if len(results.records) != 2 {
		t.Errorf("expected 2 records, got %d", len(results.records))
	}
	for _, rec := range results.records {
		if rec.failed {
			t.Error("expected success but got failure")
		}
	}
}
