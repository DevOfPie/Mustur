# Surfaces awaiting design

**Two of these are built. Both were first written the way this file exists to
prevent, and one has been put back.** Surface 5, intake, shipped at milestone 2c
as a Go template — the layout decided in code and shown to the owner afterwards.
Surface 4, the decision queue, shipped at milestone 3 the same way, from the
brief below rather than from the published plan's artboard for it.

That the second happened after the first was recorded is the part worth keeping:
the record alone was not the safeguard. The owner's answer on MUS-Q-0010 was to
rebuild the queue from its artboard, which is done. Intake is unchanged and
still stands as built.

**The standing instruction, which an earlier draft of this file deleted and a
review caught:** the owner's answer after intake was to publish a plan for every
remaining surface **before any more of them are built**. That plan exists and is
linked below. It governs milestones 4 onwards, it was not superseded by the
queue being rebuilt, and it is restated here because removing it was the
"nothing is deleted" rule being broken on the one line that constrains what gets
built next.

The rest of this file lists the surfaces v1 needs, what each one must do, and the
constraint that governs it, so that wireframing has a target list rather than a
blank page.

**Wireframes do not live in this file.** Layout options go to the owner as a
published visual plan, never as ASCII in a document or a prompt. This file is the
brief; the design is the answer to it.

**There are eight.** This file listed seven until milestone 4b needed the one it
had never named — a running session's *output*, which is not the session list.
Surface 8 below is that gap, found by trying to build against nothing, and it
has [a plan of its
own](https://plan.agent-native.com/plans/plan-6009f123020a4f58) because the
seven-surface plan does not draw it.

**The plan for the original seven is
[plan-4827b50a72674a22](https://plan.agent-native.com/plans/plan-4827b50a72674a22)**,
published 2026-08-20. Eight artboards — intake as built and as proposed, plus
the six then-unbuilt surfaces — the shell they share, and four open questions,
which were answered 2026-08-20. Five of those six are still unbuilt; the
decision queue is built, and is built from its artboard.

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

**Answered 2026-08-20: a document to read.** An identifier expands in place, and
the citation structure is never the primary object. Identifiers here are dense
and cross-referential, so the graph reading was real; what it cost is in
[decisions.md](../decisions.md#records-read-as-a-document).

### 4. Decision queue

**Built at milestone 3 from this brief, then rebuilt from the plan's artboard**
after the owner's answer on MUS-Q-0010. What the drawing settled and the brief
had not: what is blocked comes first, above the question; answers are options
carrying what each one costs, one of them recommended, rather than a text box
that made the owner reconstruct the list the asker already had; each option
expands in place, one line up front and the paragraph behind it only when asked;
and answering is one tap above the bar. The expansion is a `<details>` element,
so none of it costs script.

**Six things differ from the drawing.** An earlier version of this paragraph
said two, and a review counted the rest — so they are listed rather than
summarised.

| Difference | Why |
| --- | --- |
| The tab bar carries two tabs, not four | Sessions and Records are not built, and a tab pointing at one would be an unbuilt capability described as existing. MUS-D-0041's four still stands; MUS-Q-0012 confirms this as its interim |
| The banner on intake stays, beside the bar | They do different jobs: the bar is the fixed place the eye knows to check, the banner makes an open decision impossible to miss on whichever surface was opened. MUS-Q-0006 |
| Every open question is on one page, not one per screen | The queue is short by construction. The drawing is a single-question screen and this is a list with a rule between entries; the earlier claim to be "one question per screen" was simply false of the code beneath it |
| Options carry a radio | The drawing makes the card itself the selection. A radio is what a form can express without script, and the whole row is the control |
| There is a free-text box under the options | The drawing has none. The owner wanting to say something the list does not contain is the case a list of options is worst at, and it must not be the case the surface refuses |
| The project pill is not rendered | One project exists. A pill that always reads the same is chrome |

The Answer button is also in flow rather than in a bordered footer, so it
scrolls with the question rather than staying pinned. That is a difference the
drawing would probably lose an argument about on a short queue, and it is named
here rather than defended.

Where a blocked agent's decision arrives and where the owner answers it.

The interaction that must not fail: **an open decision is visible without
hunting for it**, on a phone, and answering it is one action. An agent is blocked
until it is answered, so latency here is work stopped.

Must show what is blocked on each decision, so the owner can tell a question that
holds up a milestone from one that holds up a sentence.

### 5. Intake

**Built at milestone 2c, without a visual plan**, and published at
`mustur.devofpie.com` behind Cloudflare Access at 2c's end. What exists is one
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

### 8. Session output

A running session's output, streamed to a browser tab. **Not the session list:**
that says which sessions exist, this is what one of them is saying.

Must answer, from a phone, off the home network: is this session running, and
what has it just said? A dropped connection must reconnect and replay what was
missed without the owner wondering whether walking into a lift killed the work.
A session that has **ended** must not look like one that is merely quiet — that
confusion is most of what this surface exists to prevent.

**The one surface in v1 that carries a client layer.** A live terminal cannot be
server-rendered. The stack table names this as the exception and keeps the rule
for everything else; a second surface wanting script is a new decision, not a
precedent.

**Sub-agents are part of it.** A session that spawns three reviewers is one pane
with three agents writing into it. They must appear as their own rows, running,
and one must be readable without losing the parent — which needs Mustur to know a
sub-agent started, and today it does not.

**The connection is the first one that carries keystrokes in**, not only records
out. A flaw here is not a wrong page; it is somebody else typing into an agent
with a checkout and a shell. The WebSocket refuses any origin but its own —
browsers exempt WebSockets from the same-origin policy and send cookies with the
handshake, so Access authenticates the person and does nothing about a socket
opened by a page they happened to visit.

Its plan is
[plan-6009f123020a4f58](https://plan.agent-native.com/plans/plan-6009f123020a4f58).
Milestone 4b builds it from that drawing, not from this brief. The owner has
settled the composer, the scrollback cap, what idle means and what supervision
does; what remains open is how sub-agents are found, whether typing is armed
separately from watching, and where a session's exit is recorded.

## Not surfaces

| Not building | Because |
| --- | --- |
| A dashboard of everything | Scoring and ranking are non-goals; a wall of tiles is how the thing you came for gets buried |
| An embedded view of any other application | Framing breaks navigation, deep links and mobile scrolling, and identity providers refuse it outright |
| A file tree or editor | The editor is not being replaced. That is what the checkout on the machine is for |
| Anything showing plan usage as a gate | Subscription usage is not readable programmatically; a gauge implying otherwise would be a false claim |
