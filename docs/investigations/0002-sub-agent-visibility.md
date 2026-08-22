# 0002 — can Mustur see a sub-agent at all?

**Status:** answered, 2026-08-22. It can be done. The protocol, the routes and
the decision rule were committed before any of it was run — the previous commit
to this file — so the git history shows the rule preceding the finding, which is
the whole reason milestone 1 was believable.
**Verified against:** Claude Code 2.1.239 on this machine; the runs and their
captured output in `hooks.log` and `stream.jsonl`, reproduced by the commands
quoted below.
**Run during:** milestone 4c of [Plan.md](../../Plan.md#milestones), which the
owner split out of 4b on [MUS-Q-0017](../../records/questions.md#mus-q-0017)
precisely so that "this cannot be done" would be a permitted verdict.

## The question

Milestone 4c's done-when is that a session which spawns sub-agents shows them as
their own rows — which are running, for how long, and one readable without
losing the parent.

Mustur cannot do that today, and the reason is not a missing feature. **Mustur
shells out to a CLI and reads one tmux pane.** Sub-agents are that CLI's
internal business: three reviewers writing into one pane, interleaved,
unlabelled, mixed with the parent's own output. Every option for *showing* them
is really an option for *finding out about them*, and until one is established
to work, the surface has nothing to render.

So this investigation asks one thing:

> **Is there any interface — documented, on-disk, or structural — by which
> Mustur can learn that a sub-agent exists, tell its output from the parent's,
> and know when it finishes?**

## What would make an answer usable

Three properties, and a route has to carry all three or it is not a route:

1. **Enumerable.** Mustur can list the sub-agents of a session it started,
   without being told about them by the agent.
2. **Separable.** A sub-agent's output can be read on its own, not carved out of
   the parent's stream by pattern-matching prose.
3. **Terminal.** Mustur can tell when one has finished, and distinguish that
   from one that has gone quiet.

A route that is enumerable and separable but never tells us a sub-agent
finished gives a surface that accumulates rows which never leave. That is worse
than no rows, so all three are required rather than most.

## Routes to try, in order

**A. The adapter places them.** tmux can hold one window per sub-agent inside
the same session, which would make all three properties structural:
`list-windows` enumerates, `pipe-pane` reads each independently, a window
closing is terminal. It requires the CLI to let something outside it choose
where a sub-agent runs. Try first because if it works it is the best answer;
expect it to fail, because a sub-agent is a call inside one process rather than
a process the adapter starts.

**B. On-disk artefacts.** Whether the CLI writes anything per sub-agent that an
outside reader can watch — a transcript, a log, a status file — and if so
whether that is documented, stable, and complete enough for the three
properties. A path that exists but is undocumented is a finding, not a route:
it would make Mustur depend on a private interface, which
[Plan.md](../../Plan.md#scope) lists under *never*.

**C. Structured output.** Whether the CLI can be run so that sub-agent lifecycle
appears in a machine-readable stream rather than in rendered prose.

**D. Pattern-matching the pane.** Reading the parent's output and inferring
sub-agents from what they print. Listed to be ruled out rather than tried: the
owner declined an inferred `waiting-for-input` on
[MUS-Q-0005](../../records/questions.md#mus-q-0005) for the same reason a wrong
sub-agent status would be worse than none — it reads as a fact once it is on a
screen.

## Decision rule, fixed before looking

- **Route accepted** only if it is enumerable, separable *and* terminal, and
  rests on an interface the CLI documents or guarantees. Anything resting on an
  undocumented path is recorded as a finding and **not** adopted, because
  depending on a private interface is a standing non-goal.
- **Verdict: cannot be done** if no route carries all three. That is a real
  outcome and it closes 4c — the milestone's done-when becomes unreachable, the
  drawing in
  [plan-6009f123020a4f58](https://plan.agent-native.com/plans/plan-6009f123020a4f58)
  stays undrawn, and Plan.md records why rather than leaving it open forever.
- **Partial is not a pass.** Two of three properties is written up as two of
  three and does not become a shipped surface with a caveat.

## What is not being tested

Whether sub-agents are worth showing. The owner asked for them; this is only
whether it is possible.

## Result

**It can be done, and without changing what a Mustur session is.** Two routes
carry all three properties. One of them was not on the list above, which is the
part worth saying plainly: the list was wrong, not merely incomplete.

### A — the adapter places them: fails, as expected

Nothing lets a process outside the CLI choose where a sub-agent runs. A
sub-agent is a tool call inside the one process, not a process the adapter
starts, so there is no window for tmux to hold and `list-windows` has nothing to
enumerate. `--tmux` exists but creates a session for a *worktree*, not for a
sub-agent. Ruled out on the mechanism rather than on a missing flag.

### E — lifecycle hooks: accepted

Not among the four routes registered. It is the one that fits, because it leaves
the pane exactly as milestones 4a and 4b built it — still a terminal, still
`send-keys`, still `pipe-pane`.

`SubagentStart` and `SubagentStop` are documented hooks. Each carries `agent_id`
and `agent_type`; `SubagentStop` also carries `last_assistant_message` and
`agent_transcript_path`. Two sub-agents launched at once produced four firings,
correctly paired by `agent_id`:

```
SubagentStart 11:07:31.430 agent=acfaa19207f078720
SubagentStart 11:07:32.551 agent=a30e534c10425573e
SubagentStop  11:07:32.707 agent=acfaa19207f078720  last_assistant_message: "ALPHA"
SubagentStop  11:07:35.423 agent=a30e534c10425573e  last_assistant_message: "BETA"
```

That is enumerable and terminal. Separable came from a tool-use hook, which
turned out to carry `agent_id` **when the tool call happens inside a sub-agent**
and to omit it in the main conversation:

```
PreToolUse 11:07:31.103 agent=-                  Agent  {"description":"Run echo ALPHA", …}
PreToolUse 11:07:34.588 agent=a30e534c10425573e  Bash   {"command":"echo BETA"}
```

So a sub-agent's activity is attributed by an identifier the CLI supplies, never
by pattern-matching prose — which is what route D was ruled out for. The
parent's own `Agent` call carries the `description`, so a row can say what the
sub-agent was asked to do rather than only that one exists.

The hooks do not have to be installed in the owner's configuration. Mustur
starts the session, so it can pass them as a `--settings` JSON string on that
one command line; the same four firings were reproduced that way, with no
settings file in the checkout and nothing written to `~/.claude`.

**What it does not give: full output while a sub-agent is still running.** Hooks
give the identifier, the type, the description, each tool as it is called, and
the final message when it ends. Live prose lives in the per-agent transcript,
whose path the CLI hands over at `SubagentStop` and not before.

### C — structured output: carries all three, and costs the pane

`--output-format stream-json` with `--forward-subagent-text` is documented and
does everything. Every sub-agent message carries `parent_tool_use_id` — the
documentation states it explicitly, at every nesting depth, so the full tree is
reconstructible — and the stream carries `task_started`, `task_progress`,
`task_updated` (`{"status":"completed","end_time":…}`) and `task_notification`
with a summary and a duration. Verified end to end, including a session held
open with `--input-format stream-json` that accepted a second message after the
first turn finished, so this is a real session and not a one-shot.

It is rejected for one reason: it requires `-p`, and `-p` is not a terminal. A
Mustur session would stop being something the owner can `tmux attach` to and
become a JSON harness Mustur renders. That is a different product, not a way of
showing sub-agents in this one. Recorded rather than adopted so the option is
findable if the pane is ever given up for other reasons.

### B — on-disk artefacts: a finding, not a route

The layout is real: each session gets a `subagents/` directory holding one
`agent-<id>.jsonl` and one `agent-<id>.meta.json` per sub-agent, the meta
carrying `agentType`, `description`, `toolUseId` and `spawnDepth`, the jsonl
carrying every message with timestamps. It would satisfy all three properties by
itself.

The decision rule set before looking says an undocumented path is a finding and
not adopted, and that is how it is recorded. The rule earns its keep here — the
directory was tempting and reading it would have worked today. Note the one
seam: `SubagentStop` hands `agent_transcript_path` over directly, so reading
*that* path when the CLI supplies it is not the same act as deriving it.

### D — pattern-matching the pane: not tried

Ruled out in advance and left ruled out. Route E makes it unnecessary.

### Verdict against the rule

Enumerable, separable and terminal, on a documented interface: **route E**. The
milestone's done-when is reachable. What remains open is how much of a running
sub-agent the surface shows, which is a decision for the owner and not a finding
of this investigation.
