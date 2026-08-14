.PHONY: infra-up infra-down migrate-up migrate-down migrate-status \
        dev-api dev-worker test-go lint-go build-go build-web openapi ci

# --- Local infrastructure ---------------------------------------------------
infra-up:
	docker compose -f infra/compose.yaml up -d

infra-down:
	docker compose -f infra/compose.yaml down

# --- Database migrations (run from services/api) -----------------------------
migrate-up:
	cd services/api && go run ./cmd/migrate up

migrate-down:
	cd services/api && go run ./cmd/migrate down

migrate-status:
	cd services/api && go run ./cmd/migrate status

# --- Development -------------------------------------------------------------
dev-api:
	cd services/api && go run ./cmd/api

dev-worker:
	cd services/worker && go run ./cmd/worker

# --- Go quality gates ---------------------------------------------------------
lint-go:
	cd services/api && gofmt -l . && go vet ./...
	cd services/worker && gofmt -l . && go vet ./...

test-go:
	cd services/api && go test ./...
	cd services/worker && go test ./...

build-go:
	cd services/api && go build -o bin/api ./cmd/api && go build -o bin/migrate ./cmd/migrate
	cd services/worker && go build -o bin/worker ./cmd/worker

# --- Frontend -----------------------------------------------------------------
build-web:
	pnpm -r build

# --- OpenAPI -------------------------------------------------------------------
openapi:
	cd services/api && go run ./cmd/api -dump-openapi > ../../docs/api/openapi.yaml

# --- Full local CI --------------------------------------------------------------
ci: lint-go test-go build-go build-web
