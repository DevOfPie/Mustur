# 0003 — can a dialog be answered without the screen, and does the terminal survive it?

**Status:** open, pre-registered 2026-09-08. **Nothing has been run.** This file
is committed before any evidence is looked at, so the git history shows the rule
preceding the finding — the same reason
[0001](0001-mandated-tool-call.md) was believable, and the same protocol
[0002](0002-sub-agent-visibility.md) used.
**Verified against:** to be filled in by the run. Claude Code 2.1.263 is what is
installed; the version actually measured goes here, because
[MUS-F-0084](../../records/findings.md#mus-f-0084) was measured at 2.1.260 and
this file must not inherit its numbers.
**Run during:** the session-channel work proposed as milestone 8. **That
milestone is deliberately not in [Plan.md](../../Plan.md#milestones) yet**, and
will not be until this returns — writing the row first is how *cannot be done*
stops being a permitted verdict.

## The question

[MUS-F-0113](../../records/findings.md#mus-f-0113) says the CLI publishes a
structured channel that works in an ordinary interactive terminal: thirty-two
lifecycle hooks, of which `PermissionRequest` fires when a permission dialog is
about to be drawn and takes back `allow`, `deny` or `ask`. That finding is
**read, not measured**. It rests on documentation and on eight event names
appearing as exact strings in the shipped binary, which is evidence that the
events exist and no evidence at all about what returning a decision does to the
pane.

Everything the proposed milestone gains over
[MUS-D-0143](../../records/decisions.md#mus-d-0143) turns on that one behaviour.
So:

> **Can Mustur learn that a dialog is pending, answer it, and have the CLI
> proceed — without reading the pane, and without the session ceasing to be a
> tmux terminal a person can attach to?**

*Cannot be done* is a permitted verdict, and it is not a neutral one: it
re-ratifies MUS-D-0143, shrinks the milestone to its interface work, and the
pane parser keeps growing a shape at a time.

## What would make an answer usable

Three properties. A route carries all three or it is not a route.

1. **Interceptable.** Mustur learns a dialog is pending, and what it is about,
   from the channel — not from a capture. The payload has to name the tool and
   carry its input, because a surface that can say only *something is waiting*
   sends the owner back to the screen, which is where they already were.
2. **Answerable.** Mustur's answer resolves the dialog and the CLI proceeds as
   if a person had pressed. Both directions count: an `allow` that lets the tool
   run, and a `deny` that stops it and tells the agent why.
3. **Terminal-preserving.** The session is still a tmux pane. Measured by
   attaching a second client to the same session after the exchange and typing
   into it, because "the process is alive" is not the property —
   [MUS-D-0016](../../records/decisions.md#mus-d-0016)'s premise is that a
   person can take the session over from a machine.

**Measured but not required: what happens when nobody answers.** A hook that
never returns is the realistic failure — the owner is asleep, the phone is in a
pocket. If the CLI waits and then draws its own dialog, the fallback is the
parser that already exists and the feature degrades into today. If it denies, or
kills the turn, or hangs the session, that is a different feature with a
different price. This is not pass/fail. It is the number that decides whether
the thing is shippable.

## The rule, fixed here

**A route passes only if all three properties hold on five consecutive trials
of the same shape, with no trial requiring a pane read.**

One failure in five fails the route. Not because four in five is a bad number,
but because a dialog is a gate on the owner's authority: one that silently does
nothing on the fifth press is worse than one that never worked, and it is the
kind of defect this repository keeps finding after shipping
([MUS-F-0088](../../records/findings.md#mus-f-0088),
[MUS-F-0091](../../records/findings.md#mus-f-0091),
[MUS-F-0104](../../records/findings.md#mus-f-0104) are all *the dialog was there
and could not be used*).

Property 3 is checked **after every trial**, not once at the end. A route that
preserves the terminal for four exchanges and loses it on the fifth has not
preserved it.

The timeout measurement is **three trials**, separately, with a hook that never
returns. What is recorded is what the CLI drew, and after how long, to the
second.

### What would falsify each property

| Property | Falsified by |
| --- | --- |
| Interceptable | The hook does not fire for a permission dialog; or fires with a payload that does not identify the tool and its input |
| Answerable | The pane still draws the dialog after a decision is returned; or the decision is ignored and the CLI's own default applies; or `deny` and `allow` are not distinguishable in what the agent does next |
| Terminal-preserving | `tmux attach` fails, or the attached client cannot type into the session, at any point after the exchange |

## Routes, in order

**A. The CLI's lifecycle hooks, on an interactive session.** Injected the way
Mustur already injects `SubagentStart` and `SubagentStop` — a `--settings` JSON
string on the command line `Start` builds
([MUS-D-0087](../../records/decisions.md#mus-d-0087)), so a pass costs no new
mechanism and nothing of the owner's configuration is touched. Tried first
because if it holds it is the whole answer: structured dialogs *and* the pane.

**B. `--print` with `stream-json` both ways.** Not re-run.
[MUS-D-0088](../../records/decisions.md#mus-d-0088) measured it on 2026-08-22
and MUS-F-0084 measured it again on 2026-09-04: it carries properties 1 and 2
and fails property 3 by construction, because `--print` is not a terminal.
Measuring a third time is ceremony, not evidence. **One fact is re-checked**,
because it is the only one that could have moved: whether the streaming flags
still require `--print` at the version under test. If they no longer do, this
route is re-opened and the investigation is rewritten around it.

**C. The background daemon's control socket.** `~/.claude/daemon/roster.json`
names a `rendezvousSock` and a `ptySock` per worker and there is a
`control.key` beside them. Undocumented internals. Tried **only if A fails**,
and expected to be rejected even if it works — a private socket that a patch
release can rename is not something to put an owner's authority through. If it
is tried, the verdict records why it was rejected rather than whether it
functioned, so nobody re-derives it.

## Harness

Everything the run produces lives in `0003-harness/` beside this file: the
settings JSON, the hook script, the driver that starts the session and sends the
prompt, the captured payloads, and the transcript of each trial. The standard
this repository already holds itself to is that a reader who does not trust the
result can reproduce it — 0002's harness scores its own pairing result without a
CLI or a network, and this one carries the captured payloads for the same
reason.

## Verdict

To be written after the run, in this file, in a separate commit.
