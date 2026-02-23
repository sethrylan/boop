# Demo

The `demo.gif` in the project README is generated automatically.

## How It Works

- **[demo.tape](demo.tape)** — A [VHS](https://github.com/charmbracelet/vhs) script that records a terminal session of `boop` as a GIF.

## CI Workflow

The [Demo workflow](../.github/workflows/demo.yml) runs automatically on PRs to `main` when relevant files change (`docs/demo.tape`, `*.go`, `go.mod`, `go.sum`). It builds `boop`, runs VHS, and commits the updated `demo.gif` back to the PR branch.
