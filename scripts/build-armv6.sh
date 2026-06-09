#!/usr/bin/env bash
set -euo pipefail

mkdir -p dist
GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/netwatchd-linux-armv6 ./cmd/netwatchd
GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/netwatch-jsonl-linux-armv6 ./cmd/netwatch-jsonl
