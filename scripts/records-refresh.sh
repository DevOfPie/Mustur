#!/usr/bin/env bash
#
# Refresh main's records export by pull request (MUS-D-0182).
#
# The export is committed on main only. What carries it there is a branch of
# its own, cut from origin/main in a worktree so whatever is checked out here is
# never touched, exported against the store, gated, pushed and proposed. It
# never merges: the export's diff is the reviewable half of the store
# (MUS-D-0024), and a refresh nobody read is not reviewed.
#
# It refuses a store that is not this machine's live one. The export renders
# every record in the store and replaces what is there, so exporting a stray or
# empty database — a fresh $XDG_DATA_HOME, a MUSTUR_DB left over from a test —
# proposes deleting the record. The live store is the one deploy/mustur.service
# names; the store read is resolved the way cmd/mustur resolves it. --db PATH
# says otherwise, deliberately.
#
# Nothing to refresh is not a failure: the worktree and branch are removed and
# it exits 0 saying so.
#
# Usage: scripts/records-refresh.sh [--db PATH]
set -euo pipefail

cd "$(dirname "$0")/.."

gh=${GH:-$HOME/.local/bin/gh}
live="$HOME/.local/share/mustur/mustur.db"

store="" told=0
while [ $# -gt 0 ]; do
  case "$1" in
    --db) store=${2:?--db needs a path}; told=1; shift 2 ;;
    -h|--help) sed -n '2,/^set -euo/p' "$0" | sed 's/^# \{0,1\}//; /^set -euo/d'; exit 0 ;;
    *) printf '  FAIL  unknown argument %s\n' "$1" >&2; exit 2 ;;
  esac
done

# cmd/mustur's defaultDB, resolved by the script the question gate shares.
if [ -z "$store" ]; then
  store=$(scripts/store-path.sh)
fi
store=$(realpath -m "$store")

if [ "$told" -eq 0 ] && [ "$store" != "$(realpath -m "$live")" ]; then
  printf '  FAIL  the store resolves to %s, not the live store %s\n' "$store" "$live" >&2
  printf '        exporting another store proposes replacing the record with it; pass --db PATH if that is meant\n' >&2
  exit 1
fi
if [ ! -f "$store" ]; then
  # openStore creates a missing database, and an empty export deletes records/.
  printf '  FAIL  no store at %s; refusing to export an empty one over main'\''s records/\n' "$store" >&2
  exit 1
fi
# A size test cannot tell an empty store from a full one: an initialised store
# holding nothing is 147,456 bytes. Count what it holds.
if ! records=$(go run ./cmd/mustur list --db "$store"); then
  printf '  FAIL  could not list the records in %s\n' "$store" >&2
  exit 1
fi
if [ -z "$records" ]; then
  printf '  FAIL  the store at %s holds no records; refusing to export it over main'\''s records/\n' "$store" >&2
  exit 1
fi
printf '  ok    %s holds %d record(s)\n' "$store" "$(grep -c '' <<<"$records")"

git fetch -q origin
stamp=$(date -u +%Y%m%dT%H%M%SZ)
branch="records/refresh-$stamp"
wt=".claude/worktrees/records-refresh-$stamp"

git worktree add -q --no-track -b "$branch" "$wt" origin/main
printf '  ok    %s on %s, cut from origin/main\n' "$wt" "$branch"

cleanup() {
  git worktree remove --force "$wt"
  git branch -q -D "$branch"
}

MUSTUR_DB=$store make -C "$wt" --no-print-directory export

if [ -z "$(git -C "$wt" status --porcelain -- records decisions.md)" ]; then
  cleanup
  printf '  ok    main'\''s export already matches %s; nothing to refresh, worktree and branch removed\n' "$store"
  exit 0
fi

# The question gate reads MUSTUR_DB through scripts/store-path.sh, so the check
# asks the store that was exported, not whatever this environment defaults to.
if ! MUSTUR_DB=$store make -C "$wt" --no-print-directory check; then
  printf '  FAIL  make check failed on the refreshed export; %s is left for reading, nothing pushed\n' "$wt" >&2
  exit 1
fi

git -C "$wt" add -A -- records decisions.md
stat=$(git -C "$wt" diff --cached --shortstat)
count=$(git -C "$wt" diff --cached --name-only | wc -l)

git -C "$wt" commit -q -F - <<EOF
Refresh the records export from the store as of $stamp

The export is committed on main only (MUS-D-0182). Branches that filed
records since the last refresh carry none of them, so main's records/ and
the generated tail of decisions.md fall behind the store until a refresh
lands. This is that refresh: \`make export\` against $store, run in a
worktree cut from origin/main, with nothing else in the commit.

$count file(s):$stat. The diff is generated; the store is the source, and
reading this diff is reading what was filed.
EOF
printf '  ok    committed %s\n' "$(git -C "$wt" log -1 --format='%h %s')"

git -C "$wt" push -q -u origin "$branch"

url=$("$gh" pr create --repo DevOfPie/Mustur --base main --head "$branch" \
  --title "Refresh the records export ($stamp)" \
  --body "$(cat <<EOF
Refreshes main's records export from the store: \`make export\` against the live store, in a worktree cut from origin/main, gated by \`make check\` before pushing.

The export is committed on main only (MUS-D-0182), so records filed on feature branches reach \`records/\` and the generated tail of \`decisions.md\` here and nowhere else.

$count file(s):$stat. Generated; no hand edits. \`scripts/records-refresh.sh\` opened this and never merges.
EOF
)")
printf '  ok    pull request %s\n' "$url"
printf '        %s stays checked out at %s until it merges\n' "$branch" "$wt"
