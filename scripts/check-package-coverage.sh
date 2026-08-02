#!/usr/bin/env sh
# Enforce a coverage floor on every package, not just on the repository total.
#
# A single total is satisfiable by averaging: one package can sit far below the
# line while better-covered neighbours carry it, and the number that moves is
# the one nobody is looking at. This checks each package against the floor so a
# regression is attributed to the package that caused it.
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

# awk only prints. It must not decide the exit status, because the sort it
# feeds would replace that status with its own and the gate would pass no
# matter what the report said.
report="$(
  go tool cover -func="$coverage_file" \
    | grep -v '^total:' \
    | awk -v threshold="$threshold" '
        {
          percent = $NF
          gsub(/%/, "", percent)
          path = $1
          sub(/\/[^\/]*\.go:[0-9]+:$/, "", path)
          sum[path] += percent
          count[path]++
        }
        END {
          for (pkg in sum) {
            average = sum[pkg] / count[pkg]
            printf "%s %6.2f%% %s\n", (average + 0 < threshold + 0 ? "FAIL" : "ok  "), average, pkg
          }
        }
      ' \
    | sort -k3,3
)"

printf '%s\n' "$report"

if printf '%s\n' "$report" | grep -q '^FAIL'; then
  printf '\nat least one package is below the required %s%%\n' "$threshold" >&2
  exit 1
fi
