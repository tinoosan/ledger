.PHONY: dev build dev-jq dev-jq-save api-validate api-docs api-docs-stop image run stop logs
.PHONY: compose-up compose-down compose-logs

dev:
	@which air >/dev/null 2>&1 || (echo "air not installed. Install: go install github.com/air-verse/air@latest" && exit 1)
	air -c .air.toml

# Run air and pretty-print JSON logs with jq
dev-jq:
	@which air >/dev/null 2>&1 || (echo "air not installed. Install: go install github.com/air-verse/air@latest" && exit 1)
	stdbuf -oL -eL air -c .air.toml 2>&1 | jq -R 'fromjson? | .'

# Run air, save raw logs, and pretty-print JSON logs
dev-jq-save:
	@which air >/dev/null 2>&1 || (echo "air not installed. Install: go install github.com/air-verse/air@latest" && exit 1)
	stdbuf -oL -eL air -c .air.toml 2>&1 | tee air.raw.log | jq -R 'fromjson? | .'

build:
	go build ./...

# -------- Docker helpers --------
# Defaults can be overridden: make image IMAGE=ghcr.io/you/ledger:dev
IMAGE ?= tinosan/ledger:dev
CONTAINER ?= ledger-api
PORT ?= 8080

image:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	docker build -t $(IMAGE) .

run:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	- docker rm -f $(CONTAINER) >/dev/null 2>&1 || true
	docker run --name $(CONTAINER) -d -p $(PORT):8080 \
	  -e LOG_FORMAT=$${LOG_FORMAT:-json} -e LOG_LEVEL=$${LOG_LEVEL:-INFO} \
	  $(IMAGE)
	@echo "Container $(CONTAINER) running on http://localhost:$(PORT)"

stop:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 0; }
	- docker rm -f $(CONTAINER) >/dev/null 2>&1 || true
	@echo "Container $(CONTAINER) stopped"

logs:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	docker logs -f $(CONTAINER)

# docker compose helpers
compose-up:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	docker compose up --build -d
	@echo "Compose stack up: api on http://localhost:8080"

compose-down:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 0; }
	docker compose down -v
	@echo "Compose stack down."

compose-logs:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	docker compose logs -f

# Validate OpenAPI using openapitools/openapi-generator-cli (requires Docker)
api-validate:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	docker run --rm -v $$(pwd)/openapi:/local openapitools/openapi-generator-cli validate -i /local/openapi.yaml

# Serve Swagger UI on http://localhost:8081 using Docker (reads openapi/openapi.yaml)
api-docs:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	docker run --name ledger-swagger-ui -d -p 8081:8080 -e SWAGGER_JSON=/spec/openapi.yaml -v $$(pwd)/openapi:/spec swaggerapi/swagger-ui
	@echo "Swagger UI: http://localhost:8081"

api-docs-stop:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 0; }
	-docker rm -f ledger-swagger-ui >/dev/null 2>&1 || true

# -------- Postgres (local) --------
.PHONY: db-up db-down db-migrate

DB_DOCKER_NAME ?= ledger-pg
DB_IMAGE ?= postgres:16
DB_USER ?= postgres
DB_PASSWORD ?= postgres
DB_NAME ?= ledger
DB_PORT ?= 5432

db-up:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 1; }
	@docker run --rm -d --name $(DB_DOCKER_NAME) \
		-e POSTGRES_USER=$(DB_USER) \
		-e POSTGRES_PASSWORD=$(DB_PASSWORD) \
		-e POSTGRES_DB=$(DB_NAME) \
		-p $(DB_PORT):5432 $(DB_IMAGE)
	@echo "Postgres up at localhost:$(DB_PORT)."

db-down:
	@command -v docker >/dev/null 2>&1 || { echo "docker not installed"; exit 0; }
	-@docker rm -f $(DB_DOCKER_NAME) >/dev/null 2>&1 || true
	@echo "Postgres container removed."

db-migrate:
	@psql "postgresql://$(DB_USER):$(DB_PASSWORD)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=disable" -f db/migrations/0001_init.sql
	@echo "Migrations applied."
