/**
 * orchestration-flow.js — Full cross-service banking orchestration test
 * ──────────────────────────────────────────────────────────────────────
 * Simulates a complete real-world banking session through the Traefik gateway.
 * All requests go to port 80 — Traefik routes them to the correct service.
 *
 * Journey (per VU iteration):
 *   1.  POST /auth/login                          → auth-svc    (get token)
 *   2.  POST /v1/accounts                         → account-svc (create sender)
 *   3.  POST /v1/accounts                         → account-svc (create receiver)
 *   4.  POST /v1/accounts/{sender}/credit         → account-svc (fund sender)
 *   5.  GET  /v1/accounts/{sender}/balance        → account-svc (verify credit)
 *   6.  POST /v1/payments/transfer                → payment-svc (transfer funds)
 *   7.  GET  /v1/accounts/{sender}/balance        → account-svc (verify debit after transfer)
 *   8.  GET  /v1/accounts/{receiver}/balance      → account-svc (verify credit after transfer)
 *   9.  GET  /v1/audit/events                     → audit-svc   (verify audit trail)
 *   10. POST /v1/notifications                    → notification-svc (send confirmation)
 *   11. POST /auth/logout                         → auth-svc    (clean up session)
 *
 * HOW TO RUN:
 *   k6 run -e SCENARIO=smoke   performance-test-k6/orchestration-flow.js
 *   k6 run -e SCENARIO=load    performance-test-k6/orchestration-flow.js
 *   k6 run -e SCENARIO=stress  performance-test-k6/orchestration-flow.js
 *
 *   # Custom gateway (e.g. staging):
 *   k6 run -e SCENARIO=load -e GATEWAY_URL=https://api.staging.mybank.com \
 *           performance-test-k6/orchestration-flow.js
 *
 *   # Save results:
 *   k6 run -e SCENARIO=load \
 *           --out json=results/orchestration-load.json \
 *           performance-test-k6/orchestration-flow.js
 */

import http                    from "k6/http";
import { check, group, sleep } from "k6";
import { Trend, Rate, Counter } from "k6/metrics";

import { GATEWAY_URL, ENV_LABEL, buildHeaders } from "./lib/config.js";
import { scenarios, baseThresholds }             from "./lib/scenarios.js";
import { safeJson, uniqueId, randomInt }         from "./lib/helpers.js";

const FLOW_NAME = "orchestration_flow";

// ── Custom metrics ─────────────────────────────────────────────────────────
const loginDuration        = new Trend(`${FLOW_NAME}_login_ms`,        true);
const createAcctDuration   = new Trend(`${FLOW_NAME}_create_acct_ms`,  true);
const creditDuration       = new Trend(`${FLOW_NAME}_credit_ms`,       true);
const balanceDuration      = new Trend(`${FLOW_NAME}_balance_ms`,      true);
const transferDuration     = new Trend(`${FLOW_NAME}_transfer_ms`,     true);
const auditDuration        = new Trend(`${FLOW_NAME}_audit_ms`,        true);
const notifDuration        = new Trend(`${FLOW_NAME}_notif_ms`,        true);
const flowSuccessRate      = new Rate(`${FLOW_NAME}_success_rate`);
const gatewayRoutingErrors = new Counter(`${FLOW_NAME}_routing_errors`);

// ── Scenario ───────────────────────────────────────────────────────────────
const SCENARIO = __ENV.SCENARIO || "smoke";

export const options = {
  scenarios: {
    [FLOW_NAME]: scenarios[SCENARIO],
  },
  thresholds: {
    ...baseThresholds,
    // SLAs per operation through the gateway (adds ~1ms Traefik overhead)
    [`${FLOW_NAME}_login_ms`]:       ["p(95)<2500"],  // bcrypt cost 12 is slow by design
    [`${FLOW_NAME}_create_acct_ms`]: ["p(95)<800"],
    [`${FLOW_NAME}_credit_ms`]:      ["p(95)<500"],
    [`${FLOW_NAME}_balance_ms`]:     ["p(95)<300"],
    [`${FLOW_NAME}_transfer_ms`]:    ["p(95)<1000"],
    [`${FLOW_NAME}_audit_ms`]:       ["p(95)<500"],
    [`${FLOW_NAME}_notif_ms`]:       ["p(95)<500"],
    [`${FLOW_NAME}_success_rate`]:   ["rate>0.95"],
    // Gateway must never return a 404 (mis-route) or 502 (backend down)
    [`${FLOW_NAME}_routing_errors`]: ["count<1"],
  },
};

export function setup() {
  console.log(`[setup] Flow     : ${FLOW_NAME}`);
  console.log(`[setup] Scenario : ${SCENARIO}`);
  console.log(`[setup] Gateway  : ${GATEWAY_URL}  (env: ${ENV_LABEL})`);
  console.log(`[setup] Routes   : all traffic through Traefik :80`);

  // Verify gateway is reachable before starting
  const ping = http.get(`${GATEWAY_URL}/auth/login`, {
    headers: { "Content-Type": "application/json" },
  });
  if (ping.status === 0) {
    throw new Error(`Gateway unreachable at ${GATEWAY_URL} — is Traefik running? (make services-up)`);
  }
  return {};
}

export default function () {
  let flowOk       = true;
  let accessToken  = "";
  let refreshToken = "";
  let senderAcct   = "";
  let receiverAcct = "";
  const creditAmount   = randomInt(100000, 1000000);  // 1,000 – 10,000 NGN
  const transferAmount = Math.floor(creditAmount / 2); // transfer half

  // ── Step 1: Login ───────────────────────────────────────────────────────
  group("01 Login → auth-svc via gateway", () => {
    const res = http.post(
      `${GATEWAY_URL}/auth/login`,
      JSON.stringify({ username: "admin", password: "Admin@12345" }),
      { headers: buildHeaders("application/json") }
    );

    loginDuration.add(res.timings.duration);

    // 404 = gateway mis-route, 502 = backend down — both are gateway errors
    if (res.status === 404 || res.status === 502) {
      gatewayRoutingErrors.add(1);
      console.error(`[Step01 GATEWAY ERROR] status=${res.status} — check Traefik routing`);
    }

    const body = safeJson(res);
    const ok = check(res, {
      "01 login: routed by gateway":   (r) => r.status !== 404 && r.status !== 502,
      "01 login: status 200":          (r) => r.status === 200,
      "01 login: has access_token":    () => !!body?.data?.access_token,
      "01 login: has refresh_token":   () => !!body?.data?.refresh_token,
    });

    if (ok) {
      accessToken  = body.data.access_token;
      refreshToken = body.data.refresh_token;
    } else {
      flowOk = false;
      console.error(`[Step01 FAIL] VU${__VU} status=${res.status} body=${(res.body||"").substring(0,200)}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 2 & 3: Create sender + receiver accounts ───────────────────────
  const accounts = ["sender", "receiver"];

  for (const role of accounts) {
    group(`0${accounts.indexOf(role)+2} Create ${role} account → account-svc via gateway`, () => {
      const iban = `GB${String(__VU).padStart(2,"0")}${role.toUpperCase().slice(0,4)}${String(__ITER).padStart(4,"0")}${String(Date.now()).slice(-6)}`;

      const res = http.post(
        `${GATEWAY_URL}/v1/accounts`,
        JSON.stringify({
          customer_id: uniqueId(`CUST-${role.toUpperCase()}`),
          currency:    "NGN",
          iban:        iban,
        }),
        { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
      );

      createAcctDuration.add(res.timings.duration);

      if (res.status === 404 || res.status === 502) gatewayRoutingErrors.add(1);

      const body = safeJson(res);
      const ok = check(res, {
        [`0${accounts.indexOf(role)+2} ${role}: routed correctly`]: (r) => r.status !== 404 && r.status !== 502,
        [`0${accounts.indexOf(role)+2} ${role}: status 201`]:       (r) => r.status === 201,
        [`0${accounts.indexOf(role)+2} ${role}: has id`]:           () => !!body?.data?.id,
        [`0${accounts.indexOf(role)+2} ${role}: status ACTIVE`]:    () => body?.data?.status === "ACTIVE",
      });

      if (ok) {
        if (role === "sender")   senderAcct   = body.data.id;
        if (role === "receiver") receiverAcct = body.data.id;
      } else {
        flowOk = false;
        console.error(`[Create ${role} FAIL] VU${__VU} status=${res.status} body=${(res.body||"").substring(0,200)}`);
      }
    });

    if (!flowOk) break;
    sleep(0.2);
  }

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }

  // ── Step 4: Credit sender ───────────────────────────────────────────────
  group("04 Credit sender → account-svc via gateway", () => {
    const res = http.post(
      `${GATEWAY_URL}/v1/accounts/${senderAcct}/credit`,
      JSON.stringify({
        amount:    creditAmount,
        reference: uniqueId("DEP"),
      }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    creditDuration.add(res.timings.duration);
    if (res.status === 404 || res.status === 502) gatewayRoutingErrors.add(1);

    const ok = check(res, {
      "04 credit: routed correctly": (r) => r.status !== 404 && r.status !== 502,
      "04 credit: status 200":       (r) => r.status === 200,
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Credit FAIL] VU${__VU} status=${res.status} body=${(res.body||"").substring(0,200)}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 5: Verify sender balance after credit ──────────────────────────
  group("05 Sender balance after credit → account-svc via gateway", () => {
    const res = http.get(
      `${GATEWAY_URL}/v1/accounts/${senderAcct}/balance`,
      { headers: buildHeaders(null, { Authorization: `Bearer ${accessToken}` }) }
    );

    balanceDuration.add(res.timings.duration);
    if (res.status === 404 || res.status === 502) gatewayRoutingErrors.add(1);

    const body = safeJson(res);
    const ok = check(res, {
      "05 balance: routed correctly":    (r) => r.status !== 404 && r.status !== 502,
      "05 balance: status 200":          (r) => r.status === 200,
      "05 balance: equals credit amount": () => body?.data?.balance === creditAmount,
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Balance FAIL] VU${__VU} status=${res.status} balance=${body?.data?.balance} expected=${creditAmount}`);
    }
  });

  if (!flowOk) { flowSuccessRate.add(false); sleep(1); return; }
  sleep(0.2);

  // ── Step 6: Transfer sender → receiver (via payment-svc) ────────────────
  // payment-svc scaffold returns 501 until E4 is implemented.
  // Test validates gateway routing is correct even for unimplemented endpoints.
  let transferOk = false;

  group("06 Transfer → payment-svc via gateway", () => {
    const res = http.post(
      `${GATEWAY_URL}/v1/payments/transfer`,
      JSON.stringify({
        from_account_id:  senderAcct,
        to_account_id:    receiverAcct,
        amount:           transferAmount,
        currency:         "NGN",
        reference:        uniqueId("TXN"),
        description:      "Orchestration test transfer",
      }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    transferDuration.add(res.timings.duration);
    if (res.status === 404 || res.status === 502) gatewayRoutingErrors.add(1);

    const ok = check(res, {
      "06 transfer: routed to payment-svc":  (r) => r.status !== 404 && r.status !== 502,
      // 201 = implemented, 501 = scaffold (both are valid gateway routing results)
      "06 transfer: payment-svc responded":  (r) => r.status === 201 || r.status === 501 || r.status === 200,
    });

    if (res.status === 201 || res.status === 200) {
      transferOk = true;
    }

    if (!ok) {
      console.error(`[Transfer FAIL] VU${__VU} status=${res.status} body=${(res.body||"").substring(0,200)}`);
      // Don't abort — transfer being 501 is expected until payment-svc E4
    }
  });

  sleep(0.3);

  // ── Step 7 & 8: Verify balances after transfer (only if transfer succeeded) ──
  if (transferOk) {
    group("07 Sender balance after transfer → account-svc via gateway", () => {
      const res = http.get(
        `${GATEWAY_URL}/v1/accounts/${senderAcct}/balance`,
        { headers: buildHeaders(null, { Authorization: `Bearer ${accessToken}` }) }
      );

      balanceDuration.add(res.timings.duration);
      const body = safeJson(res);
      const expectedSender = creditAmount - transferAmount;

      check(res, {
        "07 sender balance: status 200":       (r) => r.status === 200,
        "07 sender balance: reflects transfer": () => body?.data?.balance === expectedSender,
      });
    });

    sleep(0.2);

    group("08 Receiver balance after transfer → account-svc via gateway", () => {
      const res = http.get(
        `${GATEWAY_URL}/v1/accounts/${receiverAcct}/balance`,
        { headers: buildHeaders(null, { Authorization: `Bearer ${accessToken}` }) }
      );

      balanceDuration.add(res.timings.duration);
      const body = safeJson(res);

      check(res, {
        "08 receiver balance: status 200":         (r) => r.status === 200,
        "08 receiver balance: received funds":       () => body?.data?.balance === transferAmount,
      });
    });

    sleep(0.2);
  }

  // ── Step 9: Check audit trail ───────────────────────────────────────────
  group("09 Audit events → audit-svc via gateway", () => {
    const res = http.get(
      `${GATEWAY_URL}/v1/audit/events?limit=10`,
      { headers: buildHeaders(null, { Authorization: `Bearer ${accessToken}` }) }
    );

    auditDuration.add(res.timings.duration);
    if (res.status === 404 || res.status === 502) gatewayRoutingErrors.add(1);

    const body = safeJson(res);
    check(res, {
      "09 audit: routed to audit-svc":  (r) => r.status !== 404 && r.status !== 502,
      "09 audit: status 200":           (r) => r.status === 200,
      "09 audit: returns list":         () => Array.isArray(body?.data) || Array.isArray(body?.data?.events),
    });
  });

  sleep(0.2);

  // ── Step 10: Send notification ──────────────────────────────────────────
  group("10 Send notification → notification-svc via gateway", () => {
    const res = http.post(
      `${GATEWAY_URL}/v1/notifications`,
      JSON.stringify({
        recipient:  "test@banking.local",
        channel:    "EMAIL",
        subject:    "Transfer Confirmation",
        body:       `Transfer of NGN ${(transferAmount/100).toFixed(2)} processed — ref ${uniqueId("TXN")}`,
      }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    notifDuration.add(res.timings.duration);
    if (res.status === 404 || res.status === 502) gatewayRoutingErrors.add(1);

    check(res, {
      "10 notification: routed correctly":    (r) => r.status !== 404 && r.status !== 502,
      "10 notification: accepted":            (r) => r.status === 200 || r.status === 201 || r.status === 202,
    });
  });

  sleep(0.2);

  // ── Step 11: Logout ─────────────────────────────────────────────────────
  group("11 Logout → auth-svc via gateway", () => {
    const res = http.post(
      `${GATEWAY_URL}/auth/logout`,
      JSON.stringify({ refresh_token: refreshToken }),
      { headers: buildHeaders("application/json", { Authorization: `Bearer ${accessToken}` }) }
    );

    if (res.status === 404 || res.status === 502) gatewayRoutingErrors.add(1);

    check(res, {
      "11 logout: routed correctly": (r) => r.status !== 404 && r.status !== 502,
      "11 logout: status 200 or 204": (r) => r.status === 200 || r.status === 204,
    });
  });

  flowSuccessRate.add(flowOk);
  sleep(1);
}

export function teardown(data) {
  console.log(`[teardown] ${FLOW_NAME} complete.`);
  console.log(`[teardown] Gateway: ${GATEWAY_URL}`);
}
