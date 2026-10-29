#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(go env GOVERSION)" != go1.27.3 ]]; then
  echo '请使用 Go 1.27.3。' >&2
  exit 1
fi
unformatted=$(gofmt -l cmd internal native-sample)
if [[ -n "$unformatted" ]]; then
  echo "$unformatted" >&2
  exit 1
fi
go test -race ./...
go vet ./...
go build ./cmd/agent-manager
rm -f agent-manager
(cd native-sample && go test ./... && go vet ./...)
