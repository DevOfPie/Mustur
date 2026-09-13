#!/usr/bin/env bash
# Which event actually says a dialog is pending.
#
# The verdict credits PreToolUse with telling Mustur that a dialog is waiting.
# PreToolUse fires on every tool call, so that only holds if a firing means a
# dialog. This runs both events at once, in one session, over two prompts -- one
# the CLI prompts on and one it does not -- with the hook returning nothing, so
# the CLI's own permission flow decides what happens.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
DIR="${H0003_DIR:?H0003_DIR unset}"
FIRST="${1:-30}"; ROUNDS="${2:-3}"

EVENTS=PreToolUse+PermissionRequest
n=$FIRST

fired() { [ -f "$DIR/payload-$1-$2.json" ] && echo yes || echo NO; }
tool() { python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("tool_name",""))' "$DIR/payload-$1-$2.json" 2>/dev/null || echo -; }
suggests() { python3 -c 'import json,sys; print("yes" if "permission_suggestions" in json.load(open(sys.argv[1])) else "no")' "$DIR/payload-$1-$2.json" 2>/dev/null || echo -; }

printf '%-6s %-14s %-11s %-6s %-18s %-13s %s\n' trial case PreToolUse tool PermissionRequest suggestions "dialog drawn"
for _ in $(seq "$ROUNDS"); do
  for c in prompts quiet; do
    if [ "$c" = quiet ]; then
      mkdir -p "$DIR/work-$n"
      printf 'the quiet case reads this file\n' > "$DIR/work-$n/notes.txt"
      export H0003_PROMPT='Read the file notes.txt with the Read tool, nothing else, then say what it says'
    else
      export H0003_PROMPT='Run exactly this with the Bash tool, nothing else: touch ran-the-tool'
    fi
    out="$("$HERE/trial.sh" "$n" pass 600 25 "$EVENTS" 2>&1)"
    drawn=$(printf '%s' "$out" | sed -n 's/^dialog drawn: *//p')
    printf '%-6s %-14s %-11s %-6s %-18s %-13s %s\n' \
      "$n" "$c" "$(fired "$n" PreToolUse)" "$(tool "$n" PreToolUse)" \
      "$(fired "$n" PermissionRequest)" "$(suggests "$n" PermissionRequest)" "$drawn"
    tmux -L investigation0003 kill-session -t "h0003-$n" 2>/dev/null
    n=$((n+1))
  done
done
