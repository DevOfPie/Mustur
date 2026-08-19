# Harness for investigation 0001

Protocol, decision rule and result: [../0001-mandated-tool-call.md](../0001-mandated-tool-call.md).

To rebuild the throwaway repositories from the seed trees here, in a directory
outside any real project:

```bash
EV=$HOME/warehouse-evidence/2026-08-19-mustur-milestone-1   # or anywhere
mkdir -p $EV/{repos,logs,transcripts,stub}
cp mustur-route-stub.js $EV/stub/
for r in throwaway-a throwaway-b; do
  cp -r repos/$r $EV/repos/$r
  git -C $EV/repos/$r init -b main
  git -C $EV/repos/$r add -A
  git -C $EV/repos/$r commit -m "seed throwaway repo for milestone 1 disproof"
  (cd $EV/repos/$r && claude mcp add --scope project --transport stdio mustur \
     --env MUSTUR_LOG=$EV/logs/calls.jsonl -- node $EV/stub/mustur-route-stub.js)
  git -C $EV/repos/$r add .mcp.json
  git -C $EV/repos/$r commit -m "register the mustur stub project-scoped"
  git -C $EV/repos/$r -c tag.gpgSign=false tag seed
done
```

Then:

```bash
MUSTUR_EV=$EV bash run-wave.sh     # ~20 sequential headless sessions
MUSTUR_EV=$EV python3 score.py     # prints the table, writes logs/score.json
```

The scorer trusts only `logs/calls.jsonl` (written by the stub server) for the
gating metric; transcripts are used for server-connection validity and tool
ordering.
