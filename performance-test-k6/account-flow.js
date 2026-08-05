/**
 * account-flow.js — Full banking flow load test
 * ─────────────────────────────────────────────────────────────
 * Simulates a complete banking session:
 *   1. POST /auth/login          → authenticate as ADMIN
 *   2. POST /v1/accounts         → create a new account
 *   3. GET  /v1/accounts/{id}    → verify account details
 *   4. POST /v1/accounts/{id}/credit  → deposit funds
 *   5. GET  /v1/accounts/{id}/balance → assert balance increased
 *   6. POST /v1/accounts/{id}/debit   → withdraw funds
 *   7. GET  /v1/accounts/{id}/balance → assert balance decreased
 *   8. POST /auth/logout         → revoke token
 *
 * Each VU creates its own isolated account — no shared state between VUs.
 * IBAN is generated per-VU per-iteration to avoid unique constraint conflicts.
 *
 * HOW TO RUN:
 *   k6 run -e SCENARIO=smoke   performance-test-k6/account-flow.js
 *   k6 run -e SCENARIO=load    performance-test-k6/account-flow.js
 *   k6 run -e SCENARIO=stress  performance-test-k6/account-flow.js
 *
 *   Staging:
 *   k6 run -e SCENARIO=load \
 *           -e AUTH_URL=https://auth.staging.api.com \
 *           -e ACCOUNT_URL=https://account.staging.api.com \
 *           performance-test-k6/account-flow.js
 *
 *   Save results:
 *   k6 run -e SCENARIO=load \
 *           --out json=results/account-load-$(date +%Y%m%d).json \
 *           performance-test-k6/account-flow.js
 */

import http                    from "k6/http";
import { check, group, sleep } from "k6";
import { Trend, Rate }         from "k6/metrics";

import { AUTH_URL, ACCOUNT_URL, ENV_LABEL, buildHeaders } from "./lib/config.js";
import { scenarios, baseThresholds }                       from "./lib/scenarios.js";
import { safeJson, uniqueId }                              from "./lib/helpers.js";

const FLOW_NAME = "account_flow";

// ── Custom metrics ────────────────────────────────────────────
const loginDuration         = new Trend(`${FLOW_NAME}_login_duration`,          true);
const createAccountDuration = new Trend(`${FLOW_NAME}_create_account_duration`, true);
const getAccountDuration    = new Trend(`${FLOW_NAME}_get_account_duration`,    true);
const creditDuration        = new Trend(`${FLOW_NAME}_credit_duration`,         true);
const debitDuration         = new Trend(`${FLOW_NAME}_debit_duration`,          true);
const balanceDuration       = new Trend(`${FLOW_NAME}_balance_duration`,        true);
const flowSuccessRate       = new Rate(`${FLOW_NAME}_success_rate`);

// ── Scenario selection ────────────────────────────────────────
const SCENARIO = __ENV.SCENARIO || "smoke";

export const options = {
  scenarios: {
    [FLOW_NAME]: scenarios[SCENARIO],
  },
  thresholds: {
    ...baseThresholds,
    [`${FLOW_NAME}_login_duration`]:          ["p(95)<2000"],  // bcrypt cost 12
    [`${FLOW_NAME}_create_account_duration`]: ["p(95)<800"],
    [`${FLOW_NAME}_get_account_duration`]:    ["p(95)<300"],
    [`${FLOW_NAME}_credit_duration`]:         ["p(95)<500"],
    [`${FLOW_NAME}_debit_duration`]:          ["p(95)<500"],
    [`${FLOW_NAME}_balance_duration`]:        ["p(95)<300"],
    [`${FLOW_NAME}_success_rate`]:            ["rate>0.95"],
  },
};

export function setup() {
  console.log(`[setup] Flow     : ${FLOW_NAME}`);
  console.log(`[setup] Scenario : ${SCENARIO}`);
  console.log(`[setup] Env      : ${ENV_LABEL}`);
  console.log(`[setup] Auth     : ${AUTH_URL}`);
  console.log(`[setup] Account  : ${ACCOUNT_URL}`);
  return {};
}

export default function () {
  let flowOk       = true;
  let accessToken  = "";
  let refreshToken = "";
  let accountId    = "";

  // ── Step 1: Login as ADMIN ────────────────────────────────
  group("Step 1: Login", function () {
    const res = http.post(
      `${AUTH_URL}/auth/login`,
      JSON.stringify({ username: "admin", password: "Admin@12345" }),
      { headers: buildHeaders("application/json") }
    );

    loginDuration.add(res.timings.duration);

    const body = safeJson(res);
    const ok = check(res, {
      "login: status 200":        (r) => r.status === 200,
      "login: has access_token":  () => !!body?.data?.access_token,
      "login: has refresh_token": () => !!body?.data?.refresh_token,
    });

    if (ok) {
      accessToken  = body.data.access_token;
      refreshToken = body.data.refresh_token;
    } else {
      flowOk = false;
      console.error(`[Login FAIL] VU${__VU} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 2: Create Account ────────────────────────────────
  // IBAN is unique per VU+iteration to avoid DB constraint violations.
  // Format: GB + 2-digit VU + 10-digit iter+timestamp (max 34 chars, starts GB).
  const iban = `GB${String(__VU).padStart(2, "0")}BANK${String(__ITER).padStart(4, "0")}${String(Date.now()).slice(-8)}`;

  group("Step 2: Create Account", function () {
    const res = http.post(
      `${ACCOUNT_URL}/v1/accounts`,
      JSON.stringify({
        customer_id: uniqueId("CUST"),
        currency:    "NGN",
        iban:        iban,
      }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    createAccountDuration.add(res.timings.duration);

    const body = safeJson(res);
    const ok = check(res, {
      "create: status 201":    (r) => r.status === 201,
      "create: has id":        () => !!body?.data?.id,
      "create: currency NGN":  () => body?.data?.currency === "NGN",
      "create: status ACTIVE": () => body?.data?.status === "ACTIVE",
    });

    if (ok) {
      accountId = body.data.id;
    } else {
      flowOk = false;
      console.error(`[CreateAccount FAIL] VU${__VU} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 3: Get Account ───────────────────────────────────
  group("Step 3: Get Account", function () {
    const res = http.get(
      `${ACCOUNT_URL}/v1/accounts/${accountId}`,
      { headers: buildHeaders(null, { Authorization: `Bearer ${accessToken}` }) }
    );

    getAccountDuration.add(res.timings.duration);

    const body = safeJson(res);
    const ok = check(res, {
      "get: status 200":   (r) => r.status === 200,
      "get: id matches":   () => body?.data?.id === accountId,
      "get: balance zero": () => body?.data?.balance === 0,
    });

    if (!ok) {
      flowOk = false;
      console.error(`[GetAccount FAIL] VU${__VU} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 4: Credit ────────────────────────────────────────
  const creditAmount = 100000; // NGN 1,000.00 (minor units)

  group("Step 4: Credit", function () {
    const res = http.post(
      `${ACCOUNT_URL}/v1/accounts/${accountId}/credit`,
      JSON.stringify({
        amount:    creditAmount,
        reference: uniqueId("DEP"),
      }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    creditDuration.add(res.timings.duration);

    const ok = check(res, {
      "credit: status 200": (r) => r.status === 200,
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Credit FAIL] VU${__VU} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 5: Check balance after credit ───────────────────
  group("Step 5: Balance after Credit", function () {
    const res = http.get(
      `${ACCOUNT_URL}/v1/accounts/${accountId}/balance`,
      { headers: buildHeaders(null, { Authorization: `Bearer ${accessToken}` }) }
    );

    balanceDuration.add(res.timings.duration);

    const body = safeJson(res);
    const ok = check(res, {
      "balance: status 200":           (r) => r.status === 200,
      "balance: equals credit amount": () => body?.data?.balance === creditAmount,
      "balance: currency NGN":         () => body?.data?.currency === "NGN",
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Balance FAIL] VU${__VU} status=${res.status} balance=${safeJson(res)?.balance} expected=${creditAmount}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 6: Debit ─────────────────────────────────────────
  const debitAmount = 50000; // NGN 500.00

  group("Step 6: Debit", function () {
    const res = http.post(
      `${ACCOUNT_URL}/v1/accounts/${accountId}/debit`,
      JSON.stringify({
        amount:    debitAmount,
        reference: uniqueId("WDR"),
      }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    debitDuration.add(res.timings.duration);

    const ok = check(res, {
      "debit: status 200": (r) => r.status === 200,
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Debit FAIL] VU${__VU} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 7: Check balance after debit ────────────────────
  group("Step 7: Balance after Debit", function () {
    const res = http.get(
      `${ACCOUNT_URL}/v1/accounts/${accountId}/balance`,
      { headers: buildHeaders(null, { Authorization: `Bearer ${accessToken}` }) }
    );

    balanceDuration.add(res.timings.duration);

    const expectedBalance = creditAmount - debitAmount;
    const body = safeJson(res);
    const ok = check(res, {
      "balance: status 200":     (r) => r.status === 200,
      "balance: reflects debit": () => body?.data?.balance === expectedBalance,
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Balance FAIL] VU${__VU} balance=${body?.data?.balance} expected=${expectedBalance}`);
    }
  });

  sleep(0.2);

  // ── Step 8: Logout ────────────────────────────────────────
  group("Step 8: Logout", function () {
    const res = http.post(
      `${AUTH_URL}/auth/logout`,
      JSON.stringify({ refresh_token: refreshToken }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    check(res, {
      "logout: status 200": (r) => r.status === 200,
    });
  });

  flowSuccessRate.add(flowOk);
  sleep(1);
}

export function teardown() {
  console.log(`[teardown] ${FLOW_NAME} complete.`);
}
