/**
 * lib/config.js
 * ─────────────────────────────────────────────────────────────
 * Dynamic environment config — semua via env variable saat run.
 * File ini tidak perlu diubah untuk ganti environment atau URL.
 *
 * HOW TO RUN:
 *   # Local (default)
 *   k6 run performance-test-k6/project-a/my-flow.js
 *
 *   # Staging — pass URL langsung
 *   k6 run -e BASE_URL=https://staging.api.com performance-test-k6/project-a/my-flow.js
 *
 *   # Staging dengan auth
 *   k6 run -e BASE_URL=https://staging.api.com -e BEARER_TOKEN=xxx performance-test-k6/project-a/my-flow.js
 *
 *   # Custom header tambahan (misal API key)
 *   k6 run -e BASE_URL=https://staging.api.com -e API_KEY=abc123 performance-test-k6/project-a/my-flow.js
 */

// URL wajib di-pass via -e BASE_URL — tidak ada hardcode
export const BASE_URL = __ENV.BASE_URL || "http://localhost:8089";

// Label untuk logging — otomatis detect dari URL
export const ENV_LABEL = __ENV.BASE_URL
  ? (__ENV.BASE_URL.includes("staging") ? "STAGING" : "REMOTE")
  : "LOCAL";

/**
 * buildHeaders(contentType?, extraHeaders?)
 * ─────────────────────────────────────────
 * Buat headers request secara dinamis.
 * Developer bisa inject header tambahan sesuai kebutuhan project mereka.
 *
 * Base headers yang selalu disertakan (kalau env variable-nya di-set):
 *   Authorization: Bearer {BEARER_TOKEN}
 *
 * @param {string|null} contentType  - "application/json" | null (untuk multipart)
 * @param {object}      extraHeaders - header tambahan spesifik project/flow
 * @returns {object}
 *
 * Contoh pemakaian di script:
 *   // JSON biasa
 *   buildHeaders("application/json")
 *
 *   // Multipart (upload file) — contentType null, k6 set otomatis
 *   buildHeaders(null)
 *
 *   // Tambah custom header
 *   buildHeaders("application/json", { "X-Merchant-ID": "123", "X-Request-ID": uniqueId("REQ") })
 *
 *   // Pakai API key dari env variable
 *   buildHeaders("application/json", { "X-Api-Key": __ENV.API_KEY })
 */
export function buildHeaders(contentType = null, extraHeaders = {}) {
  return {
    // Content-Type hanya di-set kalau ada (multipart tidak perlu)
    ...(contentType ? { "Content-Type": contentType } : {}),

    // Auth token dari env variable — kalau kosong, tidak disertakan
    ...(__ENV.BEARER_TOKEN ? { Authorization: `Bearer ${__ENV.BEARER_TOKEN}` } : {}),

    // Header tambahan dari developer — merge di atas base headers
    // Kalau ada key yang sama, extraHeaders menang (override)
    ...extraHeaders,
  };
}
