#!/bin/sh
HERE=$(cd "$(dirname "$0")" && pwd)
cd "$HERE" || exit 1
# The prompt below is deliberately single-quoted: it is text handed to an agent,
# not shell. The backticks are part of what the agent reads.
# shellcheck disable=SC2016
timeout 300 claude -p \
  'Launch exactly one general-purpose subagent via the Task tool whose entire job is to run the shell command `echo SUBAGENT-RAN` and report its output. Wait for it, then reply with just that output. Do nothing else.' \
  --model sonnet \
  --output-format stream-json \
  --verbose \
  --forward-subagent-text \
  --allowedTools "Task,Bash(echo:*)" \
  > stream.jsonl 2> err.txt
echo "exit=$?"
