#!/usr/bin/env sh
# Run every fuzz target for a bounded time.
#
# `go test ./...` executes a fuzz target's seed corpus only, which is a handful
# of inputs the author already thought of — the property is asserted but never
# searched. `-fuzz` accepts one target per package per invocation, so the
# targets have to be enumerated rather than globbed, and a target added without
# touching this script would otherwise never run. Discovery is therefore done
# from the source, not from a list kept here.
set -eu

fuzztime="${1:-60s}"

if ! command -v go >/dev/null 2>&1; then
  printf 'go is required to run fuzz targets\n' >&2
  exit 1
fi

targets="$(grep -rlE '^func Fuzz[A-Za-z0-9_]*\(' --include='*_test.go' . | sort)"
if [ -z "$targets" ]; then
  printf 'no fuzz targets found; this script would pass vacuously\n' >&2
  exit 1
fi

status=0
count=0
for file in $targets; do
  package="./$(dirname "$file" | sed 's|^\./||')"
  for target in $(grep -oE '^func Fuzz[A-Za-z0-9_]*' "$file" | sed 's/^func //'); do
    count=$((count + 1))
    printf '\n=== %s %s (%s)\n' "$package" "$target" "$fuzztime"
    # -run '^$' skips the ordinary tests so the whole budget goes to the search.
    if ! go test "$package" -run '^$' -fuzz "^${target}\$" -fuzztime "$fuzztime"; then
      status=1
    fi
  done
done

printf '\nran %d fuzz target(s) at %s each\n' "$count" "$fuzztime"
exit "$status"
