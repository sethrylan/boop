# boop

Yet another replacement to [ab](https://httpd.apache.org/docs/2.4/programs/ab.html)/[boom](https://github.com/tarekziade/boom)/[hey](https://github.com/rakyll/hey) for HTTP/2 benchmarking.


## Installation

### From Source

```bash
go install github.com/sethrylan/boop
```

### From Releases

Download the appropriate binary for your platform from the [releases page](https://github.com/sethrylan/boop/releases).

## Usage

```sh
boop https://google.com/
```

**100 concurrent requests**

```sh
boop -c 100 -q 1 https://google.com/
```

**POST with Auth**

```sh
boop -m POST -d '{"key": "value"}' \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $Token" \
  https://example.com/api
```

**Live metrics**

```sh
boop -live https://google.com/
```

### Options

```
  -n int
        Total requests to perform (default 200)
  -c int
        Concurrency level (workers) (default 50)
  -q float
        Per‑worker QPS (0 = unlimited) (default 0)
  -m string
        HTTP method (default "GET")
  -d string
        Request body. Use @file to read a file
  -t duration
        Per‑request timeout (default 30s)
  -k
        Skip TLS certificate verification
  -h2
        Enable HTTP/2 (default true)
  -no-keepalive
        Disable HTTP keep-alives
  -no-redirect
        Do not follow redirects
  -trace
        Output per request connection trace
  -live
        Display live metrics graph
  -H string
        Custom header. Repeatable.
```

## License

MIT License. See [LICENSE](LICENSE) for details.
