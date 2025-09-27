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

On start, the in-memory store seeds 1 user and 2 accounts; their IDs are logged for quick testing.

## API Docs

- OpenAPI spec: `openapi/openapi.yaml` (also served at `GET /openapi.yaml`)
- Swagger UI via Docker: `make api-docs` → http://localhost:8081 (stop with `make api-docs-stop`)
- Validate spec: `make api-validate` (Docker) or `./tmp/bin/validate openapi/openapi.yaml` (installed during validation step)

## Endpoints (summary)

- Health/ops
  - `GET /healthz` — liveness
  - `GET /readyz` — readiness
- Entries
  - `GET /entries?user_id=...` — list
  - `POST /entries` — create (validates invariants; returns created entry)
  - `GET /entries/{id}?user_id=...` — fetch one
  - `POST /entries/reverse` — reverse an existing entry (flipped lines)
  - `GET /idempotency/entries/{client_entry_id}?user_id=...` — resolve previously-created entry
- Accounts
  - `GET /accounts?user_id=...[&method=&vendor=&type=]` — list (+filters)
  - `POST /accounts` — create
  - `PATCH /accounts/{id}?user_id=...` — update name/method/vendor/metadata
  - `DELETE /accounts/{id}?user_id=...` — soft delete (metadata.active=false)
  - `GET /accounts/{id}/balance?user_id=...[&as_of=...]` — signed balance (minor units)
  - `GET /accounts/{id}/ledger?user_id=...[&from=&to=&limit=&cursor=]` — paginated feed with running balance
- Reports
  - `GET /trial-balance?user_id=...[&as_of=...]` — net debit/credit per account

See OpenAPI for detailed request/response schemas.

## Core Rules (Accounts)

- Immutable identity
  - `id`, `type` (asset/liability/equity/revenue/expense), `currency` never change
- Editable descriptive fields
  - `name`, path (`type:method:vendor` via `method` + `vendor`), and `metadata`
  - Path is normalized lowercase and unique per user
- Soft deletes only
  - Deactivate by setting `metadata.active=false`; no hard deletes
- System accounts
  - `metadata.system=true` → forbid PATCH/DELETE
- Misclassification
  - Don’t retag type/currency; create a new account and post entries to move balances

## Invariants (Entries)

- At least 2 lines
- All lines same currency as entry
- Each line amount > 0
- Sum(debits) == Sum(credits)
- All accounts belong to `user_id`

## Examples (curl)

Replace placeholders with your seeded IDs (printed at startup) or your own.

Create an entry:

```
curl -sS -X POST http://localhost:8080/entries \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id": "<user_id>",
    "date": "2025-09-27T12:00:00Z",
    "currency": "USD",
    "memo": "Lunch",
    "category": "eating_out",
    "client_entry_id": "dev-1",
    "lines": [
      { "account_id": "<cash_account_id>",   "side": "debit",  "amount_minor": 1500 },
      { "account_id": "<income_account_id>", "side": "credit", "amount_minor": 1500 }
    ]
  }'
```

Reverse an entry:

```
curl -sS -X POST http://localhost:8080/entries/reverse \
  -H 'Content-Type: application/json' \
  -d '{ "user_id": "<user_id>", "entry_id": "<entry_id>" }'
```

Create an account:

```
curl -sS -X POST http://localhost:8080/accounts \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id": "<user_id>",
    "name": "Monzo Current",
    "currency": "USD",
    "type": "asset",
    "method": "Bank",
    "vendor": "Monzo"
  }'
```

Get account balance:

```
curl -sS "http://localhost:8080/accounts/<account_id>/balance?user_id=<user_id>"
```

Account ledger (paginated):

```
curl -sS "http://localhost:8080/accounts/<account_id>/ledger?user_id=<user_id>&limit=50"
```

Trial balance:

```
curl -sS "http://localhost:8080/trial-balance?user_id=<user_id>"
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

---

This service is intentionally small and explicit. If something feels unclear, it likely wants a comment right above it—PRs welcome.
