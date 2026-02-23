#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Build and install
cd "$REPO_ROOT"
go build -o boop .
sudo install boop /usr/local/bin/boop
