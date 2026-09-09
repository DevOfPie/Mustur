#!/usr/bin/env bash
# One trial, end to end and unattended.
#
# Starts an interactive Claude Code session in a tmux pane on a socket of its
# own, answers the workspace-trust dialog the CLI draws before any hook can
# exist, gives it a prompt that needs Bash permission, and waits.
#
# Deliberately the same shape Mustur's adapter uses: a detached tmux session,
# the hook injected with --settings on the command line Start builds, nothing of
# the owner's configuration touched.
set -euo pipefail
N="${1:?trial number}"
MODE="${2:-allow}"
TIMEOUT="${3:-600}"
WAIT="${4:-25}"
EVENT="${5:-PreToolUse}"

DIR="${H0003_DIR:?H0003_DIR unset}"
SOCK=investigation0003
SESSION="h0003-$N"
WORK="$DIR/work-$N"
T() { tmux -L "$SOCK" "$@"; }

mkdir -p "$WORK"
echo "$MODE" > "$DIR/mode"
echo "$N" > "$DIR/trial"
rm -f "$WORK/ran-the-tool"; rm -f "$DIR"/payload-"$N"-*.json "$DIR"/fired-at-"$N"-*

HOOK="$(cd "$(dirname "$0")" && pwd)/hook.sh"
ENTRY="[{\"hooks\":[{\"type\":\"command\",\"command\":\"$HOOK\",\"timeout\":$TIMEOUT}]}]"
SETTINGS="{\"hooks\":{\"${EVENT:-PreToolUse}\":$ENTRY}}"

T kill-session -t "$SESSION" 2>/dev/null || true
T new-session -d -s "$SESSION" -c "$WORK" -x 200 -y 60 \
  env H0003_DIR="$DIR" claude \
    --settings "$SETTINGS" --setting-sources "" \
    --permission-mode manual --model opus
T set-option -t "$SESSION" window-size manual >/dev/null
T resize-window -t "$SESSION" -x 200 -y 60 >/dev/null

# The trust dialog. It is drawn before the session exists, so no hook can reach
# it -- which is itself a result, recorded in the verdict rather than here.
for _ in $(seq 30); do
  if T capture-pane -p -t "$SESSION" | grep -q 'trust this folder'; then
    T send-keys -t "$SESSION" Down; sleep 1; T send-keys -t "$SESSION" Enter
    break
  fi
  sleep 1
done

for _ in $(seq 30); do
  T capture-pane -p -t "$SESSION" | grep -q 'for shortcuts' && break
  sleep 1
done

T send-keys -t "$SESSION" -l 'Run exactly this with the Bash tool, nothing else: touch ran-the-tool'
T send-keys -t "$SESSION" Enter
sleep "$WAIT"

echo "--- trial $N (mode=$MODE timeout=$TIMEOUT) ---"
echo "hook fired:   $(ls "$DIR"/payload-"$N"-*.json >/dev/null 2>&1 && echo yes || echo NO)"
echo "tool ran:     $([ -f "$WORK/ran-the-tool" ] && echo yes || echo no)"
echo "dialog drawn: $(T capture-pane -p -t "$SESSION" | grep -q 'Do you want to proceed' && echo YES || echo no)"
T capture-pane -p -t "$SESSION" > "$DIR/pane-$N.txt"
