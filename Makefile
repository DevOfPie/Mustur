# Every target runs offline against this working tree, by hand, today —
# workflow.md's rule. CI (ci/proposed/ci.yml, once the owner applies it) calls
# these same targets and adds nothing of its own: what a check *does* lives
# here, what a check *is* lives in the workflow file. ci/proposed/README.md
# argues that split.

SHELL := bash

.PHONY: check check-links check-adoption shellcheck go-check verify-records \
        build serve seed export workflow-proposals help

check: check-links check-adoption shellcheck go-check verify-records ## Every commit gate this tree can enforce mechanically

check-links: ## Tracked markdown: links and anchors resolve, table rows match their headers
	@scripts/check-links.sh

check-adoption: ## strucgu.yaml parses, pins are exact, every mapped role path is tracked
	@scripts/check-adoption.sh

shellcheck: ## Tracked shell scripts pass shellcheck
	@files=$$(git ls-files '*.sh'); \
	if [ -z "$$files" ]; then \
	  echo "  ok    no shell scripts"; \
	else \
	  shellcheck $$files && echo "  ok    shellcheck over $$(echo $$files | wc -w) script(s)"; \
	fi

go-check: ## The Go tree builds, vets clean, and its tests pass
	@go build ./... && go vet ./... && go test ./... && echo "  ok    go build, vet and test"

verify-records: ## The committed export cites only identifiers it defines
	@go run ./cmd/mustur verify --records records

build: ## The binary, in this directory
	@go build -o mustur ./cmd/mustur && echo "  ok    ./mustur"

seed: ## Put what already exists into an empty store
	@go run ./cmd/mustur seed

export: ## Render the store into records/
	@go run ./cmd/mustur export --out records

serve: ## Serve the one tool call on loopback
	@go run ./cmd/mustur serve

workflow-proposals: ## Which ci/proposed/ workflows the owner has not applied yet
	@scripts/workflow-proposals.sh

help: ## This list
	@grep -E '^[a-z-]+:.*##' Makefile | sed -E 's/:[^#]*## /  —  /'
