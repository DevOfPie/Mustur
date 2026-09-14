# Every target runs offline against this working tree, by hand, today —
# workflow.md's rule — except `questions`, which reads the store and says it did
# not run where there is none (MUS-D-0183). CI (ci/proposed/ci.yml, once the
# owner applies it) calls these same targets and adds nothing of its own: what
# a check *does* lives here, what a check *is* lives in the workflow file.
# ci/proposed/README.md argues that split.

SHELL := bash

.PHONY: check check-links check-adoption shellcheck go-check tidy-check verify-records conformance \
        questions surfaces export-scope-test export-scope build serve seed export records-refresh audit \
        install install-service deploy workflow-proposals help

check: check-links check-adoption shellcheck go-check tidy-check verify-records conformance questions surfaces export-scope-test export-scope ## Every commit gate this tree can enforce mechanically

# The export is committed on main only (MUS-D-0182): a branch that commits it
# carries the whole store at that minute, and two open at once conflict in files
# git cannot know are generated (MUS-F-0066). The test drives the gate against a
# throwaway repository first, because a gate that has only ever passed here has
# not been shown to refuse anything.
export-scope-test: ## The export-scope gate refuses records/ and the decisions.md tail, and allows the rest
	@scripts/test-export-scope.sh

export-scope: ## A feature branch changes nothing under records/ or below decisions.md's generated marker
	@scripts/check-export-scope.sh

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

# Reads the store, never records/. Mustur acts only on its own store; the export
# is a backup and a conformance surface, and on a feature branch it is main's,
# not this branch's, so a question raised here is not in it (MUS-D-0183, which
# supersedes MUS-D-0050). The store is machine-local, so on a clone and in CI
# there is none, and the gate says out loud that it did not run rather than
# reading the export in its place. It never runs the binary against a missing
# store: openStore creates an empty one, and an empty store passes silently,
# which would be "no store" reported as "no buried question".
questions: ## No open question in the store was left unsurfaced as a prompt
	@store=$$(scripts/store-path.sh); \
	  if [ ! -s "$$store" ]; then \
	    echo "  skip  question gate did not run: no store at $$store, and it reads only the store (MUS-D-0183)"; \
	    exit 0; \
	  fi; \
	  go run ./cmd/mustur questions --gate --db "$$store" --project MUS \
	    && echo "  ok    no open question of this project's in $$store was left unsurfaced"

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

# Refuses on a feature branch, where the export is not committed (MUS-D-0182).
# FORCE=1 renders anyway, for resolving a conflicted export by hand.
export: ## Render the store into records/ and the generated tail of decisions.md — main and records/refresh-* only
	@if [ "$${FORCE:-}" != 1 ] && ! branch=$$(scripts/export-branch.sh); then \
	  echo "  FAIL  $${branch:-this detached HEAD} does not commit the export; it is committed on main only (MUS-D-0182)"; \
	  echo "        run: make records-refresh    (FORCE=1 make export renders here anyway, to resolve by hand)"; \
	  exit 1; \
	fi
	@go run ./cmd/mustur export --out records --decisions decisions.md

records-refresh: ## Export the live store onto a records/refresh-* branch cut from main, gate it, and open a PR. DB=PATH for another store
	@scripts/records-refresh.sh $(if $(DB),--db "$(DB)")

serve: ## Serve the one tool call on loopback
	@go run ./cmd/mustur serve

workflow-proposals: ## Which ci/proposed/ workflows the owner has not applied yet
	@scripts/workflow-proposals.sh

help: ## This list
	@grep -E '^[a-z-]+:.*##' Makefile | sed -E 's/:[^#]*## /  —  /'
