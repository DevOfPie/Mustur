#!/usr/bin/env bash
# What the CLI does when the hook never returns -- the realistic failure, with
# the owner asleep. The hook's timeout is shortened so the behaviour can be
# observed in seconds rather than the documented ten-minute default; what is
# measured is what the CLI does, not how long it waits by default.
set -uo pipefail
DIR="${H0003_DIR:?H0003_DIR unset}"
HERE="$(cd "$(dirname "$0")" && pwd)"
TIMEOUT="${1:-20}"; FIRST="${2:-21}"; LAST="${3:-23}"

for n in $(seq "$FIRST" "$LAST"); do
  "$HERE/trial.sh" "$n" hold "$TIMEOUT" 3 PreToolUse >/dev/null 2>&1
  start=$(cat "$DIR/fired-at-$n-PreToolUse" 2>/dev/null || echo 0)
  outcome=unresolved; at=
  for _ in $(seq 60); do
    pane=$(tmux -L investigation0003 capture-pane -p -t "h0003-$n" 2>/dev/null)
    if printf '%s' "$pane" | grep -q 'Do you want to proceed'; then
      outcome="the CLI drew its own dialog"; at=$(date +%s.%N); break
    fi
    if [ -f "$DIR/work-$n/ran-the-tool" ]; then
      outcome="the tool ran anyway"; at=$(date +%s.%N); break
    fi
    if printf '%s' "$pane" | grep -qiE 'refus|denied|blocked by|hook'; then
      outcome="the call was refused"; at=$(date +%s.%N); break
    fi
    sleep 2
  done
  waited=$(python3 -c "print(f'{max(0.0,$at-$start):.1f}s')" 2>/dev/null || echo '-')
  printf 'trial %-3s timeout=%-4s %-30s after %s\n' "$n" "${TIMEOUT}s" "$outcome" "$waited"
  tmux -L investigation0003 capture-pane -p -t "h0003-$n" > "$DIR/pane-$n.txt" 2>/dev/null
  tmux -L investigation0003 kill-session -t "h0003-$n" 2>/dev/null
done
