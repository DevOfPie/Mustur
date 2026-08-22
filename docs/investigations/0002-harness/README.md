# 0002 harness

Everything [investigation 0002](../0002-sub-agent-visibility.md) claims, and the
means to disbelieve it. A review found the investigation naming captured files
that were not in the tree and promising commands it never quoted, so this
directory exists to make the finding checkable by somebody who does not trust
the author.

Machine paths in the captures are replaced with `/home/owner/…`. Nothing else is
edited: the payloads are as the CLI emitted them, one per line, prefixed with
the hook event and the wall clock the recorder stamped.

## What is here

| File | Is |
| --- | --- |
| `record.sh` | The hook. Appends the event name, the time and the raw payload to `hooks.log`, and always exits 0 |
| `order.sh` | Launches three sub-agents of one type with distinct descriptions, so pairing can be scored |
| `score.py` | Scores order-pairing against ground truth over `captured/hooks-run*.log` |
| `count.py` | Counts events per run, which is how the missing tool calls were shown to be the model's choice rather than lost hooks |
| `stream.sh` | The structured-output route: one sub-agent under `--output-format stream-json --forward-subagent-text` |
| `bidi.sh` | The same, held open on `--input-format stream-json`, sending a second message after the first turn's result |
| `captured/` | The output of all of the above, from the runs the investigation cites |

## Reproducing

The scorer runs against what is committed and needs no CLI and no network:

```
python3 docs/investigations/0002-harness/score.py
python3 docs/investigations/0002-harness/count.py
```

`score.py` prints the pairing result the investigation quotes — six agents with
ground truth across three runs, six paired correctly, none wrongly. `count.py`
prints three `PreToolUse/Agent`, three `SubagentStart` and three `SubagentStop`
per run against two `PreToolUse/Bash`, which is what establishes that the
missing tool call is a sub-agent answering without running the command rather
than a hook that failed to fire.

Running the captures again needs Claude Code on `PATH` and costs tokens:

```
sh docs/investigations/0002-harness/order.sh 1
sh docs/investigations/0002-harness/stream.sh
sh docs/investigations/0002-harness/bidi.sh
```

`order.sh` writes `hooks.log` and copies it to `hooks-run<N>.log` beside itself,
not into `captured/`; move it there deliberately rather than overwriting the
evidence a run happens to disagree with.

## What the numbers are, and are not

Six agents is small for a mechanism whose failure is a wrong label. It is stated
as six rather than dressed up, the pairing it supports is bounded rather than
trusted (`MUS-Q-0026`), and a row with nothing to pair carries no task at all.
