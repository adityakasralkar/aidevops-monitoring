#!/usr/bin/env bash
# End-to-end check of signup, login, projects and logout against a running server.
#
# Usage (server must be running, e.g. `make run`):
#   tests/auth/auth_flow.sh                 # main flow
#   tests/auth/auth_flow.sh --rate-limit    # also check login rate limiting
#
# Reads TEST_USER_EMAIL and TEST_USER_PASSWORD from the environment or .env.
# BASE_URL defaults to http://localhost:8080.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if [ -f "$ROOT/.env" ]; then
  set -a; . "$ROOT/.env"; set +a
fi

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${TEST_USER_EMAIL:?set TEST_USER_EMAIL in .env}"
PASSWORD="${TEST_USER_PASSWORD:?set TEST_USER_PASSWORD in .env}"
# Lowercased once here: macOS's bash 3.2 mis-parses quotes nested in "$(...)".
EMAIL_LOWER="$(printf '%s' "$EMAIL" | tr '[:upper:]' '[:lower:]')"
CHECK_RATE_LIMIT=false
[ "${1:-}" = "--rate-limit" ] && CHECK_RATE_LIMIT=true

PASSED=0
FAILED=0
BODY=""
STATUS=""

# call METHOD PATH [JSON_BODY] [TOKEN] — sets STATUS and BODY.
call() {
  local method="$1" path="$2" data="${3:-}" token="${4:-}"
  local args=(-s -o /tmp/auth_flow_body.$$ -w '%{http_code}' -X "$method" "$BASE_URL$path")
  [ -n "$data" ] && args+=(-H 'Content-Type: application/json' --data "$data")
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  STATUS="$(curl "${args[@]}")"
  BODY="$(cat /tmp/auth_flow_body.$$ 2>/dev/null)"
  rm -f /tmp/auth_flow_body.$$
}

# check NAME EXPECTED_STATUS [TEXT_THAT_MUST_APPEAR] [TEXT_THAT_MUST_NOT_APPEAR]
check() {
  local name="$1" want="$2" contains="${3:-}" absent="${4:-}"
  local ok=true
  [ "$STATUS" = "$want" ] || ok=false
  [ -n "$contains" ] && ! grep -qF -- "$contains" <<<"$BODY" && ok=false
  [ -n "$absent" ] && grep -qF -- "$absent" <<<"$BODY" && ok=false
  if $ok; then
    echo "PASS  $name ($STATUS)"
    PASSED=$((PASSED + 1))
  else
    echo "FAIL  $name — got $STATUS, want $want; body: $BODY"
    FAILED=$((FAILED + 1))
  fi
}

# json_field FIELD — prints a top-level string field from BODY.
json_field() {
  python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get(sys.argv[2], ""))' "$BODY" "$1" 2>/dev/null
}

creds() { printf '{"email":"%s","password":"%s"}' "$1" "$2"; }

echo "Testing $BASE_URL as $EMAIL"
echo

call GET /healthz
check "health check" 200 '"status":"ok"'

# Signup: 201 the first time, 409 on later runs. Both mean the account exists.
call POST /api/v1/auth/signup "$(creds "$EMAIL" "$PASSWORD")"
if [ "$STATUS" = "409" ]; then
  check "signup (account already exists)" 409 '"email_taken"'
else
  check "signup" 201 "\"email\":\"$EMAIL_LOWER\"" "password"
fi

call POST /api/v1/auth/signup "$(creds "$EMAIL" "$PASSWORD")"
check "duplicate signup is rejected" 409 '"email_taken"'

call POST /api/v1/auth/signup "$(creds "not-an-email" "$PASSWORD")"
check "invalid email is rejected" 400 '"validation_failed"'

call POST /api/v1/auth/signup "$(creds "short-pass-$$@example.com" "short")"
check "short password is rejected" 400 '"validation_failed"'

call POST /api/v1/auth/login "$(creds "$EMAIL" "definitely-wrong-password")"
check "wrong password is rejected" 401 '"invalid_credentials"'

call POST /api/v1/auth/login "$(creds "$EMAIL" "$PASSWORD")"
check "login" 200 '"token_type":"Bearer"'
TOKEN="$(json_field token)"
[ -n "$TOKEN" ] && echo "      token received (${TOKEN:0:6}…)"

call GET /api/v1/auth/me "" "$TOKEN"
check "me with token" 200 "\"email\":"

call GET /api/v1/auth/me
check "me without token is rejected" 401 '"missing_token"'

call GET /api/v1/auth/me "" "not-a-real-token"
check "me with a fake token is rejected" 401 '"invalid_token"'

PROJECT="flow-test-$(date +%s)"
call POST /api/v1/projects "{\"name\":\"$PROJECT\",\"environment\":\"test\"}" "$TOKEN"
check "create project" 201 '"api_key":"aidv_test_'

call POST /api/v1/projects "{\"name\":\"$PROJECT\",\"environment\":\"test\"}" "$TOKEN"
check "duplicate project is rejected" 409 '"project_exists"'

call POST /api/v1/projects "{\"name\":\"$PROJECT\",\"environment\":\"staging\"}" "$TOKEN"
check "invalid environment is rejected" 400 '"validation_failed"'

call POST /api/v1/projects "{\"name\":\"$PROJECT\",\"environment\":\"test\"}"
check "create project without login is rejected" 401 '"missing_token"'

call GET /api/v1/projects "" "$TOKEN"
check "list projects (no keys shown)" 200 "\"name\":\"$PROJECT\"" '"api_key"'

call POST /api/v1/auth/logout "" "$TOKEN"
check "logout" 204

call GET /api/v1/auth/me "" "$TOKEN"
check "token no longer works after logout" 401 '"invalid_token"'

if $CHECK_RATE_LIMIT; then
  # Uses a random unknown email so the real test account is never locked.
  VICTIM="ratelimit-$$-$(date +%s)@example.com"
  for _ in 1 2 3 4 5; do
    call POST /api/v1/auth/login "$(creds "$VICTIM" "wrong-password")"
  done
  call POST /api/v1/auth/login "$(creds "$VICTIM" "wrong-password")"
  check "6th failed login is rate limited" 429 '"too_many_requests"'
fi

echo
echo "Passed: $PASSED  Failed: $FAILED"
[ "$FAILED" -eq 0 ]
