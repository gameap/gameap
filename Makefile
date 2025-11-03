GO = go

.PHONY: lint
lint:
	 golangci-lint --timeout 5m0s run ./...

.PHONY: lint-fix
lint-fix:
	golangci-lint run --fix --timeout 5m0s ./...