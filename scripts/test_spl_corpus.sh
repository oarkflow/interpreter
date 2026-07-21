#!/usr/bin/env bash
set -uo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
run_integrations=0
timeout_seconds=${SPL_CORPUS_TIMEOUT_SECONDS:-20}

case "${1:-}" in
  "") ;;
  --integration) run_integrations=1 ;;
  -h|--help)
    echo "Usage: $0 [--integration]"
    echo "Checks every SPL file and executes safe/expected-failure cases."
    echo "--integration also executes scripts requiring plugins, services, or host access."
    exit 0
    ;;
  *) echo "unknown option: $1" >&2; exit 2 ;;
esac

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/spl-corpus.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT INT TERM

spltool="$work_dir/spltool"
interpreter="$work_dir/interpreter"

cd "$repo_root"
echo "Building SPL tools..."
go build -o "$spltool" ./cmd/spltool || exit 1
go -C cmd/interpreter build -o "$interpreter" . || exit 1

run_timed() {
  local log_file=$1
  shift
  local timeout_marker="$log_file.timeout"
  rm -f "$timeout_marker"
  "$@" >"$log_file" 2>&1 &
  local command_pid=$!
  (
    sleep "$timeout_seconds"
    if kill -0 "$command_pid" 2>/dev/null; then
      : >"$timeout_marker"
      kill -TERM "$command_pid" 2>/dev/null || true
    fi
  ) &
  local watchdog_pid=$!
  wait "$command_pid"
  local status=$?
  kill "$watchdog_pid" 2>/dev/null || true
  wait "$watchdog_pid" 2>/dev/null || true
  if [ -f "$timeout_marker" ]; then
    return 124
  fi
  return "$status"
}

classify_script() {
  case "$1" in
    examples/all_in_one.spl)
      echo success
      ;;
    *)
      echo unclassified
      ;;
  esac
}

scripts_file="$work_dir/scripts.list"
find examples testdata -type f -name '*.spl' -print | LC_ALL=C sort >"$scripts_file"

total=0
checked=0
executed=0
expected_failures=0
fixtures=0
manual=0
integrations_skipped=0
failures=0

while IFS= read -r script_file; do
  total=$((total + 1))
  check_log="$work_dir/check-$total.log"
  if run_timed "$check_log" "$spltool" check "$script_file"; then
    checked=$((checked + 1))
  else
    status=$?
    echo "FAIL check: $script_file (status $status)" >&2
    sed -n '1,80p' "$check_log" >&2
    failures=$((failures + 1))
    continue
  fi

  class=$(classify_script "$script_file")
  run_log="$work_dir/run-$total.log"
  case "$class" in
    success)
      if run_timed "$run_log" "$interpreter" "$script_file"; then
        echo "PASS $script_file"
        executed=$((executed + 1))
      else
        status=$?
        echo "FAIL run: $script_file (status $status)" >&2
        sed -n '1,120p' "$run_log" >&2
        failures=$((failures + 1))
      fi
      ;;
    expected-failure)
      if run_timed "$run_log" "$interpreter" "$script_file"; then
        echo "FAIL expected failure succeeded: $script_file" >&2
        failures=$((failures + 1))
      else
        status=$?
        if [ "$status" -eq 124 ]; then
          echo "FAIL expected failure timed out: $script_file" >&2
          failures=$((failures + 1))
        else
          echo "PASS expected failure: $script_file"
          expected_failures=$((expected_failures + 1))
        fi
      fi
      ;;
    fixture)
      echo "PASS fixture check: $script_file"
      fixtures=$((fixtures + 1))
      ;;
    manual)
      echo "PASS manual-only check: $script_file"
      manual=$((manual + 1))
      ;;
    integration)
      if [ "$run_integrations" -eq 0 ]; then
        echo "PASS integration check (execution skipped): $script_file"
        integrations_skipped=$((integrations_skipped + 1))
      elif run_timed "$run_log" "$interpreter" "$script_file"; then
        echo "PASS integration: $script_file"
        executed=$((executed + 1))
      else
        status=$?
        echo "FAIL integration: $script_file (status $status)" >&2
        sed -n '1,120p' "$run_log" >&2
        failures=$((failures + 1))
      fi
      ;;
    *)
      echo "FAIL unclassified SPL file: $script_file" >&2
      failures=$((failures + 1))
      ;;
  esac
done <"$scripts_file"

echo
echo "SPL corpus summary"
echo "  discovered:           $total"
echo "  checked:              $checked"
echo "  executed successfully: $executed"
echo "  expected failures:    $expected_failures"
echo "  fixtures checked:     $fixtures"
echo "  manual-only checked:  $manual"
echo "  integrations skipped: $integrations_skipped"
echo "  failures:             $failures"

if [ "$checked" -ne "$total" ] || [ "$failures" -ne 0 ]; then
  exit 1
fi
