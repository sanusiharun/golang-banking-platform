# How to Run — Banking Platform

This guide covers local development setup from zero to running services.

---

## Prerequisites

- Go 1.26+
- Docker Desktop (or Docker + Docker Compose v2)
- `make`
- `git`

---

## Step 1 — Clone and enter the project

```bash
git clone <your-repo-url>
cd golang-banking-platform
```

---

## Step 2 — Set up environment files

Each stack owns its own `.env` file. None are committed — copy from the `.env.example` in each folder.

### 2a. Datasource (databases)

```bash
cp datasource/.env.example datasource/.env
```

Fill in passwords — or use the defaults from `CREDENTIALS.txt` for local dev. No changes needed if you use the defaults.

### 2b. auth-svc

```bash
cp services/auth-svc/.env.example services/auth-svc/.env
```

Open `services/auth-svc/.env` and fill in:

| Key | Value |
|-----|-------|
| `AUTH_DB_PASSWORD` | `auth_svc_pass_local` (from CREDENTIALS.txt) |
| `JWT_PRIVATE_KEY_B64` | Generate with `make gen-keys` or copy from a teammate |
| `JWT_PUBLIC_KEY_B64` | Same key pair as above |

> **Important:** auth-svc holds **both** private and public keys. It is the only service that signs tokens.

### 2c. account-svc

```bash
cp services/account-svc/.env.example services/account-svc/.env
```

Open `services/account-svc/.env` and fill in:

| Key | Value |
|-----|-------|
| `ACCOUNT_DB_PASSWORD` | `account_svc_pass_local` (from CREDENTIALS.txt) |
| `JWT_PUBLIC_KEY_B64` | **Copy exactly** from `services/auth-svc/.env` — must match |

> account-svc holds the **public key only**. It verifies tokens but cannot issue them.

### 2d. Monitoring

```bash
cp monitoring/.env.example monitoring/.env
```

Open `monitoring/.env` and fill in:

| Key | Value |
|-----|-------|
| `GRAFANA_ADMIN_PASSWORD` | Any password you want for Grafana login |
| `DISCORD_WEBHOOK_URL` | From Discord: channel → Integrations → Webhooks (optional) |

---

## Step 3 — Generate JWT keys (first time only)

```bash
make gen-keys
```

This outputs `JWT_PRIVATE_KEY_B64` and `JWT_PUBLIC_KEY_B64`. Copy both into `services/auth-svc/.env`, and copy only the public key into `services/account-svc/.env`.

---

## Step 4 — Start infrastructure

```bash
# Start databases (Postgres, MySQL, MongoDB, Redis)
make datasource-up

# Start observability stack (Prometheus, Grafana, Jaeger, Loki, Alertmanager)
make monitoring-up
```

---

## Step 5 — Run migrations

```bash
make migrate-auth      # creates banking_auth schema + seeds users
make migrate-account   # creates banking_accounts schema
```

---

## Step 6 — Run services

### Option A — Run locally (recommended for development)

Each service runs as a native Go process and streams logs to `./logs/`:

```bash
# In terminal 1
make run-auth-svc

# In terminal 2
make run-account-svc
```

Or run both at once (background):

```bash
make run-all
```

Follow logs:

```bash
make logs-follow
```

### Option B — Run in Docker

```bash
make services-up
```

---

## Step 7 — Verify everything is running

| Service | URL |
|---------|-----|
| auth-svc | http://localhost:8080/healthz |
| account-svc | http://localhost:8081/healthz |
| Grafana | http://localhost:3000 (admin / admin) |
| Prometheus | http://localhost:9090 |
| Jaeger | http://localhost:16686 |
| Alertmanager | http://localhost:9093 |

---

## Quick test

```bash
# Login
curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@banking.local","password":"Admin@12345"}' | jq .

# Use the returned access_token for authenticated requests
curl -s http://localhost:8081/accounts \
  -H "Authorization: Bearer <access_token>" | jq .
```

---

## Full stack in one command

```bash
make stack-up    # datasource + monitoring + services (Docker)
make stack-down  # stop everything
```

---

## Common issues

**`make run-auth-svc` fails with "connection refused"**
→ Make sure `make datasource-up` ran successfully and Postgres is healthy.

**`JWT_PUBLIC_KEY_B64` mismatch between services**
→ Copy the value directly from `services/auth-svc/.env` — do not regenerate separately.

**Grafana shows no data**
→ Run `make monitoring-up` and wait 30 seconds for Prometheus to scrape the first metrics.

**Discord alerts not arriving**
→ Check `DISCORD_WEBHOOK_URL` in `monitoring/.env` and verify the container is running:
```bash
docker logs banking-alertmanager-discord
```

---

## Folder structure for .env files

```
golang-banking-platform/
├── datasource/
│   ├── .env.example      ← committed, safe to read
│   └── .env              ← gitignored, your local secrets
├── monitoring/
│   ├── .env.example      ← committed, safe to read
│   └── .env              ← gitignored, your local secrets
└── services/
    ├── auth-svc/
    │   ├── .env.example  ← committed, safe to read
    │   └── .env          ← gitignored, your local secrets
    └── account-svc/
        ├── .env.example  ← committed, safe to read
        └── .env          ← gitignored, your local secrets
```

Each `.env.example` is the template. Each `.env` is your local copy with real credentials — never committed.
