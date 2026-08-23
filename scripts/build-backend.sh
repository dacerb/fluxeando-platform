#!/usr/bin/env sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$root/apps/desktop/resources/bin/mac-arm64" "$root/apps/desktop/resources/bin/win-x64"
(cd "$root/services/cashflow-api" && GOOS=darwin GOARCH=arm64 go build -o "$root/apps/desktop/resources/bin/mac-arm64/cashflow-api" ./cmd/api)
(cd "$root/services/cashflow-api" && GOOS=windows GOARCH=amd64 go build -o "$root/apps/desktop/resources/bin/win-x64/cashflow-api.exe" ./cmd/api)
