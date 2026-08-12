#!/usr/bin/env bash
#
# Hits every endpoint of a running deployment and prints the status of each.
#
# This is a reachability and auth check, not a contract check: it confirms each
# route is mounted, answers, and enforces the credential it is supposed to.
# It does not compare response bodies against a recorded baseline — verifying
# payload shapes is a manual pass.
#
# Usage:
#   BASE_URL=https://api.example.com \
#   MC_API_KEY=...            # tenant external API key
#   INTERNAL_API_KEY=...      # x-internal-api-key
#   BEARER_TOKEN=...          # NextAuth go-token JWT
#   TENANT_ID=...             # a tenant id, for the AI endpoints
#   EMAIL=...                 # a member's email, for the member endpoints
#   ./scripts/smoke.sh
#
# Every variable is optional. Requests whose credential is missing are skipped
# and reported as SKIP, so a partial set still exercises what it can.

set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8181}"
MC_API_KEY="${MC_API_KEY:-}"
INTERNAL_API_KEY="${INTERNAL_API_KEY:-}"
BEARER_TOKEN="${BEARER_TOKEN:-}"
TENANT_ID="${TENANT_ID:-}"
EMAIL="${EMAIL:-}"

pass=0
fail=0
skipped=0

green=$'\033[32m'; red=$'\033[31m'; yellow=$'\033[33m'; dim=$'\033[2m'; reset=$'\033[0m'

# check <expected-status> <label> <curl args...>
check() {
  local expected="$1" label="$2"; shift 2
  local status
  status=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$@" 2>/dev/null)

  if [[ "$status" == "$expected" ]]; then
    printf '%s  PASS%s  %-52s %s\n' "$green" "$reset" "$label" "$status"
    pass=$((pass + 1))
  else
    printf '%s  FAIL%s  %-52s %s (expected %s)\n' "$red" "$reset" "$label" "$status" "$expected"
    fail=$((fail + 1))
  fi
}

skip() {
  printf '%s  SKIP%s  %-52s %s%s%s\n' "$yellow" "$reset" "$1" "$dim" "$2" "$reset"
  skipped=$((skipped + 1))
}

section() { printf '\n%s== %s%s\n' "$dim" "$1" "$reset"; }

printf 'Smoke test against %s\n' "$BASE_URL"

# ---------- health & docs ----------

section "health"
# 503 here means the process is up but a dependency is not. Run this first:
# every check below it will fail in a way that looks like a routing problem.
check 200 "GET  /health"            "$BASE_URL/health"

section "docs"
check 200 "GET  /docs/"             "$BASE_URL/docs/"
check 200 "GET  /docs/swagger.yaml" "$BASE_URL/docs/swagger.yaml"

# ---------- auth enforcement ----------
#
# These run with no credentials on purpose: an endpoint that answers 200 here
# is not enforcing the credential it should.

section "auth enforcement (no credentials)"
check 401 "GET  /api/v1/user/informations"      "$BASE_URL/api/v1/user/informations"
check 401 "GET  /api/v1/vitrine"                "$BASE_URL/api/v1/vitrine"
check 401 "GET  /api/v1/comments"               "$BASE_URL/api/v1/comments"
check 401 "GET  /api/v1/student/report"         "$BASE_URL/api/v1/student/report"
check 401 "GET  /api/v1/users/purchases"        "$BASE_URL/api/v1/users/purchases"
check 401 "GET  /api/v1/users/payment-events"   "$BASE_URL/api/v1/users/payment-events"
check 401 "GET  /api/v1/user/activities"        "$BASE_URL/api/v1/user/activities"
check 401 "GET  /api/v1/user/activity/summary"  "$BASE_URL/api/v1/user/activity/summary"
check 401 "GET  /api/v1/user/lessons/completed" "$BASE_URL/api/v1/user/lessons/completed"
check 401 "POST /api/v1/social"                 -X POST "$BASE_URL/api/v1/social"
check 401 "GET  /api/v1/ai/lessons"             "$BASE_URL/api/v1/ai/lessons"
check 401 "GET  /api/v1/ai/tenants"             "$BASE_URL/api/v1/ai/tenants"
check 401 "POST /api/v1/sso/generate-token"     -X POST "$BASE_URL/api/v1/sso/generate-token"
check 401 "POST /api/v1/auth"                   -X POST "$BASE_URL/api/v1/auth"
check 401 "GET  /api/comments"                  "$BASE_URL/api/comments"
check 401 "POST /api/lessons/pdf-process"       -X POST "$BASE_URL/api/lessons/pdf-process"
check 401 "POST /api/lessons/process-all-pdfs"  -X POST "$BASE_URL/api/lessons/process-all-pdfs"
check 401 "GET  /api/lessons/x/pdf-pages"       "$BASE_URL/api/lessons/x/pdf-pages"
check 401 "POST /api/lessons/x/pdf-regenerate"  -X POST "$BASE_URL/api/lessons/x/pdf-regenerate"
check 401 "POST /imports/members"               -X POST "$BASE_URL/imports/members"

# ---------- tenant API key ----------

section "tenant endpoints (mc-api-key)"
if [[ -z "$MC_API_KEY" ]]; then
  skip "tenant endpoints" "set MC_API_KEY"
else
  H=(-H "mc-api-key: $MC_API_KEY")

  check 200 "GET  /api/v1/vitrine"                "${H[@]}" "$BASE_URL/api/v1/vitrine"
  check 200 "GET  /api/v1/comments"               "${H[@]}" "$BASE_URL/api/v1/comments"
  check 200 "GET  /api/v1/student/report"         "${H[@]}" "$BASE_URL/api/v1/student/report"
  check 200 "GET  /api/v1/user/informations"      "${H[@]}" "$BASE_URL/api/v1/user/informations"
  check 200 "GET  /api/v1/user/activities"        "${H[@]}" "$BASE_URL/api/v1/user/activities"
  check 200 "GET  /api/v1/user/activity/summary"  "${H[@]}" "$BASE_URL/api/v1/user/activity/summary"

  # These require an email; without one they answer 400, which still proves
  # the route and its auth are wired.
  if [[ -n "$EMAIL" ]]; then
    check 200 "GET  /api/v1/users/purchases"        "${H[@]}" "$BASE_URL/api/v1/users/purchases?email=$EMAIL"
    check 200 "GET  /api/v1/users/payment-events"   "${H[@]}" "$BASE_URL/api/v1/users/payment-events?email=$EMAIL"
    check 200 "GET  /api/v1/user/lessons/completed" "${H[@]}" "$BASE_URL/api/v1/user/lessons/completed?email=$EMAIL"
    check 200 "POST /api/v1/auth (magic link)"      "${H[@]}" -X POST \
      -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\"}" "$BASE_URL/api/v1/auth"
  else
    check 400 "GET  /api/v1/users/purchases (no email)" "${H[@]}" "$BASE_URL/api/v1/users/purchases"
    check 400 "GET  /api/v1/users/payment-events (no email)" "${H[@]}" "$BASE_URL/api/v1/users/payment-events"
    skip "GET  /api/v1/user/lessons/completed" "set EMAIL"
    skip "POST /api/v1/auth" "set EMAIL"
  fi
fi

# ---------- internal API key ----------

section "internal endpoints (x-internal-api-key)"
if [[ -z "$INTERNAL_API_KEY" ]]; then
  skip "internal endpoints" "set INTERNAL_API_KEY"
else
  H=(-H "x-internal-api-key: $INTERNAL_API_KEY")

  check 200 "GET  /api/v1/ai/tenants" "${H[@]}" "$BASE_URL/api/v1/ai/tenants"

  if [[ -n "$TENANT_ID" ]]; then
    check 200 "GET  /api/v1/ai/lessons" "${H[@]}" "$BASE_URL/api/v1/ai/lessons?tenantId=$TENANT_ID"
    check 200 "GET  /api/v1/ai/transcription-stats" "${H[@]}" \
      "$BASE_URL/api/v1/ai/transcription-stats?tenantId=$TENANT_ID"
  else
    check 400 "GET  /api/v1/ai/lessons (no tenantId)" "${H[@]}" "$BASE_URL/api/v1/ai/lessons"
    skip "GET  /api/v1/ai/transcription-stats" "set TENANT_ID"
  fi

  # 404 proves the route is mounted and authorised: the lesson id is fake.
  check 404 "GET  /api/lessons/{id}/pdf-pages" "${H[@]}" "$BASE_URL/api/lessons/smoke-test-id/pdf-pages"
fi

# ---------- bearer token ----------

section "admin endpoints (Bearer)"
if [[ -z "$BEARER_TOKEN" ]]; then
  skip "POST /imports/members" "set BEARER_TOKEN"
else
  # An empty member list is rejected with 400 — enough to prove auth passes
  # without importing anyone.
  check 400 "POST /imports/members (empty body)" \
    -H "Authorization: Bearer $BEARER_TOKEN" -H 'Content-Type: application/json' \
    -X POST -d '{}' "$BASE_URL/imports/members"
fi

# ---------- not exercised ----------

section "not exercised"
skip "POST /api/v1/videos/upload"            "uploads a real file to Bunny"
skip "POST /api/v1/sso/generate-token"       "mints a live SSO token"
skip "POST /api/v1/sso/validate-token"       "consumes a one-time token"
skip "POST /api/v1/ai/tenants/process-lessons" "enqueues real transcription work"
skip "POST /api/lessons/pdf-process"         "starts real PDF processing"
skip "POST /api/lessons/process-all-pdfs"    "starts real PDF processing"

printf '\n%s%d passed%s, %s%d failed%s, %s%d skipped%s\n' \
  "$green" "$pass" "$reset" "$red" "$fail" "$reset" "$yellow" "$skipped" "$reset"

[[ "$fail" -eq 0 ]]
