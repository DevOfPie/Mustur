#!/usr/bin/env bash
# The five consecutive trials the rule asks for, and the attach check after each.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
FIRST="${1:-11}"; LAST="${2:-15}"; MODE="${3:-allow}"

printf '%-6s %-12s %-9s %-13s %-9s %s\n' trial "hook fired" "tool ran" "dialog drawn" "attach" "types in"
for n in $(seq "$FIRST" "$LAST"); do
  out="$("$HERE/trial.sh" "$n" "$MODE" 600 25 PreToolUse 2>&1)"
  fired=$(printf '%s' "$out" | sed -n 's/^hook fired: *//p')
  ran=$(printf '%s' "$out"   | sed -n 's/^tool ran: *//p')
  drawn=$(printf '%s' "$out" | sed -n 's/^dialog drawn: *//p')

  # Property 3, after every trial rather than once: a second client attaches,
  # renders, and types.
  tmux -L outer0003 kill-server 2>/dev/null
  tmux -L outer0003 new-session -d -s w -x 200 -y 60 "tmux -L investigation0003 attach -t h0003-$n"
  sleep 3
  att=no; tmux -L outer0003 capture-pane -p -t w 2>/dev/null | grep -q 'for shortcuts' && att=yes
  marker="attached-$n-$RANDOM"
  tmux -L outer0003 send-keys -t w -l "$marker" 2>/dev/null
  sleep 2
  typed=no; tmux -L investigation0003 capture-pane -p -t "h0003-$n" 2>/dev/null | grep -q "$marker" && typed=yes
  tmux -L outer0003 kill-server 2>/dev/null
  tmux -L investigation0003 kill-session -t "h0003-$n" 2>/dev/null

  printf '%-6s %-12s %-9s %-13s %-9s %s\n' "$n" "$fired" "$ran" "$drawn" "$att" "$typed"
done
