#!/usr/bin/env bash
#
# A feature branch commits no part of the records export, and a refresh branch
# commits nothing else (MUS-D-0182).
#
# `make export` renders the whole store, so a branch that commits its output
# carries every record filed anywhere up to that minute, and two branches open
# at once conflict in generated files (MUS-F-0066). The owner chose to export on
# main only. What that means for a branch, mechanically:
#
#   - nothing under records/ changes, tracked or newly added;
#   - no line of decisions.md changes at or below its generated marker. Above
#     the marker is hand-written prose and stays a branch's to change.
#
# A records/refresh-* branch is the mirror image, because its name is not
# evidence of anything: a branch can be called that and carry code. It passes
# only if everything it changes is under records/ or at or below the marker; a
# file outside those, or a line above the marker, fails and is named.
#
# main passes and says why: it is where the export lands. Everything else is
# compared against its merge base with origin/main, which is what the branch's
# pull request will show. A pull request whose head is called main is not main
# (scripts/export-branch.sh).
#
# CI clones at depth 1, where origin/main is not there to compare with and a
# gate that found nothing would be reporting on history it never read
# (MUS-F-0020). So it is fetched; and if it cannot be, the check says out loud
# that it was skipped rather than passing.
#
# Usage: scripts/check-export-scope.sh
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

marker='<!-- mustur:generated from=MUS-D-0121 -->'

refresh=0
if branch=$(scripts/export-branch.sh); then
  case "$branch" in
    records/refresh-*) refresh=1 ;;
    *)
      printf '  ok    export scope: %s is where the export is committed, so records/ may change (MUS-D-0182)\n' "$branch"
      exit 0
      ;;
  esac
fi
name=${branch:-"this detached HEAD"}

skip() {
  printf '  skip  export scope NOT checked on %s: %s\n' "$name" "$1"
  exit 0
}

if ! git rev-parse -q --verify refs/remotes/origin/main >/dev/null; then
  # An explicit refspec, because a checkout made for one ref does not
  # necessarily map origin's branches to remote-tracking refs at all.
  git fetch -q --no-tags --depth=200 origin +refs/heads/main:refs/remotes/origin/main 2>/dev/null \
    || skip "origin/main is absent and fetching it failed"
fi

if ! base=$(git merge-base HEAD refs/remotes/origin/main 2>/dev/null); then
  # A shallow checkout can hold origin/main and still not reach the commit it
  # shares with HEAD. Deepen once before giving up.
  if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
    git fetch -q --no-tags --deepen=200 origin "$(git rev-parse HEAD)" 2>/dev/null || true
  fi
  base=$(git merge-base HEAD refs/remotes/origin/main 2>/dev/null) \
    || skip "HEAD shares no history with origin/main that this checkout can reach"
fi

fails=0

# Tracked changes against the merge base, and new files the commit would add —
# a gate that only sees staged or committed files passes over the one somebody
# is about to `git add`.
changed() {
  { git diff --no-renames --name-only "$base" -- "$@"
    git ls-files -o --exclude-standard -- "$@"; } | sort -u
}

if [ "$refresh" -eq 1 ]; then
  while IFS= read -r f; do
    case "$f" in
      ''|records/*|decisions.md) continue ;;
    esac
    printf '  FAIL  %s changes on %s, and a refresh branch carries only the export\n' "$f" "$name"
    fails=$((fails + 1))
  done < <(changed .)
else
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    printf '  FAIL  %s changes on %s, and records/ is committed on main only\n' "$f" "$name"
    fails=$((fails + 1))
  done < <(changed records)
fi

# decisions.md, line by line. The marker's line in the base and in the working
# tree bound the generated tail on each side of the diff: a hunk reaching either
# is a change to the tail, and a hunk reaching a line before either is a change
# to the prose. One hunk can be both.
m_old=$(git show "$base:decisions.md" 2>/dev/null | grep -nF -m1 -- "$marker" | cut -d: -f1)
m_new=$(grep -nF -m1 -- "$marker" decisions.md 2>/dev/null | cut -d: -f1)

if [ -n "$m_old" ] && [ -z "$m_new" ]; then
  printf '  FAIL  decisions.md no longer carries its generated marker (base :%s)\n' "$m_old"
  fails=$((fails + 1))
elif [ -n "$m_old" ]; then
  while IFS= read -r hunk; do
    # @@ -a[,b] +c[,d] @@ — a count left out is one.
    if [[ $hunk =~ ^@@\ -([0-9]+)(,([0-9]+))?\ \+([0-9]+)(,([0-9]+))?\ @@ ]]; then
      a=${BASH_REMATCH[1]} b=${BASH_REMATCH[3]:-1}
      c=${BASH_REMATCH[4]} d=${BASH_REMATCH[6]:-1}
      below=0 above=0
      # An old side of zero lines is an insertion after line a.
      if [ "$b" -eq 0 ]; then
        if [ "$a" -ge "$m_old" ]; then below=1; else above=1; fi
      else
        [ $((a + b - 1)) -ge "$m_old" ] && below=1
        [ "$a" -lt "$m_old" ] && above=1
      fi
      if [ "$d" -gt 0 ]; then
        [ $((c + d - 1)) -ge "$m_new" ] && below=1
        [ "$c" -lt "$m_new" ] && above=1
      fi
      if [ "$refresh" -eq 0 ] && [ "$below" -eq 1 ]; then
        printf '  FAIL  decisions.md:%s changes the generated tail below the marker at :%s\n' "$c" "$m_new"
        fails=$((fails + 1))
      elif [ "$refresh" -eq 1 ] && [ "$above" -eq 1 ]; then
        printf '  FAIL  decisions.md:%s changes the prose above the marker at :%s, and a refresh branch carries only the export\n' "$c" "$m_new"
        fails=$((fails + 1))
      fi
    fi
  done < <(git diff -U0 --no-renames "$base" -- decisions.md | grep '^@@')
fi

if [ "$refresh" -eq 1 ]; then
  if [ "$fails" -gt 0 ]; then
    printf '  %d change(s) outside the export on %s. A refresh branch carries only records/ and the\n' "$fails" "$name"
    printf '        tail of decisions.md (MUS-D-0182); anything else belongs on a branch of its own\n'
    exit 1
  fi
  printf '  ok    export scope: %s changes nothing but records/ and the decisions.md tail\n' "$name"
  exit 0
fi

if [ "$fails" -gt 0 ]; then
  printf '  %d export change(s) on %s. The export is committed on main only (MUS-D-0182):\n' "$fails" "$name"
  printf '        git restore --source=%s -- records   drops them here, the tail of decisions.md\n' "${base:0:12}"
  printf '        is restored by hand, and make records-refresh carries the export to main\n'
  exit 1
fi
printf '  ok    export scope: %s changes nothing under records/ or below the decisions.md marker\n' "$name"
