#!/usr/bin/env bash
#
# strucgu.yaml is a hand-edited claim about this tree, and a claim no reader can
# check is the defect the file exists to prevent. This gate checks what a
# checker would refuse to start on, plus the one drift this repository has
# already had (module pins that name releases the catalog moved past — its
# fix is PR #2):
#
#   - the file parses, and declares schema strucgu/adoption@1
#   - every module pin is exact (x.y.z) — a checker refuses a floating pin
#   - every mapped role path exists and is tracked (~ means deliberately
#     unmapped and is fine)
#   - decision-log carries `form`, and `effective_from` — without the latter an
#     audit cannot start at all, which is worse than a finding
#
# It is a sanity gate, not the conformance audit. The audit is milestone 2b's,
# runs StrucGu's full check vocabulary, and belongs to the product.
#
# Usage: scripts/check-adoption.sh
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

python3 - <<'EOF'
import re, subprocess, sys
import yaml

fails = 0
def fail(msg):
    global fails
    print(f"  FAIL  strucgu.yaml: {msg}")
    fails += 1

try:
    doc = yaml.safe_load(open("strucgu.yaml"))
except Exception as e:
    print(f"  FAIL  strucgu.yaml does not parse: {e}")
    sys.exit(1)

if doc.get("schema") != "strucgu/adoption@1":
    fail(f"schema is {doc.get('schema')!r}, not 'strucgu/adoption@1'")

tracked = set(subprocess.run(["git", "ls-files"], capture_output=True,
                             text=True, check=True).stdout.splitlines())

modules = doc.get("modules") or {}
if not modules:
    fail("declares no modules")

checked_roles = 0
for mod, block in modules.items():
    version = str(block.get("version", ""))
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        fail(f"{mod} pin {version!r} is not an exact x.y.z version")
    if not block.get("adopted"):
        fail(f"{mod} has no adopted date")
    for role, path in (block.get("roles") or {}).items():
        if path is None:  # `~`, deliberately unmapped
            continue
        checked_roles += 1
        p = str(path)
        if p.endswith("/"):
            if not any(t.startswith(p) for t in tracked):
                fail(f"{mod}.{role} maps to {p}, which holds nothing tracked")
        elif p not in tracked:
            fail(f"{mod}.{role} maps to {p}, which is not a tracked file")

dl = modules.get("decision-log")
if dl is not None:
    if not dl.get("form"):
        fail("decision-log declares no form; the record is unparseable to a checker")
    if not dl.get("effective_from"):
        fail("decision-log has no effective_from; an audit cannot start without it")

if fails:
    print(f"  {fails} adoption-record problem(s)")
    sys.exit(1)
print(f"  ok    {len(modules)} module(s), {checked_roles} mapped role path(s) tracked")
EOF
