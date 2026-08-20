# Decision log

Why Mustur is built the way it is. [Plan.md](Plan.md) states *what* is true; this
file states *why*, so the contract stays terse and the reasoning is still
recoverable.

Entries are append-only and dated. A later entry may correct an earlier one; the
earlier text is left in place with a pointer rather than edited away.

Everything before this repository existed is in
[IdeaWarehouse `agent-workflow-web-platform`](https://github.com/DevOfPie/IdeaWarehouse/blob/main/ideas/agent-workflow-web-platform.md)
— the argument, the counter-argument, four retracted claims of its own, and the
measurements. This file does not repeat it; it records the decisions.

---

## Index

Navigation only. Rows are appended when entries are, and never removed.

| Entry | Covers |
| --- | --- |
| [Why this is not a local file](#why-this-is-not-a-local-file) | The two requirements nothing local can serve |
| [Inject, never offer](#inject-never-offer) | The measurement that constrains every module |
| [Link-out is conditional](#link-out-is-conditional) | New tab, and it must earn its place |
| [The store is reached by tooling, not symlinks](#the-store-is-reached-by-tooling-not-symlinks) | Why the symlinked-skills experiment was dropped |
| [Decisions are enforced, block then route](#decisions-are-enforced-block-then-route) | The requirement that came from an agent's failure |
| [The composer is built, not linked](#the-composer-is-built-not-linked) | The order-of-magnitude decision |
| [Mustur owns the sessions it talks to](#mustur-owns-the-sessions-it-talks-to) | And never attaches to one it did not start |
| [Mustur is its own first project](#mustur-is-its-own-first-project) | And touches no other until onboarding |
| [Records take StrucGu's module roles](#records-take-strucgus-module-roles) | And why LinkCtrl's shape differs |
| [Intake is in v1](#intake-is-in-v1) | The gap found by surveying the estate |
| [Mustur becomes the runner StrucGu did not ship](#mustur-becomes-the-runner-strucgu-did-not-ship) | And what that puts downstream of it |
| [Estimates here run long](#estimates-here-run-long) | The owner's calibration |
| [The composer takes the thought first](#the-composer-takes-the-thought-first) | Route second; last-active default keeps one tap |
| [Records read as a document](#records-read-as-a-document) | In-place identifier expansion; pivot kept for the record |
| [Home is the session list](#home-is-the-session-list) | With a pinned banner for open decisions |
| [Sessions are supervised through tmux](#sessions-are-supervised-through-tmux) | Crash-survival free; a terminal can still attach |
| [Decision detail is generated on demand](#decision-detail-is-generated-on-demand) | One paragraph up front; deeper costs tokens only when asked |
| [Phone surfaces show only the recent](#phone-surfaces-show-only-the-recent) | ~1 hour, then the archive |
| [Invisibility is taught once](#invisibility-is-taught-once) | First run and empty state, never permanent chrome |
| [Mustur is written in Go](#mustur-is-written-in-go) | The case survived removing the tooling argument |
| [The product stores in SQLite, exports to markdown](#the-product-stores-in-sqlite-exports-to-markdown) | Insert-only by trigger; the export is the audit surface |
| [Identifiers are project, role, serial](#identifiers-are-project-role-serial) | The naming that has to be right before the first insert |
| [The store holds records, the contract files keep their prose](#the-store-holds-records-the-contract-files-keep-their-prose) | Addressable without a second copy of the rationale |
| [The export is committed under records](#the-export-is-committed-under-records) | The audit surface a reader can check without the binary |
| [One record shape, not a table per kind](#one-record-shape-not-a-table-per-kind) | What a new kind costs |
| [The SQLite driver is pure Go](#the-sqlite-driver-is-pure-go) | cgo would end the static binary |
| [What the mandate keeps from the fixture, and what it does not](#what-the-mandate-keeps-from-the-fixture-and-what-it-does-not) | Three differences from what milestone 1 scored |
| [Four of StrucGu's five roles are implemented, and the repository adopts five modules](#four-of-strucgus-five-roles-are-implemented-and-the-repository-adopts-five-modules) | Corrects an entry the build overtook |
| [A milestone is read by agents that did not build it](#a-milestone-is-read-by-agents-that-did-not-build-it) | Three lenses, spawned fresh, changing nothing |
| [Stopping takes a reason from a table](#stopping-takes-a-reason-from-a-table) | The default after a milestone is the next milestone |
| [Pull requests are stacked](#pull-requests-are-stacked) | A fix in the base reaches everything above it |
| [The audit is not a gate until someone asks](#the-audit-is-not-a-gate-until-someone-asks) | How a gate dies |

---

## 2026-08-19 — decisions taken at the planning session

All of the following were the owner's, taken in one session and put as prompts
rather than written into a report. The order below is the order they were taken.

### Why this is not a local file

Symlinks, skills, `@path` imports and `--add-dir` are single-machine and
single-user by construction. Routing across repositories and machines, and
bringing other people in for feedback, are the first requirements here that no
local arrangement can serve **at all** rather than serve badly. Everything else
in this plan is a convenience; these two are the reason it exists.

The routing failure is measured rather than felt: the `/work` registry is a
one-row table inside a global command file, and a project can bypass it entirely
by installing a global skill whose trigger clause names no repository — which is
what happened, and why it fires inside another project's sessions.

### Inject, never offer

Measured: **0 memory operations in 114 turns** against a pre-seeded store, while
harness-injected facts survived all 138 compact-resumes. Agents do not call a
store they must choose to call.

So a design that *offers* documentation will not be read. The answer is to
mandate the call from a repo-local file that loads unconditionally, keeping the
bulk outside. The split is supported by measurement rather than taste:
repository overviews in context files are unhelpful, while **instructions in them
are well followed** — the half kept in the repo is the half that works.

**This is the load-bearing bet of the entire project, and it has never been
measured.** Milestone 1 exists to measure it, and its decision rule says kill
rather than shrink.

### Link-out is conditional

Embedding is ruled out by evidence — in-frame navigation is linearized into the
top-level session history so back and forward break by design, a cross-origin
frame's URL cannot be read by the parent so deep links require path-proxying, and
identity providers refuse to be framed at all.

Link-out is therefore the only working form, and it is permitted only under two
conditions, both the owner's:

1. It opens in a **new tab and does not remove the existing one**. On mobile,
   kept in a tab group where the platform allows.
2. It is **necessary** — no reasonable alternative exists — **or better** in
   performance or function than integrating.

Condition 1 also disposes of the objection that link-out is the app-switching the
single-platform requirement exists to prevent: the tab the owner is in is never
navigated away from.

### The store is reached by tooling, not symlinks

The alternative considered was symlinking a documentation directory into
`~/.claude/skills/`, so the corpus loaded lazily — a skill's description loads at
launch, its body only when used — rather than being imported at launch, which
does not reduce context.

Dropped, because `~/.claude/skills/` is per-account. Everything there is offered
in every repository, which is precisely the mechanism that cross-fires between
projects. It would have bought lazy loading by making the routing defect worse.

A tool registered with `claude mcp add --scope project` has the same lazy
property — nothing enters context until it is called — without the account-wide
scope, because the `.mcp.json` is committed to the checkout. Strictly better on
both axes.

**What is lost:** the experiment would have measured the real context cost of a
lazy path, and that measurement will now never be taken. The tool path should be
measured in its place.

### Decisions are enforced, block then route

An agent may not report work complete while an open decision exists that was
never surfaced as a prompt. The decision travels to a queue the owner answers
from any device, and the answer is injected back into the session that raised it.

This is the only requirement in the plan that came from a failure of the agent
writing it: three open decisions were written into a pull request body under a
heading announcing them, with options and consequences, and the owner's response
was that calls to action and questions in prose are useless. Put as prompts
instead, the answers inverted the plan.

Prior art says this is enforceable precisely because the operator is an agent.
Every generation of falsification-first tooling built for humans has died —
humans route around gates that cost them something, and one such tool built
immutability and then removed it on user demand. An agent executes the protocol
it is given.

### The composer is built, not linked

Composition is the largest complaint and the only one that bills on every
message: no multi-line input, awkward editing, no spell check, on a laptop that
drops keys.

An existing tool already provides browser and phone agent chat, so under the
link-out rule above this could have been a link. The owner decided to build it,
and to route it to the local agent, working with agents other than Claude where
possible.

**This is the order-of-magnitude decision and it revives the strongest objection
in the idea file** — that the category is saturated, that ~150 near-identical
tools exist, and that the category leader shut its hosted layer down. Three
things reduce that and none dismiss it: that shutdown was for want of a business
model at a cost structure this never has; Mustur's composer is the only one that
will sit in front of Mustur's own routing and records; and it is scoped to
sessions Mustur itself starts, so it never reimplements or attaches to anything
it does not own.

It remains the least certain estimate in the plan, the piece most likely to be
abandoned, and the piece without which the rest is a registry with a web page.

### Mustur owns the sessions it talks to

Mustur starts and supervises long-lived sessions through a small adapter on each
machine, shelling out to whatever CLI is configured.

It therefore **never attaches to a session it did not start**. No documented
interface exposes another process's session, so attaching would mean depending on
a private one, which is a standing non-goal.

The consequence is worth stating plainly because it will be discovered in week
one otherwise: **a session left running in a terminal is invisible in Mustur and
will not become visible.** If the mental model is "open Mustur, continue what I
left in tmux", that model is wrong.

This also satisfies the client constraint: one browser tab against N server-side
sessions keeps the client flat as concurrency grows, which is the property the
current terminal workflow has and an editor instance per project does not.

### Mustur is its own first project

Mustur keeps its own records in the same shape the other projects use, so the
eventual transition is exercised on the one project that can afford to get it
wrong.

And **no file in any other project is touched** until Mustur is reasonably built.
Onboarding a project is a milestone with its own verdict. A half-built router
that has already edited eight repositories is worse than no router.

This invalidated the first disproof designed for milestone 1, which registered a
stub inside two existing repositories. It now runs in two throwaway repositories
and loses nothing, because a global skill is selected by description match and
cross-fires into a scratch repository exactly as it does into a real one.

### Records take StrucGu's module roles

`decision-log`, `findings-queue`, `investigations`, `triage-rule`, `work-units` —
already specified with fixtures and an audit vocabulary, and already declared by
four repositories.

LinkCtrl's `M`/`F`/`D` shape is not a competing standard to be reconciled:
**StrucGu was built from the structure LinkCtrl started, and the transition never
happened.** So LinkCtrl predating the spec is expected, and mapping it is a task
for that repository's agents at onboarding rather than an obstacle to this
choice.

Note the distinction, since it is easy to lose: Mustur the *product* implements
all five roles as its record shape. Mustur the *repository* adopts three of them
today, because nothing is built and the other two have nothing to point at. See
[strucgu.yaml](strucgu.yaml).

### Intake is in v1

Found by surveying the estate rather than by argument: the two most-used workflow
surfaces across the projects are both append-only capture paths, and one of them
exists specifically because capture from a phone was otherwise impossible.
Mustur was to be the phone surface with no capture at all.

It captures into Mustur's own `findings-queue` only, carries a routing hint where
one is obvious, and defaults to the idea inbox where it is not.

### Mustur becomes the runner StrucGu did not ship

StrucGu specifies manifests, adoption records, audit output, fixtures and
remediation, and its README says in terms that it ships no runner, no binary.
Four repositories declare adoption against a spec nothing executes, and audits
are run by hand with an external checker.

Mustur is about to become the system of record for exactly what StrucGu
describes, and its block-then-route machinery is the enforcement an audit gate
needs. So the audit ships **with** the records rather than after them — auditing
records Mustur owns is far cheaper than auditing files it does not.

**What this puts downstream of Mustur, stated before it is discovered:** four
repositories' adoption records now depend on a service one person maintains. The
blast radius of abandoning Mustur grows from Mustur to every repository that
declared a module. And if StrucGu later ships its own runner there will be two
implementations of one spec, with Mustur's the unofficial one. Nothing prevents
that and no agreement covers it.

### Estimates here run long

The owner's calibration, given 2026-08-19: estimates produced by an agent are
significantly longer than the work actually takes.

The estimate in the idea file moved three times in one day, always upward, and
always because the estate was surveyed rather than reasoned about. That is a
reason to distrust the direction as well as the magnitude.

So an effort figure in this project is an **upper bound, not a plan**, it must
state what it is anchored on, and it must never be the reason scope is cut. The
practice that produced the drift — decompose the work, estimate each piece, add
them up — is the practice to avoid; anchor on what comparable work actually took
instead.

## 2026-08-19 — decisions taken at the evening design review

All the owner's, taken as prompts against the published wireframes
(the visual plan's canvas holds the chosen and not-chosen variants) and
[docs/stack-evidence.md](docs/stack-evidence.md).

### The composer takes the thought first

Text before destination, because that is the order the thought actually
arrives in. The route row defaults to the last active session, so the fast
path costs the same one tap as a session-first design would; the idea inbox is
a route like any session, which folds intake into the composer instead of
keeping two capture muscles.

Accepted with the choice: the routing step must never misfire silently, and
the build is larger than session-first would have been. Session-first stays on
the plan's canvas for the record.

### Records read as a document

A record is a page: identifier chips expand in place — one action from bare
identifier to meaning, no agent round trip — and cited-by sits at the end.
Pivot navigation was drawn, considered, and kept for the record; it earns
reconsideration only if cross-references become dense enough that hopping
beats reading.

### Home is the session list

Sessions are what the app is opened to check. A pinned banner appears whenever
a decision is open, which keeps decision latency visible without making the
queue the front door.

### Sessions are supervised through tmux

The per-machine adapter starts every session inside tmux and supervises it
there. What that buys, from the prior-art survey in
[docs/stack-evidence.md](docs/stack-evidence.md): crash-survival and
scrollback come free, sessions outlive adapter restarts, and a terminal can
still attach to a session Mustur started. The attach rule is untouched —
Mustur still never attaches to a session it did not start; the arrow only
points the other way. Interactive sessions also sit on the safer side of the
paused metering change recorded in [queue.md](queue.md).

### Decision detail is generated on demand

A decision prompt carries one short context paragraph from the raising agent —
nearly free, since the material is already in its window. Anything deeper is
generated only when the owner asks, by injecting the request into the blocked
session. Tokens are spent on demand rather than speculatively.

### Phone surfaces show only the recent

An answered decision or a filed confirmation stays on its surface for roughly
an hour, then leaves for the archive. History is never lost, only moved off
the working surface.

### Invisibility is taught once

That a terminal session is invisible in Mustur is taught on first run and in
the session list's empty state, and nowhere else. Permanent explanatory chrome
was drawn, reviewed, and rejected: a note a user needs once is noise every
time after.

### Mustur is written in Go

Taken after the owner asked whether the recommendation survived ignoring what
happens to be installed. It does, on measurements in
[docs/stack-evidence.md](docs/stack-evidence.md): an 8.3 MB static binary
against a 224 MB Node runtime, the official Go MCP SDK mounting as a plain
`http.Handler` beside the server-rendered routes, and stdlib HTML templating.
The known cost — no official Claude Agent SDK — was mostly discharged by the
tmux decision above: a tmux adapter drives the same interactive CLI a human
would and reads the same on-disk session records, so the SDK's headless loop is
not on the path. The residual cost, accepted: `creack/pty` is pinned at a
pseudo-version until it tags again.

### The product stores in SQLite, exports to markdown

The owner's call: everything the product owns lives in one database, because
that is the cleanest and fastest setup for each new project onboarded. Accepted
with it, named before building:

1. Agent search is a tool call over FTS5 — better than grep for the access
   path agents actually have, since records were never reachable by grep from
   another checkout anyway.
2. Append-only is enforced by triggers and by the tool layer exposing only
   insert and read — stronger than git convention.
3. **The conformance audit runs against a deterministic markdown export**,
   because StrucGu's checks speak in files, headings and links. The export is
   also the human-readable backup. This is the one real obligation the choice
   creates.
4. History is an immutable event log with a materialized latest, or success
   criterion 7 has nothing to stand on.

This repository's own contract files stay files; the database governs what the
product stores.

## 2026-08-19 — decisions taken while building milestone 2

The first milestone that writes code. Every entry below was taken while
building it. Three were the owner's answer to a prompt and say so in their own
first line — the identifier scheme, what the seed carries, and where the export
lives; the rest are the builder's, taken under decisions already on record.

### Identifiers are project, role, serial

`MUS-D-0001`: a three-letter project prefix, a role letter, a four-digit
serial. The owner's call, from three offered shapes.

The prefix is what a bare serial cannot do. Records cite each other, the store
is append-only, and a second project moves in at
[a milestone of its own](Plan.md#milestones) — so an identifier written today
has to still mean one thing on the day a second project's records sit beside
these. `D-0001` would have been shorter and would have collided that day, with
no way to rewrite what was already written.

The costs, accepted:

1. **The serial is a ceiling.** Four digits, fixed rather than grown, because
   widening it later resorts every identifier written before it.
2. **The prefix is assigned by hand.** Two projects can choose the same three
   letters, and nothing notices until their records are in one store.
3. **The identifier says nothing about the record.** A dated slug would have
   read better in prose; it would also have frozen the wording of the day it
   was written, which append-only makes permanent.

### The store holds records, the contract files keep their prose

Everything this repository already held is in the store and addressable: the
twenty-one decisions that predate this milestone plus the six taken during it,
nine milestones, six findings, the accepted investigation. What is *in* each record is a title, a date, and one line — plus
a link to the prose it summarises. The rationale itself stays in
[decisions.md](decisions.md), [Plan.md](Plan.md) and
[queue.md](queue.md).

The owner chose to seed everything that exists rather than start empty. Copying
the prose as well would have satisfied that literally and created the thing
[the composer decision](#the-composer-is-built-not-linked) warns about in
another form: two copies, one of them edited, no way for a reader to tell
which. The summary is one line precisely because that is the smallest surface
on which the copy and the source can disagree.

What this costs: a record's one line can drift from the file it points at, and
nothing detects it. What it buys: every decision has an identifier today,
without this repository having two decision logs.

### The export is committed under records

The owner's call, from three offered locations. [`records/`](records/),
regenerated by `mustur export`, committed.

The database is the record and a binary file in git is a record nobody can
review. The export is the half a reader can check: it lands in the diff, it is
what [the audit](Plan.md#milestones) will run against, and it is the backup if
the database is lost. Every insert therefore produces a commit-worthy diff,
which is the cost, taken deliberately.

Rendering is deterministic — sorted by identifier, no timestamp of the run — so
a diff shows what changed in the records and nothing else. That is asserted by
a test, not by inspection.

### One record shape, not a table per kind

One shape carries every StrucGu module role and every routing row: identifier,
kind, title, date, prose, named citations, ordered fields.

A table per kind buys typed columns and costs a second insert path, a second
export path and a second audit path for every kind added. This project's own
[known limitations](Plan.md#known-limitations) say the requirements are
discoverable only by use and every boundary is provisional, and a schema with
eight tables is the most expensive kind of boundary to be wrong about.

The cost is real: nothing in the database says a work unit has a risks section.
That obligation lives in the module spec and is checked by the audit, which is
where StrucGu already puts it.

### The SQLite driver is pure Go

`modernc.org/sqlite`, not the cgo binding.

[Go was chosen](#mustur-is-written-in-go) on a measurement of an 8.3 MB static
binary against a 224 MB Node runtime. A cgo driver ends the static binary, so
the driver that keeps it is the one that fits the decision already taken. The
pure-Go translation is slower than the C library; no number here claims how
much, because none has been measured on this workload.

### What the mandate keeps from the fixture, and what it does not

The tool is `mustur_route` on server `mustur`, taking `repository` and `task` —
the name and the arguments the [milestone 1 harness](docs/investigations/0001-mandated-tool-call.md)
scored — and the clause in this repository's context file is the fixture's
clause, word for word.

Twenty of twenty runs honoured that clause. Rewording it would have made the
measurement about something else, and a bet this whole plan rests on is not
worth re-running because a sentence read better.

**Three things differ from what was scored, and the entry exists to name all
three.** An earlier draft of it named two, and an independent review found the
third; the correction is the reason this entry reads as it does.

1. `id` and `kind`, optional arguments that narrow what comes back. A call
   written against the stub still works.
2. A sentence saying what to do when the tool is absent — report it and
   continue. That weakens the pressure the disproof measured, and it is taken
   knowingly: this repository's own sessions would otherwise stop whenever the
   server is not running. An absent tool reported is worth more than a session
   that cannot start.
3. **The transport.** The fixture registered the stub over stdio, where the
   client launches the server and the tool is therefore always present; the
   disproof ruled *invalid* every run whose server had not connected. What
   ships is HTTP on loopback, where the tool is absent unless somebody has run
   `make serve` — which is the excluded state, made ordinary. Difference 2
   exists because of difference 3.

So milestone 1's result covers the stdio fixture, and what ships is a
descendant of it. The owner's call, 2026-08-20, on being shown the gap: correct
the claims rather than re-run the disproof or retreat to stdio. README.md and
CLAUDE.md no longer say the shipped clause is the measured one. Re-scoring the
clause on the transport that ships is available and has not been done, and
nothing in this repository checks that the clause still says what was measured.

## 2026-08-20 — corrections found by review

Milestone 2's first independent review. Each entry here corrects an earlier
one; the earlier text stays where it is, unedited, as the standing rule
requires.

### Four of StrucGu's five roles are implemented, and the repository adopts five modules

Corrects [Records take StrucGu's module roles](#records-take-strucgus-module-roles),
which says the product implements all five module roles as its record shape and
that the repository adopts three modules because nothing is built. Both halves
have stopped being true and the earlier entry cannot be edited, so this is the
pointer.

**The product implements four.** `decision`, `finding`, `investigation` and
`work-unit` are record kinds; `triage-rule` is not, and nothing plans one.
That module describes a triage document rather than a set of records, so
"implements all five as its record shape" was wrong when it was written rather
than overtaken — the fifth has no records to shape. Whether Mustur should hold
a triage rule as a record at all is unasked, and it is a line in
[queue.md](queue.md) rather than a decision here.

**The repository adopts five.** `triage-rule`, `decision-log`,
`findings-queue`, `investigations` and, since milestone 2, `work-units`. The
earlier entry's count was already one behind the file it described.

A note on the mechanism, because two standing rules in
[workflow.md](workflow.md) pull against each other here: "superseded decisions
stay, with a pointer" and "never edit an entry". A pointer added to the earlier
entry would be an edit of it. The pointer therefore lives in the later entry,
which is the only reading under which both rules hold. That ambiguity in the
contract is queued rather than settled here.

## 2026-08-20 — how a finished milestone is checked

### A milestone is read by agents that did not build it

Owner-set, after milestone 2 was built and reviewed by nobody but the agent that
built it. Mechanical gates ran and passed; they check that links resolve and
that a cache rebuilds. They check nothing about whether the design is right.

The process is [LinkCtrl's](https://github.com/DevOfPie/LinkCtrl/blob/main/docs/build-notes/phase-loop.md),
which spawns one reviewer per milestone and states the reason plainly: everyone
already in the loop is looking at the milestone being built, and the defect that
keeps costing that project a reopening is the other kind — a change that makes
an already-shipped claim false, which nobody was looking at.

Three differences from the original, all deliberate:

1. **Three reviewers, not one.** LinkCtrl's milestones are increments and its
   reviewer reads one diff. Milestones here are whole subsystems, and the three
   asks — this milestone's done-when, the shipped claims, this file's own gates
   — compete for attention inside one context. Split across three, none can be
   traded against another.
2. **A contract lens, which LinkCtrl has no need of.** Most of what this
   repository can get wrong is prose: a capability in the present tense that
   does not exist, a number nothing measured, a decision written out as an
   argument instead of raised as a prompt. Those are gates in
   [workflow.md](workflow.md) and no code reviewer reads them.
3. **Findings are dispositioned in the pull request**, in public, as fixed,
   deferred or disputed. LinkCtrl disposes of them at acceptance, which it has a
   separate actor for; this repository does not, so the disposition has to be
   visible instead.

What is taken unchanged, because both are the whole point: the reviewer is given
the milestone, the branch and the contract and **not** the builder's report — a
reviewer handed the report reviews the report — and a reviewer that found
nothing says so in those words, because silence and *did not look* are the same
string.

This entry sits below the corrections it produced, because entries are appended
and never reordered. Its first run is
[the one above](#2026-08-20--corrections-found-by-review): fifteen findings
fixed, four deferred, one reopening taken to the owner, and one conflict in
[workflow.md](workflow.md) reported rather than settled.

The cost, accepted: three agents per milestone, and a milestone cannot be
accepted while they are running.

### Stopping takes a reason from a table

Owner-set 2026-08-20, lifted from
[LinkCtrl's phase loop](https://github.com/DevOfPie/LinkCtrl/blob/main/docs/build-notes/phase-loop.md).

The failure it prevents is specific and this repository has already committed
it once: a milestone lands, the work reports what it did, and the turn ends —
not because anything blocked it but because finishing something feels like a
place to stop. LinkCtrl names every excuse that has ended one of its runs and
rules each one out, because "it looked like a clean boundary" is
indistinguishable from the next iteration.

So the stop conditions are a closed table, and the not-reasons are written down
beside them. The one worth stating twice: context running long is not a reason.
Context is summarised and the run continues, so wrapping up early trades a
working run for a problem that handles itself.

Two rows are this repository's rather than LinkCtrl's. A review returning a
**reopening** stops the run, because a falsified claim on a shipped milestone is
scheduling and scheduling is the owner's. Red CI stops it, because the next
milestone would otherwise be built on a build nobody has looked at — LinkCtrl
learned that one from nine days of red CI that every local gate reported green
through.

### Pull requests are stacked

Owner-set 2026-08-20. Each branch is cut from the one before it and its pull
request targets that predecessor rather than `main`.

The reason is the cost of the review step this repository just adopted. Three
reviewers per milestone produce findings against a base; if the pieces above it
are independent branches, the same finding is found and fixed once per branch.
Stacked, a fix in the base reaches everything above it by a rebase.

What it costs: a stack merges bottom-up, so a base held up holds up everything
above it, and every branch above a merged base has to be re-pointed and rebased.
That bookkeeping is only worth paying when the pieces genuinely build on each
other — a milestone that does not decompose ships as one pull request, and
saying so is the right answer rather than a failure to stack.

## 2026-08-20 — decisions taken while building milestone 2b

### The audit is not a gate until someone asks

Exit zero whenever the audit ran, non-zero only when it could not. Findings are
output, and `--gate` is how a consumer asks for them to be failure.

StrucGu's argument, adopted whole: a check that fails on day one in a repository
with required status checks is made non-required within the hour, and then
nobody looks at it again. A dead gate is worse than no gate because it looks
like coverage.

This repository's first run has four findings — three are `queue.md`'s shape,
already recorded as a finding, and one is a commit that removed lines from this
file. Gating on day one would have meant either fixing those under time pressure
or turning the audit off, and both are worse than reading them.

Where the audit *runs* is a separate question, and it is the owner's rather
than this entry's. A later entry records it.
