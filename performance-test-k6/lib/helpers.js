/**
 * lib/helpers.js
 * ─────────────────────────────────────────────────────────────
 * Utility functions yang sering dipakai di semua flow.
 * Import seperlunya — tidak harus pakai semua.
 */

/**
 * Parse JSON response dengan aman.
 * Kalau body bukan JSON (misal server error HTML), return null — tidak crash.
 *
 * @param {object} response - k6 response object (r)
 * @returns {object|null}
 *
 * Contoh:
 *   const body = safeJson(res);
 *   const id   = body?.data?.refund_id;
 */
export function safeJson(response) {
  try {
    return JSON.parse(response.body);
  } catch {
    return null;
  }
}

/**
 * Generate unique order/transaction ID per iterasi.
 * Format: {prefix}-VU{n}-I{n}-{timestamp}
 * Dijamin unik — tidak ada collision antar VU atau iterasi.
 *
 * @param {string} prefix - contoh: "ORD", "TXN", "RFD"
 * @returns {string}
 *
 * Contoh:
 *   uniqueId("ORD")  →  "ORD-VU3-I7-1716876543210"
 */
export function uniqueId(prefix = "ID") {
  return `${prefix}-VU${__VU}-I${__ITER}-${Date.now()}`;
}

/**
 * Pilih item random dari array.
 *
 * @param {Array} arr
 * @returns {*}
 *
 * Contoh:
 *   const merchant = randomFrom(merchants);
 */
export function randomFrom(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

/**
 * Generate angka random dalam range tertentu (inklusif).
 *
 * @param {number} min
 * @param {number} max
 * @returns {number}
 *
 * Contoh:
 *   const amount = randomInt(10000, 50000000);
 */
export function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

/**
 * Build nama file xlsx dengan tanggal tertentu.
 * Default offset 0 = hari ini, -1 = kemarin.
 *
 * @param {string} prefix   - contoh: "REFUND", "RECURRING"
 * @param {string} seq      - sequence number, default "0001"
 * @param {number} dayOffset - hari relatif dari hari ini
 * @returns {string}
 *
 * Contoh:
 *   xlsxFilename("REFUND")      →  "REFUND202605170001.xlsx"
 *   xlsxFilename("REFUND", -1)  →  "REFUND202605160001.xlsx"
 */
export function xlsxFilename(prefix, seq = "0001", dayOffset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + dayOffset);
  const y   = d.getFullYear();
  const mo  = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${prefix}${y}${mo}${day}${seq}.xlsx`;
}

/**
 * Log step info yang rapi — berguna untuk debug saat test jalan.
 * Otomatis include VU number dan iterasi.
 *
 * @param {string} step  - nama step
 * @param {string} msg   - pesan tambahan
 */
export function logStep(step, msg = "") {
  console.log(`[VU${__VU}|I${__ITER}] ${step}${msg ? " — " + msg : ""}`);
}
