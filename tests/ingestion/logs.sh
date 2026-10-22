#!/usr/bin/env bash
# End-to-end check of log ingestion against a running server.
#
# Usage (server must be running, e.g. `make run`):
#   tests/ingestion/logs.sh
#
# Logs in with TEST_USER_EMAIL / TEST_USER_PASSWORD (from .env), creates a
# fresh project, sends logs with its API key, then checks MongoDB (through
# the aidevops-mongo Docker container) that every log landed in the right
# collection. BASE_URL defaults to http://localhost:8080.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if [ -f "$ROOT/.env" ]; then
  set -a; . "$ROOT/.env"; set +a
fi

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${TEST_USER_EMAIL:?set TEST_USER_EMAIL in .env}"
PASSWORD="${TEST_USER_PASSWORD:?set TEST_USER_PASSWORD in .env}"
DB_NAME="${MONGO_DB_NAME:-aidevops}"
MONGO_CONTAINER="${MONGO_CONTAINER:-aidevops-mongo}"

PASSED=0
FAILED=0
BODY=""
STATUS=""

# call METHOD PATH [JSON_BODY] [HEADER] — sets STATUS and BODY.
call() {
  local method="$1" path="$2" data="${3:-}" header="${4:-}"
  local args=(-s -o /tmp/logs_body.$$ -w '%{http_code}' -X "$method" "$BASE_URL$path")
  [ -n "$data" ] && args+=(-H 'Content-Type: application/json' --data "$data")
  [ -n "$header" ] && args+=(-H "$header")
  STATUS="$(curl "${args[@]}")"
  BODY="$(cat /tmp/logs_body.$$ 2>/dev/null)"
  rm -f /tmp/logs_body.$$
}

# check NAME EXPECTED_STATUS [TEXT_THAT_MUST_APPEAR]
check() {
  local name="$1" want="$2" contains="${3:-}"
  if [ "$STATUS" = "$want" ] && { [ -z "$contains" ] || grep -qF -- "$contains" <<<"$BODY"; }; then
    echo "PASS  $name ($STATUS)"
    PASSED=$((PASSED + 1))
  else
    echo "FAIL  $name — got $STATUS, want $want; body: $BODY"
    FAILED=$((FAILED + 1))
  fi
}

# check_count NAME MONGO_FILTER COLLECTION WANT
# Counts documents in MongoDB. An empty answer (query or connection
# problem) always fails, so a broken check can never pass by accident.
check_count() {
  local name="$1" filter="$2" coll="$3" want="$4"
  local query="db.$coll.countDocuments($filter)"
  local got
  got="$(mongo_eval "$query")"
  if [ -n "$got" ] && [ "$got" = "$want" ]; then
    echo "PASS  $name ($got)"
    PASSED=$((PASSED + 1))
  else
    echo "FAIL  $name — got '${got}', want $want (query: $query)"
    FAILED=$((FAILED + 1))
  fi
}

json_field() {
  python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get(sys.argv[2], ""))' "$BODY" "$1" 2>/dev/null
}

mongo_eval() {
  docker exec "$MONGO_CONTAINER" mongosh "$DB_NAME" --quiet --eval "$1" 2>/dev/null | tr -d '[:space:]'
}

echo "Testing $BASE_URL"
echo

# ── Setup: account, login, project ──
call POST /api/v1/auth/signup "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
[ "$STATUS" = "201" ] || [ "$STATUS" = "409" ] || { echo "FAIL  setup signup: $STATUS $BODY"; exit 1; }

call POST /api/v1/auth/login "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
check "setup: login" 200
TOKEN="$(json_field token)"

call POST /api/v1/projects "{\"name\":\"logs-test-$(date +%s)\",\"environment\":\"test\"}" "Authorization: Bearer $TOKEN"
check "setup: create project" 201 '"api_key":"aidv_test_'
KEY="$(json_field api_key)"
PROJECT_ID="$(json_field id)"
KEY_HEADER="X-API-Key: $KEY"

# ── Auth ──
call POST /api/v1/telemetry/logs '{"service_name":"a","severity":"INFO","message":"m"}'
check "missing API key is rejected" 401 '"missing_api_key"'

call POST /api/v1/telemetry/logs '{"service_name":"a","severity":"INFO","message":"m"}' "X-API-Key: aidv_test_fake"
check "wrong API key is rejected" 401 '"invalid_api_key"'

# ── Accepted logs ──
call POST /api/v1/telemetry/logs '{"service_name":"checkout","severity":"INFO","message":"order placed","attributes":{"order_id":"A1","amount":499}}' "$KEY_HEADER"
check "single INFO log" 202 '"status":"accepted"'

call POST /api/v1/telemetry/logs '{"service_name":"checkout","severity":"ERROR","message":"payment gateway timeout","stack_trace":"at pay.go:42"}' "$KEY_HEADER"
check "single ERROR log" 202 '"id":'

call POST /api/v1/telemetry/logs '[
  {"service_name":"api","severity":"INFO","message":"request ok"},
  {"service_name":"api","severity":"info","message":"request ok"},
  {"service_name":"api","severity":"WARN","message":"slow query"},
  {"service_name":"db","severity":"ERROR","message":"pool exhausted"},
  {"service_name":"db","severity":"ERROR","message":"pool exhausted"},
  {"service_name":"db","severity":"CRITICAL","message":"database down"}
]' "$KEY_HEADER"
check "batch of 6 logs" 202 '"count":6'

# ── Rejected logs ──
call POST /api/v1/telemetry/logs '{"service_name":"a","severity":"DEBUG","message":"m"}' "$KEY_HEADER"
check "unknown severity is rejected" 400 '"validation_failed"'

call POST /api/v1/telemetry/logs '[{"service_name":"a","severity":"INFO","message":"ok"},{"service_name":"a","severity":"INFO"}]' "$KEY_HEADER"
check "batch with one bad log is rejected" 400 'logs[1]'

call POST /api/v1/telemetry/logs '{"service_name":"a","severity":"INFO","message":"m","project_id":"other"}' "$KEY_HEADER"
check "sender cannot set project_id" 400 'unknown field'

# ── Stored in MongoDB ──
if ! command -v docker >/dev/null || ! docker ps --format '{{.Names}}' | grep -qx "$MONGO_CONTAINER"; then
  echo "SKIP  MongoDB checks (container $MONGO_CONTAINER not running)"
else
  sleep 1 # let the background workers finish writing
  # Filters are built in plain variables: macOS's bash 3.2 mis-parses
  # quotes nested inside "$(...)".
  P="project_id: ObjectId('$PROJECT_ID')"
  check_count "ErrorLogs has 4 logs for the project" "{$P}" ErrorLogs 4
  check_count "AppLogs has 4 logs for the project" "{$P}" AppLogs 4
  check_count "every error log is PENDING diagnosis" "{$P, diagnosis_status: 'PENDING'}" ErrorLogs 4
  check_count "no app log has a diagnosis status" "{$P, diagnosis_status: {\$exists: true}}" AppLogs 0
  check_count "severity stored in capitals" "{$P, severity: 'INFO'}" AppLogs 3
  check_count "attributes stored" "{$P, 'attributes.order_id': 'A1'}" AppLogs 1
fi

echo
echo "Passed: $PASSED  Failed: $FAILED"
[ "$FAILED" -eq 0 ]
