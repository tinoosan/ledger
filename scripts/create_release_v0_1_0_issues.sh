#!/usr/bin/env bash
set -euo pipefail

# Requires GitHub CLI: https://cli.github.com/
# Auth: gh auth login
# Repo: run from repo root (gh uses current repo), or export GH_REPO=owner/name

label_release="release"
label_version="v0.1.0"

ensure_label() {
  local name="$1"; local color="$2"; local desc="$3"
  if ! gh label view "$name" >/dev/null 2>&1; then
    gh label create "$name" --color "$color" --description "$desc" >/dev/null
  fi
}

ensure_label "$label_release" "0366d6" "Release-related tasks"
ensure_label "$label_version" "8250df" "Version 0.1.0 tasks"

create_issue() {
  local title="$1"; shift
  gh issue create --title "$title" --label "$label_release,$label_version" --body "$*"
}

create_issue "v0.1.0: Verify Postgres schema invariants" \
"Ensure DB constraints mirror service rules (account types, positive minor units, unique (user, group, vendor, type, currency), idempotency). Confirm indexes support ordered scans (entries: user_id,date,id)."

create_issue "v0.1.0: Add migration 002 if schema changed" \
"Add follow-up SQL migration to adjust any schema changes since 0001; validate idempotence and safe reruns."

create_issue "v0.1.0: Unit tests for Postgres store" \
"Cover accounts (list/get/create/update), entries (list/get/create/update), and idempotency mapping. Use a test DB and rollback between tests."

create_issue "v0.1.0: Batch endpoints tests" \
"Test /v1/accounts/batch and /v1/entries/batch for success, validation failures, and idempotency (same key + identical body returns cached response; mismatch returns 409)."

create_issue "v0.1.0: Lint and formatting pass" \
"Run golangci-lint (if configured) or standard gofmt/vet; address warnings."

create_issue "v0.1.0: Finalize OpenAPI and validate" \
"Review schemas, examples, and status codes; run make api-validate; confirm Swagger UI renders all endpoints."

create_issue "v0.1.0: Ensure 'group' naming is consistent" \
"Search and replace any remaining 'method' names in code/docs. Confirm DB column uses \"group\" and JSON uses group."

create_issue "v0.1.0: Verify pagination and filters" \
"Confirm list endpoints respect filters and pagination/cursor semantics (entries ordered by date,id)."

create_issue "v0.1.0: Healthz/readyz under DB failure" \
"Simulate DB down and reconnect; ensure health endpoints and logs behave predictably (no panics; clear errors)."

create_issue "v0.1.0: Logging hygiene" \
"Verify request IDs in logs; no sensitive data logged; useful context on errors."

create_issue "v0.1.0: Minimal metrics plan" \
"Decide on basic metrics (requests, errors, latency) and collection approach (optional for 0.1.0)."

create_issue "v0.1.0: Security and config review" \
"Re-check input validation, document required env vars and defaults (DATABASE_URL, DEV_SEED, LOG_*), and secrets handling."

create_issue "v0.1.0: CORS policy" \
"If exposed in browsers, decide CORS policy and implement middleware as needed."

create_issue "v0.1.0: Build and tag Docker image" \
"Build a versioned Docker image (v0.1.0); push to registry; update docs with pull/run instructions."

create_issue "v0.1.0: Validate compose + README" \
"Follow README using docker compose end-to-end; fix any gaps."

create_issue "v0.1.0: Changelog and release notes" \
"Draft CHANGELOG.md and GitHub release notes highlighting features and setup instructions."

echo "Created issues for v0.1.0 with labels: $label_release, $label_version"

