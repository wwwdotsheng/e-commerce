#!/usr/bin/env bash
set -euo pipefail
: "${API_TEST_BASE_URL:?Explicitly set the target service origin}"
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$project_dir/tmp/test-results"
docker run --rm --network host \
  -v "$project_dir:/workspace:ro" \
  -v "$HOME/go/pkg/mod:/go/pkg/mod" \
  -v "$HOME/.cache/go-build:/root/.cache/go-build" \
  -w /workspace \
  -e API_TEST_BASE_URL \
  -e API_TEST_HOST="${API_TEST_HOST:-}" \
  -e API_TEST_ALLOW_WRITES="${API_TEST_ALLOW_WRITES:-0}" \
  "${GOLANG_IMAGE:-golang:1.26}" \
  go test ./tests/acceptance -v -count=1 -timeout=3m \
  2>&1 | tee "$project_dir/tmp/test-results/acceptance.log"
