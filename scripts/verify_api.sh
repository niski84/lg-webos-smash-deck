#!/usr/bin/env bash
# Smoke-tests for the LG WebOS Smash Deck API.
set -euo pipefail

BASE="${1:-http://localhost:8088}"
PASS=0; FAIL=0

check() {
  local desc="$1" url="$2" method="${3:-GET}" body="${4:-}"
  local args=(-s -o /dev/null -w "%{http_code}" -X "$method" -H "Content-Type: application/json")
  [[ -n "$body" ]] && args+=(-d "$body")
  local code
  code="$(curl "${args[@]}" "$url")"
  if [[ "$code" == "200" ]]; then
    echo "  PASS ($code) $desc"
    PASS=$((PASS+1))
  else
    echo "  FAIL ($code) $desc"
    FAIL=$((FAIL+1))
  fi
}

echo "=== LG WebOS Smash Deck API smoke tests ==="
echo "    Target: $BASE"
echo ""

check "GET /api/health"   "$BASE/api/health"
check "GET /api/settings" "$BASE/api/settings"
check "GET /api/state"    "$BASE/api/state"
check "GET /api/logs"     "$BASE/api/logs"
check "GET /"             "$BASE/"

# Method enforcement
echo ""
echo "--- Method enforcement ---"
bad=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/health")
if [[ "$bad" == "405" ]]; then
  echo "  PASS (405) DELETE /api/health correctly rejected"
  PASS=$((PASS+1))
else
  echo "  FAIL ($bad) DELETE /api/health should return 405"
  FAIL=$((FAIL+1))
fi

echo ""
echo "Results: PASS=$PASS  FAIL=$FAIL"
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
