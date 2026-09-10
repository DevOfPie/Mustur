#!/usr/bin/env bash
# Does the permission flow run while a PreToolUse hook is still thinking?
#
# PreToolUse fires about 65ms before PermissionRequest when both return at once,
# which says nothing about whether the CLI waited. This holds PreToolUse open
# until its timeout and lets PermissionRequest through: if PermissionRequest
# still fires within that 65ms, the two run concurrently and Mustur can learn
# what the dialog is while it still holds the answer channel open. If it fires
# only after the timeout, the CLI waits, and an answer to a dialog Mustur can
# describe has to be a keypress.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
DIR="${H0003_DIR:?H0003_DIR unset}"
TIMEOUT="${1:-20}"; FIRST="${2:-50}"; LAST="${3:-52}"

printf '%-6s %-12s %-18s %-12s %s\n' trial PreToolUse PermissionRequest delta verdict
for n in $(seq "$FIRST" "$LAST"); do
  "$HERE/trial.sh" "$n" holdpre "$TIMEOUT" $((TIMEOUT + 15)) PreToolUse+PermissionRequest >/dev/null 2>&1
  pre=$(cat "$DIR/fired-at-$n-PreToolUse" 2>/dev/null || echo 0)
  perm=$(cat "$DIR/fired-at-$n-PermissionRequest" 2>/dev/null || echo 0)
  python3 -c "
pre, perm, t = $pre, $perm, $TIMEOUT
if perm == 0:
    print(f'{$n:<6} {\"yes\" if pre else \"NO\":<12} {\"never fired\":<18} {\"-\":<12} the flow does not run at all while PreToolUse holds')
else:
    d = perm - pre
    v = 'concurrent: the flow runs while PreToolUse holds' if d < t/2 else 'sequential: the CLI waited for PreToolUse'
    print(f'{$n:<6} {\"yes\":<12} {\"yes\":<18} {d:+.3f}s{\"\":<4} {v}')
"
  tmux -L investigation0003 kill-session -t "h0003-$n" 2>/dev/null
done
