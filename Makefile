# The checks this repository is held to. `make` runs all of them.
#
# They are here rather than only in CI so that the same commands are available
# before a change is pushed as after it. A check that only exists on a server
# is a check nobody runs while they are working.

GO ?= go

.PHONY: all
all: fmt vet test

## fmt — every file is gofmt-clean. Reports the offenders rather than
## rewriting them, so that `make` never changes the tree under you.
.PHONY: fmt
fmt:
	@offenders=$$(gofmt -l .); \
	if [ -n "$$offenders" ]; then \
		echo "not gofmt-clean:"; echo "$$offenders"; exit 1; \
	fi
	@echo "gofmt: clean"

.PHONY: vet
vet:
	$(GO) vet ./...

## test — with the race detector, because a library about ordering should not
## have ordering bugs of its own.
.PHONY: test
test:
	$(GO) test -race -count=1 ./...

.PHONY: cover
cover:
	$(GO) test -count=1 -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

## money — no float may appear on any path an amount travels. Comments may
## discuss floats; code may not contain them.
##
## The comment is removed from each line and what remains is examined. The
## earlier version discarded any line that carried a comment at all, which meant
## `amount float64 // helper` passed: the bypass was a comment. Two limits
## remain, and they are stated in the README beside the claim rather than left
## for a reader to discover — a check whose reach is unpublished is a check
## nobody can weigh. It reads text, not syntax, so a float named inside a string
## literal containing // is invisible to it; and it sees every .go file in the
## tree, so a file the compiler skips is still checked, never the reverse.
.PHONY: money
money:
	@hits=$$(grep -rn --include='*.go' -E 'float|Float' . \
		| sed -E 's,/\*.*\*/,,; s,//.*,,' \
		| grep -E 'float|Float' || true); \
	if [ -n "$$hits" ]; then \
		echo "float found in executable code:"; echo "$$hits"; exit 1; \
	fi
	@echo "money: no float on any path an amount travels"

## size — the promise is that this can be verified line by line, so the size
## is a number the reader is owed. The README states it and a test checks it;
## this prints it.
.PHONY: size
size:
	@lib=$$(cat $$(find . -name '*.go' ! -name '*_test.go') | grep -v '^[[:space:]]*//' | grep -cv '^[[:space:]]*$$'); \
	tst=$$(cat $$(find . -name '*_test.go') | grep -v '^[[:space:]]*//' | grep -cv '^[[:space:]]*$$'); \
	echo "library and command: $$lib"; echo "tests:               $$tst"; \
	echo "total:               $$((lib + tst))"

## example — runs the worked example the README shows. Exits 1 because the
## example has findings in it; that is the point of it.
.PHONY: example
example:
	-$(GO) run ./cmd/ledger-reconcile -in testdata/example.json

## lint — golangci-lint, configured in .golangci.yml. Skipped rather than
## failed when it is not installed: a contributor without it should still be
## able to run everything else.
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "lint: golangci-lint not installed, skipping"; \
	fi

.PHONY: ci
ci: fmt vet money lint test
