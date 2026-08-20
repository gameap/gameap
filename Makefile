GO = go

.PHONY: lint
lint:
	 golangci-lint --timeout 5m0s run ./...

.PHONY: lint-fix
lint-fix:
	golangci-lint run --fix --timeout 5m0s ./...

.PHONY: frontend-stub
frontend-stub:
	@if [ ! -f web/static/dist/index.html ]; then \
		mkdir -p web/static/dist/streamsaver; \
		printf '%s' '<!doctype html><html lang="en"><head><meta charset="utf-8"><title>GameAP</title><script>/* unit-test stub bundle */</script></head><body><div id="app"></div></body></html>' > web/static/dist/index.html; \
		printf '%s' '<!doctype html><html><body><script>/* unit-test stub mitm */</script></body></html>' > web/static/dist/streamsaver/mitm.html; \
		echo "frontend-stub: wrote stub index.html + streamsaver/mitm.html"; \
	else \
		echo "frontend-stub: real bundle present, leaving as-is"; \
	fi

.PHONY: test
test:
	go test -race -parallel 8 \
		$(shell go list ./... | \
			grep -v /test/ | \
			grep -v /repositories/testing | \
			grep -v /migrations | \
			grep -v /pkg/plugin/examples | \
			grep -v /pkg/plugin/proto | \
			grep -v /pkg/plugin/sdk | \
			grep -v /pkg/proto | \
			grep -v /pkg/testcontainer \
		)