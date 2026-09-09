#!/usr/bin/env bash
# The PermissionRequest hook under test.
#
# It does three things and nothing else: write the payload it was handed where
# the run can read it, do what MODE says, and print a decision. Mustur's real
# hook would post the payload to the surface and block on the owner; MODE=hold
# is that case with the owner never answering.
set -u
DIR="${H0003_DIR:?H0003_DIR unset}"
MODE="$(cat "$DIR/mode" 2>/dev/null || echo allow)"
N="$(cat "$DIR/trial" 2>/dev/null || echo 0)"

payload="$(cat)"
printf '%s\n' "$payload" > "$DIR/payload-$N.json"
date +%s.%N > "$DIR/fired-at-$N"

case "$MODE" in
  hold)
    # Never return. What the CLI does about that is the measurement.
    sleep 86400
    ;;
  deny)
    printf '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":"deny","decisionReason":"Refused by investigation 0003"}}\n'
    ;;
  *)
    printf '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":"allow","decisionReason":"Allowed by investigation 0003"}}\n'
    ;;
esac
