/**
 * lib/scenarios.js
 * ─────────────────────────────────────────────────────────────
 * Standard scenario definitions — smoke, load, stress.
 * Developer tinggal pilih via env variable SCENARIO.
 *
 * Usage:
 *   k6 run -e SCENARIO=smoke  performance-test-k6/my-flow.js  ← default
 *   k6 run -e SCENARIO=load   performance-test-k6/my-flow.js
 *   k6 run -e SCENARIO=stress performance-test-k6/my-flow.js
 *
 * Kapan pakai apa:
 *   smoke  → setiap sebelum deploy, confirm flow tidak broken
 *   load   → baseline measurement di staging (expected traffic)
 *   stress → cari breaking point, jalankan berkala atau sebelum release besar
 */

export const scenarios = {

  // 1 VU, 1 kali jalan — pastikan flow bekerja end-to-end
  smoke: {
    executor:   "per-vu-iterations",
    vus:        1,
    iterations: 1,
  },

  // Ramp up perlahan, hold di peak, ramp down
  // Gunakan di staging untuk ukur baseline performa
  load: {
    executor:  "ramping-vus",
    startVUs:  0,
    stages: [
      { duration: "30s", target: 5  },  // warm up
      { duration: "1m",  target: 10 },  // normal load
      { duration: "30s", target: 10 },  // hold di peak
      { duration: "30s", target: 0  },  // cool down
    ],
  },

  // Push ke 3x peak untuk cari breaking point
  // Jalankan sebelum release besar atau sprint review
  stress: {
    executor:  "ramping-vus",
    startVUs:  0,
    stages: [
      { duration: "30s", target: 10 },
      { duration: "1m",  target: 20 },
      { duration: "1m",  target: 30 },
      { duration: "30s", target: 0  },
    ],
  },
};

/**
 * Standard thresholds yang berlaku untuk semua flow.
 * Developer bisa extend ini dengan threshold spesifik milik mereka.
 */
export const baseThresholds = {
  http_req_failed: ["rate<0.05"],   // error rate di bawah 5%
};
