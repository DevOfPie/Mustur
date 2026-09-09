#!/usr/bin/env bash
# The hook under test.
#
# It writes the payload it was handed where the run can read it, does what MODE
# says, and prints a decision shaped for whichever event called it. Mustur's
# real hook would post the payload to the surface and block on the owner;
# MODE=hold is that case with the owner never answering.
set -u
DIR="${H0003_DIR:?H0003_DIR unset}"
MODE="$(cat "$DIR/mode" 2>/dev/null || echo allow)"
N="$(cat "$DIR/trial" 2>/dev/null || echo 0)"

payload="$(cat)"
EVENT="$(printf '%s' "$payload" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("hook_event_name",""))' 2>/dev/null || echo '')"
printf '%s\n' "$payload" > "$DIR/payload-$N-$EVENT.json"
date +%s.%N > "$DIR/fired-at-$N-$EVENT"

[ "$MODE" = hold ] && sleep 86400

case "$EVENT" in
  PreToolUse)
    if [ "$MODE" = deny ]; then
      printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Refused by investigation 0003"}}\n'
    else
      printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"Allowed by investigation 0003"}}\n'
    fi
    ;;
  PermissionRequest)
    if [ "$MODE" = deny ]; then
      printf '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":"deny","permissionDecisionReason":"Refused by investigation 0003"}}\n'
    else
      printf '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":"allow"}}\n'
    fi
    ;;
esac
