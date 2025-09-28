.PHONY: dev build dev-jq dev-jq-save api-validate api-docs api-docs-stop

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
