# Every target runs offline against this working tree, by hand, today —
# workflow.md's rule. CI (ci/proposed/ci.yml, once the owner applies it) calls
# these same targets and adds nothing of its own: what a check *does* lives
# here, what a check *is* lives in the workflow file. ci/proposed/README.md
# argues that split.

SHELL := bash

.PHONY: check check-links check-adoption shellcheck go-check tidy-check verify-records conformance \
        questions surfaces build serve seed export audit install install-service deploy workflow-proposals help

check: check-links check-adoption shellcheck go-check tidy-check verify-records conformance questions surfaces ## Every commit gate this tree can enforce mechanically

check-links: ## Tracked markdown: links and anchors resolve, table rows match their headers
	@scripts/check-links.sh

check-adoption: ## strucgu.yaml parses, pins are exact, every mapped role path is tracked
	@scripts/check-adoption.sh

# Tracked *and* newly added: `git ls-files` alone lists neither, so a gate run
# before `git add` passed over four new scripts and CI failed on all four. The
# gate has to see what the commit will contain, not what the last one did.
shellcheck: ## Shell scripts in the commit pass shellcheck
	@files=$$(git ls-files -c -o --exclude-standard '*.sh'); \
	if [ -z "$$files" ]; then \
	  echo "  ok    no shell scripts"; \
	else \
	  shellcheck $$files && echo "  ok    shellcheck over $$(echo $$files | wc -w) script(s)"; \
	fi

go-check: ## The Go tree builds, vets clean, and its tests pass
	@go build ./... && go vet ./... && go test ./... && echo "  ok    go build, vet and test"

# The status below is the test's, not grep's. Piping straight into grep made a
# failing conformance run print FAIL and still exit 0 — a target that reports a
# failure and passes anyway is the shape this whole check exists to catch.
conformance: ## How many of StrucGu's fixture states this checker matched, out loud
	@out=$$(go test ./internal/audit/ -run TestConformsToTheCatalogFixtures -v 2>&1); \
	  status=$$?; \
	  echo "$$out" | grep -E "fixture trees|SKIP|FAIL" || true; \
	  exit $$status

# docs/ui-surfaces.md asked in prose for a surface to be drawn before it was
# built, and was ignored seven times — twice after the owner had answered on the
# same subject (MUS-F-0027). The owner's answer was that the gate should enforce
# it rather than the file request it (MUS-Q-0061). Same shape as `questions`
# below: the rule holds because this refuses, not because a file asks.
surfaces: ## Every page served is a surface docs/ui-surfaces.md briefed first
	@out=$$(go test ./internal/web/ -count=1 \
	  -run 'TestEveryServedPageIsABriefedSurface|TestEveryBriefedPathIsActuallyServed' 2>&1); \
	  status=$$?; \
	  if [ $$status -eq 0 ]; then \
	    echo "  ok    $$(grep -c '^\*\*Serves\*\*' docs/ui-surfaces.md) surface(s) name the path they serve"; \
	  else \
	    echo "$$out"; \
	  fi; \
	  exit $$status

# Reads records/, not the store. The store is machine-local, so a store-backed
# gate could only skip on a clone and in CI — and it could not tell "no store"
# from "no buried question", which is the substitution DL-03 already made once
# here. Against the tree there is nothing to skip: an absent or empty
# questions.md is the tree saying there are none, which is a fact and not a gap.
questions: ## No open question was left unsurfaced as a prompt
	@go run ./cmd/mustur questions --gate --records records \
	  && echo "  ok    no open question in records/ was left unsurfaced"

# go.mod said a directly imported package was `// indirect` for one commit, and
# nothing noticed. An earlier version of this comment said "a whole milestone",
# which was longer than the truth. The file that documents the dependency surface
# is the one place a wrong claim about it should not survive.
tidy-check: ## go.mod and go.sum are what the imports actually need
	@go mod tidy -diff >/dev/null 2>&1 \
	  && echo "  ok    go.mod matches the imports" \
	  || { echo "  FAIL  go.mod is not tidy — run: go mod tidy"; go mod tidy -diff; exit 1; }

verify-records: ## The committed export cites only identifiers it defines
	@go run ./cmd/mustur verify --records records

install: ## Build the binary into ~/.local/bin, where the unit expects it
	@go build -o "$$HOME/.local/bin/mustur" ./cmd/mustur \
	  && echo "  ok    $$HOME/.local/bin/mustur $$($$HOME/.local/bin/mustur version)"

deploy: install ## Build, install and restart, so a change is live in one command
	@systemctl --user restart mustur
	@systemctl --user is-active --quiet mustur \
	  && echo "  ok    restarted; the running binary is this tree" \
	  || { echo "  FAIL  mustur did not come back: systemctl --user status mustur"; exit 1; }

install-service: install ## Install the systemd user unit. Does NOT enable or start it
	@install -Dm644 deploy/mustur.service "$$HOME/.config/systemd/user/mustur.service" \
	  && systemctl --user daemon-reload \
	  && echo "  ok    unit installed and not enabled." \
	  && echo "        Enabling it publishes the box: put Cloudflare Access in front of" \
	  && echo "        the hostname first. docs/ingress.md carries the order and the reason."

audit: ## Check this tree against the StrucGu modules it adopts
	@go run ./cmd/mustur audit

build: ## The binary, in this directory
	@go build -o mustur ./cmd/mustur && echo "  ok    ./mustur"

seed: ## Put what already exists into an empty store
	@go run ./cmd/mustur seed

export: ## Render the store into records/ and the generated tail of decisions.md
	@go run ./cmd/mustur export --out records --decisions decisions.md

serve: ## Serve the one tool call on loopback
	@go run ./cmd/mustur serve

workflow-proposals: ## Which ci/proposed/ workflows the owner has not applied yet
	@scripts/workflow-proposals.sh

help: ## This list
	@grep -E '^[a-z-]+:.*##' Makefile | sed -E 's/:[^#]*## /  —  /'
