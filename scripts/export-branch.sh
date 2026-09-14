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
# branch to report, so the name GitHub gives comes first: the pull request's
# head, then the ref a push was made to, then git's own answer.
#
# Usage: scripts/export-branch.sh
# Prints the branch (empty on a detached HEAD with nothing to name it). Exits 0
# when that branch commits the export, 1 when it does not.
set -uo pipefail

branch=${GITHUB_HEAD_REF:-${GITHUB_REF_NAME:-}}
if [ -z "$branch" ]; then
  branch=$(git branch --show-current 2>/dev/null || true)
fi
printf '%s\n' "$branch"

case "$branch" in
  main|records/refresh-*) exit 0 ;;
esac
exit 1
