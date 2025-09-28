# Ledger Service (API Layer)

A small ledger service focused on validation and storage of journal entries and accounts. The HTTP layer is intentionally minimal and readable: handlers parse/shape I/O, while business rules live in services.

## Quick Start

- Prerequisites: Go, optionally Docker (for Swagger UI/OpenAPI validation)
- Run (live reload):
  - `make dev` (uses Air). If missing: `go install github.com/air-verse/air@latest`
- Run (plain):
  - `go run ./cmd`
- Logs:
  - `LOG_FORMAT` = `json` (default) | `text`
  - `LOG_LEVEL`  = `DEBUG` | `INFO` | `WARNING` | `ERROR`

On start, the in-memory store seeds 1 user and 3 accounts (including the system OpeningBalances); their IDs are logged for quick testing.

## API Docs

- OpenAPI spec: `openapi/openapi.yaml` (also served at `GET /openapi.yaml`)
- Swagger UI via Docker: `make api-docs` → http://localhost:8081 (stop with `make api-docs-stop`)
- Validate spec: `make api-validate` (Docker) or `./tmp/bin/validate openapi/openapi.yaml` (installed during validation step)

## Endpoints (summary)

- Health/ops
  - `GET /healthz` — liveness
  - `GET /readyz` — readiness
- Entries
  - `GET /entries?user_id=...` — list (filters: currency, memo, category, is_reversed)
  - `POST /entries` — create (validates invariants; returns created entry)
  - `POST /v1/entries/batch` — create many entries in one call (canonical; requires Idempotency-Key)
  - `GET /entries/{id}?user_id=...` — fetch one
  - `POST /entries/reverse` — reverse an existing entry (flipped lines)
  
- Accounts
- `GET /accounts?user_id=...` — list (filters: name, currency, group, vendor, type, system, active)
  - `POST /accounts` — create
  - `POST /v1/accounts/batch` — create many in one call (canonical; requires Idempotency-Key)
- `PATCH /accounts/{id}?user_id=...` — update name/group/vendor/metadata
  - `DELETE /accounts/{id}?user_id=...` — soft delete (active=false)
  - `GET /accounts/{id}/balance?user_id=...[&as_of=...]` — signed balance (minor units)
  - `GET /accounts/{id}/ledger?user_id=...[&from=&to=&limit=&cursor=]` — paginated feed with running balance
  - `GET /accounts/opening-balances?user_id=...&currency=...` — returns the currency-matched OpeningBalances account (creates if missing)
- Reports
  - `GET /trial-balance?user_id=...[&as_of=...]` — net debit/credit per account grouped by currency

See OpenAPI for detailed request/response schemas.

## Core Rules (Accounts)

- Immutable identity
  - `id`, `type` (asset/liability/equity/revenue/expense), `currency` never change
- Editable descriptive fields
- `name`, path (`type:group:vendor` via `group` + `vendor`), and `metadata`
  - Path is normalized lowercase and unique per user (per currency)
  - OpeningBalances has path `equity:openingbalances`; vendor is `System`
- Soft deletes only
  - Deactivate by setting `active=false`; no hard deletes
- System accounts
  - `system=true` → forbid PATCH/DELETE
  - Reserved: `Equity:OpeningBalances` (path `equity:openingbalances:system`)
  - Created automatically for a user when their first account is created
  - Immutable identity; used for initial balances and migrations
- Misclassification
  - Don’t retag type/currency; create a new account and post entries to move balances

## Invariants (Entries)

- At least 2 lines
- All lines same currency as entry (422 `currency_mismatch` if not)
- Each line amount > 0
- Sum(debits) == Sum(credits)
- All accounts belong to `user_id`

## Examples (curl)

Replace placeholders with your seeded IDs (printed at startup) or your own.

Create an entry (with metadata):

```
curl -sS -X POST http://localhost:8080/entries \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id": "<user_id>",
    "date": "2025-09-27T12:00:00Z",
    "currency": "USD",
    "memo": "Lunch",
    "category": "eating_out",
    "metadata": {
      "tracker.source": "monzo",
      "tracker.source_txn_id": "tx_7FQ…",
      "tracker.import_batch_id": "2025-09-28T10:00",
      "tracker.input_hash": "sha256:…",
      "tracker.rule_id": "eating_out_card_v1",
      "tracker.method": "card"
    },
    "lines": [
      { "account_id": "<cash_account_id>",   "side": "debit",  "amount_minor": 1500 },
      { "account_id": "<income_account_id>", "side": "credit", "amount_minor": 1500 }
    ]
  }'

Create an entry with an idempotency header (safe retries):

```
curl -sS -X POST http://localhost:8080/entries \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: <opaque-key-from-client>' \
  -d '{
    "user_id": "<user_id>",
    "date": "2025-09-27T12:00:00Z",
    "currency": "USD",
    "memo": "Lunch",
    "category": "eating_out",
    "lines": [
      { "account_id": "<cash_account_id>",   "side": "debit",  "amount_minor": 1500 },
      { "account_id": "<income_account_id>", "side": "credit", "amount_minor": 1500 }
    ]
  }'
```
If the same `Idempotency-Key` is reused for the same `user_id`, the server responds `200 OK` with the original entry instead of creating a duplicate.
```

Reverse an entry:

```
curl -sS -X POST http://localhost:8080/entries/reverse \
  -H 'Content-Type: application/json' \
  -d '{ "user_id": "<user_id>", "entry_id": "<entry_id>" }'
```

Create an account:

```
curl -sS -X POST http://localhost:8080/v1/accounts \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id": "<user_id>",
    "name": "Monzo Current",
    "currency": "USD",
    "type": "asset",
    "group": "Bank",
    "vendor": "Monzo",
    "metadata": { "bank.iban": "DE89 3704 0044 0532 0130 00" }
  }'
```

Get account balance (currency equals account currency):

```
curl -sS "http://localhost:8080/accounts/<account_id>/balance?user_id=<user_id>"
```

Account ledger (paginated):

```
curl -sS "http://localhost:8080/accounts/<account_id>/ledger?user_id=<user_id>&limit=50"
```

Trial balance (grouped by currency):

```
curl -sS "http://localhost:8080/trial-balance?user_id=<user_id>"
```

Batch create accounts (with metadata; requires Idempotency-Key):

```
curl -sS -X POST http://localhost:8080/v1/accounts/batch \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: batch-1' \
  -d '{
    "user_id": "<user_id>",
    "accounts": [
      {"name": "Groceries",  "currency": "USD", "type": "expense", "group": "Category", "vendor": "General", "metadata": {"report.tag": "food"}},
      {"name": "Eating Out", "currency": "USD", "type": "expense", "group": "Category", "vendor": "General"}
    ]
  }'
```

Get or create OpeningBalances for a currency:

```
curl -sS "http://localhost:8080/v1/accounts/opening-balances?user_id=<user_id>&currency=GBP"
```

## Project Layout

- `cmd/main.go` — wiring, logger, dev seed, HTTP server
- `internal/httpapi` — routers, middleware, handlers, DTOs, logging
- `internal/service/journal` — entries: validate, create, reverse, balances
- `internal/service/account` — accounts: create/list/update/deactivate
- `internal/storage/memory` — in-memory repo+writer for dev/tests
- `internal/ledger` — domain entities (Account, JournalEntry, etc.)
- `openapi/openapi.yaml` — OpenAPI 3.0 spec

## Development & Testing

- Live reload: `make dev`
- Tests: `go test ./...`
- Swagger UI: `make api-docs` → http://localhost:8081
- Validate spec: `make api-validate` (Docker) or `./tmp/bin/validate openapi/openapi.yaml`
- Local stack (API + Postgres):
  - `make compose-up` → API at http://localhost:8080
  - `make compose-logs` → follow logs
  - `make compose-down` → stop and remove volumes

### Dev Seed Banner

On startup in dev, the service prints a banner with IDs for quick testing:

```
==================== DEV SEED ====================
user_id: <uuid>
opening_balances_account_id: <uuid>
cash_account_id: <uuid>
income_account_id: <uuid>
==================================================
```

- Memory backend always seeds.
- Postgres backend seeds when `DEV_SEED=true` (enabled in docker-compose).

---

This service is intentionally small and explicit. If something feels unclear, it likely wants a comment right above it—PRs welcome.

## Notes on duplicates and traceability

- Let apps decide duplicates. The ledger validates and stores balanced entries; it does not dedupe by itself.
- If you need safe retries, prefer an optional idempotency header your client controls (not a field on the entry).
- For traceability, keep opaque metadata fields the caller can fill (e.g., source transaction id, import batch, input hash, rule id). The service stores `metadata` as-is and echoes it back.
Batch create entries (requires Idempotency-Key):

```
curl -sS -X POST http://localhost:8080/v1/entries/batch \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: batch-2' \
  -d '{
    "entries": [
      {
        "user_id": "<user_id>",
        "date": "2025-09-27T12:00:00Z",
        "currency": "USD",
        "memo": "Initial groceries",
        "category": "groceries",
        "lines": [
          { "account_id": "<cash_account_id>",   "side": "credit", "amount_minor": 5000 },
          { "account_id": "<groceries_account_id>", "side": "debit", "amount_minor": 5000 }
        ]
      }
    ]
  }'

## Idempotency & Batches

- Single-entry POST `/v1/entries`: optional Idempotency-Key; apps decide whether to dedupe
- Batch POST `/v1/accounts/batch` and `/v1/entries/batch` (canonical): require Idempotency-Key; atomic all-or-nothing; request-level idempotency uses body-hash (409 on mismatch)

## Balance & Trial Balance

- Balance: `GET /v1/accounts/{id}/balance` always returns account currency; `as_of` inclusive
- Trial balance: grouped by currency; no cross-currency sums

## Metadata Semantics

- `meta.Metadata` validates keys, values, size; `Set` is best-effort; call `Validate()` before persisting
```

## Postgres Preparation

- Storage package: `internal/storage/postgres` implements the same interfaces as the in-memory store (account + entry readers/writers, idempotency, and batch transactions).
- Schema & migrations: see `db/migrations/0001_init.sql` and `db/README.md` for a minimal, invariant-enforcing schema (users, accounts, entries, entry_lines, entry_idempotency) plus indexes for ordered scans `(user_id, date, id)`.
- Local DB helpers: `make db-up`, `make db-migrate`, `make db-down` run a Postgres 16 container and apply the initial migration.
- Configuration: when you’re ready to wire Postgres in `cmd/main.go`, read `DATABASE_URL` (e.g., `postgresql://user:pass@host:5432/db?sslmode=disable`) and build the server using the Postgres store instead of memory.
- Money handling: lines persist `amount_minor` (bigint). Code reconstructs `money.Amount` using the entry currency to avoid float errors.
- Idempotency: `(user_id, key)` unique mapping to `entry_id` with `ON CONFLICT DO NOTHING` for safe retries.
- Uniqueness: DB enforces uniqueness over `(user_id, lower(group), lower(vendor), type, upper(currency))`. The service continues to slugify/normalize for paths.
- Transactions: batch endpoints (accounts and entries) leverage a store `BeginTx` that wraps `pgx.Tx` to insert all-or-nothing.

Wiring example (later):
- Detect `DATABASE_URL`; if set, initialize `postgres.Open(ctx, dsn)` and pass it anywhere `store` is used today. Keep memory as the default for dev.

## Environment Variables

- `DATABASE_URL`: if set, use Postgres store. Example: `postgresql://user:pass@host:5432/db?sslmode=disable`
- `DEV_SEED`: `true|1|yes` to insert a dev user and a few accounts (Postgres mode). Always enabled for memory mode.
- `LOG_FORMAT`: `json` (default) or `text`
- `LOG_LEVEL`: `DEBUG | INFO | WARNING | ERROR`

## Idempotency (How To Test)

- Single entry (`POST /v1/entries`): optional `Idempotency-Key`; if present, persisted in Postgres (`entry_idempotency`).
- Batch endpoints (`/v1/accounts/batch`, `/v1/entries/batch`): require `Idempotency-Key`; request-level idempotency uses a normalized body hash and an in-memory cache.

Tips
- Reuse the same key only if the body is exactly identical (including dates and order).
- When iterating, prefer a fresh key or compute the key from the request body.

Postman pre-request script (deterministic key)

```
const raw = pm.request.body && pm.request.body.mode === 'raw' ? pm.request.body.raw : '';
const hash = CryptoJS.SHA256(raw).toString(CryptoJS.enc.Hex);
pm.variables.set('IDEMPOTENCY_KEY', hash);
```

Then set header: `Idempotency-Key: {{IDEMPOTENCY_KEY}}`.

CLI example (compute key from body)

```
KEY=$(jq -S -c . < payload.json | shasum -a 256 | cut -d' ' -f1)
curl -sS -X POST http://localhost:8080/v1/accounts/batch \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $KEY" \
  --data @payload.json
```

## Local Containers (Compose)

- Start: `make compose-up`
- Logs: `make compose-logs` (shows the dev seed banner with IDs)
- Stop: `make compose-down`

The DB auto-initializes using `db/migrations/0001_init.sql` on first run.

Release planning and pre-v0.1.0 tasks are tracked as GitHub issues.
