#!/usr/bin/env bash
# Guided live demo of the monitoring server, one step per Enter press.
#
# Usage (MongoDB + server running: `make up`, then `make run`):
#   tests/demo.sh            # full demo
#   tests/demo.sh --no-wait  # run straight through (practice / check)
#
# Each run uses a fresh demo account, so it can be repeated any time.
set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DB_NAME="${MONGO_DB_NAME:-aidevops}"
MONGO_CONTAINER="${MONGO_CONTAINER:-aidevops-mongo}"
WAIT=true
[ "${1:-}" = "--no-wait" ] && WAIT=false

RUN_ID="$(date +%H%M%S)"
EMAIL="demo-$RUN_ID@example.com"
PASSWORD="demo-pass-2026"

BOLD=$'\033[1m'; DIM=$'\033[2m'; GREEN=$'\033[32m'; RED=$'\033[31m'; CYAN=$'\033[36m'; YELLOW=$'\033[33m'; RESET=$'\033[0m'

STATUS=""
BODY=""

# step NUMBER TITLE SAY — prints a step header and waits for Enter.
step() {
  echo
  echo "${BOLD}${CYAN}━━━ Step $1 — $2 ━━━${RESET}"
  echo "${YELLOW}Say:${RESET} $3"
  if $WAIT; then
    read -r -p "${DIM}(press Enter to run)${RESET}" _
  fi
}

# call METHOD PATH [JSON] [HEADER] — runs the request and prints the result.
call() {
  local method="$1" path="$2" data="${3:-}" header="${4:-}"
  local args=(-s -o /tmp/demo_body.$$ -w '%{http_code}' -X "$method" "$BASE_URL$path")
  [ -n "$data" ] && args+=(-H 'Content-Type: application/json' --data "$data")
  [ -n "$header" ] && args+=(-H "$header")
  echo "${DIM}→ $method $path${RESET}"
  STATUS="$(curl "${args[@]}")"
  BODY="$(cat /tmp/demo_body.$$ 2>/dev/null)"
  rm -f /tmp/demo_body.$$
  local color="$GREEN"
  case "$STATUS" in 4*|5*) color="$RED" ;; esac
  echo "${color}${BOLD}HTTP $STATUS${RESET}"
  if [ -n "$BODY" ]; then
    printf '%s' "$BODY" | python3 -m json.tool 2>/dev/null | head -20 || echo "$BODY"
  fi
}

json_field() {
  python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get(sys.argv[2], ""))' "$BODY" "$1" 2>/dev/null
}

mongo_eval() {
  docker exec "$MONGO_CONTAINER" mongosh "$DB_NAME" --quiet --eval "$1" 2>/dev/null
}

clear
echo "${BOLD}AIDevOps Monitoring — live demo${RESET}"
echo "Server: $BASE_URL   Demo user: $EMAIL"

# ── 1 ───────────────────────────────────────────────────────────────
step 1 "Server and database are up" \
  "The Go server is running and connected to MongoDB. /healthz checks both."
call GET /healthz

# ── 2 ───────────────────────────────────────────────────────────────
step 2 "Sign up" \
  "A user creates an account. The password is hashed with bcrypt; it is never stored or returned."
call POST /api/v1/auth/signup "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"

# ── 3 ───────────────────────────────────────────────────────────────
step 3 "Log in" \
  "Login returns a session token valid for 24 hours. Only its SHA-256 hash is stored in the Sessions collection."
call POST /api/v1/auth/login "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
TOKEN="$(json_field token)"

# ── 4 ───────────────────────────────────────────────────────────────
step 4 "Register a project and get its API key" \
  "The user registers the app they want to monitor. The API key is shown only once; the database keeps only its hash and a short prefix."
call POST /api/v1/projects "{\"name\":\"checkout-service-$RUN_ID\",\"environment\":\"test\"}" "Authorization: Bearer $TOKEN"
KEY="$(json_field api_key)"
PROJECT_ID="$(json_field id)"
echo "${DIM}(Compass → Applications: you will see api_key_hash and key_prefix, never the full key)${RESET}"

# ── 5 ───────────────────────────────────────────────────────────────
step 5 "Security: wrong key, no key, bad data" \
  "Every telemetry request is checked. A wrong or missing key is rejected with 401, and invalid data with 400 and a clear message."
call POST /api/v1/telemetry/logs '{"service_name":"checkout","severity":"ERROR","message":"x"}' "X-API-Key: aidv_test_this-key-is-fake"
call POST /api/v1/telemetry/logs '{"service_name":"checkout","severity":"ERROR","message":"x"}'
call POST /api/v1/telemetry/logs '{"service_name":"checkout","severity":"LOUD","message":"x"}' "X-API-Key: $KEY"

# ── 6 ───────────────────────────────────────────────────────────────
step 6 "The app sends logs (single + batch)" \
  "The app sends logs with its key. The server validates, queues and replies 202 at once — the app never waits for the database. Workers save in the background: ERROR and CRITICAL go to ErrorLogs marked PENDING for AI diagnosis, INFO and WARN go to AppLogs."
call POST /api/v1/telemetry/logs '{"service_name":"checkout","severity":"ERROR","message":"payment gateway timeout after 30s","stack_trace":"at PaymentClient.charge (payment.go:142)","attributes":{"http.route":"/pay","order_id":"A-1042"}}' "X-API-Key: $KEY"
call POST /api/v1/telemetry/logs '[
  {"service_name":"checkout","severity":"INFO","message":"order placed","attributes":{"order_id":"A-1043"}},
  {"service_name":"checkout","severity":"WARN","message":"slow database query: 1.8s"},
  {"service_name":"database","severity":"CRITICAL","message":"connection pool exhausted (50/50)"}
]' "X-API-Key: $KEY"

# ── 7 ───────────────────────────────────────────────────────────────
step 7 "The app sends metrics" \
  "Metrics use the same key and pipeline. Three types: gauge (a value now), sum (a running total) and histogram (a distribution, so p95 latency can be computed)."
call POST /api/v1/telemetry/metrics '[
  {"service_name":"database","metric_name":"db_active_connections","metric_type":"gauge","value":50},
  {"service_name":"checkout","metric_name":"http.server.requests","metric_type":"sum","value":1532},
  {"service_name":"checkout","metric_name":"http.server.duration","metric_type":"histogram","unit":"ms","count":10,"sum":2140,"min":35,"max":980,"bucket_bounds":[100,250,500],"bucket_counts":[3,3,2,2]}
]' "X-API-Key: $KEY"

# ── 8 ───────────────────────────────────────────────────────────────
step 8 "Data stored in MongoDB, per project" \
  "Everything is saved under this project's id only. Now I'll show the same in Compass."
sleep 1
P="ObjectId('$PROJECT_ID')"
QUERY="const p=$P;
print('ErrorLogs        :', db.ErrorLogs.countDocuments({project_id:p}), '(PENDING:', db.ErrorLogs.countDocuments({project_id:p, diagnosis_status:'PENDING'}) + ')');
print('AppLogs          :', db.AppLogs.countDocuments({project_id:p}));
print('TelemetryMetrics :', db.TelemetryMetrics.countDocuments({project_id:p}));
print('');
print('Latest error log:');
printjson(db.ErrorLogs.find({project_id:p},{_id:0,service_name:1,severity:1,message:1,diagnosis_status:1}).sort({timestamp:-1}).limit(1).toArray()[0]);"
mongo_eval "$QUERY" || echo "${RED}(MongoDB container not reachable — show Compass instead)${RESET}"
echo "${DIM}(Compass → database aidevops → ErrorLogs / AppLogs / TelemetryMetrics)${RESET}"

# ── 9 ───────────────────────────────────────────────────────────────
step 9 "Load: many logs at once" \
  "Now 10 requests of 500 logs each arrive together — 5,000 logs. The queue is bounded: what fits is accepted (202); if it is full the server answers 503 so the sender retries, instead of crashing or running out of memory."
python3 -c 'import json; print(json.dumps([{"service_name":"load-test","severity":["INFO","WARN","ERROR","CRITICAL"][i%4],"message":"load log %d"%i} for i in range(500)]))' > /tmp/demo_batch.$$
for _ in 1 2 3 4 5 6 7 8 9 10; do
  curl -s -o /dev/null -w '%{http_code}\n' -X POST "$BASE_URL/api/v1/telemetry/logs" \
    -H 'Content-Type: application/json' -H "X-API-Key: $KEY" --data @/tmp/demo_batch.$$ >> /tmp/demo_codes.$$ &
done
wait
rm -f /tmp/demo_batch.$$
ACCEPTED=$(grep -c '^202$' /tmp/demo_codes.$$ || true)
BUSY=$(grep -c '^503$' /tmp/demo_codes.$$ || true)
rm -f /tmp/demo_codes.$$
echo "${GREEN}${BOLD}Accepted: $ACCEPTED requests ($((ACCEPTED * 500)) logs)${RESET}   ${YELLOW}Busy (503): $BUSY${RESET}"
sleep 2
mongo_eval "const p=$P; print('Load-test logs saved in MongoDB:', db.ErrorLogs.countDocuments({project_id:p, service_name:'load-test'}) + db.AppLogs.countDocuments({project_id:p, service_name:'load-test'}))"

# ── 10 ──────────────────────────────────────────────────────────────
step 10 "Clean shutdown (in the server terminal)" \
  "When I press Ctrl+C on the server, it stops taking requests, saves everything still in the queue, then closes the database. The last log line shows accepted = saved, dropped = 0."
echo "${DIM}→ Switch to the server terminal and press Ctrl+C now.${RESET}"

echo
echo "${BOLD}${GREEN}Demo complete.${RESET} Next phases: OpenTelemetry endpoints, AI diagnosis of PENDING errors, dashboard, fault-injection evaluation."
