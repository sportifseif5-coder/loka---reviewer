.PHONY: build test vet fmt lint status-source progress-hint golden update-golden offline ci desktop dogfood local-model-install local-model-start local-model-status local-model-pull

build:
	go build ./...

test:
	go test ./... -count=1

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

lint: fmt vet status-source

# Enforce that work-status markers live only in docs/PROGRESS.md.
status-source:
	./scripts/check-status-source.sh

# Advisory rot check: reminds the author to tick docs/PROGRESS.md. Never fails.
progress-hint:
	./scripts/progress-hint.sh

golden:
	go test ./internal/review/ -run TestGolden -count=1 -update

update-golden:
	go test ./internal/review/ -run TestGolden -count=1 -update

offline:
	./scripts/test-offline.sh

desktop:
	go build -tags desktop -o /tmp/loka ./desktop/

dogfood:
	./scripts/dogfood.sh

# Phase 1.9 local-model environment. install/start/status never pull weights.
# local-model-pull is the later step that downloads the model.
local-model-install:
	./scripts/local-model-install.sh

local-model-start:
	./scripts/local-model-start.sh

local-model-status:
	./scripts/local-model-status.sh

local-model-pull:
	./scripts/local-model-pull.sh

ci: lint test offline build desktop progress-hint
