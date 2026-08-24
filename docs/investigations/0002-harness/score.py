import os, json, re, sys, glob

for path in sorted(glob.glob(os.path.join(os.path.dirname(os.path.abspath(__file__)), "captured", "hooks-run*.log"))):
    events = []
    for line in open(path):
        line = line.strip()
        if not line:
            continue
        ev, ts, payload = line.split(" ", 2)
        events.append((ev, ts, json.loads(payload)))
    # Keep file order, which is the order the hook processes appended.
    pending = []          # descriptions from the parent's Agent calls, in order
    order_pair = {}       # agent_id -> description, paired FIFO
    truth = {}            # agent_id -> word, from the sub-agent's own Bash call
    for ev, ts, d in events:
        if ev == "PreToolUse" and d.get("tool_name") == "Agent" and not d.get("agent_id"):
            pending.append((d.get("tool_input") or {}).get("description", "?"))
        elif ev == "SubagentStart":
            order_pair[d["agent_id"]] = pending.pop(0) if pending else None
        elif ev == "PreToolUse" and d.get("agent_id") and d.get("tool_name") == "Bash":
            cmd = (d.get("tool_input") or {}).get("command", "")
            m = re.search(r"echo\s+(\w+)", cmd)
            if m:
                truth[d["agent_id"]] = m.group(1)

    right = wrong = unknown = 0
    for aid, word in truth.items():
        desc = order_pair.get(aid)
        if desc is None:
            unknown += 1
        elif word.lower() in desc.lower():
            right += 1
        else:
            wrong += 1
    print("%s  paired-correctly=%d  paired-wrongly=%d  unpaired=%d  (of %d agents with ground truth)"
          % (path.rsplit("/", 1)[1], right, wrong, unknown, len(truth)))
    for aid, word in truth.items():
        print("    %s  ran echo %-6s order-pairing said %r" % (aid, word, order_pair.get(aid)))
