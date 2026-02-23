# boop

<p>
  <picture >
    <source media="(prefers-color-scheme: dark)" srcset="docs/boop-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="docs/boop-light.png">
    <img alt="Boop Boop" src="docs/boop-dark.png" width="20%">
  </picture>
  <br>
  <a href="https://github.com/sethrylan/boop/releases"><img src="https://img.shields.io/github/release/sethrylan/boop.svg" alt="Latest Release"></a>
  <a href="https://github.com/sethrylan/boop/actions"><img src="https://github.com/sethrylan/boop/workflows/ci/badge.svg" alt="Build Status"></a>
</p>

Yet another HTTP/2 benchmark tool, just like [ApacheBench/ab](https://httpd.apache.org/docs/2.4/programs/ab.html)/[boom](https://github.com/tarekziade/boom)/[hey](https://github.com/rakyll/hey), but not those.

## Usage

```sh
boop https://google.com
```

**100 concurrent requests**

```sh
boop -c 100 -q 1 https://google.com
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
boop -live https://google.com
```

![boop](./docs/demo.gif)

### Options

```
Usage: boop [options] <url>
  -H value
    	Custom header. Repeatable.
  -c int
    	Concurrency level, a.k.a., number of workers (default 10)
  -d string
    	Request body. Use @file to read a file
  -h2
    	Enable HTTP/2 (default true)
  -k	Skip TLS certificate verification
  -live
    	Display live metrics graph
  -m string
    	HTTP method (default "GET")
  -n int
    	Total requests to perform (default 9223372036854775806)
  -no-keepalive
    	Disable HTTP keep-alives
  -no-redirect
    	Do not follow redirects
  -q float
    	Per‑worker RPS (0 = unlimited)
  -t duration
    	Per‑request timeout (default 30s)
  -trace
    	Output per request connection trace
```

## Installation

**From Source**

```bash
go install github.com/sethrylan/boop@latest
```

**Or, from [Releases](https://github.com/sethrylan/boop/releases)**

