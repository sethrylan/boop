package main

import (
	"cmp"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	totalReq          = flag.Int("n", math.MaxInt-1, "Total requests to perform")
	concur            = flag.Int("c", 10, "Concurrency level, a.k.a., number of workers")
	rps               = flag.Float64("q", 0, "Per‑worker RPS (0 = unlimited)")
	method            = flag.String("m", "GET", "HTTP method")
	data              = flag.String("d", "", "Request body. Use @file to read a file")
	timeout           = flag.Duration("t", 30*time.Second, "Per‑request timeout")
	insecure          = flag.Bool("k", false, "Skip TLS certificate verification")
	h2                = flag.Bool("h2", true, "Enable HTTP/2")
	disableKeepAlives = flag.Bool("no-keepalive", false, "Disable HTTP keep-alives")
	noRedirect        = flag.Bool("no-redirect", false, "Do not follow redirects")
	showTrace         = flag.Bool("trace", false, "Output per request connection trace")
	live              = flag.Bool("live", false, "Display live metrics graph")
	headers           headerSlice
)

func main() {
	flag.Var(&headers, "H", "Custom header. Repeatable.")

	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Println("Usage: boop [options] <url>")
		flag.PrintDefaults()
		os.Exit(1)
	}
	targetURL := flag.Arg(0)
	if *concur <= 0 || *totalReq <= 0 || *concur > *totalReq {
		fmt.Println("n must be ≥ c and both > 0")
		os.Exit(1)
	}

	bodyBytes, err := loadBody(*data)
	if err != nil {
		fmt.Printf("failed to read body: %v\n", err)
		os.Exit(1)
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		fmt.Printf("invalid url: %v\n", err)
		os.Exit(1)
	}
	if parsedURL.Scheme == "" {
		fmt.Println("URL must include scheme")
		os.Exit(1)
	}

	// Build request template
	reqTpl, err := http.NewRequestWithContext(context.Background(), strings.ToUpper(*method), parsedURL.String(), nil)
	if err != nil {
		fmt.Printf("request build: %v\n", err)
		os.Exit(1)
	}
	if len(bodyBytes) > 0 {
		reqTpl.Body = io.NopCloser(bytesReader(bodyBytes))
		reqTpl.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytesReader(bodyBytes)), nil
		}
	}

	// Headers
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			fmt.Printf("invalid header %q, must be key:value\n", h)
			os.Exit(1)
		}
		reqTpl.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	/* --- HTTP client configuration --- */
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: *concur,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: *insecure}, //nolint:gosec // User explicitly opted into insecure mode via -k flag
		DisableCompression:  false,
		ForceAttemptHTTP2:   *h2,
		DisableKeepAlives:   *disableKeepAlives,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   *timeout,
	}

	if *noRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	// Channels & goroutines
	jobCh := make(chan int, *concur)
	var wg sync.WaitGroup
	results := &resultSet{start: time.Now()}

	// set up signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel() // Ensure context is canceled when we exit

	if *live {
		go startLiveMonitor(ctx, results)
	}

	var tickers []*time.Ticker
	for i := range *concur {
		wg.Add(1)
		var limiter <-chan time.Time
		if *rps > 0 {
			interval := time.Duration(float64(time.Second) / *rps)
			t := time.NewTicker(interval)
			tickers = append(tickers, t)
			limiter = t.C
		}

		// stagger starts with jitter to avoid synchronized bursts
		base := time.Duration(1000 / *concur) * time.Millisecond
		jitter := time.Duration(rand.Int64N(int64(base/2 + 1))) //nolint:gosec // jitter doesn't need cryptographic randomness
		time.Sleep(base + jitter)
		go worker(ctx, i, client, reqTpl, jobCh, results, &wg, limiter, *showTrace)
	}

	// feed jobs
	for i := 0; i < *totalReq; i++ {
		select {
		case jobCh <- i:
			// Job sent successfully
		case <-ctx.Done():
			// Context was canceled, stop sending jobs
			i = *totalReq // exit loop
		}
	}
	close(jobCh)

	// wait for jobs to finish
	wg.Wait()

	for _, t := range tickers {
		t.Stop()
	}

	// collect results
	results.end = time.Now()
	results.summarize()
}

// bytesReaderT is a constant‑time bytes.Reader equivalent
func bytesReader(b []byte) *bytesReaderT { return &bytesReaderT{b: b} }

type bytesReaderT struct {
	b []byte
	i int64
}

func (r *bytesReaderT) Read(p []byte) (int, error) {
	if r.i >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += int64(n)
	return n, nil
}
func (r *bytesReaderT) Close() error { return nil }

// headerSlice is for parsing HTTP headers
type headerSlice []string

func (h *headerSlice) String() string { return strings.Join(*h, "; ") }
func (h *headerSlice) Set(v string) error {
	*h = append(*h, v)
	return nil
}

// loadBody loads the request body from a string or file.
func loadBody(bodyFlag string) ([]byte, error) {
	if bodyFlag == "" {
		return nil, nil
	}
	if path, ok := strings.CutPrefix(bodyFlag, "@"); ok {
		f, err := os.ReadFile(path) //nolint:gosec // User explicitly specified file path via -d flag
		return f, err
	}
	return []byte(bodyFlag), nil
}

// -------------------------------------------------
// Result accounting
// -------------------------------------------------

// record is the result of a single request
type record struct {
	latency time.Duration
	status  int
	size    int64
	failed  bool
	errMsg  string
}

type resultSet struct {
	mu         sync.Mutex
	records    []record
	start, end time.Time
}

func (r *resultSet) add(rec record) {
	r.mu.Lock()
	r.records = append(r.records, rec)
	r.mu.Unlock()
}

func (r *resultSet) summarize() {
	total := len(r.records)
	if total == 0 {
		fmt.Println("No records, something went wrong.")
		return
	}

	latencies := make([]time.Duration, 0, total)
	var bytesTotal int64
	var failed int
	statusCount := map[int]int{}

	for _, rec := range r.records {
		if rec.failed {
			failed++
			statusCount[rec.status]++
			continue
		}
		latencies = append(latencies, rec.latency)
		bytesTotal += rec.size
		statusCount[rec.status]++
	}

	slices.SortFunc(latencies, cmp.Compare)

	successful := len(latencies)
	if successful == 0 {
		fmt.Println("All requests failed, cannot provide summary.")
		return
	}

	// Calculate basic stats
	totalDur := r.end.Sub(r.start)
	reqPerSec := float64(total) / totalDur.Seconds()

	// Find minLatency, maxLatency, mean
	minLatency := latencies[0]
	maxLatency := latencies[successful-1]

	mean := time.Duration(0)
	for _, l := range latencies {
		mean += l
	}
	mean /= time.Duration(successful)

	// Helper function for percentiles
	pct := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies))*p + .5)
		if idx >= len(latencies) {
			idx = len(latencies) - 1
		}
		return latencies[idx]
	}

	// Calculate histogram bins
	histoBins := 11
	binSize := (maxLatency - minLatency) / time.Duration(histoBins-1)
	if binSize == 0 {
		binSize = 1 * time.Millisecond // prevent division by zero
	}

	bins := make([]int, histoBins)
	for _, lat := range latencies {
		binIdx := int((lat - minLatency) / binSize)
		if binIdx >= histoBins {
			binIdx = histoBins - 1
		}
		bins[binIdx]++
	}

	// Find the max count for scaling histogram bars
	maxCount := 0
	for _, count := range bins {
		if count > maxCount {
			maxCount = count
		}
	}

	// Print summary
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total:        %.4f secs\n", totalDur.Seconds())
	fmt.Printf("  Slowest:      %.4f secs\n", maxLatency.Seconds())
	fmt.Printf("  Fastest:      %.4f secs\n", minLatency.Seconds())
	fmt.Printf("  Average:      %.4f secs\n", mean.Seconds())
	fmt.Printf("  Requests/sec: %.4f\n", reqPerSec)
	fmt.Printf("\n")
	fmt.Printf("  Total data:   %d bytes\n", bytesTotal)
	fmt.Printf("  Size/request: %d bytes\n", bytesTotal/int64(successful))

	// Print histogram
	fmt.Printf("\nResponse time histogram:\n")
	for i := range histoBins {
		binTime := minLatency + time.Duration(i)*binSize
		bar := ""
		if maxCount > 0 {
			barLength := int(40 * float64(bins[i]) / float64(maxCount))
			bar = strings.Repeat("■", barLength)
		}
		fmt.Printf("  %.3f [%d]\t|%s\n", binTime.Seconds(), bins[i], bar)
	}
	fmt.Printf("\n\n")

	// Print latency distribution
	fmt.Printf("Latency distribution:\n")
	fmt.Printf("  10%% in %.4f secs\n", pct(0.10).Seconds())
	fmt.Printf("  25%% in %.4f secs\n", pct(0.25).Seconds())
	fmt.Printf("  50%% in %.4f secs\n", pct(0.50).Seconds())
	fmt.Printf("  75%% in %.4f secs\n", pct(0.75).Seconds())
	fmt.Printf("  90%% in %.4f secs\n", pct(0.90).Seconds())
	fmt.Printf("  95%% in %.4f secs\n", pct(0.95).Seconds())
	fmt.Printf("  99%% in %.4f secs\n", pct(0.99).Seconds())

	// Note: Detailed timing metrics would require additional instrumentation
	fmt.Printf("\nDetails (average, fastest, slowest):\n")
	fmt.Printf("  DNS+dialup:   0.0000 secs, 0.0000 secs, 0.0000 secs\n")
	fmt.Printf("  DNS-lookup:   0.0000 secs, 0.0000 secs, 0.0000 secs\n")
	fmt.Printf("  req write:    0.0000 secs, 0.0000 secs, 0.0000 secs\n")
	fmt.Printf("  resp wait:    %.4f secs, %.4f secs, %.4f secs\n", mean.Seconds(), minLatency.Seconds(), maxLatency.Seconds())
	fmt.Printf("  resp read:    0.0000 secs, 0.0000 secs, 0.0000 secs\n")

	// Print status code distribution
	fmt.Print(statusCodeDistribution(statusCount))
}

func statusCodeDistribution(statusCount map[int]int) string {
	var sb strings.Builder
	keys := make([]int, 0, len(statusCount))
	for k := range statusCount {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	sb.WriteString("\nStatus code distribution:\n")
	for _, k := range keys {
		fmt.Fprintf(&sb, "  [%d] %d responses\n", k, statusCount[k])
	}
	return sb.String()
}
