# 0003 — can a dialog be answered without the screen, and does the terminal survive it?

**Status:** answered, 2026-09-08. **It can be done, and not through the event
the question named.** The rule, the properties and the routes were committed
before any of it was run — the previous commit to this file — so the git history
shows the rule preceding the finding, the same protocol
[0001](0001-mandated-tool-call.md) and
[0002](0002-sub-agent-visibility.md) used.
**Verified against:** Claude Code **2.1.263** for the five consecutive trials
and **2.1.266** for the confirmation, because the CLI updated itself in the
middle of the run. That is recorded rather than tidied away: the result holds
across the update, and it was not measured on one version and claimed for the
other. **The committed panes do not support that split** — all three print
2.1.266, including the one whose working directory is trial 11, which is one of
the five. Which version ran those trials cannot now be recovered;
[MUS-F-0123](../../records/findings.md#mus-f-0123) records it, and what the
result did not depend on is [below](#the-fallback-re-measured-on-the-version-under-test). [MUS-F-0084](../../records/findings.md#mus-f-0084) was measured at 2.1.260
and none of its numbers are inherited here.
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

**Route A passes. The dialog can be answered without the screen, and the
terminal survives it — through `PreToolUse`, not through `PermissionRequest`.**

### What the rule asked for

| | Result |
| --- | --- |
| **Interceptable** | Yes, on every one of seven firings. The payload names the tool and carries its input — `{"tool_name":"Bash","tool_input":{"command":"touch ran-the-tool","description":"Create empty file ran-the-tool"}}` — plus `session_id`, `transcript_path` and `tool_use_id`. Captured whole in [captured/payload-pretooluse.json](0003-harness/captured/payload-pretooluse.json). **This row also said the payload carried `permission_suggestions` that are literally the drawn dialog's own options, and it does not** — that field is `PermissionRequest`'s, and the cited file never held it. [Corrected below](#corrected-after-the-verdict), where what each event actually says is measured |
| **Answerable** | Yes, both directions. `allow` ran the tool and drew no dialog; `deny` stopped it and the reason reached the agent verbatim — *"the tool call came back with \"Refused by investigation 0003\" rather than running"* ([captured/pane-deny.txt](0003-harness/captured/pane-deny.txt), which is **trial 3** rather than one of the five) |
| **Terminal-preserving** | Yes, checked after every trial. A second client attached over a separate tmux socket, rendered the whole conversation and the CLI's input box, and typed into it — the typed marker appeared in the inner pane on all five |

Five consecutive trials, 11 through 15, all three properties, no trial requiring
a pane read. Trial 26 repeated it on 2.1.266 after the update. **The rule is
met.**

### What happens when nobody answers

Three trials with a hook that never returns, its timeout shortened to 20s so the
behaviour could be watched in seconds rather than at the documented 600s default.
All three ended the same way: **the CLI waits out the hook and then draws its own
dialog** — 21.3s, 24s and 20s from the hook firing.

That is the outcome that makes this shippable. The fallback for an owner who is
asleep is the pane parser that already exists, so the feature degrades into
today rather than into a hung session or a silent refusal.
[captured/pane-timeout-fallback.txt](0003-harness/captured/pane-timeout-fallback.txt),
which is **trial 24** rather than one of the three; no committed artefact carries
the three numbers themselves. The re-run at
[captured/hold-2.1.267.txt](0003-harness/captured/hold-2.1.267.txt) does, which
is the only reason they can be checked at all.

### The event the question named does not work

**`PermissionRequest` fired and its decision was ignored.** Trials 0 and 1: the
hook received a full payload
([captured/payload-permissionrequest.json](0003-harness/captured/payload-permissionrequest.json))
and returned the documented object —

```json
{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":"allow"}}
```

— and the pane drew the dialog anyway, and was still drawing it minutes later.
Retried with the exact key names the documentation quotes; same result.

`PreToolUse` with `permissionDecision` suppressed it on the first attempt and on
every attempt after.

So MUS-F-0113's central claim was right about the channel and wrong about the
event, which is the whole reason this was measured rather than built on. Whether
`PermissionRequest`'s decision object works only where a permission host exists —
`--print` and the SDK — was not established here and is not needed: the route
that works is the one that keeps the terminal.

### One dialog no hook can reach

The workspace-trust prompt is drawn **before the session exists**, so no hook is
installed when it appears. Every trial answered it by hand through `send-keys`.
It is not a defect and it is not in the rule's way — a session Mustur starts in
a directory the owner has already trusted never sees it — but a module that
assumes every dialog is interceptable would hang on the first one of a session's
life.

### What this does not say

It does not say the picker half of the parser can go. Nothing here touched the
model picker, the effort cycler or the toggles of
[MUS-F-0101](../../records/findings.md#mus-f-0101): those are drawn on the
owner's own keypress, no hook fires, and
[MUS-F-0113](../../records/findings.md#mus-f-0113) said so before this ran.

It does not say route C was tried. Route A passed, so the daemon's control
socket was not touched, which is what the ordering was for.

Route B was not re-run, as pre-registered. The one fact re-checked is unchanged:
at 2.1.266 `--output-format`, `--input-format` and `--include-partial-messages`
are all still marked *only works with --print*.

## Corrected after the verdict

**2026-09-10, at Claude Code 2.1.267.** The verdict's Interceptable row cited
the wrong payload for the wrong event, and the sentence it built on that
citation is the one a milestone would have been designed around. Both are
corrected here rather than edited away, because how the error was found is part
of what the reader needs: the file the row cites was opened while pricing the
milestone, and the field the row names is not in it.

### What each event actually says

`permission_suggestions` is `PermissionRequest`'s field.
[captured/payload-pretooluse.json](0003-harness/captured/payload-pretooluse.json)
— the file the row cites, captured in trial 11 — carries
`session_id`, `transcript_path`, `cwd`, `scratchpad_dir`, `prompt_id`,
`permission_mode`, `effort`, `hook_event_name`, `tool_name`, `tool_input` and
`tool_use_id`, and no suggestions at all.
[captured/payload-permissionrequest.json](0003-harness/captured/payload-permissionrequest.json)
is where they were seen.

That left a question the five trials could not answer, because every one of them
used a tool call the CLI prompts on: **does a `PreToolUse` firing mean a dialog
was going to be drawn?** Six more trials, both hooks installed in one session and
the hook returning nothing so the CLI's own permission flow decided:

| Case | Prompt | `PreToolUse` | `PermissionRequest` | Carried suggestions |
| --- | --- | --- | --- | --- |
| prompts | `touch ran-the-tool` through Bash | 3 of 3 | 3 of 3 | 3 of 3 |
| quiet | Read a file in the working directory | 3 of 3 | 0 of 3 | — |

Trials 30 through 35, alternating, in
[captured/signal-2.1.267.txt](0003-harness/captured/signal-2.1.267.txt); the
quiet case's payload is
[captured/payload-pretooluse-quiet.json](0003-harness/captured/payload-pretooluse-quiet.json)
and the prompting case's is
[captured/payload-permissionrequest-2.1.267.json](0003-harness/captured/payload-permissionrequest-2.1.267.json).
Run by [signal.sh](0003-harness/signal.sh), which is `trial.sh` with two
additions: an event list joined by `+`, so both hooks are installed at once, and
a `pass` mode that records the firing and returns nothing.

**So the event that says a dialog is pending is the one whose decision is
ignored, and the event that can answer fires on every tool call.** Nothing here
is evidence against the three properties — the trials that established them all
used a call that prompts, and it held on every one. What it is evidence against
is the design the verdict's wording invited: a surface fed by `PreToolUse` alone
does not know which tool calls the owner would ever have been asked about. It
would have to ask about all of them, keep a permission policy of its own, or use
the two events as a pair. That is [MUS-F-0121](../../records/findings.md#mus-f-0121),
and what it costs the milestone is [MUS-Q-0094](../../records/questions.md#mus-q-0094)'s
to answer.

### The fallback, re-measured on the version under test

The result that makes this shippable was measured at 2.1.263 and the CLI has
moved four patch versions since. Three more hold trials at 2.1.267, same
shortened 20s timeout: **the CLI waits out the hook and draws its own dialog**,
at 21.2s, 21.4s and 21.3s.
[captured/hold-2.1.267.txt](0003-harness/captured/hold-2.1.267.txt). Unchanged,
and now measured on the version this repository would build against.

### What the harness will and will not reproduce

`trial.sh` changed shape between the first trials and the last: one hardcoded
event became an argument, `--model haiku` became `--model opus`, and `hook.sh`
renamed the `PermissionRequest` deny key. So the early trials do not re-run from
the scripts beside them without being told what they were.
[0003-harness/README.md](0003-harness/README.md) is the table of which arguments
produce which trial, written because a reviewer found the gap rather than because
anybody noticed while running them.
