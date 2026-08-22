#!/bin/sh
HERE=$(cd "$(dirname "$0")" && pwd)
cd "$HERE" || exit 1
# Append one line per hook firing: the event name, the wall clock, and the raw
# payload the CLI put on stdin.
LOG=$HERE/hooks.log
printf '%s %s ' "$1" "$(date -u +%H:%M:%S.%3N)" >> "$LOG"
cat >> "$LOG"
printf '\n' >> "$LOG"
exit 0
