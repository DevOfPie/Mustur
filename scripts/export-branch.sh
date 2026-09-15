#!/usr/bin/env bash
#
# Which branch this is, and whether it is one that commits the records export.
#
# The export is committed on main only (MUS-D-0182). A feature branch that runs
# `make export` commits the whole store as it stood that minute, records whose
# code lives on other branches included, and two such branches conflict in
# files git cannot know are generated (MUS-F-0066). So main's export is
# refreshed by a pull request of its own, from a branch named records/refresh-*,
# and that and main are the only branches that commit it.
#
# One detector, used by every gate and target that turns on the answer, so the
# three of them cannot disagree about which branch they are on.
#
# CI checks a pull request out as a detached merge commit, where git has no
# branch to report, so the names GitHub gives come first. A pull request is
# recognised by GITHUB_BASE_REF (or GITHUB_HEAD_REF), which GitHub sets on
# pull_request runs only; its branch is GITHUB_HEAD_REF, and it is never main,
# whatever that head is called — a fork's pull request from its own `main` is a
# branch of somebody else's, not this repository's main. Otherwise the ref a push
# was made to (GITHUB_REF_NAME), then git's own answer.
#
# The name alone is not trusted for records/refresh-*: check-export-scope.sh
# holds such a branch to carrying nothing but the export.
#
# Usage: scripts/export-branch.sh
# Prints the branch (empty on a detached HEAD with nothing to name it). Exits 0
# when that branch commits the export, 1 when it does not.
set -uo pipefail

pull_request=0
if [ -n "${GITHUB_BASE_REF:-}" ] || [ -n "${GITHUB_HEAD_REF:-}" ]; then
  pull_request=1
  branch=${GITHUB_HEAD_REF:-}
else
  branch=${GITHUB_REF_NAME:-}
  if [ -z "$branch" ]; then
    branch=$(git branch --show-current 2>/dev/null || true)
  fi
fi
printf '%s\n' "$branch"

case "$branch" in
  records/refresh-*) exit 0 ;;
  main) [ "$pull_request" -eq 0 ] && exit 0 ;;
esac
exit 1
