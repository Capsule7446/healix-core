#!/usr/bin/env sh
set -eu

coverage_file="${1:-coverage.out}"
threshold="${2:-80}"

case "$threshold" in
  ''|*[!0-9.]*|.*|*.*.*)
    printf 'coverage threshold must be a number: %s\n' "$threshold" >&2
    exit 1
    ;;
esac

if [ "$(awk -v threshold="$threshold" 'BEGIN { print (threshold >= 0 && threshold <= 100) ? 0 : 1 }')" -ne 0 ]; then
  printf 'coverage threshold must be between 0 and 100: %s\n' "$threshold" >&2
  exit 1
fi

if [ ! -f "$coverage_file" ]; then
  printf 'coverage file not found: %s\n' "$coverage_file" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  printf 'go is required to inspect coverage\n' >&2
  exit 1
fi

actual="$(go tool cover -func="$coverage_file" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
if [ -z "$actual" ]; then
  printf 'coverage total not found in %s\n' "$coverage_file" >&2
  exit 1
fi

awk -v actual="$actual" -v threshold="$threshold" 'BEGIN {
  if (actual + 0 < threshold + 0) {
    printf "coverage %.2f%% is below required %.2f%%\n", actual, threshold
    exit 1
  }
  printf "coverage %.2f%% meets required %.2f%%\n", actual, threshold
}'
