#!/usr/bin/env bash
#
# check-export-scope.sh refuses what MUS-D-0182 refuses and passes what it
# allows, driven against a throwaway repository with an origin of its own.
#
# A gate is tested by making it fail. Run against this repository only, it
# would pass on every clean branch and prove nothing about the four cases that
# matter: a change under records/, a change below the decisions.md marker, a
# change above it, and a refresh branch that is allowed all of them.
#
# Usage: scripts/test-export-scope.sh
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The branch the check reads comes from CI's variables first; a test run inside
# CI must not inherit the real pull request's name.
unset GITHUB_HEAD_REF GITHUB_REF_NAME

g() { git -C "$tmp/work" -c user.name=test -c user.email=test@example.invalid "$@"; }

git init -q --bare "$tmp/origin.git"
git init -q -b main "$tmp/work"
mkdir -p "$tmp/work/scripts" "$tmp/work/records"
cp "$here/check-export-scope.sh" "$here/export-branch.sh" "$tmp/work/scripts/"
printf '# Records\n' >"$tmp/work/records/README.md"
printf '%s\n' '# Decisions' 'prose above' '<!-- mustur:generated from=MUS-D-0121 -->' 'generated one' 'generated two' \
  >"$tmp/work/decisions.md"
g add -A && g commit -q -m base
g remote add origin "$tmp/origin.git"
g push -q origin main
g fetch -q origin

fails=0
# expect STATUS DESCRIPTION BRANCH EDIT — EDIT runs inside the working tree on a
# fresh branch cut from main.
expect() {
  local want=$1 what=$2 branch=$3 edit=$4 out got
  g reset -q --hard
  g clean -qfd
  g switch -q -C "$branch" origin/main
  (cd "$tmp/work" && eval "$edit")
  out=$("$tmp/work/scripts/check-export-scope.sh" 2>&1)
  got=$?
  if [ "$got" -eq "$want" ]; then
    printf '  ok    export scope %s: exit %d\n' "$what" "$got"
  else
    printf '  FAIL  export scope %s: exit %d, want %d\n%s\n' "$what" "$got" "$want" "$out"
    fails=$((fails + 1))
  fi
}

expect 1 "fails on a change under records/"      feature/records "printf 'more\n' >>records/README.md"
expect 1 "fails on a new file under records/"    feature/newfile "printf 'x\n' >records/new.md"
expect 1 "fails on a change below the marker"    feature/below   "sed -i 's/generated two/edited/' decisions.md"
expect 1 "fails on an append to the tail"        feature/append  "printf 'generated three\n' >>decisions.md"
expect 1 "fails when the marker is edited away"  feature/marker  "sed -i '/mustur:generated/d' decisions.md"
expect 0 "passes on a change above the marker"   feature/above   "sed -i 's/prose above/prose edited/' decisions.md"
expect 0 "passes a committed change above it"    feature/commit  "sed -i '2a added prose' decisions.md && git -c user.name=t -c user.email=t@e.invalid commit -qam above"
expect 0 "passes a refresh branch on records/"   records/refresh-20260914T000000Z "printf 'more\n' >>records/README.md && printf 'generated three\n' >>decisions.md"

[ "$fails" -eq 0 ]
