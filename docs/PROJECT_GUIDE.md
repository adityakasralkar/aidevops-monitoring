# AIDevOps Monitoring — Project Guide

Everything built so far: what each part does, how it works inside, and how to test it.

**Status (22 Oct 2026):** the backend pipeline for the mid-sem viva is complete and merged into `main`. Accounts, projects with API keys, log ingestion and metric ingestion all work end to end and are tested.

---

## Contents

1. [What the system does](#1-what-the-system-does)
2. [What has been built](#2-what-has-been-built)
3. [Architecture](#3-architecture)
4. [Project structure](#4-project-structure)
5. [Configuration](#5-configuration-env)
6. [Feature 1 — Server foundation](#6-feature-1--server-foundation)
7. [Feature 2 — Database setup](#7-feature-2--database-setup)
8. [Feature 3 — User accounts](#8-feature-3--user-accounts)
9. [Feature 4 — Projects and API keys](#9-feature-4--projects-and-api-keys)
10. [Feature 5 — Log ingestion](#10-feature-5--log-ingestion)
11. [Feature 6 — Metric ingestion](#11-feature-6--metric-ingestion)
12. [The ingest queue (how saving works)](#12-the-ingest-queue-how-saving-works)
13. [API reference](#13-api-reference)
14. [Data model (MongoDB)](#14-data-model-mongodb)
15. [Security summary](#15-security-summary)
16. [Testing guide](#16-testing-guide)
17. [Results so far](#17-results-so-far)
18. [Decisions made](#18-decisions-made)
19. [What comes next](#19-what-comes-next)

---

## 1. What the system does

A **self-hosted monitoring platform**. Any service (an app, an API, a worker) can send its **logs** and **metrics** to this server. The server:

1. checks the request belongs to a registered project (API key),
2. validates the data,
3. replies immediately (`202 Accepted`) so the sending app never waits,
4. saves the data to MongoDB in the background, separated per project,
5. *(later phase)* sends serious errors to an AI that explains the cause and suggests a fix,
6. *(later phase)* shows everything on a dashboard.

---

## 2. What has been built

| # | Feature | Status | Pull request |
|---|---|---|---|
| 0 | Setup: repo, Docker, MongoDB 7.0, Compass | ✅ Done | #1 |
| 1 | Server foundation: config, logging, `/healthz`, clean shutdown | ✅ Done | #2 |
| 2 | Database setup: collections + indexes created automatically | ✅ Done | #2 (extended in #3, #5) |
| 3 | User accounts: signup, login, logout, me, rate limiting | ✅ Done | #3 |
| 4 | Projects + API keys (shown once, stored hashed) | ✅ Done | #3 |
| 5 | Log ingestion: single or batch, queue, workers, severity routing, retry, drain | ✅ Done | #4 |
| 6 | Metric ingestion: gauge, sum, histogram | ✅ Done | #5 |
| — | Demo prep: Docker image, demo script, load test, rehearsal | ⏳ Next | — |
| — | OpenTelemetry endpoints (`/v1/logs`, `/v1/metrics`) | ⭐ Stretch / after viva | — |
| — | AI diagnosis, dashboard, fault testing, thesis | 📅 After viva | — |

**Size:** 38 Go files (~5,700 lines including tests), ~160 automated test cases, 3 end-to-end test scripts.

---

## 3. Architecture

```
 ┌──────────────┐   POST /api/v1/telemetry/logs     ┌──────────────────────────────────────────┐
 │ Your service │   POST /api/v1/telemetry/metrics  │            Go ingestion server           │
 │ (any app)    │ ───────────────────────────────▶ │                                          │
 └──────────────┘   header: X-API-Key              │  1. API-key check  → which project?      │
                                                    │  2. validate body  → 400 if bad          │
 ┌──────────────┐   /api/v1/auth/*                  │  3. put on queue   → 202 at once         │
 │ User (curl / │   /api/v1/projects                │     (queue full    → 503, retry later)   │
 │ dashboard)   │ ───────────────────────────────▶ │                                          │
 └──────────────┘   header: Authorization: Bearer   │  4. background workers save in batches   │
                                                    └───────────────────┬──────────────────────┘
                                                                        │
                                                    ┌───────────────────▼──────────────────────┐
                                                    │                MongoDB 7.0               │
                                                    │ Users · Sessions · Applications          │
                                                    │ ErrorLogs · AppLogs · TelemetryMetrics   │
                                                    └──────────────────────────────────────────┘
```

**Tech stack**

| Part | Choice | Why |
|---|---|---|
| Language | Go 1.27 | Fast, built-in concurrency, single binary |
| HTTP | `net/http` (standard library, Go 1.22+ routing) | Project rule: no web frameworks |
| Database | MongoDB 7.0 in Docker (Atlas later) | Flexible documents for logs/metrics |
| DB driver | Official MongoDB Go driver v2 | Official, maintained |
| Passwords | bcrypt (`golang.org/x/crypto`) | Industry standard |
| Logging | `log/slog` (standard library) | Structured logs, text in dev / JSON in prod |
| Queue | Go channels + goroutines | Project rule: no Kafka/RabbitMQ |

---

## 4. Project structure

```
aidevops-monitoring/
├── docker-compose.yml          MongoDB 7.0 (server container comes in demo prep)
├── Makefile                    shortcuts: make run, make test, make authflow …
├── .env.example                template for .env (real .env is never committed)
├── server/                     the Go server
│   ├── go.mod / go.sum         module + dependencies
│   ├── cmd/server/main.go      starts everything, clean shutdown
│   └── internal/
│       ├── config/             reads + validates .env settings
│       ├── auth/               passwords, tokens, API keys, email checks (no DB/HTTP)
│       ├── model/              data shapes: User, Session, Project, LogRecord, MetricPoint
│       ├── store/              MongoDB: connect, collections, indexes, reads/writes
│       ├── ingest/             the queue + background workers
│       └── api/                HTTP routes, handlers, middleware, rate limiter
└── tests/
    ├── auth/auth_flow.sh       end-to-end: accounts + projects
    └── ingestion/
        ├── logs.sh             end-to-end: logs, checked in MongoDB
        └── metrics.sh          end-to-end: metrics, checked in MongoDB
```

**Dependency direction** (keeps the code clean): `api → ingest / auth → model` and `ingest → store → model`. `model` imports nothing internal.

---

## 5. Configuration (`.env`)

The server reads **only environment variables**. `make run` loads `.env` into the environment first; Docker Compose will do the same with `env_file`.

| Variable | Default | Allowed | Meaning |
|---|---|---|---|
| `APP_ENV` | `development` | `development`, `production` | Text logs in development, JSON logs in production |
| `PORT` | `8080` | 1–65535 | HTTP port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` | How much the server logs |
| `MONGO_URI` | `mongodb://localhost:27017` | must start `mongodb://` or `mongodb+srv://` | Database address |
| `MONGO_DB_NAME` | `aidevops` | MongoDB name rules, ≤ 63 bytes | Database name |
| `ADMIN_SECRET` | *(none — required)* | ≥ 32 characters | Still required by config; no longer used for projects (removal listed as future cleanup) |
| `INGEST_QUEUE_SIZE` | `5000` | 500 – 1,000,000 | Max items waiting per queue (logs and metrics each) |
| `INGEST_WORKERS` | `4` | 1–256 | Background writers per queue |
| `SHUTDOWN_TIMEOUT` | `15s` | 1s – 5m | Time allowed to finish and save everything on stop |
| `TEST_USER_EMAIL` / `TEST_USER_PASSWORD` | — | — | Used only by the test scripts |

**How config loading works** (`internal/config/config.go`)
- Every value is trimmed; an empty value counts as "not set" and gets the default.
- **All** problems are collected and reported together, so a broken `.env` is fixed in one go:
  ```
  server: invalid configuration:
  PORT: must be an integer between 1 and 65535, got "abc"
  LOG_LEVEL: must be one of debug, info, warn, error, got "loud"
  ADMIN_SECRET: is required (generate one with: openssl rand -hex 32)
  ```
- **Secrets never leak:** `ADMIN_SECRET` has its own `Secret` type that prints as `[REDACTED]` in logs and with `fmt`. The password inside a `MONGO_URI` is masked (`xxxxx`) in logs. Error messages never echo the secret or the URI.

---

## 6. Feature 1 — Server foundation

**File:** `cmd/server/main.go`

### What it does
Starts the server in a fixed order and stops it safely.

### How it works — startup
1. Load and validate config (stop with a clear message if wrong).
2. Create the logger (text in development, JSON in production).
3. Listen for **Ctrl+C (SIGINT)** and **SIGTERM**.
4. Connect to MongoDB (10 s limit), ping it, create collections + indexes (30 s limit).
5. Start the two ingest queues (logs, metrics) and their workers.
6. Create the HTTP server with protective timeouts:
   - `ReadHeaderTimeout 5s`, `ReadTimeout 15s`, `WriteTimeout 15s`, `IdleTimeout 60s`, max headers 1 MiB — slow or stuck clients cannot hold connections forever.
7. Start listening on `PORT`.

### How it works — shutdown (Ctrl+C)
1. Stop accepting new HTTP requests; let running requests finish.
2. **Drain both queues**: save everything still waiting to MongoDB.
3. Print counters: `accepted`, `saved`, `dropped` for each queue.
4. Close the MongoDB connection.
5. All within `SHUTDOWN_TIMEOUT`. A second Ctrl+C kills immediately if something hangs.

If the port is already in use, the server reports `bind: address already in use` and still shuts down cleanly.

### `GET /healthz`
- `200 {"status":"ok","db":"ok"}` — server and database are working.
- `503 {"status":"degraded","db":"unreachable"}` — database down. Waits at most **2 seconds**; the real error goes to the log only, never to the client.

---

## 7. Feature 2 — Database setup

**File:** `internal/store/mongo.go`

### What it does
On every start, makes sure all collections and indexes exist.

### How it works
- `Connect` creates the client and **pings immediately** (driver v2 does no network I/O on connect, so the ping makes startup fail fast if MongoDB is down). Server-selection timeout is 5 s instead of the default 30 s, so outages show up quickly.
- `EnsureSchema` creates each collection; "already exists" (MongoDB code 48) is fine. This is race-safe even if two servers start together.
- Indexes are created with fixed names. Creating an identical index again does nothing; an index with the **same name but different keys** stops startup with an error (no silent wrong index).

### Collections and indexes

| Collection | Indexes | Purpose |
|---|---|---|
| `Users` | `email_unique` | One account per email |
| `Sessions` | `token_hash_unique`, `expires_at_ttl` (TTL), `user_id` | Fast token lookup; MongoDB deletes expired sessions automatically |
| `Applications` | `api_key_hash_unique`, `owner_project_env_unique`, `owner_created` | Key lookup on every telemetry request; no duplicate project names per user + environment; list newest first |
| `ErrorLogs` | `project_timestamp`, `project_diagnosis_status` | Latest errors; errors waiting for AI diagnosis |
| `AppLogs` | `project_timestamp` | Latest info/warn logs |
| `TelemetryMetrics` | `project_timestamp`, `project_metric_timestamp` | Latest metrics; one metric over time (charts) |

---

## 8. Feature 3 — User accounts

**Files:** `internal/auth/*`, `internal/api/auth_handlers.go`, `middleware.go`, `ratelimit.go`, `internal/store/users.go`, `sessions.go`

### What it does
Users sign up with email + password, log in to get a **session token**, and use that token to manage their projects.

| Endpoint | Login needed | Result |
|---|---|---|
| `POST /api/v1/auth/signup` | No | `201` account created |
| `POST /api/v1/auth/login` | No | `200` with token (valid 24 h) |
| `POST /api/v1/auth/logout` | Yes | `204` token invalidated |
| `GET /api/v1/auth/me` | Yes | `200` your account |

"Login needed" = header `Authorization: Bearer <token>`.

### How it works

**Signup**
1. Email is trimmed, lowercased and format-checked (`Aditya@X.com` = `aditya@x.com`; must be a plain address with a dot in the domain, ≤ 254 chars).
2. Password must be **8–72 bytes** (72 is bcrypt's limit — longer passwords are rejected, not silently cut).
3. Password is hashed with **bcrypt, cost 12** (~250 ms per hash: fine for one login, very slow for an attacker). A random salt means the same password gives a different hash each time.
4. Saved to `Users`. Duplicate email → `409 email_taken`.
5. Max **10 signups per IP per hour** → otherwise `429`.

**Login**
1. Look up the user by normalised email.
2. Check the password with bcrypt.
3. **Same answer for wrong email and wrong password:** `401 invalid_credentials`, "invalid email or password". If the email doesn't exist, a dummy bcrypt check still runs so the response takes the **same time** — attackers can't discover registered emails by message or by timing.
4. On success: create a random token (32 bytes from Go's secure random generator), store **only its SHA-256 hash** in `Sessions` with `expires_at = now + 24h`, and return the token once. Response has `Cache-Control: no-store`.

**Brute-force protection (rate limiting)**
- Failed logins are counted per **email** (max 5 per 15 min — protects one account) and per **IP** (max 20 per 15 min — stops one client trying many accounts).
- Over the limit → `429 too_many_requests` with a `Retry-After` header. Even the correct password is refused while blocked.
- A successful login clears the email's failure count.
- Counters live in memory (reset on restart; fine for one server).

**Checking a token on each request (`requireUser`)**
1. Read `Authorization: Bearer <token>` (word "Bearer" any case). Missing/malformed → `401 missing_token`.
2. Hash the token, find a session that is **not expired** (checked in the query too, because MongoDB's TTL cleanup runs only once a minute).
3. Load the user. Unknown/expired token or deleted user → `401 invalid_token`.

**Logout** deletes only the current session — other devices stay logged in.

---

## 9. Feature 4 — Projects and API keys

**Files:** `internal/auth/apikey.go`, `internal/api/project_handlers.go`, `internal/store/projects.go`

### What it does
A logged-in user registers a **project** (one app they want to monitor) and receives its **API key**. The app sends that key with every log/metric.

| Endpoint | Result |
|---|---|
| `POST /api/v1/projects` `{"name":"my-shop","environment":"test"}` | `201` with `api_key` (**shown only once**) |
| `GET /api/v1/projects` | `200` list of **your** projects (never any keys) |

### How it works
- **Name:** 1–64 characters (letters, digits, space, `_`, `-`, `.`), starting with a letter or digit.
- **Environment:** `live` or `test` (any case).
- **Key format:** `aidv_<env>_<43 random characters>`, e.g. `aidv_test_7Kq9xP2…`
  - `aidv_` makes leaked keys recognisable (like GitHub/Stripe tokens).
  - `<env>` prevents mixing test and live data.
  - 32 random bytes = impossible to guess.
- **Stored:** only the **SHA-256 hash** + a short display prefix (`aidv_test_7Kq9`). The full key exists only in the creation response.
- **Ownership:** each project has `owner_id`; users only ever see their own. Same name allowed in a different environment or for a different user; duplicate → `409 project_exists`.

---

## 10. Feature 5 — Log ingestion

**Files:** `internal/api/telemetry.go`, `internal/ingest/*`, `internal/store/logs.go`

### What it does
Apps send logs; the server checks the API key, validates, queues and replies `202` immediately; workers save them to MongoDB.

```
POST /api/v1/telemetry/logs
X-API-Key: aidv_test_...
Content-Type: application/json
```
**Single log**
```json
{
  "service_name": "checkout-api",
  "severity": "ERROR",
  "message": "database connection pool exhausted",
  "stack_trace": "at db.Pool.get (pool.go:88)",
  "timestamp": "2026-10-19T10:15:00Z",
  "attributes": { "http.route": "/pay", "retries": 3, "cached": false }
}
```
**Batch:** a JSON array of up to **500** such objects.

### Field rules

| Field | Rule |
|---|---|
| `service_name` | required, 1–100 characters |
| `severity` | required: `INFO`, `WARN`, `ERROR`, `CRITICAL` (any case; stored in capitals) |
| `message` | required (not blank), ≤ 32 KiB |
| `stack_trace` | optional, ≤ 64 KiB |
| `timestamp` | optional RFC 3339; missing → server time; > 5 min in the future → rejected |
| `attributes` | optional flat map of text / number / true-false; ≤ 50 keys; key ≤ 100 chars, not starting with `$`; text value ≤ 1 KiB; no nested objects, arrays or null |
| anything else | rejected (`unknown field`) — e.g. a sender **cannot** set `project_id` |

### How it works
1. **API-key check (`requireProject`):** read `X-API-Key` → SHA-256 → find project. Missing → `401 missing_api_key`; unknown → `401 invalid_api_key`; key environment ≠ project environment → `401 env_mismatch`.
2. **Body:** max 5 MiB; must be one object or an array of 1–500 objects.
3. **Validation is all-or-nothing:** one bad log rejects the whole batch with `400`, naming the item: `logs[3]: severity must be one of INFO, WARN, ERROR, CRITICAL`.
4. Each log gets its own `_id`, `project_id` (from the key), and `received_at`.
5. **Queue:** the whole batch is accepted or none of it. Not enough room → `503 queue_full` (`Retry-After: 1`). Shutting down → `503 shutting_down`.
6. **Reply:** `202 {"status":"accepted","id":"…"}` (single) or `202 {"status":"accepted","count":6,"ids":[…]}` (batch).
7. **Workers save in the background** (see §12), routing by severity:
   - `ERROR`, `CRITICAL` → **ErrorLogs**, with `diagnosis_status: "PENDING"` (ready for the AI layer)
   - `INFO`, `WARN` → **AppLogs**

---

## 11. Feature 6 — Metric ingestion

**Files:** `internal/api/metrics.go`, `internal/store/logs.go` (`InsertMetrics`)

### What it does
Apps send numeric measurements. Same API key, same queue pattern, same all-or-nothing batches (≤ 500) as logs. Stored in **TelemetryMetrics**.

```
POST /api/v1/telemetry/metrics
X-API-Key: aidv_test_...
```

### The three metric types

| Type | Meaning | Example | Fields |
|---|---|---|---|
| `gauge` | a value right now | CPU 73%, 42 DB connections | `value` |
| `sum` | a running total | 1,532 requests so far | `value` |
| `histogram` | a distribution | response times in buckets | `count`, `sum`, `min`, `max`, `bucket_bounds`, `bucket_counts` |

```json
{"service_name":"db","metric_name":"db_active_connections","metric_type":"gauge","value":42}
```
```json
{"service_name":"api","metric_name":"http.server.duration","metric_type":"histogram","unit":"ms",
 "count":10,"sum":1234.5,"min":12,"max":480,
 "bucket_bounds":[100,250,500],"bucket_counts":[4,3,3,0]}
```
The histogram reads: 4 requests ≤ 100 ms, 3 in 100–250 ms, 3 in 250–500 ms, 0 above 500 ms.

### Field rules
- `metric_name`: 1–200 chars, letters/digits/`.`/`_`/`-`/`/`, starting with a letter (e.g. `http.server.duration`).
- `metric_type`: `gauge`, `sum`, `histogram` (any case). `unit` optional, ≤ 30 chars.
- **gauge / sum:** `value` required; histogram fields not allowed. A value of **0 is kept** (stored as a real zero, not dropped).
- **histogram:** `value` not allowed; `count` (whole number ≥ 0) and `sum` required; `bucket_counts` required; `bucket_bounds` strictly increasing (≤ 100); `bucket_counts` has **one more** entry than `bucket_bounds`; counts ≥ 0 and **add up to `count`**; `min ≤ max`. These rules guarantee p95/p99 latency can be computed later.
- `service_name`, `timestamp`, `attributes`: same rules as logs.

---

## 12. The ingest queue (how saving works)

**Files:** `internal/ingest/queue.go`, `internal/ingest/logs.go`

One generic queue type (`Queue[T]`, like `Queue<T>` in TypeScript) is used twice: one for logs, one for metrics.

```
 handler ──Enqueue(batch)──▶ [ channel, capacity INGEST_QUEUE_SIZE ] ──▶ worker 1 ─┐
                                                                    ──▶ worker 2 ─┼─▶ MongoDB
                                                                    ──▶ worker 3 ─┤
                                                                    ──▶ worker 4 ─┘
```

| Mechanism | How it works | Why |
|---|---|---|
| **Bounded** | Room is counted in items (not requests). A batch reserves room for all its items in one atomic step (`CompareAndSwap`) | Memory can never grow without limit; batches are never half-accepted |
| **Never blocks** | If there's no room, `Enqueue` returns `ErrQueueFull` at once → `503` | The sending app gets a fast answer; OpenTelemetry senders retry automatically |
| **Merging writes** | A worker takes one batch, then grabs any others already waiting (up to 500 items) and writes them in one `InsertMany` | Light load: saved instantly. Heavy load: big efficient writes |
| **Retry** | Failed write retried after 100 ms, 500 ms, 2 s (4 attempts); each attempt limited to 10 s | Short MongoDB hiccups lose nothing; a frozen DB can't hang a worker |
| **No duplicates** | Every item has its `_id` before queuing; inserts are unordered; on retry, "duplicate key" errors mean "already saved" and count as success | Retrying a half-saved batch stores only the missing items |
| **Room freed after saving** | The counter drops only after the write finishes | Bounds items waiting *and* being written |
| **Drain** | On shutdown: stop accepting → workers empty the queue → exit. If `SHUTDOWN_TIMEOUT` runs out, in-flight writes are cancelled and the rest counted as dropped | Normal shutdown loses nothing; losses are always visible |
| **Counters** | `accepted`, `saved`, `dropped`, `queued` (atomic) | Evaluation numbers: accepted/sec vs saved/sec |
| **Safe close** | A read/write lock ensures nobody sends on the channel while it is being closed | Sending on a closed channel would crash Go |

**Trade-off to mention at the viva:** the queue is in memory, so a hard crash (power loss, `kill -9`) loses what is queued. A normal stop (Ctrl+C / SIGTERM) loses nothing.

---

## 13. API reference

### Endpoints

| Method + path | Auth | Success | Errors |
|---|---|---|---|
| `GET /healthz` | — | 200 | 503 |
| `POST /api/v1/auth/signup` | — | 201 | 400, 409, 415, 429 |
| `POST /api/v1/auth/login` | — | 200 | 400, 401, 429 |
| `POST /api/v1/auth/logout` | Bearer | 204 | 401 |
| `GET /api/v1/auth/me` | Bearer | 200 | 401 |
| `POST /api/v1/projects` | Bearer | 201 | 400, 401, 409 |
| `GET /api/v1/projects` | Bearer | 200 | 401 |
| `POST /api/v1/telemetry/logs` | X-API-Key | 202 | 400, 401, 413, 503 |
| `POST /api/v1/telemetry/metrics` | X-API-Key | 202 | 400, 401, 413, 503 |

Wrong HTTP method → `405`; unknown path → `404`.

### Error format (always the same)
```json
{"error": {"code": "invalid_credentials", "message": "invalid email or password"}}
```

| Code | Status | When |
|---|---|---|
| `invalid_json` | 400 / 413 / 415 | Broken JSON, empty body, unknown field, wrong type, body too large, wrong `Content-Type` |
| `validation_failed` | 400 | A field breaks a rule |
| `email_taken` | 409 | Signup with an existing email |
| `invalid_credentials` | 401 | Wrong email or password |
| `missing_token` / `invalid_token` | 401 | No / bad / expired session token |
| `too_many_requests` | 429 | Rate limit hit (`Retry-After` header) |
| `project_exists` | 409 | Same project name + environment |
| `missing_api_key` / `invalid_api_key` / `env_mismatch` | 401 | API-key problems |
| `queue_full` / `shutting_down` | 503 | Server busy / stopping (`Retry-After` header) |
| `internal_error` | 500 | Unexpected problem (details only in server log) |

### Request rules for all JSON endpoints
- `Content-Type: application/json` required.
- Exactly one JSON value; unknown fields rejected.
- Max body: 1 MiB (accounts, projects), 5 MiB (telemetry).

---

## 14. Data model (MongoDB)

**Users** — `_id`, `email` (unique, lowercase), `password_hash` (bcrypt), `created_at`

**Sessions** — `_id`, `user_id`, `token_hash` (SHA-256, unique), `created_at`, `expires_at` (TTL: auto-deleted)

**Applications** (projects) — `_id`, `owner_id`, `project_name`, `environment` (`live`/`test`), `api_key_hash` (SHA-256, unique), `key_prefix`, `created_at`

**ErrorLogs** (ERROR, CRITICAL) — `_id`, `project_id`, `service_name`, `severity`, `message`, `stack_trace`, `attributes`, `timestamp`, `received_at`, `diagnosis_status` (`PENDING`)

**AppLogs** (INFO, WARN) — same as ErrorLogs without `diagnosis_status`

**TelemetryMetrics** — `_id`, `project_id`, `service_name`, `metric_name`, `metric_type`, `unit`, `value` (gauge/sum) **or** `count`, `sum`, `min`, `max`, `bucket_bounds`, `bucket_counts` (histogram), `attributes`, `timestamp`, `received_at`

Notes:
- Times are UTC with millisecond precision (MongoDB's precision), so API responses match stored values exactly.
- `timestamp` = when the event happened (from the sender); `received_at` = when the server accepted it.
- Attribute names may contain dots (`http.route`). They are stored as **one key**; to query them use `$getField` (a plain `attributes.http.route` path means nested objects). How the dashboard will query these is an open design point.

---

## 15. Security summary

| Threat | Protection |
|---|---|
| Stolen database | Passwords: bcrypt. Session tokens and API keys: SHA-256 hashes only. Nothing usable in plain text |
| Password guessing | bcrypt cost 12 + rate limits per email and per IP (`429`) |
| Finding registered emails | Same message and same timing for wrong email and wrong password |
| Guessing tokens / keys | 32 bytes of secure randomness (2²⁵⁶ possibilities) |
| One project writing into another | `project_id` comes only from the API key; senders cannot set it |
| Seeing other users' data | Every project query filters by `owner_id` |
| Mixing test and live data | Environment inside the key must match the project |
| Huge / malicious requests | Body size limits, field limits, unknown fields rejected, HTTP timeouts |
| Secrets in logs or errors | `Secret` type prints `[REDACTED]`; Mongo password masked; errors never echo secrets; emails/passwords never logged |
| Leaking internals | `500` responses are generic; real errors only in the server log |
| Responses cached with secrets | `Cache-Control: no-store` on login and project-creation responses |

Known limits (fine for now, listed for later): rate-limit counters are in memory (single server); signup reveals if an email exists (needs email verification to avoid); `X-Forwarded-For` is ignored for rate limiting (correct unless behind a trusted proxy).

---

## 16. Testing guide

### 16.1 One-time setup

```bash
# 1. Start MongoDB
make up

# 2. Create .env from the template (skip if you already have one)
cp .env.example .env

# 3. Generate the admin secret (still required by config)
sed -i '' "s/^ADMIN_SECRET=.*/ADMIN_SECRET=$(openssl rand -hex 32)/" .env

# 4. Set a test account for the scripts (any email, password 8–72 chars)
#    Edit .env:  TEST_USER_EMAIL=...  TEST_USER_PASSWORD=...
```

### 16.2 Automated tests

| Command | What it runs | Needs MongoDB? | Time |
|---|---|---|---|
| `make test` | All unit tests (config, auth, ingest, api with fakes) | No | ~15 s |
| `make check` | Format + vet + unit tests (run before every commit) | No | ~20 s |
| `make itest` | Integration tests against real MongoDB (throwaway databases, auto-deleted) | Yes | ~5 s |
| `cd server && go test -race -p 1 ./...` | Unit tests with the race detector (finds concurrency bugs) | No | ~2–3 min |

What the unit tests cover, in short:
- **config** — defaults, every validation rule, all errors together, secrets never printed.
- **auth** — password rules, bcrypt hashing, token randomness, API-key format, email normalisation.
- **ingest** — severity routing, batch merging, all-or-nothing queuing, room freed after saving, retry then success, drop after retries, drain saves everything, drain timeout counts losses, concurrent senders.
- **api** — every endpoint, every error path, rate limits (with a fake clock), session expiry, ownership, same-answer login, timing, log/metric validation, 503s, body limits.
- **store (integration)** — collections + indexes, duplicate email/project, expired sessions hidden, listing without key hashes, log/metric inserts, retry without duplicates.

### 16.3 End-to-end scripts (real server + real MongoDB)

Start the server in one terminal:
```bash
make run
```
Then in another terminal:

| Command | Checks | Expected |
|---|---|---|
| `make authflow` | health, signup, duplicate, bad email/password, wrong password, login, me, fake/missing token, create project, duplicate project, bad environment, list (no keys), logout, token dead after logout | `Passed: 17  Failed: 0` |
| `tests/auth/auth_flow.sh --rate-limit` | all of the above + 6th failed login → 429 | `Passed: 18` |
| `make logflow` | missing/wrong key, single INFO/ERROR, batch of 6, bad severity, bad item in batch, fake `project_id`, then **counts in MongoDB** (ErrorLogs 4, AppLogs 4, all PENDING, attributes) | `Passed: 16  Failed: 0` |
| `make metricflow` | missing key, gauge, gauge 0, sum + histogram batch, bad type, bad histogram, bad item, then **counts in MongoDB** | `Passed: 14  Failed: 0` |

Each script prints `PASS`/`FAIL` per check and exits non-zero if anything fails. Database checks fail if MongoDB returns nothing, so they cannot pass by accident.

### 16.4 Manual walk-through with curl

```bash
# Health
curl -i http://localhost:8080/healthz

# Sign up and log in
curl -i -X POST http://localhost:8080/api/v1/auth/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@example.com","password":"demo-pass-123"}'

curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@example.com","password":"demo-pass-123"}'
# → copy "token" from the response
TOKEN=paste-token-here

# Create a project → copy "api_key" (shown only once!)
curl -s -X POST http://localhost:8080/api/v1/projects \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"demo-shop","environment":"test"}'
KEY=paste-api-key-here

# Send one error log
curl -i -X POST http://localhost:8080/api/v1/telemetry/logs \
  -H 'Content-Type: application/json' -H "X-API-Key: $KEY" \
  -d '{"service_name":"checkout","severity":"ERROR","message":"payment timeout"}'

# Send a batch of logs
curl -i -X POST http://localhost:8080/api/v1/telemetry/logs \
  -H 'Content-Type: application/json' -H "X-API-Key: $KEY" \
  -d '[{"service_name":"api","severity":"INFO","message":"ok"},
       {"service_name":"db","severity":"CRITICAL","message":"database down"}]'

# Send metrics
curl -i -X POST http://localhost:8080/api/v1/telemetry/metrics \
  -H 'Content-Type: application/json' -H "X-API-Key: $KEY" \
  -d '{"service_name":"db","metric_name":"db_active_connections","metric_type":"gauge","value":42}'

# Things that must fail
curl -i -X POST http://localhost:8080/api/v1/telemetry/logs \
  -H 'Content-Type: application/json' -H "X-API-Key: aidv_test_wrong" \
  -d '{"service_name":"a","severity":"INFO","message":"m"}'          # → 401 invalid_api_key
curl -i -X POST http://localhost:8080/api/v1/telemetry/logs \
  -H 'Content-Type: application/json' -H "X-API-Key: $KEY" \
  -d '{"service_name":"a","severity":"LOUD","message":"m"}'          # → 400 validation_failed
```

### 16.5 Check in MongoDB Compass

Connect to `mongodb://localhost:27017`, open database **`aidevops`**:

| Collection | What you should see |
|---|---|
| `Users` | your email + `password_hash` starting `$2a$12$` (never the password) |
| `Sessions` | `token_hash` (64 hex chars), `expires_at` 24 h after `created_at`; gone after logout |
| `Applications` | `project_name`, `environment`, `api_key_hash`, `key_prefix` like `aidv_test_7Kq9` (never the full key), `owner_id` |
| `ErrorLogs` | your ERROR/CRITICAL logs with `diagnosis_status: "PENDING"` |
| `AppLogs` | your INFO/WARN logs |
| `TelemetryMetrics` | gauges with `value`, histograms with buckets |
| Indexes tab | the index names from §7 |

### 16.6 Failure tests (good to show at the viva)

| Test | How | Expected |
|---|---|---|
| Bad config | `PORT=abc make run` | All problems listed together, exit code 1 |
| Port in use | start a second server | `bind: address already in use`, clean exit |
| Database down | `docker stop aidevops-mongo`, then `curl /healthz` | `503 {"db":"unreachable"}` within 2 s |
| Server can't reach DB at start | stop Mongo, then `make run` | fails within ~10 s with a clear error |
| Clean shutdown | Ctrl+C while sending logs | log line `ingest queue stopped … dropped=0`, then `server stopped cleanly` |

---

## 17. Results so far

**Flood test 1 — logs** (40 parallel requests × 500 logs = 20,000, Ctrl+C after 0.3 s):

| | |
|---|---|
| Accepted | 25 requests (`202`) = 12,500 logs |
| Refused | 15 requests (`503 queue_full`) — correct back-pressure, queue capacity 5,000 |
| Saved in MongoDB | **12,500** (6,250 ErrorLogs + 6,250 AppLogs) |
| Dropped | **0** |

**Flood test 2 — logs + metrics together** (20 + 20 requests × 500, Ctrl+C after 0.3 s):

| Queue | Accepted | Saved | Dropped |
|---|---|---|---|
| logs | 7,500 | 7,500 | 0 |
| metrics | 7,000 | 7,000 | 0 |

**Every accepted item was saved, even with shutdown in the middle of a flood.**

Proper throughput numbers (accepted/sec, saved/sec) come from the load test in demo prep.

---

## 18. Decisions made

(Full log with reasons in `PROJECT_MASTER.md` §11.)

| # | Decision |
|---|---|
| 20 | Monorepo: `server/`, `dashboard/`, `tests/`, one `docker-compose.yml` |
| 21 | Aditya's own app is the telemetry sender (no demo-apps folder) |
| 22 | MongoDB 7.0 locally (8.x can't start on Docker Desktop's kernel); Atlas later |
| 24 | Cleaned `.env` variable names |
| 25 | Viva re-plan: custom-format pipeline first; OpenTelemetry endpoints become a stretch goal |
| 26 | No AI mention anywhere in git history |
| 27 | One-line commits (`feat` / `fix` / plain); PRs title only |
| 28 | Minimal user accounts (signup/login) replace admin-secret registration |
| 29 | Logs endpoint: single or batch ≤ 500, all-or-nothing |
| 30 | Metrics endpoint mirrors the logs pipeline; histogram rules |

Earlier decisions still in force: #10 reply-first ingestion with bounded queue + drain, #12 `project_id` from the key, #13 metric types, #14 key format, #15 ErrorLogs/AppLogs split.

---

## 19. What comes next

**Before the viva (23 Oct)**
1. Dockerfile + server in `docker-compose.yml` → one `docker compose up` starts everything.
2. Guided demo script (press Enter for each step).
3. Load test → accepted/sec and saved/sec numbers.
4. Rehearsal from a clean start; tag `v0.1-midsem`; backup screen recording.
5. UI design prototype and 3–5 slides.

**After the viva**
- OpenTelemetry endpoints `/v1/logs`, `/v1/metrics` (JSON + protobuf) reusing this pipeline.
- AI diagnosis of `PENDING` errors (explains + suggests; never changes code).
- Dashboard.
- Fault-injection evaluation.
- Future items (PRD §6a): forgot password, email verification, refresh tokens, roles/teams, social login, login page, API-key lookup cache, remove `ADMIN_SECRET`, attribute-key query design.

### Make commands (quick reference)

| Command | Does |
|---|---|
| `make up` / `make down` | Start / stop MongoDB |
| `make run` | Run the server (loads `.env`) |
| `make test` | Unit tests |
| `make itest` | Integration tests (needs MongoDB) |
| `make check` | Format + vet + unit tests |
| `make authflow` | End-to-end accounts check |
| `make logflow` | End-to-end logs check |
| `make metricflow` | End-to-end metrics check |
| `make logs` | Follow container logs |
