# Surfaces awaiting design

**One of these is designed and it should not have been, not this way.** Surface
5, intake, shipped at milestone 2c as a Go template — the layout decided in code
and shown to the owner afterwards, which is what the rule three paragraphs down
exists to prevent. Recorded rather than tidied away: the owner's answer was to
publish a plan for every remaining surface before any more of them are built.

The rest of this file lists the surfaces v1 needs, what each one must do, and the
constraint that governs it, so that wireframing has a target list rather than a
blank page.

**Wireframes do not live in this file.** Layout options go to the owner as a
published visual plan, never as ASCII in a document or a prompt. This file is the
brief; the design is the answer to it.

**The plan for all seven is
[plan-4827b50a72674a22](https://plan.agent-native.com/plans/plan-4827b50a72674a22)**,
published 2026-08-20. Eight artboards — intake as built and as proposed, plus
the six unbuilt surfaces — the shell they share, and four open questions. No
further surface is built before it is answered.

## The constraints every surface inherits

| Constraint | Where it comes from |
| --- | --- |
| **One tab, N sessions.** Client cost must not grow with concurrency — the laptop has 1.5 GB free of 15.7 GB | [Plan.md](../Plan.md) principle 3 |
| **Phone is a first-class viewport**, not a responsive afterthought. It is the only device off the home network | The problem this project exists for |
| **A link opens a new tab and never replaces the current one**, and must be necessary or better than integrating | [decisions.md](../decisions.md#link-out-is-conditional) |
| **Never framed.** No backend is embedded in an iframe | [decisions.md](../decisions.md#link-out-is-conditional) |
| **Nothing described in the present tense that is not built** | [workflow.md](../workflow.md) |

## The surfaces

### 1. Composer

The reason the phone matters. Multi-line, editable before sending, spell-checked
by the browser, and reachable off the home network without a terminal.

Must answer: which session am I talking to, and how do I switch without losing
what I have typed? Drafts survive a dropped connection and a backgrounded phone.

**Open question for design:** whether a message is composed against a chosen
session, or composed first and routed second. The second matches how the owner
actually works — the thought arrives before the destination is decided — and is
harder.

### 2. Session list

Every session Mustur owns, across every machine. Which project, which machine,
running or idle, and what it last said.

Must make it obvious that **a session left in a terminal is not here and will not
appear** — that is the likeliest week-one surprise and the interface should not
let the owner form the wrong model.

### 3. Records

Milestones, findings, decisions, investigations and work units, in
[StrucGu's roles](../strucgu.yaml). Every record addressable by identifier with a
canonical URL.

Must answer: given a bare identifier on screen, how does a **human** expand it in
one action, with no round trip to an agent? That is the original complaint and it
is a reading surface before it is anything else.

**Open question for design:** identifiers are dense and cross-referential — a
finding cites decisions, a decision cites milestones. Whether that is a graph to
navigate or a document to read is unresolved and it changes the whole surface.

### 4. Decision queue

Where a blocked agent's decision arrives and where the owner answers it.

The interaction that must not fail: **an open decision is visible without
hunting for it**, on a phone, and answering it is one action. An agent is blocked
until it is answered, so latency here is work stopped.

Must show what is blocked on each decision, so the owner can tell a question that
holds up a milestone from one that holds up a sentence.

### 5. Intake

**Built at milestone 2c, on loopback, without a visual plan.** What exists is one
textarea, one button and a list of what was filed in the last hour. It is the
baseline a plan should argue with rather than a design anyone chose.

One box. Append a line and leave. Under fifteen seconds, and it must never
require a decision to file — naming a thing requires understanding it, and at
capture time you do not.

Routing hint where one is obvious, defaulting to the idea inbox where it is not.

### 6. Routing

The registry: repositories, machines, cross-repo projects. Mostly written by
hand, mostly read by agents.

Must answer: is this checkout actually where the registry says it is? The
dispatcher contract this implements verifies before entering rather than trusting
a row, so the surface has to show a stale row as stale.

### 7. Audit

The conformance run over Mustur's own records, in the form StrucGu specifies.

Must be reproducible by someone who does not trust it, and must show which checks
are waived and why — a waiver that is invisible is a check that silently stopped
running.

## Not surfaces

| Not building | Because |
| --- | --- |
| A dashboard of everything | Scoring and ranking are non-goals; a wall of tiles is how the thing you came for gets buried |
| An embedded view of any other application | Framing breaks navigation, deep links and mobile scrolling, and identity providers refuse it outright |
| A file tree or editor | The editor is not being replaced. That is what the checkout on the machine is for |
| Anything showing plan usage as a gate | Subscription usage is not readable programmatically; a gauge implying otherwise would be a false claim |
