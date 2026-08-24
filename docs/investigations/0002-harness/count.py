import os, json, glob

for f in sorted(glob.glob(os.path.join(os.path.dirname(os.path.abspath(__file__)), "captured", "hooks-run*.log"))):
    c, bad = {}, 0
    for line in open(f):
        line = line.strip()
        if not line:
            continue
        try:
            ev, ts, p = line.split(" ", 2)
            d = json.loads(p)
        except Exception:
            bad += 1
            continue
        k = ev + ("/" + str(d.get("tool_name", "")) if ev == "PreToolUse" else "")
        c[k] = c.get(k, 0) + 1
    print(f.rsplit("/", 1)[1], c, "unparsable:", bad)
