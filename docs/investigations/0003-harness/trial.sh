#!/usr/bin/env bash
# One trial. Starts an interactive Claude Code session in a tmux pane on a
# socket of its own, gives it a prompt that needs Bash permission, and leaves
# the pane up for the caller to inspect.
#
# Deliberately the same shape Mustur's adapter uses: a detached tmux session,
# the hook injected with --settings on the command line, nothing of the owner's
# configuration touched.
set -euo pipefail
N="${1:?trial number}"
MODE="${2:-allow}"
TIMEOUT="${3:-600}"

DIR="${H0003_DIR:?H0003_DIR unset}"
SOCK=investigation0003
SESSION="h0003-$N"
WORK="$DIR/work-$N"

mkdir -p "$WORK"
echo "$MODE" > "$DIR/mode"
echo "$N" > "$DIR/trial"
rm -f "$WORK/ran-the-tool"

HOOK="$(cd "$(dirname "$0")" && pwd)/hook.sh"
SETTINGS=$(cat <<JSON
{"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":"$HOOK","timeout":$TIMEOUT}]}]}}
JSON
)

tmux -L "$SOCK" kill-session -t "$SESSION" 2>/dev/null || true
tmux -L "$SOCK" new-session -d -s "$SESSION" -c "$WORK" -x 200 -y 60 \
  env H0003_DIR="$DIR" claude \
    --settings "$SETTINGS" \
    --setting-sources "" \
    --permission-mode manual \
    --model haiku
tmux -L "$SOCK" set-option -t "$SESSION" window-size manual >/dev/null
tmux -L "$SOCK" resize-window -t "$SESSION" -x 200 -y 60 >/dev/null
echo "started $SESSION (mode=$MODE timeout=$TIMEOUT)"
