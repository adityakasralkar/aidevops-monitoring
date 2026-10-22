#!/usr/bin/env bash
# End-to-end check of metric ingestion against a running server.
#
# Usage (server must be running, e.g. `make run`):
#   tests/ingestion/metrics.sh
#
# Logs in with TEST_USER_EMAIL / TEST_USER_PASSWORD (from .env), creates a
# fresh project, sends gauge, sum and histogram metrics with its API key,
# then checks MongoDB (through the aidevops-mongo Docker container) that
# they were stored correctly. BASE_URL defaults to http://localhost:8080.
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
  local args=(-s -o /tmp/metrics_body.$$ -w '%{http_code}' -X "$method" "$BASE_URL$path")
  [ -n "$data" ] && args+=(-H 'Content-Type: application/json' --data "$data")
  [ -n "$header" ] && args+=(-H "$header")
  STATUS="$(curl "${args[@]}")"
  BODY="$(cat /tmp/metrics_body.$$ 2>/dev/null)"
  rm -f /tmp/metrics_body.$$
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

call POST /api/v1/projects "{\"name\":\"metrics-test-$(date +%s)\",\"environment\":\"test\"}" "Authorization: Bearer $TOKEN"
check "setup: create project" 201 '"api_key":"aidv_test_'
KEY="$(json_field api_key)"
PROJECT_ID="$(json_field id)"
KEY_HEADER="X-API-Key: $KEY"

# ── Auth ──
call POST /api/v1/telemetry/metrics '{"service_name":"db","metric_name":"cpu","metric_type":"gauge","value":1}'
check "missing API key is rejected" 401 '"missing_api_key"'

# ── Accepted metrics ──
call POST /api/v1/telemetry/metrics '{"service_name":"db","metric_name":"db_active_connections","metric_type":"gauge","value":42}' "$KEY_HEADER"
check "gauge" 202 '"status":"accepted"'

call POST /api/v1/telemetry/metrics '{"service_name":"db","metric_name":"queue_depth","metric_type":"gauge","value":0}' "$KEY_HEADER"
check "gauge with value 0" 202 '"id":'

call POST /api/v1/telemetry/metrics '[
  {"service_name":"api","metric_name":"http.server.requests","metric_type":"sum","value":1532,"attributes":{"http.route":"/pay"}},
  {"service_name":"api","metric_name":"http.server.duration","metric_type":"histogram","unit":"ms",
   "count":10,"sum":1234.5,"min":12,"max":480,"bucket_bounds":[100,250,500],"bucket_counts":[4,3,3,0]}
]' "$KEY_HEADER"
check "batch: sum + histogram" 202 '"count":2'

# ── Rejected metrics ──
call POST /api/v1/telemetry/metrics '{"service_name":"db","metric_name":"cpu","metric_type":"counter","value":1}' "$KEY_HEADER"
check "unknown metric_type is rejected" 400 '"validation_failed"'

call POST /api/v1/telemetry/metrics '{"service_name":"api","metric_name":"lat","metric_type":"histogram","count":5,"sum":1,"bucket_bounds":[10],"bucket_counts":[1,1]}' "$KEY_HEADER"
check "histogram whose buckets do not add up is rejected" 400 'add up to 2 but count is 5'

call POST /api/v1/telemetry/metrics '[{"service_name":"db","metric_name":"cpu","metric_type":"gauge","value":1},{"service_name":"db","metric_name":"cpu","metric_type":"gauge"}]' "$KEY_HEADER"
check "batch with one bad metric is rejected" 400 'metrics[1]'

# ── Stored in MongoDB ──
if ! command -v docker >/dev/null || ! docker ps --format '{{.Names}}' | grep -qx "$MONGO_CONTAINER"; then
  echo "SKIP  MongoDB checks (container $MONGO_CONTAINER not running)"
else
  sleep 1 # let the background workers finish writing
  # Filters are built in plain variables: macOS's bash 3.2 mis-parses
  # quotes nested inside "$(...)".
  P="project_id: ObjectId('$PROJECT_ID')"
  check_count "TelemetryMetrics has 4 metrics for the project" "{$P}" TelemetryMetrics 4
  check_count "gauge value stored" "{$P, metric_name: 'db_active_connections', value: 42}" TelemetryMetrics 1
  check_count "zero value stored (not dropped)" "{$P, metric_name: 'queue_depth', value: 0}" TelemetryMetrics 1
  check_count "histogram stored with 4 buckets" "{$P, metric_type: 'histogram', count: 10, bucket_counts: {\$size: 4}}" TelemetryMetrics 1
  # Attribute names may contain dots (OpenTelemetry style, e.g. "http.route").
  # They are stored as one key, so they are read with $getField; a plain
  # 'attributes.http.route' path would look for nested objects instead.
  ROUTE="{\$getField: {field: 'http.route', input: '\$attributes'}}"
  check_count "dotted attribute name stored as one key" "{$P, metric_type: 'sum', \$expr: {\$eq: [$ROUTE, '/pay']}}" TelemetryMetrics 1
fi

echo
echo "Passed: $PASSED  Failed: $FAILED"
[ "$FAILED" -eq 0 ]
