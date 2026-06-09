# binarystruct — developer tasks.
GO ?= go
BENCHPKG := ./bench
BENCHFLAGS ?= -run=^$$ -bench=. -benchmem -count=6
UNSAFE_OUT := /tmp/bs-bench-unsafe.txt
SAFE_OUT := /tmp/bs-bench-safe.txt

.PHONY: bench bench-run bench-smoke test help

help: ## list targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  %-12s %s\n", $$1, $$2}'

bench: ## run the cross-mode benchmark suite and regenerate the README comparison tables
	$(GO) generate $(BENCHPKG)
	$(GO) test $(BENCHFLAGS) $(BENCHPKG) | tee $(UNSAFE_OUT)
	$(GO) test $(BENCHFLAGS) -tags safe_binarystruct $(BENCHPKG) | tee $(SAFE_OUT)
	$(GO) run $(BENCHPKG)/collate -unsafe $(UNSAFE_OUT) -safe $(SAFE_OUT) -readme README.md -readme-ja README_ja.md

bench-run: ## run the suite and print the table to stdout (do not touch the READMEs)
	$(GO) test $(BENCHFLAGS) $(BENCHPKG) | tee $(UNSAFE_OUT)
	$(GO) test $(BENCHFLAGS) -tags safe_binarystruct $(BENCHPKG) | tee $(SAFE_OUT)
	$(GO) run $(BENCHPKG)/collate -unsafe $(UNSAFE_OUT) -safe $(SAFE_OUT)

bench-smoke: ## verify the benchmarks compile & run quickly in both modes (CI bitrot guard)
	$(GO) test -run=^$$ -bench=. -benchtime=1x $(BENCHPKG)
	$(GO) test -run=^$$ -bench=. -benchtime=1x -tags safe_binarystruct $(BENCHPKG)

test: ## run the full test matrix (both runtime modes + the codegen module)
	$(GO) test ./...
	$(GO) test -tags safe_binarystruct ./...
	cd binarystruct-codegen && $(GO) test ./...
