# Common tasks. Run `make help` for the list.
.PHONY: help up down logs run test fmt vet check

help:            ## Show available commands
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'

up:              ## Start local stack (MongoDB)
	docker compose up -d

down:            ## Stop local stack
	docker compose down

logs:            ## Follow container logs
	docker compose logs -f

run:             ## Run the Go server locally
	cd server && go run ./cmd/server

test:            ## Run all Go tests
	cd server && go test ./...

fmt:             ## Format Go code
	cd server && gofmt -w .

vet:             ## Static checks
	cd server && go vet ./...

check: fmt vet test  ## Format + vet + test (run before every commit)
