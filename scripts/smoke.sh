#!/usr/bin/env bash
# Smoke the USD customer journey against a running gateway.
# Requires: curl, jq, gateway on $BASE (default http://localhost:8080)
set -euo pipefail
BASE="${BASE:-http://localhost:8080}"
EMAIL="smoke-$(date +%s)@example.com"
PASS="password123"

echo "register $EMAIL"
curl -fsS -X POST "$BASE/api/v1/auth/register" -H 'content-type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"full_name\":\"Smoke User\"}" >/dev/null

echo "login"
TOKEN=$(curl -fsS -X POST "$BASE/api/v1/auth/login" -H 'content-type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" | jq -r .access_token)

auth=(-H "authorization: Bearer $TOKEN" -H 'content-type: application/json')

echo "open account"
ACC=$(curl -fsS -X POST "$BASE/api/v1/accounts" "${auth[@]}" -d '{"type":"SAVINGS","currency":"USD"}' | jq -r .id)

echo "deposit 100"
curl -fsS -X POST "$BASE/api/v1/transactions/deposit" "${auth[@]}" \
  -H 'idempotency-key: smoke-dep' \
  -d "{\"account_id\":\"$ACC\",\"amount\":\"100.00\",\"currency\":\"USD\"}" >/dev/null

echo "withdraw 40"
curl -fsS -X POST "$BASE/api/v1/transactions/withdraw" "${auth[@]}" \
  -H 'idempotency-key: smoke-wd' \
  -d "{\"account_id\":\"$ACC\",\"amount\":\"40.00\",\"currency\":\"USD\"}" >/dev/null

BAL=$(curl -fsS "$BASE/api/v1/accounts/$ACC/balance" -H "authorization: Bearer $TOKEN" | jq -r .balance)
echo "balance=$BAL"
test "$BAL" = "60.00" || test "$BAL" = "60"
echo "smoke ok"
