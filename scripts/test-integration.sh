#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$project_dir/tmp/test-results"
docker run --rm --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$project_dir:/workspace:ro" \
  -v "$project_dir/tmp/test-results:/reports" \
  -v "$HOME/go/pkg/mod:/go/pkg/mod" \
  -v "$HOME/.cache/go-build:/root/.cache/go-build" \
  -w /workspace \
  -e GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
  -e IMAGE_REPOSITORY="${IMAGE_REPOSITORY:-}" \
  -e GIN_MODE=release \
  -e APP_OTEL_ENABLED=false \
  -e APP_LOG_FILENAME=/tmp/ecommerce-test.log \
  -e APP_LOG_CONSOLE_LEVEL=error \
  -e APP_DATABASE_LOG_LEVEL=error \
  "${GOLANG_IMAGE:-golang:1.26}" sh -c \
  'go vet ./... &&
   go test -race ./internal/config ./internal/order -count=1 &&
   go test -race ./tests -count=1 -timeout=5m -ginkgo.randomize-all -ginkgo.no-color -ginkgo.junit-report=/reports/integration.xml' \
  2>&1 | tee "$project_dir/tmp/test-results/integration.log"
