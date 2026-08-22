# 0002 — can Mustur see a sub-agent at all?

**Status:** protocol registered, 2026-08-22. This file is committed with the
question, the routes to be tried and the decision rule, and **no result** — the
git history is what shows the rule preceded the finding, which is the whole
reason milestone 1 was believable.
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

Not run yet. This section is deliberately empty at the commit that registers the
protocol.
