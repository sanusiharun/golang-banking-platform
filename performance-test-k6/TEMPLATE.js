/**
 * ╔══════════════════════════════════════════════════════════════╗
 * ║              LOAD TEST TEMPLATE — copy file ini              ║
 * ║         Rename sesuai flow yang ditest, misal:               ║
 * ║         settlement-flow.js / recurring-payment-flow.js       ║
 * ╚══════════════════════════════════════════════════════════════╝
 *
 * CARA PAKAI:
 *   1. Copy file ini, rename sesuai nama flow kamu
 *   2. Isi bagian yang ada komentar  ← EDIT THIS
 *   3. Jangan ubah bagian lib imports dan options structure
 *
 * HOW TO RUN:
 *   k6 run -e SCENARIO=smoke   performance-test-k6/nama-flow-kamu.js   ← selalu mulai dari sini
 *   k6 run -e SCENARIO=load    performance-test-k6/nama-flow-kamu.js
 *   k6 run -e SCENARIO=stress  performance-test-k6/nama-flow-kamu.js
 *
 *   Ganti environment (default: local):
 *   k6 run -e ENV=staging -e SCENARIO=load performance-test-k6/nama-flow-kamu.js
 *
 *   Simpan hasil ke file:
 *   k6 run -e SCENARIO=load --out json=results/nama-flow-load-$(date +%Y%m%d).json performance-test-k6/nama-flow-kamu.js
 */

import http                    from "k6/http";
import { check, group, sleep } from "k6";
import { Trend, Rate }         from "k6/metrics";
import { SharedArray }         from "k6/data";

// Lib — jangan diubah
import { BASE_URL, ENV_LABEL, buildHeaders } from "./lib/config.js";
import { scenarios, baseThresholds }          from "./lib/scenarios.js";
import { safeJson, uniqueId, randomFrom, randomInt } from "./lib/helpers.js";

// ──────────────────────────────────────────────────────────────
// [1] NAMA FLOW  ← EDIT THIS
// Dipakai di log dan sebagai identifier di output k6
// ──────────────────────────────────────────────────────────────
const FLOW_NAME = "template_flow";  // WAJIB pakai underscore (_), bukan hyphen (-) — aturan k6
                                    // contoh: "refund_flow", "settlement_flow", "recurring_payment_flow"

// ──────────────────────────────────────────────────────────────
// [2] CUSTOM METRICS  ← EDIT THIS
// Buat satu Trend per step penting di flow kamu.
// Format: new Trend("nama_metric", true)  → true = satuan ms
// ──────────────────────────────────────────────────────────────
const step1Duration = new Trend(`${FLOW_NAME}_step1_duration`, true);
const step2Duration = new Trend(`${FLOW_NAME}_step2_duration`, true);
// tambah sesuai jumlah step: const step3Duration = new Trend(...)

const flowSuccessRate = new Rate(`${FLOW_NAME}_success_rate`);

// ──────────────────────────────────────────────────────────────
// [3] TEST DATA  ← EDIT THIS
// Kalau perlu SharedArray, definisikan di sini.
// Kalau tidak perlu data pool, hapus bagian ini.
// ──────────────────────────────────────────────────────────────
const testData = new SharedArray("testData", function () {
  return [
    // { field1: "value1", field2: "value2" },
    // { field1: "value3", field2: "value4" },
  ];
});

// ──────────────────────────────────────────────────────────────
// [4] OPTIONS — jangan ubah struktur, cukup tambah threshold
// ──────────────────────────────────────────────────────────────
const SCENARIO = __ENV.SCENARIO || "smoke";

export const options = {
  scenarios: {
    [FLOW_NAME]: scenarios[SCENARIO],
  },
  thresholds: {
    ...baseThresholds,  // standard threshold dari lib (jangan hapus)

    // ← EDIT THIS: tambah threshold spesifik flow kamu
    [`${FLOW_NAME}_step1_duration`]: ["p(95)<500"],   // ganti angkanya sesuai SLA
    [`${FLOW_NAME}_step2_duration`]: ["p(95)<300"],
    [`${FLOW_NAME}_success_rate`]:   ["rate>0.95"],
  },
};

// ──────────────────────────────────────────────────────────────
// [5] SETUP — jangan hapus, isi kalau perlu (misal: baca file)
// ──────────────────────────────────────────────────────────────
export function setup() {
  console.log(`[setup] Flow     : ${FLOW_NAME}`);
  console.log(`[setup] Scenario : ${SCENARIO}`);
  console.log(`[setup] Env      : ${ENV_LABEL} → ${BASE_URL}`);

  // Kalau perlu baca file xlsx:
  // const fileContent = open("../test-data/NAMAFILE.xlsx", "b");
  // return { fileContent };

  return {};
}

// ──────────────────────────────────────────────────────────────
// [6] FLOW UTAMA  ← EDIT THIS
// Ini bagian yang paling banyak kamu ubah.
// Tulis step-step bisnis flow kamu di sini.
// ──────────────────────────────────────────────────────────────
export default function (data) {
  let flowOk = true;

  // ----------------------------------------------------------
  // STEP 1 — ganti nama group dan isi request kamu  ← EDIT THIS
  // ----------------------------------------------------------
  group("Step 1: Nama Step Pertama", function () {
    const payload = JSON.stringify({
      // field: "value",  ← isi sesuai request body API kamu
    });

    const res = http.post(`${BASE_URL}/your-endpoint`, payload, {
      headers: buildHeaders("application/json"),
      // Tambah header custom kalau perlu:
      // headers: buildHeaders("application/json", { "X-Merchant-ID": "123" }),
    });

    step1Duration.add(res.timings.duration);

    const ok = check(res, {
      "step1: status 200":   (r) => r.status === 200,
      "step1: success true": (r) => safeJson(r)?.success === true,
      // tambah check lain sesuai response API kamu
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Step 1 FAIL] VU${__VU} status=${res.status} body=${res.body.substring(0, 150)}`);
    }
  });

  // Stop iterasi kalau step 1 gagal
  if (!flowOk) {
    flowSuccessRate.add(false);
    sleep(1);
    return;
  }

  sleep(0.5);  // jeda antar step — sesuaikan dengan behaviour user nyata

  // ----------------------------------------------------------
  // STEP 2 — ganti nama group dan isi request kamu  ← EDIT THIS
  // ----------------------------------------------------------
  group("Step 2: Nama Step Kedua", function () {
    const res = http.get(`${BASE_URL}/your-other-endpoint`, {
      headers: buildHeaders(),
    });

    step2Duration.add(res.timings.duration);

    const ok = check(res, {
      "step2: status 200": (r) => r.status === 200,
      // tambah check lain
    });

    if (!ok) {
      flowOk = false;
      console.error(`[Step 2 FAIL] VU${__VU} status=${res.status}`);
    }
  });

  // ----------------------------------------------------------
  // Tambah STEP 3, 4, dst kalau flow kamu punya lebih banyak step
  // Copy blok group() di atas dan sesuaikan
  // ----------------------------------------------------------

  flowSuccessRate.add(flowOk);
  sleep(1);
}

// ──────────────────────────────────────────────────────────────
// [7] TEARDOWN — jangan hapus, isi kalau perlu cleanup
// ──────────────────────────────────────────────────────────────
export function teardown() {
  console.log(`[teardown] ${FLOW_NAME} selesai.`);
}
