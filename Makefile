.PHONY: all build api worker migrate web test lint clean deps dev dev-api dev-web

VERSION ?= dev
GOFLAGS ?= -trimpath -ldflags="-s -w"

all: build web

deps:
	go mod download
	cd web && npm install

build: api worker migrate drpctl

api:
	CGO_ENABLED=0 go build $(GOFLAGS) -o bin/drp-api ./cmd/api

worker:
	CGO_ENABLED=0 go build $(GOFLAGS) -o bin/drp-worker ./cmd/worker

migrate:
	CGO_ENABLED=0 go build $(GOFLAGS) -o bin/drp-migrate ./cmd/migrate

drpctl:
	CGO_ENABLED=0 go build $(GOFLAGS) -o bin/drpctl ./cmd/drpctl

web:
	cd web && npm run build

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ web/dist/

release: build web
	mkdir -p dist/drp-billing-$(VERSION)
	cp bin/drp-api bin/drp-worker dist/drp-billing-$(VERSION)/
	cp -r migrations dist/drp-billing-$(VERSION)/
	cp -r web/dist dist/drp-billing-$(VERSION)/web-dist
	cp deploy/scripts/*.sh dist/drp-billing-$(VERSION)/
	cp .env.example dist/drp-billing-$(VERSION)/

dev:
	@chmod +x scripts/dev.sh
	@./scripts/dev.sh

dev-api:
	unset GOROOT; GOTOOLCHAIN=$${GOTOOLCHAIN:-go1.27.1} go run ./cmd/api

dev-web:
	cd web && npm run dev -- --host 0.0.0.0 --port 5173

run-api:
	unset GOROOT; GOTOOLCHAIN=$${GOTOOLCHAIN:-go1.27.1} go run ./cmd/api

run-worker:
	unset GOROOT; GOTOOLCHAIN=$${GOTOOLCHAIN:-go1.27.1} go run ./cmd/worker

migrate-up:
	@test -f .env || (echo "missing .env — copy from .env.example" && exit 1)
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status
