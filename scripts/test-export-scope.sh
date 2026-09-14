#!/usr/bin/env bash
#
# check-export-scope.sh refuses what MUS-D-0182 refuses and passes what it
# allows, driven against a throwaway repository with an origin of its own.
#
# A gate is tested by making it fail. Run against this repository only, it
# would pass on every clean branch and prove nothing about the cases that
# matter: a change under records/ however it arrives, a change below the
# decisions.md marker, a change above it, a refresh branch that is allowed the
# export and nothing else, and which branch the run believes it is on.
#
# Usage: scripts/test-export-scope.sh
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The branch the check reads comes from CI's variables first; a test run inside
# CI must not inherit the real pull request's names.
unset GITHUB_BASE_REF GITHUB_HEAD_REF GITHUB_REF_NAME

g() { git -C "$tmp/work" -c user.name=test -c user.email=test@example.invalid "$@"; }
commit() { git -c user.name=t -c user.email=t@e.invalid commit -q "$@"; }

git init -q --bare "$tmp/origin.git"
git init -q -b main "$tmp/work"
mkdir -p "$tmp/work/scripts" "$tmp/work/records"
cp "$here/check-export-scope.sh" "$here/export-branch.sh" "$here/check-links.sh" "$tmp/work/scripts/"

# check-links.sh reads anchors from `go run ./cmd/mustur anchors`, which this
# throwaway tree cannot build. A stand-in on PATH answers for it: records/README.md
# has one heading, `known`, and nothing else has any. What is under test is which
# broken links fail and which wait, not how headings become anchors.
mkdir -p "$tmp/bin"
cat >"$tmp/bin/go" <<'STUB'
#!/usr/bin/env bash
for f in "$@"; do
  case "$f" in
    *records/README.md) printf '%s\tknown\n' "$(realpath "$f")" ;;
  esac
done
STUB
chmod +x "$tmp/bin/go"
printf '# Records\n' >"$tmp/work/records/README.md"
printf '# Old\n' >"$tmp/work/records/old.md"
printf '%s\n' '# Decisions' 'prose above' '<!-- mustur:generated from=MUS-D-0121 -->' 'generated one' 'generated two' \
  >"$tmp/work/decisions.md"
g add -A && g commit -q -m base
g remote add origin "$tmp/origin.git"
g push -q origin main
g fetch -q origin

fails=0
# expect SCRIPT STATUS DESCRIPTION BRANCH EDIT PATTERN [NAME=VALUE...]
#
# EDIT runs inside the working tree on BRANCH, freshly cut from origin/main.
# PATTERN, when not empty, has to appear in the output: an exit status alone
# cannot tell a skip from a pass. The NAME=VALUE pairs are the environment the
# script runs in, which is how CI's variables are simulated.
expect() {
  local script=$1 want=$2 what=$3 branch=$4 edit=$5 pattern=$6 out got
  shift 6
  g reset -q --hard
  g clean -qfd
  g switch -q -C "$branch" origin/main
  (cd "$tmp/work" && eval "$edit")
  out=$(env "$@" "$tmp/work/scripts/$script" 2>&1)
  got=$?
  if [ "$got" -ne "$want" ]; then
    printf '  FAIL  %s %s: exit %d, want %d\n%s\n' "${script%.sh}" "$what" "$got" "$want" "$out"
    fails=$((fails + 1))
  elif [ -n "$pattern" ] && ! grep -qF -- "$pattern" <<<"$out"; then
    printf '  FAIL  %s %s: exit %d, but the output lacks %q\n%s\n' "${script%.sh}" "$what" "$got" "$pattern" "$out"
    fails=$((fails + 1))
  else
    printf '  ok    %s %s: exit %d\n' "${script%.sh}" "$what" "$got"
  fi
}
scope() { expect check-export-scope.sh "$@"; }

rec="printf 'more\n' >>records/README.md"

scope 1 "fails on a change under records/"      feature/records "$rec" ""
scope 1 "fails on a new file under records/"    feature/newfile "printf 'x\n' >records/new.md" ""
scope 1 "fails on a deleted file under records/" feature/deleted "git rm -q records/old.md" "records/old.md"
scope 1 "fails on a committed change to records/" feature/committed "$rec && commit -am rec" "records/README.md"
scope 1 "fails on a change below the marker"    feature/below   "sed -i 's/generated two/edited/' decisions.md" ""
scope 1 "fails on an append to the tail"        feature/append  "printf 'generated three\n' >>decisions.md" ""
scope 1 "fails when the marker is edited away"  feature/marker  "sed -i '/mustur:generated/d' decisions.md" ""
scope 0 "passes on a change above the marker"   feature/above   "sed -i 's/prose above/prose edited/' decisions.md" ""
scope 0 "passes a committed change above it"    feature/commit  "sed -i '2a added prose' decisions.md && commit -am above" ""
scope 0 "passes main"                            main            "$rec" "main is where the export is committed"

# A detached HEAD has no name, so it is a feature branch, not main.
scope 1 "fails a detached HEAD on records/"     feature/detach  "$rec && git switch -q --detach" "this detached HEAD"
scope 0 "passes a clean detached HEAD"          feature/detach  "git switch -q --detach" "this detached HEAD changes nothing"

# The refresh branch: the export, and only the export.
scope 0 "passes a refresh branch on records/"   records/refresh-20260914T000000Z "$rec && printf 'generated three\n' >>decisions.md" \
  "changes nothing but records/"
scope 1 "fails a refresh branch changing a file outside the export" records/refresh-20260914T000001Z \
  "$rec && printf 'x\n' >outside.txt && git add outside.txt && printf '# x\n' >>scripts/export-branch.sh" \
  "scripts/export-branch.sh changes on records/refresh-20260914T000001Z"
scope 1 "fails a refresh branch changing prose above the marker" records/refresh-20260914T000002Z \
  "sed -i 's/prose above/prose edited/' decisions.md" "changes the prose above the marker"

# Which branch the run is on: a pull request's head first, and a pull request
# is never main; then the ref a push was made to; then git.
scope 1 "fails a pull request whose head is main" main "$rec" "changes on main," \
  GITHUB_BASE_REF=main GITHUB_HEAD_REF=main GITHUB_REF_NAME=83/merge
scope 1 "reads GITHUB_HEAD_REF over GITHUB_REF_NAME and git" main "$rec" "changes on feature/pr," \
  GITHUB_BASE_REF=main GITHUB_HEAD_REF=feature/pr GITHUB_REF_NAME=main
scope 1 "fails a pull request named only by GITHUB_HEAD_REF=main" main "$rec" "changes on main," \
  GITHUB_HEAD_REF=main
scope 0 "passes a pull request from a clean refresh branch" feature/x "$rec" "records/refresh-1 changes nothing but" \
  GITHUB_BASE_REF=main GITHUB_HEAD_REF=records/refresh-1 GITHUB_REF_NAME=84/merge
scope 0 "reads GITHUB_REF_NAME over git on a push" feature/push "$rec" "main is where" \
  GITHUB_REF_NAME=main
scope 1 "reads git when CI names nothing"        feature/git     "$rec" "changes on feature/git,"

# origin/main absent: fetched, then checked.
scope 1 "fetches origin/main when it is absent" feature/fetch "$rec && git update-ref -d refs/remotes/origin/main" \
  "records/README.md changes on feature/fetch,"
# origin/main absent and unreachable: skipped out loud, never passed quietly.
scope 0 "skips out loud when origin/main cannot be fetched" feature/nofetch \
  "$rec && git update-ref -d refs/remotes/origin/main && git remote set-url origin '$tmp/nowhere.git'" \
  "export scope NOT checked on feature/nofetch: origin/main is absent and fetching it failed"
g remote set-url origin "$tmp/origin.git"
g fetch -q origin

# check-links: a link from outside records/ into it that does not resolve waits
# for a refresh branch everywhere else, main included, because records/ there
# is main's last export (MUS-D-0182). On a refresh branch it fails.
links() { expect check-links.sh "$@" "PATH=$tmp/bin:$PATH"; }
doc="mkdir -p docs && printf '%s\n' '[known](../records/README.md#known)' \
  '[new heading](../records/README.md#mus-x-0001)' '[new file](../records/work-units/NEW.md#mus-w-0001)' >docs/links.md"

links 0 "defers a heading and a file into records/ on a feature branch" feature/links "$doc" \
  "defer 2 link(s) into records/ do not resolve on feature/links"
links 0 "defers them on main"                   main            "$doc" "defer 2 link(s) into records/ do not resolve on main"
links 0 "defers them on a pull request whose head is main" feature/pr "$doc" "do not resolve on main" \
  GITHUB_BASE_REF=main GITHUB_HEAD_REF=main
links 1 "fails them on a refresh branch"        records/refresh-20260914T000003Z "$doc" \
  "FAIL  docs/links.md -> ../records/work-units/NEW.md#mus-w-0001 (no such file)"
links 1 "fails a broken heading on a refresh branch" records/refresh-20260914T000004Z "$doc" \
  "FAIL  docs/links.md -> ../records/README.md#mus-x-0001 (no such heading)"
links 1 "never defers a link from inside records/" feature/inside "printf '[x](missing.md)\n' >records/links.md" \
  "FAIL  records/links.md -> missing.md (no such file)"
links 1 "never defers a link that is not into records/" feature/outside "printf '[x](nowhere.md#a)\n' >elsewhere.md" \
  "FAIL  elsewhere.md -> nowhere.md#a (no such file)"
links 0 "passes a resolving link into records/ without deferring it" records/refresh-20260914T000005Z \
  "printf '[known](records/README.md#known)\n' >ok.md" "ok    1 links resolve"

[ "$fails" -eq 0 ]
