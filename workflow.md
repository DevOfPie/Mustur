# Workflow

Operating rules for whoever is working this repo, human or model.

**Style is deliberate.** Terse, trigger-first, no rationale. This file is read on
every task, so it is optimized for scanning and low token cost. Rationale belongs
in [decisions.md](decisions.md).

**Precedence.** [Plan.md](Plan.md) is the scope contract and wins on *what*. This
file wins on *process*. If they conflict, the conflict is a bug — report it, do
not pick.

**Provisional.** The general rules here are expected to move to
[StrucGu](https://github.com/DevOfPie/StrucGu) and be replaced by references.
Keep them liftable.

**Every rule below is executable by hand today.** That was free while nothing
was built; since milestone 2 it is a constraint, and `make check` is where it is
kept — every gate runs offline against the working tree.

---

## Gates

```
before anything      → milestone 1 has run and passed
before any estimate  → say what it is based on; estimates here run long
before touching a
  file outside this
  repository         → DON'T. Onboarding is a milestone with its own verdict.
before a milestone
  is accepted        → the reviewers below have read it, and every finding is
                       dispositioned in the open
```

## After a milestone

**A milestone is not done until agents that did not build it have read it.**
Lifted from [LinkCtrl's phase loop](https://github.com/DevOfPie/LinkCtrl/blob/main/docs/build-notes/phase-loop.md),
which spawns one reviewer per milestone for a failure the builder structurally
cannot see. Owner-set 2026-08-20.

```
built, gates green  → spawn the reviewers, synchronously. Acceptance waits.
reviewers report    → post the findings to the pull request, before any fix
fixes committed     → a second comment saying what changed and what did not
```

### Who is spawned

Three, fresh, in parallel, each independent of the others. One reviewer would
weigh the three lenses against each other inside one context; three cannot.

| Reviewer | Asks |
| --- | --- |
| **Done-when** | Does the tree satisfy every clause of this milestone's row in [Plan.md](Plan.md#milestones), read against the code rather than against anybody's account of it |
| **Shipped claims** | Does anything in this diff make an already-true claim false — in Plan.md, README.md, CLAUDE.md, an entry in [decisions.md](decisions.md), [strucgu.yaml](strucgu.yaml), or a past investigation's recorded result |
| **Contract** | Do the gates in this file hold over the diff: present tense, links, one topic per commit, other repositories untouched, no unmeasured number, decisions.md appended and never edited, and every decision raised as a prompt rather than as prose |

The second is the one that earns the step. Both the builder and whoever accepts
the work are looking at the milestone being built; the defect neither is looking
at is the change that quietly falsifies something already shipped.

### What they are given

The milestone number, the branch, and this file. **Nothing else** — a reviewer
handed the builder's report reviews the report.

### What they return

```
always  → data, with file:line evidence for every finding
never   → preamble, restatement of the task, closing summary
nothing found → say so in those words
```

Silence and *did not look* are the same string, so a reviewer that found nothing
says it.

**A reviewer changes nothing.** No code, no tests, no records, no queue line. It
reports; whoever is running the milestone acts.

### What a finding is

A finding is not a rejection. Each one is dispositioned in the open, in the pull
request, as one of:

| Disposition | Means |
| --- | --- |
| Fixed | In this milestone, in a commit that names the finding |
| Deferred | A line in [queue.md](queue.md), with the reason it is not being fixed now |
| Disputed | Argued down in the pull request, with the evidence that refutes it |

A finding that falsifies a **shipped** milestone's claim is not any of those. It
is a reopening, reopening is scheduling, and scheduling is the owner's — so it
reaches them as a prompt.

## Triggers

### A decision needs the owner

```
always  → a prompt (AskUserQuestion)
never   → prose, a report, a PR body, a heading called "open questions"
```

A well-formed decision written out in prose, with options and costs and a
recommendation, is still the failure. Completeness is not what makes a request
findable; arriving on the prompt surface is.

### A PR needs the owner's eyes

```
→ take it out of draft
```

Draft means still working. Ready means input is wanted.

### An idea occurs while working something else

```
in scope     → work it now
out of scope → append one line to queue.md, continue what you were on
```

### A claim is about to be written

```
measured   → state the measurement and where it came from
believed   → label it an assertion
neither    → do not write it
```

No numbers without a measurement. No capability described in the present tense
that does not exist.

### An estimate is about to be given

```
anchor on   → what comparable work actually took
never       → decompose-and-pad
always      → state the basis, and that estimates here have run long
```

The owner's calibration, 2026-08-19: agent-produced estimates are significantly
longer than the work takes. An estimate is an upper bound, not a plan, and must
never be the reason scope is cut.

## Before committing

| Gate | Condition |
| --- | --- |
| Present tense | No unbuilt capability described as existing |
| Links | Every relative link and anchor in tracked `.md` resolves |
| Scope | One topic per commit |
| Other repositories | Untouched |

Commit messages are long prose explaining *why*. The diff shows what.

## Standing rules

**Nothing is deleted.** Superseded decisions stay, with a pointer.

**[decisions.md](decisions.md) is append-only.** Never edit an entry. A later
entry corrects an earlier one, and the earlier text stays.

**Claims must be verifiable by a reader who does not trust you.**

**Stop and ask** for anything the owner would reasonably want to decide — in a
prompt.

## Dispatch from outside

The global `/work` registry routes `mustur` here. Declared kinds:

```
milestone  → work the lowest milestone in Plan.md not yet passed, under this file
```

Any other kind is unknown: report it, carrying the table above.

## Quick reference

```
Plan.md              scope contract: vision, scope table, milestones, limits
decisions.md         append-only: why any of this is the way it is
queue.md             append a line; out-of-scope ideas go here
docs/ui-surfaces.md  the surfaces needing design, and what each must do
strucgu.yaml         which files play which StrucGu roles
```
