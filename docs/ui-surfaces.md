# Surfaces awaiting design

**Nine of these are built, and seven of the nine were first written the way this
file exists to prevent.** Surface 5, intake, shipped at milestone 2c as a Go
template — the layout decided in code and shown to the owner afterwards. Surface
4, the decision queue, shipped at milestone 3 the same way, from the brief below
rather than from the published plan's artboard for it. Surface 1, the composer,
shipped at milestone 5 as a box inside another surface's page, with this file
amended to say that was where it lived. Surfaces 9 to 12, the authentication
pages, shipped at milestone 5b with no plan at all — four at once, described in
the paragraph below.

Seven of nine is the number to argue with. Recording each instance has not
stopped the next one; that is [MUS-Q-0053](../records/questions.md#mus-q-0053)'s
neighbour in `queue.md` and not something another paragraph here fixes.

Intake was the first and predates any such record. That the *other two*
happened after one existed is the part worth keeping: **the record alone was not
the safeguard, twice.** The owner's answer on
MUS-Q-0010 was to rebuild the queue from its artboard, which is done; their
answer on MUS-Q-0034, the same question about the same plan, was to rebuild the
composer from its artboard, which is also done. Intake is unchanged and still
stands as built.

Surface 8, the session output, is the third. It is the first built from a
drawing before anything existed to redraw — surface 4 was built from its brief
at milestone 3 and rebuilt from the plan's artboard afterwards, which is a
different order and was, until a review corrected this line, described here as
though surface 8 got there first.

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

**There are twelve.** This file listed seven until milestone 4b needed the one
it had never named — a running session's *output*, which is not the session
list. Surface 8 below is that gap, found by trying to build against nothing, and
it has [a plan of its
own](https://plan.agent-native.com/plans/plan-6009f123020a4f58) because the
seven-surface plan does not draw it.

Surfaces 9 to 12 are the authentication pages, and they arrived the worst way
this file knows: **built first and listed afterwards.** Milestone 5b wrote
`/signin`, `/invite/{token}`, `/account/passkey` and `/account` straight into Go
templates. Nothing was routed around — they were never in any plan to route
around — which is a different failure from surfaces 4 and 1 and not a smaller
one, because the standing instruction below covers *every remaining surface* and
these were four of them.

They now have [a plan of their
own](https://plan.agent-native.com/plans/plan-b1277e4f36f24da3), published
2026-08-25 and reviewed the same day, and the owner's answer on
[MUS-Q-0043](../records/questions.md#mus-q-0043) is that they are rebuilt from
whatever it settles. They were. What already existed became the baseline to
argue with rather than the thing being defended, which is the only useful shape
for a surface that arrived this way.

**Twelve comments came back, and five said the same thing:** take the
explanatory prose out of the wireframes. That is worth recording as a fact about
how these were written rather than as twelve separate corrections — the pages
were narrating themselves. What survives is the line telling somebody with no
account where one comes from, and the line naming the command that makes the
first owner. The rest of the changes are in
[MUS-D-0106](../records/decisions.md#mus-d-0106).

**The plan for the original seven is
[plan-4827b50a72674a22](https://plan.agent-native.com/plans/plan-4827b50a72674a22)**,
published 2026-08-20. Eight artboards — intake as built and as proposed, plus
the six then-unbuilt surfaces — the shell they share, and four open questions,
which were answered 2026-08-20. Three of those six are still unbuilt; the
decision queue is built from its artboard, the composer since milestone 5, and
the records document since 5b — which arrived inside another milestone's branch
and is [MUS-Q-0054](../records/questions.md#mus-q-0054).

**The tab bar is not locked to the viewport**, on any surface. The owner filed
MUS-F-0032 from a phone: on the session view, output arrives forever and carries
the bar down with it, so the thing you navigate by recedes as you watch. The
session view is a repair — its own CSS sets `min-height:100vh`, `#out{flex:1}`
and `nav{margin-top:auto}`, which is an app shell that never caps its height.
What the document surfaces should do is a choice, and it is drawn in
[plan-ba6b90e7d9064d09](https://plan.agent-native.com/plans/plan-ba6b90e7d9064d09)
rather than decided in code, and the owner answered it: **pinned below 60rem, a
left rail above it, the rail replacing the bar rather than joining it**
(MUS-D-0118). The rail is the same `<nav>` every surface already ends with,
moved by a media query, so one nav exists in the DOM at every width and these
surfaces keep working with script blocked. Drawn before built, which is the
thing MUS-F-0027 says this file keeps failing to do.

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

**Answered on 2026-08-19** as `MUS-D-0013`, *"The composer takes the thought
first"*, taken at the evening design review against the published wireframes.
This file went on calling it open until milestone 5 came to build it, and
milestone 5's first attempt then attributed the answer to the surfaces plan on
2026-08-20 — a plan published the day *after* the decision was recorded, whose
four answered questions are Records, the tab bar, the routing guess and the
audit. The composer is not among them. Getting that wrong is what let two thirds
of the decision go unbuilt.

`MUS-D-0013` has three clauses. Two are built and the third is declined:

| Clause | How |
| --- | --- |
| Text before destination | The box is the screen; the route row is beneath it |
| The route row defaults to the last active session | The adapter reads `session_activity` and the composer orders by it. Before this it was raw `tmux list-sessions` order, which is alphabetical, and the records described that as last-active |
| The idea inbox is a route like any session… | Built. The inbox is a destination beside the sessions, and choosing it files a record under its own prefix rather than typing into anything |
| …**folding intake into the composer** | **Declined** on [MUS-Q-0036](../records/questions.md#mus-q-0036). Intake is proven and fast from a locked phone and the reply box is where a person already is, so three surfaces can start a message and nothing is retired |

**One draft, not one per session**, which follows from thought-first: what is
being written is a thought, and where it goes is chosen after. A draft keyed per
project would be lost at exactly the moment the design exists to protect. The
composer and the session view's reply box share the one key.

It is **not a fourth tab** — the four are Sessions, Decisions, Intake and
Records (`MUS-D-0041`) — and it is reached from the session view. It **is** the
second surface in this repository carrying script, which the owner took
deliberately on `MUS-Q-0034`: a draft cannot survive a backgrounded phone
without something running in the page. The form posts and works with the script
blocked; the script keeps the draft and nothing else.

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
| The tab bar carries three tabs, not four | Records is not built, and a tab pointing at it would be an unbuilt capability described as existing. Sessions joined at 4b. MUS-D-0041's four still stands; MUS-Q-0012 confirms this as its interim |
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

**One of the two surfaces in v1 that carry a client layer**, and the only one
with no alternative. A live terminal cannot be server-rendered. The stack table names this as the exception and keeps the rule
for everything else; a second surface wanting script is a new decision, not a
precedent.

**Sub-agents arrived at milestone 4c**, and not by the route this file expected.
They moved out of 4b on MUS-Q-0017 because showing them requires Mustur to know
a sub-agent exists, and reading one pane does not tell it. What settled the
question is [investigation 0002](investigations/0002-sub-agent-visibility.md),
whose rule was fixed before the evidence was looked at.

The adapter cannot place a sub-agent anywhere — a sub-agent is a call inside the
CLI's one process, so there is no window to put it in. The CLI's lifecycle hooks
say when one starts, which tool it is in, and when it stops, each tagged with an
identifier the CLI supplies. Rows above the output are therefore attributed
rather than inferred, and the pane is untouched. What they do not carry is a
sub-agent's prose while it runs; that arrives when it finishes.

**The connection is the first one that carries keystrokes in**, not only records
out. A flaw here is not a wrong page; it is somebody else typing into an agent
with a checkout and a shell. The WebSocket refuses any origin but its own —
browsers exempt WebSockets from the same-origin policy and send cookies with the
handshake, so Access authenticates the person and does nothing about a socket
opened by a page they happened to visit.

Its plan is
[plan-6009f123020a4f58](https://plan.agent-native.com/plans/plan-6009f123020a4f58).
Milestone 4b builds it from that drawing, not from this brief. The owner has
settled the composer, the scrollback cap, what idle means, what supervision
does, and — at 4c — how sub-agents are found and how much of one a row shows.
Typing is not armed separately from watching — the owner settled that on
MUS-Q-0018, "always writable, as drawn", and an edit for milestone 4c briefly
listed it as open again. Nothing on this surface is open.

### 9. Sign in

One button. A passkey needs nothing typed, so there is no address field, no
password field and no forgotten-password path — none of them exist to be
forgotten.

Must answer: what does somebody see who has no account here? It must never say
whether an address is known, and the empty case — nobody has an account at all —
is a different page, because with no accounts there is nobody to send an
invitation and the machine makes the first owner.

### 10. Accept an invitation

Who is being invited, to what, and with which role, before anything is created.
Accepting is one action because the invitation carries the role it grants.

Must answer: what does a bad invitation look like? One message for expired,
already spent and never existed — anything finer is an oracle for somebody
guessing tokens.

### 11. Account

Your roles and your passkeys, and nothing about anybody else — the owner settled
that directly on
[MUS-Q-0046](../records/questions.md#mus-q-0046). A project reads as
*Mustur (MUS)*, in full and with the tag, because an invited reader has never
seen the tag and the tag is what every identifier uses.

**Adding a passkey happens here**, in place, rather than on a page of its own.
The first drawing gave it a page holding a heading and one button, which the
review called what it was. The cost is named rather than absorbed: a WebAuthn
ceremony needs the browser's credentials API, so this page carries script and
this page carries script
([MUS-Q-0047](../records/questions.md#mus-q-0047)). Everything else on it works
without.

**The count is six**, not the four an earlier version of this line claimed:
surfaces 9 and 10 are one template and 11 and 12 are another, and each loads its
script for both. Nobody drew that consequence, which is why the number is
written out per surface — 1, 8, 9, 10, 11, 12 — rather than asserted. Whether
six is the right thing to count at all is
[MUS-Q-0053](../records/questions.md#mus-q-0053).

Must answer: what the surface refuses, and when you find out. The last passkey
cannot be removed and the only owner cannot stand down. The banner that
pre-announced the second was cut in review, so both are now met at the control
rather than before it.

### 12. People and invitations

Owners only, and a second screen rather than the bottom of the account page
([MUS-Q-0045](../records/questions.md#mus-q-0045)) — which is also where the
room came from, since the people rows overlapped on a phone when they shared a
screen.

Must answer: **an invitation link is a secret shown once.** It is never stored,
so a truncated one is a secret destroyed and the only recovery is issuing
another. It is shown whole, with a copy button.

Must also answer: a control that appears to do something must do it. Changing a
role saves it; there is no separate button to press afterwards, which the review
named as the failure it is.

## Not surfaces

| Not building | Because |
| --- | --- |
| A dashboard of everything | Scoring and ranking are non-goals; a wall of tiles is how the thing you came for gets buried |
| An embedded view of any other application | Framing breaks navigation, deep links and mobile scrolling, and identity providers refuse it outright |
| A file tree or editor | The editor is not being replaced. That is what the checkout on the machine is for |
| Anything showing plan usage as a gate | Subscription usage is not readable programmatically; a gauge implying otherwise would be a false claim |
