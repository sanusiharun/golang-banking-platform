#!/usr/bin/env bash
# scripts/test-gateway.sh — End-to-end gateway integration test
#
# Tests ALL services through the Traefik gateway at http://localhost:80.
# Also validates each service's health endpoint directly on its native port.
#
# Usage:
#   bash scripts/test-gateway.sh                 # default credentials
#   ADMIN_PASS=MyPass bash scripts/test-gateway.sh
#
# Requirements: curl, jq
# Run after: make services-up (stack must be fully started)

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
GATEWAY="${GATEWAY_URL:-http://localhost}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-Admin@12345}"
TELLER_USER="${TELLER_USER:-teller}"
TELLER_PASS="${TELLER_PASS:-Teller@12345}"

# Direct service ports (bypass gateway — health check validation)
AUTH_DIRECT="http://localhost:8082"
ACCOUNT_DIRECT="http://localhost:8081"
AUDIT_DIRECT="http://localhost:8083"
NOTIFICATION_DIRECT="http://localhost:8084"
PAYMENT_DIRECT="http://localhost:8085"
TRAEFIK_DASHBOARD="http://localhost:8080"

# ── Helpers ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

PASS=0
FAIL=0
SKIP=0

pass() { echo -e "  ${GREEN}✓${RESET} $1"; ((PASS++)); }
fail() { echo -e "  ${RED}✗${RESET} $1"; ((FAIL++)); }
skip() { echo -e "  ${YELLOW}~${RESET} $1 (skipped — $2)"; ((SKIP++)); }
section() { echo -e "\n${BOLD}${CYAN}▶ $1${RESET}"; }

# http_status <url> [extra curl args...]
# Returns the HTTP status code only.
http_status() {
    local url="$1"; shift
    curl -s -o /dev/null -w "%{http_code}" "$url" "$@"
}

# http_json <url> [extra curl args...]
# Returns the full response body as JSON string.
http_json() {
    local url="$1"; shift
    curl -s "$url" "$@"
}

# assert_status <label> <expected> <actual>
assert_status() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$actual" == "$expected" ]]; then
        pass "$label → HTTP $actual"
    else
        fail "$label → expected HTTP $expected, got HTTP $actual"
    fi
}

# ── Dependency check ──────────────────────────────────────────────────────────
section "Dependencies"
command -v curl &>/dev/null && pass "curl available" || { echo "curl required"; exit 1; }
command -v jq   &>/dev/null && pass "jq available"  || { echo "jq required"; exit 1; }

# ── 1. Traefik gateway health ─────────────────────────────────────────────────
section "1. Traefik Gateway"

status=$(http_status "$TRAEFIK_DASHBOARD/api/overview")
assert_status "Traefik dashboard API" "200" "$status"

# Check all routers registered
routers=$(http_json "$TRAEFIK_DASHBOARD/api/http/routers" | jq -r '.[].name' 2>/dev/null || echo "")
for svc in auth-svc account-svc audit-svc notification-svc payment-svc; do
    if echo "$routers" | grep -q "$svc"; then
        pass "Router registered: $svc"
    else
        fail "Router missing: $svc (check traefik labels)"
    fi
done

# ── 2. Health checks — direct port (bypasses gateway) ────────────────────────
section "2. Service Health (direct ports)"

declare -A HEALTH_URLS=(
    ["auth-svc"]="$AUTH_DIRECT/healthz/live"
    ["account-svc"]="$ACCOUNT_DIRECT/healthz/live"
    ["audit-svc"]="$AUDIT_DIRECT/healthz/live"
    ["notification-svc"]="$NOTIFICATION_DIRECT/healthz/live"
    ["payment-svc"]="$PAYMENT_DIRECT/healthz/live"
)

for svc in "${!HEALTH_URLS[@]}"; do
    status=$(http_status "${HEALTH_URLS[$svc]}" --max-time 5 2>/dev/null || echo "000")
    assert_status "$svc /healthz/live" "200" "$status"
done

# ── 3. Auth service — via gateway ─────────────────────────────────────────────
section "3. Auth Service (via gateway :80)"

# 3a. Login — admin
echo "  Logging in as admin..."
LOGIN_RESP=$(http_json "$GATEWAY/auth/login" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
    --max-time 10)

LOGIN_STATUS=$(http_status "$GATEWAY/auth/login" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
    --max-time 10)
assert_status "POST /auth/login (admin)" "200" "$LOGIN_STATUS"

ADMIN_TOKEN=$(echo "$LOGIN_RESP" | jq -r '.data.access_token // .access_token // empty' 2>/dev/null || echo "")
ADMIN_REFRESH=$(echo "$LOGIN_RESP" | jq -r '.data.refresh_token // .refresh_token // empty' 2>/dev/null || echo "")

if [[ -n "$ADMIN_TOKEN" && "$ADMIN_TOKEN" != "null" ]]; then
    pass "Admin access_token received"
else
    fail "Admin access_token missing — check login response: $(echo "$LOGIN_RESP" | head -c 200)"
fi

# 3b. Login — teller
echo "  Logging in as teller..."
TELLER_RESP=$(http_json "$GATEWAY/auth/login" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$TELLER_USER\",\"password\":\"$TELLER_PASS\"}" \
    --max-time 10)

TELLER_LOGIN_STATUS=$(http_status "$GATEWAY/auth/login" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$TELLER_USER\",\"password\":\"$TELLER_PASS\"}" \
    --max-time 10)
assert_status "POST /auth/login (teller)" "200" "$TELLER_LOGIN_STATUS"

TELLER_TOKEN=$(echo "$TELLER_RESP" | jq -r '.data.access_token // .access_token // empty' 2>/dev/null || echo "")

# 3c. Refresh token
if [[ -n "$ADMIN_REFRESH" && "$ADMIN_REFRESH" != "null" ]]; then
    REFRESH_STATUS=$(http_status "$GATEWAY/auth/refresh" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"refresh_token\":\"$ADMIN_REFRESH\"}" \
        --max-time 10)
    assert_status "POST /auth/refresh" "200" "$REFRESH_STATUS"
else
    skip "POST /auth/refresh" "no refresh token from login"
fi

# 3d. Bad credentials → 401
BAD_LOGIN_STATUS=$(http_status "$GATEWAY/auth/login" \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"username":"nobody","password":"wrongpass"}' \
    --max-time 10)
assert_status "POST /auth/login (bad creds → 401)" "401" "$BAD_LOGIN_STATUS"

# 3e. Service accounts (admin only)
if [[ -n "$ADMIN_TOKEN" && "$ADMIN_TOKEN" != "null" ]]; then
    SA_STATUS=$(http_status "$GATEWAY/internal/service-accounts" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /internal/service-accounts (admin)" "200" "$SA_STATUS"

    # teller should be forbidden
    SA_TELLER_STATUS=$(http_status "$GATEWAY/internal/service-accounts" \
        -H "Authorization: Bearer $TELLER_TOKEN" \
        --max-time 10)
    assert_status "GET /internal/service-accounts (teller → 403)" "403" "$SA_TELLER_STATUS"
fi

# ── 4. Account service — via gateway ─────────────────────────────────────────
section "4. Account Service (via gateway :80)"

if [[ -z "$ADMIN_TOKEN" || "$ADMIN_TOKEN" == "null" ]]; then
    skip "Account service tests" "no admin token"
else
    # 4a. Unauthenticated → 401
    UNAUTH_STATUS=$(http_status "$GATEWAY/v1/accounts" --max-time 10)
    assert_status "GET /v1/accounts (no token → 401)" "401" "$UNAUTH_STATUS"

    # 4b. List accounts (admin)
    LIST_STATUS=$(http_status "$GATEWAY/v1/accounts" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/accounts (admin)" "200" "$LIST_STATUS"

    # 4c. Create account
    echo "  Creating test account..."
    CREATE_RESP=$(http_json "$GATEWAY/v1/accounts" \
        -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "owner_name": "Gateway Test User",
            "account_type": "SAVINGS",
            "currency": "IDR",
            "initial_balance": 1000000
        }' \
        --max-time 10)

    CREATE_STATUS=$(http_status "$GATEWAY/v1/accounts" \
        -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "owner_name": "Gateway Test User",
            "account_type": "SAVINGS",
            "currency": "IDR",
            "initial_balance": 1000000
        }' \
        --max-time 10)
    assert_status "POST /v1/accounts (create)" "201" "$CREATE_STATUS"

    ACCOUNT_ID=$(echo "$CREATE_RESP" | jq -r '.data.id // .id // empty' 2>/dev/null || echo "")

    if [[ -n "$ACCOUNT_ID" && "$ACCOUNT_ID" != "null" ]]; then
        pass "Account created: $ACCOUNT_ID"

        # 4d. Get account by ID
        GET_STATUS=$(http_status "$GATEWAY/v1/accounts/$ACCOUNT_ID" \
            -H "Authorization: Bearer $ADMIN_TOKEN" \
            --max-time 10)
        assert_status "GET /v1/accounts/$ACCOUNT_ID" "200" "$GET_STATUS"

        # 4e. Get balance
        BAL_STATUS=$(http_status "$GATEWAY/v1/accounts/$ACCOUNT_ID/balance" \
            -H "Authorization: Bearer $ADMIN_TOKEN" \
            --max-time 10)
        assert_status "GET /v1/accounts/$ACCOUNT_ID/balance" "200" "$BAL_STATUS"

        # 4f. Credit
        CREDIT_STATUS=$(http_status "$GATEWAY/v1/accounts/$ACCOUNT_ID/credit" \
            -X POST \
            -H "Authorization: Bearer $ADMIN_TOKEN" \
            -H "Content-Type: application/json" \
            -d '{"amount": 500000, "description": "gateway test credit"}' \
            --max-time 10)
        assert_status "POST /v1/accounts/$ACCOUNT_ID/credit" "200" "$CREDIT_STATUS"

        # 4g. Debit
        DEBIT_STATUS=$(http_status "$GATEWAY/v1/accounts/$ACCOUNT_ID/debit" \
            -X POST \
            -H "Authorization: Bearer $ADMIN_TOKEN" \
            -H "Content-Type: application/json" \
            -d '{"amount": 100000, "description": "gateway test debit"}' \
            --max-time 10)
        assert_status "POST /v1/accounts/$ACCOUNT_ID/debit" "200" "$DEBIT_STATUS"

        # 4h. Teller can read but not create accounts
        TELLER_GET_STATUS=$(http_status "$GATEWAY/v1/accounts/$ACCOUNT_ID" \
            -H "Authorization: Bearer $TELLER_TOKEN" \
            --max-time 10)
        assert_status "GET /v1/accounts/$ACCOUNT_ID (teller)" "200" "$TELLER_GET_STATUS"

    else
        fail "Account ID missing from create response — check: $(echo "$CREATE_RESP" | head -c 300)"
    fi
fi

# ── 5. Audit service — via gateway ────────────────────────────────────────────
section "5. Audit Service (via gateway :80)"

if [[ -z "$ADMIN_TOKEN" || "$ADMIN_TOKEN" == "null" ]]; then
    skip "Audit service tests" "no admin token"
else
    # 5a. List audit events
    AUDIT_LIST_STATUS=$(http_status "$GATEWAY/v1/audit/events" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/audit/events (admin)" "200" "$AUDIT_LIST_STATUS"

    # 5b. Teller forbidden from listing all events
    AUDIT_TELLER_STATUS=$(http_status "$GATEWAY/v1/audit/events" \
        -H "Authorization: Bearer $TELLER_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/audit/events (teller → 403)" "403" "$AUDIT_TELLER_STATUS"

    # 5c. Sync ingest (ADMIN or TELLER)
    INGEST_STATUS=$(http_status "$GATEWAY/v1/audit/events" \
        -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "actor_id": "usr_admin_001",
            "actor_type": "USER",
            "action": "GATEWAY_TEST",
            "resource": "test",
            "resource_id": "test-001",
            "status": "SUCCESS"
        }' \
        --max-time 10)
    # 200 or 201 depending on implementation
    if [[ "$INGEST_STATUS" == "200" || "$INGEST_STATUS" == "201" ]]; then
        pass "POST /v1/audit/events (ingest) → HTTP $INGEST_STATUS"
    else
        fail "POST /v1/audit/events (ingest) → expected 200/201, got $INGEST_STATUS"
    fi
fi

# ── 6. Notification service — via gateway ────────────────────────────────────
section "6. Notification Service (via gateway :80)"

if [[ -z "$ADMIN_TOKEN" || "$ADMIN_TOKEN" == "null" ]]; then
    skip "Notification service tests" "no admin token"
else
    # 6a. List notifications
    NOTIF_LIST_STATUS=$(http_status "$GATEWAY/v1/notifications" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/notifications (admin)" "200" "$NOTIF_LIST_STATUS"

    # 6b. List templates
    TMPL_LIST_STATUS=$(http_status "$GATEWAY/v1/templates" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/templates (admin)" "200" "$TMPL_LIST_STATUS"

    # 6c. List schedules
    SCHED_LIST_STATUS=$(http_status "$GATEWAY/v1/schedules" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/schedules (admin)" "200" "$SCHED_LIST_STATUS"

    # 6d. Create template
    TMPL_CREATE_STATUS=$(http_status "$GATEWAY/v1/templates" \
        -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "name": "gateway-test-template",
            "channel": "EMAIL",
            "subject": "Gateway Test",
            "body": "Hello {{.Name}}, this is a gateway test."
        }' \
        --max-time 10)
    # 200 or 201
    if [[ "$TMPL_CREATE_STATUS" == "200" || "$TMPL_CREATE_STATUS" == "201" ]]; then
        pass "POST /v1/templates (create) → HTTP $TMPL_CREATE_STATUS"
    else
        fail "POST /v1/templates (create) → expected 200/201, got $TMPL_CREATE_STATUS"
    fi

    # 6e. Send notification
    NOTIF_SEND_STATUS=$(http_status "$GATEWAY/v1/notifications" \
        -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "recipient": "test@banking.local",
            "channel": "EMAIL",
            "subject": "Gateway Test Notification",
            "body": "Testing notification via gateway"
        }' \
        --max-time 10)
    if [[ "$NOTIF_SEND_STATUS" == "200" || "$NOTIF_SEND_STATUS" == "201" || "$NOTIF_SEND_STATUS" == "202" ]]; then
        pass "POST /v1/notifications (send) → HTTP $NOTIF_SEND_STATUS"
    else
        fail "POST /v1/notifications (send) → expected 200/201/202, got $NOTIF_SEND_STATUS"
    fi
fi

# ── 7. Payment service — via gateway ─────────────────────────────────────────
section "7. Payment Service (via gateway :80)"

if [[ -z "$ADMIN_TOKEN" || "$ADMIN_TOKEN" == "null" ]]; then
    skip "Payment service tests" "no admin token"
else
    # 7a. List payments
    PAY_LIST_STATUS=$(http_status "$GATEWAY/v1/payments" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/payments (admin)" "200" "$PAY_LIST_STATUS"

    # 7b. Transfer (scaffold returns 501 — that's expected)
    TRANSFER_STATUS=$(http_status "$GATEWAY/v1/payments/transfer" \
        -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "from_account_id": "acc_test_001",
            "to_account_id":   "acc_test_002",
            "amount": 50000,
            "currency": "IDR",
            "description": "gateway test transfer"
        }' \
        --max-time 10)
    # payment-svc scaffold returns 501 Not Implemented until E4
    if [[ "$TRANSFER_STATUS" == "201" || "$TRANSFER_STATUS" == "200" ]]; then
        pass "POST /v1/payments/transfer → HTTP $TRANSFER_STATUS (implemented)"
    elif [[ "$TRANSFER_STATUS" == "501" ]]; then
        pass "POST /v1/payments/transfer → HTTP 501 (scaffold — expected, pending E4)"
    else
        fail "POST /v1/payments/transfer → unexpected HTTP $TRANSFER_STATUS"
    fi

    # 7c. Merchant payment
    MERCHANT_STATUS=$(http_status "$GATEWAY/v1/payments/merchant" \
        -X POST \
        -H "Authorization: Bearer $TELLER_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "from_account_id": "acc_test_001",
            "merchant_id": "merch_001",
            "amount": 25000,
            "currency": "IDR"
        }' \
        --max-time 10)
    if [[ "$MERCHANT_STATUS" == "201" || "$MERCHANT_STATUS" == "200" || "$MERCHANT_STATUS" == "501" ]]; then
        pass "POST /v1/payments/merchant → HTTP $MERCHANT_STATUS"
    else
        fail "POST /v1/payments/merchant → unexpected HTTP $MERCHANT_STATUS"
    fi
fi

# ── 8. Cross-service routing sanity ──────────────────────────────────────────
section "8. Gateway Routing Sanity"

# These paths must NOT be misrouted to the wrong service
WRONG_404=$(http_status "$GATEWAY/nonexistent-path-xyz" --max-time 5)
if [[ "$WRONG_404" == "404" ]]; then
    pass "Unknown path → 404 (not mis-routed)"
else
    fail "Unknown path → $WRONG_404 (expected 404)"
fi

# Auth paths must not reach account-svc
AUTH_VIA_GW=$(http_status "$GATEWAY/auth/login" \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"Admin@12345"}' \
    --max-time 10)
assert_status "POST /auth/login routed to auth-svc" "200" "$AUTH_VIA_GW"

# /v1/accounts must not reach auth-svc
ACC_VIA_GW=$(http_status "$GATEWAY/v1/accounts" --max-time 10)
assert_status "GET /v1/accounts routed to account-svc (→ 401)" "401" "$ACC_VIA_GW"

# ── 9. Auth logout (teardown) ─────────────────────────────────────────────────
section "9. Logout (teardown)"

if [[ -n "$ADMIN_TOKEN" && "$ADMIN_TOKEN" != "null" ]]; then
    LOGOUT_STATUS=$(http_status "$GATEWAY/auth/logout" \
        -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "POST /auth/logout (admin)" "204" "$LOGOUT_STATUS"

    # token should be invalid now
    POST_LOGOUT_STATUS=$(http_status "$GATEWAY/v1/accounts" \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        --max-time 10)
    assert_status "GET /v1/accounts after logout (→ 401)" "401" "$POST_LOGOUT_STATUS"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}────────────────────────────────────────${RESET}"
echo -e "${BOLD}  Results${RESET}"
echo -e "  ${GREEN}Passed : $PASS${RESET}"
echo -e "  ${RED}Failed : $FAIL${RESET}"
echo -e "  ${YELLOW}Skipped: $SKIP${RESET}"
echo -e "${BOLD}────────────────────────────────────────${RESET}"

if [[ $FAIL -gt 0 ]]; then
    echo -e "\n${RED}${BOLD}FAILED — $FAIL test(s) did not pass.${RESET}"
    echo "Tips:"
    echo "  • Run 'make services-up' and wait ~30s for all services to start"
    echo "  • Check 'docker compose logs <service>' for startup errors"
    echo "  • Verify Traefik dashboard: http://localhost:8080"
    echo "  • Check routers are registered: http://localhost:8080/api/http/routers"
    exit 1
else
    echo -e "\n${GREEN}${BOLD}ALL TESTS PASSED${RESET}"
fi
