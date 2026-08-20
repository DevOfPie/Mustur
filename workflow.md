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
```

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
