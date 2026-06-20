/**
 * lib/config.js
 * ─────────────────────────────────────────────────────────────
 * Dynamic environment config — all values injected via -e at runtime.
 *
 * HOW TO RUN:
 *   # Local (default) — targets Docker ports
 *   k6 run performance-test-k6/auth-flow.js
 *   k6 run performance-test-k6/account-flow.js
 *
 *   # Staging
 *   k6 run -e AUTH_URL=https://auth.staging.api.com \
 *           -e ACCOUNT_URL=https://account.staging.api.com \
 *           performance-test-k6/account-flow.js
 *
 *   # Skip login — inject token directly
 *   k6 run -e BEARER_TOKEN=<token> performance-test-k6/account-flow.js
 */

// Gateway — single entry point via Traefik (all routes through port 80)
export const GATEWAY_URL           = __ENV.GATEWAY_URL           || "http://localhost";
export const TRAEFIK_DASHBOARD_URL = __ENV.TRAEFIK_DASHBOARD_URL || "http://localhost:8080";

// Per-service base URLs — used by legacy flows that hit services directly
export const AUTH_URL    = __ENV.AUTH_URL    || "http://localhost:8082";
export const ACCOUNT_URL = __ENV.ACCOUNT_URL || "http://localhost:8081";
export const AUDIT_URL          = __ENV.AUDIT_URL          || "http://localhost:8083";
export const NOTIFICATION_URL   = __ENV.NOTIFICATION_URL   || "http://localhost:8084";
export const PAYMENT_URL        = __ENV.PAYMENT_URL        || "http://localhost:8085";

// Kept for TEMPLATE.js backward compatibility
export const BASE_URL = __ENV.BASE_URL || AUTH_URL;

// Label for log output — auto-detected from URL
export const ENV_LABEL = (__ENV.GATEWAY_URL || __ENV.AUTH_URL || __ENV.ACCOUNT_URL || __ENV.BASE_URL)
  ? ((__ENV.GATEWAY_URL || __ENV.AUTH_URL || "").includes("staging") ? "STAGING" : "REMOTE")
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
