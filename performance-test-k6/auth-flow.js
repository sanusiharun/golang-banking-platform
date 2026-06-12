/**
 * auth-flow.js — Load test for auth-svc
 * ─────────────────────────────────────────────────────────────
 * Covers the full authentication lifecycle:
 *   1. POST /auth/login    → get access + refresh token
 *   2. POST /auth/refresh  → rotate tokens
 *   3. POST /auth/logout   → revoke refresh token
 *
 * HOW TO RUN:
 *   k6 run -e SCENARIO=smoke   performance-test-k6/auth-flow.js
 *   k6 run -e SCENARIO=load    performance-test-k6/auth-flow.js
 *   k6 run -e SCENARIO=stress  performance-test-k6/auth-flow.js
 *
 *   Staging:
 *   k6 run -e SCENARIO=load -e AUTH_URL=https://auth.staging.api.com \
 *           performance-test-k6/auth-flow.js
 *
 *   Save results:
 *   k6 run -e SCENARIO=load --out json=results/auth-load-$(date +%Y%m%d).json \
 *           performance-test-k6/auth-flow.js
 */

import http                    from "k6/http";
import { check, group, sleep } from "k6";
import { Trend, Rate }         from "k6/metrics";

import { AUTH_URL, ENV_LABEL, buildHeaders } from "./lib/config.js";
import { scenarios, baseThresholds }          from "./lib/scenarios.js";
import { safeJson, randomFrom }               from "./lib/helpers.js";

const FLOW_NAME = "auth_flow";

// ── Custom metrics ────────────────────────────────────────────
const loginDuration   = new Trend(`${FLOW_NAME}_login_duration`,   true);
const refreshDuration = new Trend(`${FLOW_NAME}_refresh_duration`, true);
const logoutDuration  = new Trend(`${FLOW_NAME}_logout_duration`,  true);
const flowSuccessRate = new Rate(`${FLOW_NAME}_success_rate`);

// ── Test users (seeded via 001_create_users.up.sql) ───────────
const TEST_USERS = [
  { username: "admin",  password: "Admin@12345"  },
  { username: "teller", password: "Teller@12345" },
];

// ── Scenario selection ────────────────────────────────────────
const SCENARIO = __ENV.SCENARIO || "smoke";

export const options = {
  scenarios: {
    [FLOW_NAME]: scenarios[SCENARIO],
  },
  thresholds: {
    ...baseThresholds,
    [`${FLOW_NAME}_login_duration`]:   ["p(95)<2000"],  // login SLA: 2s — bcrypt cost 12 is intentionally slow
    [`${FLOW_NAME}_refresh_duration`]: ["p(95)<300"],   // refresh SLA: 300ms
    [`${FLOW_NAME}_logout_duration`]:  ["p(95)<300"],
    [`${FLOW_NAME}_success_rate`]:     ["rate>0.95"],
  },
};

export function setup() {
  console.log(`[setup] Flow     : ${FLOW_NAME}`);
  console.log(`[setup] Scenario : ${SCENARIO}`);
  console.log(`[setup] Env      : ${ENV_LABEL} → ${AUTH_URL}`);
  return {};
}

export default function () {
  const user    = randomFrom(TEST_USERS);
  let flowOk    = true;
  let refreshToken = "";

  // ── Step 1: Login ─────────────────────────────────────────
  group("Step 1: Login", function () {
    const res = http.post(
      `${AUTH_URL}/auth/login`,
      JSON.stringify({ username: user.username, password: user.password }),
      { headers: buildHeaders("application/json") }
    );

    loginDuration.add(res.timings.duration);

    const body = safeJson(res);
    const ok = check(res, {
      "login: status 200":          (r) => r.status === 200,
      "login: has access_token":    () => !!body?.data?.access_token,
      "login: has refresh_token":   () => !!body?.data?.refresh_token,
    });

    if (ok) {
      refreshToken = body.data.refresh_token;
    } else {
      flowOk = false;
      console.error(`[Login FAIL] VU${__VU} user=${user.username} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  if (!flowOk) {
    flowSuccessRate.add(false);
    sleep(1);
    return;
  }

  sleep(0.3);

  // ── Step 2: Refresh token ─────────────────────────────────
  group("Step 2: Refresh Token", function () {
    const res = http.post(
      `${AUTH_URL}/auth/refresh`,
      JSON.stringify({ refresh_token: refreshToken }),
      { headers: buildHeaders("application/json") }
    );

    refreshDuration.add(res.timings.duration);

    const body = safeJson(res);
    const ok = check(res, {
      "refresh: status 200":        (r) => r.status === 200,
      "refresh: has access_token":  () => !!body?.data?.access_token,
      "refresh: has refresh_token": () => !!body?.data?.refresh_token,
    });

    if (ok) {
      refreshToken = body.data.refresh_token;
    } else {
      flowOk = false;
      console.error(`[Refresh FAIL] VU${__VU} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  if (!flowOk) {
    flowSuccessRate.add(false);
    sleep(1);
    return;
  }

  sleep(0.3);

  // ── Step 3: Logout ────────────────────────────────────────
  group("Step 3: Logout", function () {
    const res = http.post(
      `${AUTH_URL}/auth/logout`,
      JSON.stringify({ refresh_token: refreshToken }),
      { headers: buildHeaders("application/json") }
    );

    logoutDuration.add(res.timings.duration);

    const ok = check(res, {
      "logout: status 200": (r) => r.status === 200,
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Logout FAIL] VU${__VU} status=${res.status} body=${(res.body || "").substring(0, 200)}`);
    }
  });

  flowSuccessRate.add(flowOk);
  sleep(1);
}

export function teardown() {
  console.log(`[teardown] ${FLOW_NAME} complete.`);
}
