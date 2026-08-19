# 0001 — a mandated tool call is honoured

**Status:** protocol registered, 2026-08-19. Findings pending; this commit fixes
the protocol and the decision rule before the scored runs execute, so the git
history shows the rule preceding the result.
**Verified against:** Claude Code 2.1.235 headless (`claude -p`); models
`claude-sonnet-5` and `claude-opus-5` as resolved by the CLI aliases `sonnet`
and `opus`; Node v24.19.0; StrucGu `investigations` module 0.2.1; the stub
server, runner and scorer in [0001-harness/](0001-harness/) at this commit.
**Run during:** milestone 1 of [Plan.md](../../Plan.md#milestones).

## Context

Every module in the plan depends on one bet:
[inject, never offer](../../decisions.md#inject-never-offer). Offered stores are
measurably not called — 0 memory operations in 114 turns against a pre-seeded
store, per the measurement recorded in the idea file — but a *mandated* call
from a repo-local context file has never been measured. Milestone 1 exists to
measure it before anything is built on it.

A first disproof design (in the planning session's evidence directory) was
withdrawn: its own recorded confound would have produced a false pass. The
lesson applied here is that the test must be able to fail for the real reason,
and only for the real reason:

- **A session whose MCP server never loaded must not score as disobedience.**
  Project-scoped `.mcp.json` servers can sit unapproved in a folder the CLI has
  never trusted, in which case the tool is simply absent. Every run therefore
  records the session's init event, and a run whose init does not list server
  `mustur` as connected is invalid rather than a failure.
- **The transcript's claims are not evidence.** A model can say it called a
  tool. Only the stub server's own log — one JSON line per protocol event,
  tagged with the run id the harness sets — counts.
- **The mandate must not leak into the prompt.** Task prompts never mention the
  tool, the server or Mustur. The only pressure is the clause in the throwaway
  repository's `CLAUDE.md`, which is the mechanism the injection kit would ship.

## Protocol

Two throwaway repositories (seed trees in
[0001-harness/repos/](0001-harness/repos/)): a small Python utility and a notes
corpus, so the result does not hinge on one repository character. Each carries:

- the mandate clause, verbatim in `CLAUDE.md`:

  > **Before any other action in every session, call the `mustur_route` tool
  > (server `mustur`) with the repository name and one line on the task.** Do
  > not start work until the call has returned; its result says where this
  > repository's records and routing live.

- a `.mcp.json` written by `claude mcp add --scope project`, pointing at
  [the stub server](0001-harness/mustur-route-stub.js) — one tool,
  `mustur_route`, which logs every protocol event and returns a canned routing
  record.

Twenty scored runs: 5 tasks × 2 repositories × 2 models (`sonnet`, `opus`),
interleaved, sequential. Before each run the tree is reset to its `seed` tag, so
every session sees the identical repository. Each run is a fresh single-turn
headless session with allowed tools
`mcp__mustur__mustur_route,Read,Grep,Glob,Edit,Write`; tasks are a mix of
editing and read-only questions, listed in [run-wave.sh](0001-harness/run-wave.sh).

Environment, stated because it shapes what is measured: sessions run on the
whippy VM with the account's global configuration loaded — user-level
`CLAUDE.md`, hooks, and a user-scoped MCP server — which is the environment
every real session on this machine has. The runs measure the mandate inside
that noise, not in a cleanroom.

Two pilot runs (one per repository, one per model) validated the harness end to
end before this registration; both were honoured. Pilots are excluded from the
twenty.

## Decisions

1. **Decision rule, fixed in advance** (restating
   [Plan.md milestone 1](../../Plan.md#milestones)): pass = the mandate is
   honoured in at least **18 of 20** valid runs. Fewer means the injection
   design is killed, not shrunk.
2. **Honoured** means: the stub server's log contains a `tools/call` for
   `mustur_route` tagged with that run's id. This is the gating metric, and the
   only one.
3. **Order compliance** — no work tool (`Read`, `Grep`, `Glob`, `Edit`,
   `Write`, `Bash`, `Task`) used before the first `mustur_route` call, with
   `ToolSearch`/`Skill` excluded as harness mechanics — is reported alongside,
   and does not gate. The clause's letter says first; the bet being tested is
   that the call reliably happens at all.
4. **Invalid runs** — init event without a connected `mustur` server, or a
   harness failure before the model produced any turn — are excluded and
   re-run once, with the count reported. If more than 4 of the 20 slots come up
   invalid, the wave is void and the harness itself is the finding.

## Findings

The scored wave had not run when this protocol was committed. Findings land in
the follow-up commit, whichever way they fall.

## Reproducing

Everything needed is in [0001-harness/](0001-harness/): the stub server, the
seed trees, [run-wave.sh](0001-harness/run-wave.sh) and
[score.py](0001-harness/score.py); [its README](0001-harness/README.md) has the
commands. Raw session transcripts and the stub's call log stay outside this
repository, at `~/warehouse-evidence/2026-08-19-mustur-milestone-1/` on the
whippy VM — this repository is public and full session streams do not belong in
it.
