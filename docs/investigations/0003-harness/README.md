# 0003 harness — what ran, with which arguments

Written on 2026-09-10, after a reviewer found that the trials this investigation
records cannot all be re-run with the scripts beside them. That is the defect,
not the note: `trial.sh` changed shape between the first trials and the last, and
nothing said so.

**The scripts are the current shape.** Trials before 4 were run by an earlier
`trial.sh` that installed one event, hardcoded to `PermissionRequest`, and ran
the CLI with `--model haiku`. Today's takes the event as its fifth argument,
defaults it to `PreToolUse`, accepts several joined by `+`, and runs `--model
opus`. `hook.sh` also renamed the `PermissionRequest` deny key from
`decisionReason` to `permissionDecisionReason`, which is the retry MUS-F-0120
describes rather than the first attempt.

So a re-run of the early trials is not a re-run of what happened. Reproducing
them means passing the event by hand and changing the model back:

| Trials | What they measured | How to re-run them today |
| --- | --- | --- |
| 0–3 | `PermissionRequest` fires, its decision is ignored | `./trial.sh N allow 600 25 PermissionRequest`, with `--model haiku` restored in `trial.sh:47` |
| 11–15 | The five consecutive trials the rule asks for | `./run.sh 11 15 allow` |
| 21–23 | What happens when nobody answers | `./hold.sh 20 21 23` |
| 26 | The confirmation after the CLI updated itself | `./trial.sh 26 allow 600 25 PreToolUse` |
| 30–35 | Which event means a dialog is pending | `./signal.sh 30 3` |
| 40–42 | The timeout, re-measured at 2.1.267 | `./hold.sh 20 40 42` |
| 50–54 | Whether the permission flow runs while `PreToolUse` holds | `./order.sh 20 50 54` |

Every one of them needs `H0003_DIR` set to a scratch directory, and every one
starts a real CLI in a real tmux pane on the `investigation0003` socket.

**The captured panes are all 2.1.266**, including `pane-allow.txt`, whose working
directory is trial 11 — which the write-up attributes to 2.1.263. Both cannot be
right and this harness cannot say which is: see
[MUS-F-0123](../../../records/findings.md#mus-f-0123).
