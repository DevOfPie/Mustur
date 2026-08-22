#!/bin/sh
# Two user messages down one long-lived stdin, second sent only after the first
# turn's result arrives. If the process accepts it, a stream-json session is a
# writable channel and not just a one-shot.
HERE=$(cd "$(dirname "$0")" && pwd)
cd "$HERE" || exit 1
rm -f bidi.jsonl bidi-err.txt
python3 - <<'PY'
import json, subprocess, sys, time

p = subprocess.Popen(
    ["claude", "-p",
     "--model", "sonnet",
     "--input-format", "stream-json",
     "--output-format", "stream-json",
     "--verbose",
     "--forward-subagent-text",
     "--allowedTools", "Task,Bash(echo:*)"],
    stdin=subprocess.PIPE, stdout=subprocess.PIPE,
    stderr=open("bidi-err.txt", "wb"), text=True, bufsize=1)

def send(text):
    p.stdin.write(json.dumps({
        "type": "user",
        "message": {"role": "user", "content": [{"type": "text", "text": text}]},
    }) + "\n")
    p.stdin.flush()

send("Reply with exactly: FIRST")
out = open("bidi.jsonl", "w")
turns = 0
deadline = time.time() + 240
while time.time() < deadline:
    line = p.stdout.readline()
    if not line:
        break
    out.write(line)
    out.flush()
    d = json.loads(line)
    if d.get("type") == "result":
        turns += 1
        if turns == 1:
            send("Now launch one general-purpose subagent via the Task tool to run `echo SECOND-SUBAGENT` and report its output.")
        else:
            break
p.stdin.close()
try:
    p.wait(timeout=20)
except Exception:
    p.kill()
out.close()
print("turns completed:", turns)
PY
