#!/bin/sh
HERE=$(cd "$(dirname "$0")" && pwd)
cd "$HERE" || exit 1
R=$HERE/record.sh
rm -f hooks.log
timeout 300 claude -p \
  'Launch one general-purpose subagent via the Task tool to run `echo POSTHOOK` and report its output. Then reply with just that output.' \
  --model sonnet \
  --settings "{\"hooks\":{\"SubagentStart\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"$R SubagentStart\"}]}],\"SubagentStop\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"$R SubagentStop\"}]}],\"PreToolUse\":[{\"matcher\":\"*\",\"hooks\":[{\"type\":\"command\",\"command\":\"$R PreToolUse\"}]}],\"PostToolUse\":[{\"matcher\":\"*\",\"hooks\":[{\"type\":\"command\",\"command\":\"$R PostToolUse\"}]}]}}" \
  --allowedTools "Task,Bash(echo:*)" \
  > post-out.txt 2> post-err.txt
echo "exit=$?"
python3 - <<'PY'
import json
for line in open("$HERE/hooks.log"):
    line=line.strip()
    if not line: continue
    ev,ts,p=line.split(" ",2)
    d=json.loads(p)
    print("%-14s agent=%-18s %s" % (ev, d.get("agent_id","-"), d.get("tool_name","")))
PY
