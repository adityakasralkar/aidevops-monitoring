# Common tasks. Run `make help` for the list.
.PHONY: help up down logs run dashboard demo test itest authflow logflow metricflow fmt vet check

help:            ## Show available commands
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'

up:              ## Start local stack (MongoDB)
	docker compose up -d

down:            ## Stop local stack
	docker compose down

logs:            ## Follow container logs
	docker compose logs -f

run:             ## Run the Go server locally (loads .env if present)
	set -a; [ ! -f .env ] || . ./.env; set +a; cd server && go run ./cmd/server

dashboard:       ## Run the dashboard on http://localhost:5173 (installs packages the first time)
	cd dashboard && ([ -d node_modules ] || npm ci) && npm run dev

demo:            ## Guided live demo, one step per Enter (server must be running)
	tests/demo.sh

test:            ## Run all Go tests
	cd server && go test ./...

itest:           ## Run integration tests (needs MongoDB: make up)
	cd server && go test -tags=integration ./...

authflow:        ## End-to-end auth check against a running server (make run)
	tests/auth/auth_flow.sh

logflow:         ## End-to-end log ingestion check against a running server
	tests/ingestion/logs.sh

metricflow:      ## End-to-end metric ingestion check against a running server
	tests/ingestion/metrics.sh

fmt:             ## Format Go code
	cd server && gofmt -w .

vet:             ## Static checks
	cd server && go vet ./...

check: fmt vet test  ## Format + vet + test (run before every commit)
