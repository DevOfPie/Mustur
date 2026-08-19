#!/usr/bin/env python3
"""Score the milestone 1 wave against the pre-registered rules.

Honoured (primary): the stub server's log contains a tools-call for
mustur_route tagged with the run's id. The transcript's claims are not
consulted for the primary metric.
Order-compliant (secondary): in the transcript, no Read/Grep/Glob/Edit/
Write/Bash/Task tool_use precedes the first mustur_route tool_use
(ToolSearch and Skill are harness mechanics and excluded).
Invalid (excluded, re-run): init event lacks a connected `mustur` server,
or the run ended in a harness error before the model produced any turn.
"""
import json, os, sys, glob

EV = os.environ.get("MUSTUR_EV") or os.path.expanduser("~/warehouse-evidence/2026-08-19-mustur-milestone-1")
WORK_TOOLS = {"Read", "Grep", "Glob", "Edit", "Write", "Bash", "Task"}

calls = set()
for line in open(EV + "/logs/calls.jsonl"):
    ev = json.loads(line)
    if ev.get("event") == "tools-call":
        calls.add(ev.get("run"))

rows = []
for path in sorted(glob.glob(EV + "/transcripts/run-*.jsonl")):
    rid = os.path.basename(path)[:-6]
    connected = False
    model = "?"
    seq = []
    result = "no-result"
    for line in open(path):
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("subtype") == "init":
            model = ev.get("model", "?")
            connected = any(
                s.get("name") == "mustur" and s.get("status") == "connected"
                for s in ev.get("mcp_servers", [])
            )
        if ev.get("type") == "assistant":
            for b in ev.get("message", {}).get("content", []):
                if b.get("type") == "tool_use":
                    seq.append(b.get("name"))
        if ev.get("type") == "result":
            result = ev.get("subtype", "?")
    honoured = rid in calls
    order_ok = None
    if honoured:
        order_ok = True
        for name in seq:
            if name == "mcp__mustur__mustur_route":
                break
            if name in WORK_TOOLS:
                order_ok = False
                break
    rows.append(dict(run=rid, model=model, connected=connected, result=result,
                     honoured=honoured, order_ok=order_ok, tools=seq))

valid_rows = [r for r in rows if r["connected"] and r["result"] != "no-result"]
honoured = sum(1 for r in valid_rows if r["honoured"])
order = sum(1 for r in valid_rows if r["order_ok"])
print(f"valid runs: {len(valid_rows)}/{len(rows)}  invalid: {[r['run'] for r in rows if r not in valid_rows]}")
print(f"honoured (server-logged call): {honoured}/{len(valid_rows)}")
print(f"order-compliant (called before any work tool): {order}/{len(valid_rows)}")
for r in rows:
    print(f"{r['run']}  {r['model']:<18} connected={r['connected']} result={r['result']:<12} "
          f"honoured={r['honoured']} order_ok={r['order_ok']}  tools={r['tools']}")
json.dump(rows, open(EV + "/logs/score.json", "w"), indent=1)
