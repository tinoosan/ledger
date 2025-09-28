Postgres Setup (Local)

- Start a Postgres container:
  - `docker run --name ledger-pg -e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres -e POSTGRES_DB=ledger -p 5432:5432 -d postgres:16`
- Apply migrations:
  - `psql postgresql://postgres:postgres@localhost:5432/ledger -f db/migrations/0001_init.sql`
- DSN examples:
  - `DATABASE_URL=postgresql://postgres:postgres@localhost:5432/ledger?sslmode=disable`

Notes
- The schema enforces key invariants: account type, side enums, positive minor units, user scoping, and uniqueness over (user, group, vendor, type, currency).
- Service-level normalization still applies (group lower-cased, vendor slugging). The DB index uses lower("group"), lower(vendor) for stability.
- Lines store `amount_minor` only. Currency comes from the parent entry; code reconstructs `money.Amount` using the entry’s currency.
- Idempotency uses `(user_id, key)` primary key and maps to a single `entry_id`.
