#!/bin/sh
# Three sub-agents of the same type, launched together, each told to echo a
# different word. The parent's Agent call carries the description; SubagentStart
# carries the agent_id; the sub-agent's own Bash call carries both the agent_id
# and the word. So the Bash call is ground truth for which agent is which, and
# pairing description to agent_id by spawn order can be scored against it.
HERE=$(cd "$(dirname "$0")" && pwd)
cd "$HERE" || exit 1
R="$HERE"/record.sh
N=$1
rm -f hooks.log
# The prompt below is deliberately single-quoted: it is text handed to an agent,
# not shell. The backticks are part of what the agent reads.
# shellcheck disable=SC2016
timeout 400 claude -p \
  'Launch three general-purpose subagents via the Task tool in one go. Give them the descriptions "Task ONE", "Task TWO" and "Task THREE" respectively, and have them run `echo ONE`, `echo TWO` and `echo THREE` respectively. Wait for all three, then reply with the three outputs. Do nothing else.' \
  --model sonnet \
  --settings "{\"hooks\":{\"SubagentStart\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"$R SubagentStart\"}]}],\"SubagentStop\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"$R SubagentStop\"}]}],\"PreToolUse\":[{\"matcher\":\"*\",\"hooks\":[{\"type\":\"command\",\"command\":\"$R PreToolUse\"}]}]}}" \
  --allowedTools "Task,Bash(echo:*)" \
  > order-out.txt 2> order-err.txt
cp hooks.log "hooks-run$N.log"
echo "run $N exit=$?"
