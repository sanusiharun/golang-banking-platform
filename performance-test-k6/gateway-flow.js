/**
 * gateway-flow.js — Traefik gateway middleware validation test
 * ─────────────────────────────────────────────────────────────
 * Tests the gateway layer itself — not the business logic.
 * Validates rate limiting, security headers, routing, and circuit breaker.
 *
 * Run this as smoke test after every gateway config change (traefik/dynamic.yml).
 *
 * HOW TO RUN:
 *   k6 run -e SCENARIO=smoke performance-test-k6/gateway-flow.js
 *   k6 run -e SCENARIO=load  performance-test-k6/gateway-flow.js
 */

import http                      from "k6/http";
import { check, group, sleep }   from "k6";
import { Rate, Counter, Trend }  from "k6/metrics";

import { GATEWAY_URL, ENV_LABEL, buildHeaders } from "./lib/config.js";
import { scenarios, baseThresholds }             from "./lib/scenarios.js";
import { safeJson }                              from "./lib/helpers.js";

const FLOW_NAME = "gateway_flow";

// ── Custom metrics ─────────────────────────────────────────────────────────
const routingErrors       = new Counter(`${FLOW_NAME}_routing_errors`);
const rateLimitHitRate    = new Rate(`${FLOW_NAME}_rate_limit_hit`);
const secHeadersOkRate    = new Rate(`${FLOW_NAME}_security_headers_ok`);
const gatewayLatency      = new Trend(`${FLOW_NAME}_latency_ms`, true);
const flowSuccessRate     = new Rate(`${FLOW_NAME}_success_rate`);

const SCENARIO = __ENV.SCENARIO || "smoke";

export const options = {
  scenarios: {
    [FLOW_NAME]: scenarios[SCENARIO],
  },
  thresholds: {
    ...baseThresholds,
    [`${FLOW_NAME}_routing_errors`]:     ["count==0"],   // zero mis-routes tolerated
    [`${FLOW_NAME}_security_headers_ok`]:["rate>0.99"],  // headers on every response
    [`${FLOW_NAME}_latency_ms`]:         ["p(95)<50"],   // Traefik overhead < 50ms
    [`${FLOW_NAME}_success_rate`]:       ["rate>0.95"],
  },
};

export function setup() {
  console.log(`[setup] Flow    : ${FLOW_NAME}`);
  console.log(`[setup] Scenario: ${SCENARIO}`);
  console.log(`[setup] Gateway : ${GATEWAY_URL}  (env: ${ENV_LABEL})`);
  return {};
}

export default function () {
  let flowOk = true;

  // ── Test 1: Route resolution — each service prefix must route correctly ──
  group("01 Route resolution", () => {
    const routes = [
      { path: "/auth/login",          method: "POST", body: '{"username":"x","password":"y"}', expectedService: "auth-svc",         allowedStatuses: [200, 400, 401, 422] },
      { path: "/v1/accounts",         method: "GET",  body: null,                              expectedService: "account-svc",      allowedStatuses: [200, 401, 403] },
      { path: "/v1/audit/events",     method: "GET",  body: null,                              expectedService: "audit-svc",        allowedStatuses: [200, 401, 403] },
      { path: "/v1/notifications",    method: "GET",  body: null,                              expectedService: "notification-svc", allowedStatuses: [200, 401, 403] },
      { path: "/v1/payments",         method: "GET",  body: null,                              expectedService: "payment-svc",      allowedStatuses: [200, 401, 403, 501] },
    ];

    for (const route of routes) {
      const headers = buildHeaders(route.body ? "application/json" : null);
      const res = route.method === "POST"
        ? http.post(`${GATEWAY_URL}${route.path}`, route.body, { headers })
        : http.get(`${GATEWAY_URL}${route.path}`, { headers });

      gatewayLatency.add(res.timings.duration);

      const routed = route.allowedStatuses.includes(res.status);
      if (!routed) routingErrors.add(1);

      const ok = check(res, {
        [`route ${route.path} → ${route.expectedService} (not 404/502)`]: () => routed,
      });

      if (!ok) {
        flowOk = false;
        console.error(`[Routing FAIL] ${route.method} ${route.path} → status=${res.status} expected one of [${route.allowedStatuses}]`);
      }
    }
  });

  sleep(0.3);

  // ── Test 2: Security headers present on every response ──────────────────
  group("02 Security headers", () => {
    const res = http.get(`${GATEWAY_URL}/v1/accounts`, {
      headers: buildHeaders(null),
    });

    const secOk = check(res, {
      "X-Frame-Options: DENY":            (r) => (r.headers["X-Frame-Options"] || "").toUpperCase() === "DENY",
      "X-Content-Type-Options: nosniff":  (r) => (r.headers["X-Content-Type-Options"] || "").toLowerCase() === "nosniff",
      "Server header hidden":             (r) => !r.headers["Server"] || r.headers["Server"] === "",
      "X-Powered-By header hidden":       (r) => !r.headers["X-Powered-By"] || r.headers["X-Powered-By"] === "",
    });

    secHeadersOkRate.add(secOk);
    if (!secOk) {
      flowOk = false;
      console.error(`[SecHeaders FAIL] VU${__VU} headers=${JSON.stringify(res.headers)}`);
    }
  });

  sleep(0.3);

  // ── Test 3: Unknown path returns 404 (not a backend error) ──────────────
  group("03 Unknown route → 404", () => {
    const res = http.get(`${GATEWAY_URL}/this-path-does-not-exist-xyz`);

    const ok = check(res, {
      "unknown path: status 404": (r) => r.status === 404,
    });

    if (!ok) {
      routingErrors.add(1);
      console.error(`[UnknownRoute FAIL] VU${__VU} status=${res.status} (expected 404)`);
    }
  });

  sleep(0.3);

  // ── Test 4: Rate limiting on public auth endpoint ───────────────────────
  // Fires 25 rapid requests — should trigger 429 after threshold (configured: 20/s burst 10)
  // Only run this in smoke to avoid hammering in load tests
  if (SCENARIO === "smoke") {
    group("04 Rate limit — rapid /auth/login burst", () => {
      const burst = 25;
      let got429 = false;

      for (let i = 0; i < burst; i++) {
        const res = http.post(
          `${GATEWAY_URL}/auth/login`,
          JSON.stringify({ username: "x", password: "y" }),
          { headers: buildHeaders("application/json") }
        );

        if (res.status === 429) {
          got429 = true;
        }
      }

      rateLimitHitRate.add(got429);
      check({ got429 }, {
        "rate limit: 429 received after burst": (v) => v.got429 === true,
      });

      if (!got429) {
        console.warn(`[RateLimit] No 429 after ${burst} requests — check rate-public config in traefik/dynamic.yml`);
      }
    });

    sleep(2); // wait for rate limit window to reset
  }

  // ── Test 5: Traefik dashboard accessible ────────────────────────────────
  group("05 Traefik dashboard API", () => {
    const dash = http.get("http://localhost:8080/api/overview");

    const ok = check(dash, {
      "dashboard: status 200": (r) => r.status === 200,
    });

    if (ok) {
      const body = safeJson(dash);
      check(body, {
        "dashboard: has router count":  (b) => b?.http?.routers?.total > 0,
        "dashboard: 5 routers active":  (b) => b?.http?.routers?.total >= 5,
      });
    }
  });

  flowSuccessRate.add(flowOk);
  sleep(1);
}

export function teardown() {
  console.log(`[teardown] ${FLOW_NAME} complete.`);
}
