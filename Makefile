.PHONY: dev build

dev:
	@which air >/dev/null 2>&1 || (echo "air not installed. Install: go install github.com/air-verse/air@latest" && exit 1)
	air -c .air.toml

build:
	go build ./...

