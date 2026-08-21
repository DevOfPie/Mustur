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
| [Nothing vendors StrucGu](#nothing-vendors-strucgu) | A pinned copy of a specification goes stale silently |
| [The audit runs in CI, against a real catalog](#the-audit-runs-in-ci-against-a-real-catalog) | Evidence that ran once on one machine is not evidence |
| [The record roles are mapped at the export](#the-record-roles-are-mapped-at-the-export) | Auditing the records Mustur owns, not the prose beside them |
| [The store holds more than it did](#the-store-holds-more-than-it-did) | Corrects an enumeration the milestone overtook |
| [The database is the source, and the seed is history](#the-database-is-the-source-and-the-seed-is-history) | What a write path costs the export's reproducibility |
| [Records read as a document](#records-read-as-a-document) | An identifier expands in place |
| [The phone bar has four tabs](#the-phone-bar-has-four-tabs) | Decisions gets one of its own |
| [The routing guess is shown before filing](#the-routing-guess-is-shown-before-filing) | A default already filled in, not a question |
| [The audit is a page](#the-audit-is-a-page) | A waiver nobody sees is a check that stopped |
| [Mustur runs as a systemd user unit](#mustur-runs-as-a-systemd-user-unit) | And is not enabled until Access is in front |
| [The idea inbox is a routing target inside Mustur](#the-idea-inbox-is-a-routing-target-inside-mustur) | The fallback destination cannot be another repository |
| [A jot is filed without a decision](#a-jot-is-filed-without-a-decision) | The title is derived and the destination is guessed |
| [The unit is enabled, and the entry saying it is not stays put](#the-unit-is-enabled-and-the-entry-saying-it-is-not-stays-put) | A later entry corrects an earlier one; neither is edited |
| [Injection belongs to the milestone that owns sessions](#injection-belongs-to-the-milestone-that-owns-sessions) | Milestone 3 could not honour a clause needing milestone 4 |
| [A question is its own kind, and only some become decisions](#a-question-is-its-own-kind-and-only-some-become-decisions) | Why not a status field on an append-only record |
| [The gate turns on being asked, not on being answered](#the-gate-turns-on-being-asked-not-on-being-answered) | An absent owner must not stop the work |
| [The gate reads the tree, not the store](#the-gate-reads-the-tree-not-the-store) | A gate that could only skip on every machine but one |
| [An answer is required when the work depends on it](#an-answer-is-required-when-the-work-depends-on-it) | The owner's qualification; corrects the row above |
| [A question may be withdrawn by its raiser, never answered by them](#a-question-may-be-withdrawn-by-its-raiser-never-answered-by-them) | The enforcement was one command from being walked around |
| [The decision queue's banner is interim, and MUS-D-0041 still stands](#the-decision-queues-banner-is-interim-and-mus-d-0041-still-stands) | The claim was corrected, not the code |
| [The tool call has to know every kind, and now cannot forget one](#the-tool-call-has-to-know-every-kind-and-now-cannot-forget-one) | A hand-written list made "every record" false |
| [An answer is a choice between options, not a text box](#an-answer-is-a-choice-between-options-not-a-text-box) | A box makes the owner rebuild the list the asker had |
| [The queue was rebuilt because the plan was routed around, not because it was wrong](#the-queue-was-rebuilt-because-the-plan-was-routed-around-not-because-it-was-wrong) | Recording a failure did not stop it repeating |
| [The tab bar carries only the surfaces that exist](#the-tab-bar-carries-only-the-surfaces-that-exist) | A tab to nowhere is an unbuilt capability, described |
| [The tab bar is MUS-D-0041's, built two tabs at a time](#the-tab-bar-is-mus-d-0041s-built-two-tabs-at-a-time) | Corrects the row above, which overrode the owner by prose |
| [The stack table gains a named exception rather than losing its rule](#the-stack-table-gains-a-named-exception-rather-than-losing-its-rule) | A reopening, dispositioned forward by the owner |
| [Milestone 4 is two milestones](#milestone-4-is-two-milestones) | 4a the adapter and injection, 4b the browser tab |
| [Three timestamps were typed rather than read](#three-timestamps-were-typed-rather-than-read) | The unmeasured-number gate, applied to a clock |

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

### Nothing vendors StrucGu

The audit reads its modules from a StrucGu checkout — beside this repository, or
wherever `MUSTUR_STRUCGU` points — and refuses to run when it cannot find one.

Copying the modules in here would make the audit work everywhere with no second
checkout, and that is the whole argument for it. Against it: the adoption record
pins exact versions, StrucGu's propagation is deliberately pull-only, and a
vendored copy drifts with nothing to notice — a conformance harness run against
a stale copy proves conformance to nothing at all.

The version pin is read the way the specification says rather than the way it
first looks. A checker evaluates the module as it reads it; the pin records what
the adopter agreed to and reports drift as a notice pointing at that module's
changelog. What a checker refuses is a *floating* pin, because a new check never
ships in a minor version and accepting a range would let upstream publish
findings into every adopting repository at will.

Where the catalog comes from in an unattended run is a separate question, and
[the owner answered it](#the-audit-runs-in-ci-against-a-real-catalog).

## 2026-08-20 — decisions the review sent to the owner

Milestone 2b's review found two questions that had been settled in prose
instead of asked. Both went back as prompts and both are the owner's answers.

### The audit runs in CI, against a real catalog

The workflow checks out [StrucGu](https://github.com/DevOfPie/StrucGu) beside
this repository at a pinned ref, and the tests read `MUSTUR_STRUCGU` the way the
command already did.

Before this, the conformance harness found its catalog only as a sibling
directory and skipped otherwise — so `go test ./...` printed `ok` in CI with all
344 assertions unrun, and a green build was indistinguishable from a covered
one. The evidence this milestone rests on had run once, on one machine.

Vendoring a copy of the modules was the alternative and was rejected for the
reason [the entry above](#nothing-vendors-strucgu) gives: a pinned copy of
somebody else's specification goes stale silently, and a harness run against a
stale copy proves conformance to nothing. The cost accepted instead is a
workflow change the owner has to apply, and a CI run that depends on a second
repository being reachable.

The skip stays, for a machine without the catalog, but it now says what did not
run rather than passing quietly. Silence and *did not look* are the same string.

### The record roles are mapped at the export

`decision_log`, `findings` and `investigations` now point at
[records/](records/README.md) rather than at `decisions.md`, `queue.md` and
`docs/investigations/`. `triage_doc` stays at [workflow.md](workflow.md), which
is a rule rather than a record and which no export produces.

[Plan.md](Plan.md#scope) promises a conformance audit over the records Mustur
owns. Mapped at the prose, the audit read this repository's contract files —
a different claim, and one milestone 2b was not for. The exclusion that kept the
export out of its own audit was written during milestone 2 to stop entries being
reported twice; with the roles moved, it would instead leave every role with
nothing to read.

Three consequences, all of them real:

1. **The export has to satisfy checks written for hand-authored files.** It now
   carries an index of decisions, an evidence and a reviewed column on findings,
   and investigations as a directory of records with a template — because those
   are the shapes the modules describe. Where a record has nothing behind a
   column the cell is empty rather than filled, which is the outcome to want: a
   cell filled to satisfy a check says the opposite of what is true and is the
   same length.
2. **The queue and the record had to be reconciled.** `queue.md` is the loose
   intake and `records/findings.md` is the record; six lines that had never
   become records now have identifiers, and this file's triage section says
   which is which.
3. **`DL-03` reports every regeneration.** The decision log is now generated,
   so a rewrite removes lines every time and the check cannot tell that from an
   erasure. It is [accepted as a deviation](strucgu.yaml) with a reason and an
   expiry rather than ignored: the property `DL-03` protects is enforced
   upstream and more strongly, by a database that refuses `UPDATE` and `DELETE`.

### The store holds more than it did

Corrects [The store holds records, the contract files keep their prose](#the-store-holds-records-the-contract-files-keep-their-prose),
which enumerates twenty-seven decisions, nine milestones, six findings and one
investigation. That was true when written and is not now: the store holds
thirteen findings, and the decisions have kept accruing. The earlier entry
cannot be edited, so this is the pointer.

The enumeration is the part that went stale, not the reasoning. What that entry
decided — that a record summarises the prose it points at rather than copying it
— still holds, and this milestone did not change it. The lesson is narrower: a
count in an append-only file is a claim with an expiry date on it, and the
export's own index is the place a reader should look instead.

## 2026-08-20 — decisions taken while building milestone 2c

### The database is the source, and the seed is history

The owner's call, from three offered shapes. `mustur add` writes to the store;
[records/](records/README.md) is regenerated from the store and committed; the
bootstrap in `internal/seed/data/` stays as the record of what was imported and
stops being the thing the export is derived from.

Until now the export was reproducible from git alone, because it was rendered
from the seed and the seed is a file. That was never a property anyone chose —
it was a side effect of there being no way to write a record, which
`MUS-F-0007` recorded as the defect it is. Intake cannot exist without a write
path, so the milestone forced the question.

What it costs is the guarantee CI could give. Nothing in an unattended run can
regenerate the export and compare it, because the database is not in the
repository and will not be — a binary file in git is a record nobody can review.
What remains is `mustur verify --db`, which reports the tree differing from the
store, and the diff in the pull request, which is what a reader who does not run
the binary actually reads. Both were already the argument for committing the
export in the first place.

What it buys is that the insert-only triggers mean something. Under the
alternative — the JSON staying authoritative and the database a cache of it —
anyone could edit a record by editing a file, and the database refusing `UPDATE`
would be decoration. A guarantee enforced at the only layer nobody has to
remember is the whole reason for choosing SQLite.

The bootstrap is not deleted, and it is not maintained either. It says what the
store held on the day it was imported. `mustur seed` still refuses a store that
already holds records, so the two cannot silently diverge into a second source.

### The idea inbox is a routing target inside Mustur

The owner's, asked and answered while building intake. A jot with no obvious
destination goes to `MUS-P-0002`, a routing record here, and not to
IdeaWarehouse's `inbox.md`.

The alternative reads better in the milestone's own wording — "defaults to the
idea inbox" — and is forbidden. No file in any other project is touched before
that project is deliberately onboarded, and a capture surface whose *default*
path writes into another repository would break that rule on its first use,
every time, for the case that is meant to be ordinary rather than exceptional.

So the inbox is a record. Which record is the fallback is a field on it,
`Intake: default`, rather than a constant in the code — the registry is data,
and a fallback compiled in is one a reader cannot see and the owner cannot
move.

### A jot is filed without a decision

Naming a thing requires understanding it, and at capture time you do not. So
nothing about filing asks for a choice:

- **The title is derived** from the first line or sentence, truncated with an
  ellipsis that says it was truncated. A record still needs a claim a listing
  can show; asking for one at capture is asking for the decision.
- **The destination is guessed**, from the routing records the store already
  holds, matched on word boundaries so a name inside a longer word does not
  route a jot about something else.
- **The guess is recorded as a guess.** Every filed record carries where it
  went and why, in its own words: "the jot names DevOfPie/Mustur", or "no
  destination is obvious", or the ambiguous case in full.

That last one is the shape worth arguing for. Two matched destinations is not
an obvious hint, it is an ambiguous one, and picking between them would be
exactly the decision this surface refuses to ask for — so it falls back and
*says which two it saw*. An ambiguity reported as "nothing obvious" would hide
the one case where the routing registry needs an alias.

A project lists its repositories, so a jot naming "Mustur" matches both. That
is not an ambiguity: it is the same place at two scales, and the narrower one
wins. A container is recognised by naming the other's identifier in its own
fields rather than by a rule about kinds — the registry is data, and code that
knew projects contain repositories would be a second place to keep that true.

The surface fetches nothing: no external stylesheet, no script, no font, no
image. Its styles are a handful of inline rules, which an earlier version of
this entry called "no stylesheet" and a review corrected — the property that
matters is that a phone makes one request, not that the page is unstyled.

Measured on loopback, ten filings: median 0.5 ms, worst 0.9 ms; a review
re-measured independently at 0.35 and 0.55. The page is 3,071 bytes empty and
4,112 after ten filings, because the recency list grows; the single figure this
entry first gave was one reading of a transient page and could not be
reproduced. None of it is the fifteen-second claim, which is about a phone off
the home network and cannot be measured until the ingress exists.

## 2026-08-20 — the surfaces plan's four questions, answered

All four are the owner's, answered against
[the published plan](https://plan.agent-native.com/plans/plan-4827b50a72674a22)
rather than in prose. Three matched the recommendation; one did not, and that
one changed three artboards.

### Records read as a document

An identifier expands in place — no round trip to an agent, no new tab. The
counts down the left are the only navigation.

[docs/ui-surfaces.md](docs/ui-surfaces.md) called this unresolved and said it
changes the whole surface, which was right: the alternative was a graph, with the
citation structure as the primary object. Records here do cite each other
densely enough for that to be tempting — thirty-nine decisions, several of which
exist only to correct another.

What the document reading costs is exactly that: the citation structure is never
the primary object, and a reader tracing why one entry corrects another walks it
one expansion at a time. What it buys is the original complaint, answered
directly — a human meeting a bare identifier expands it in one action.

### The phone bar has four tabs

Sessions, Decisions, Intake, Records. Decisions carries a count when anything is
open and nothing when it is not.

Against the recommendation, which was three tabs with the decision queue reached
from a banner on the session list. The owner's reasoning holds better than mine:
an open decision is work stopped, and a banner on another screen is a thing that
can be scrolled past. A fixed place the eye already knows to check is worth more
than the quarter of a bar it occupies.

The cost is real and stated: a tab that is empty most of the time. What it shows
on those days is the one question the answer left open.

### The routing guess is shown before filing

As a chip carrying the destination, tappable to change.

This is the closest thing to a contradiction in the intake surface, and it is
worth naming rather than smoothing over: that surface exists to never ask for a
decision at capture time, and a control offering a choice is a decision on the
screen. What makes it safe is that the chip is a default already filled in —
ignoring it files exactly what the shipped version would have filed, and tapping
it is available only to somebody who already knows the answer.

The alternative was leaving the guess recorded and invisible, which is what
shipped. That defers every wrong route to a correction made later, from a
different surface, by somebody who has to notice it first.

### The audit is a page

The same run the command emits, rendered read-only.

A waiver nobody sees is a check that silently stopped running, and nobody runs
`make audit` on a phone. The cost is one more surface to keep true around
something that already works from a terminal.

## 2026-08-21 — decisions taken while wiring the ingress

### Mustur runs as a systemd user unit

The owner's call, from three shapes. `deploy/mustur.service` runs as the account
that owns the store, needs no root, restarts on failure, and starts at boot
because that account already has lingering enabled. It is the same shape the
Remote Control service on this machine already uses.

The alternative considered was a system unit with `DynamicUser`, like the
tunnel beside it. Stronger isolation, and it would have meant moving the store
out of a home directory first — a change to where the record lives, made for the
convenience of a service file, which is the wrong order.

**The unit is installed and deliberately not enabled.** The tunnel already
routes `mustur.devofpie.com` to `127.0.0.1:7777`, so enabling the service is the
act that publishes the intake box. The surface reads the filer's identity from a
header Cloudflare Access sets at the edge, and `cloudflared` passes client
headers through — so until an Access application exists, anyone reaching that
hostname could file a jot and claim to be anyone. Starting first and securing
after would mean a window where that is true, and the window is the whole risk.
[docs/ingress.md](docs/ingress.md) carries the order and the check that confirms
it.

The unit is confined to what it needs: `ProtectSystem=strict`, `ProtectHome` set
to read-only, and exactly two writable paths — the store and the export tree.

## 2026-08-21 — corrections the milestone 2c review forced

### The unit is enabled, and the entry saying it is not stays put

The entry above says the unit is installed and deliberately not enabled, because
enabling it is what publishes the intake box and Access is not in front of the
hostname yet. It was true when it was written and false by the end of the same
branch: the owner added the Access application, the gate went up before anything
listened, and the unit was enabled. Entries are never edited, so it stays as
written and this is the later entry that corrects it. `MUS-D-0045` is the record.

**The ordering it argued for was right and is not being abandoned.** What
changed is that it is now stated as a rule about any host rather than as a note
about this one, in `deploy/mustur.service` and in
[docs/ingress.md](docs/ingress.md): never enable the unit on a host whose
hostname is not already behind Access. A note about a state passes out of date
the moment the state does; a rule does not.

**`Restart=on-failure` was wrong.** systemd counts a `SIGTERM` as a clean exit,
so a TERMed process was left dead with no restart scheduled. That is not
hypothetical — a reviewer's `pkill -f "mustur serve"`, aimed at its own throwaway
instance, matched this unit's `ExecStart` and took the production service down,
and nothing brought it back or noticed. It is `Restart=always` now, confirmed by
sending the main process a TERM and watching it return with `NRestarts=1`.

The reason nothing noticed is the more useful half. The check
[docs/ingress.md](docs/ingress.md) offers is a `curl` against the public
hostname, and **Cloudflare Access answers 302 whether or not the origin is up** —
it is a gate check, not a health check. The file also had 502 backwards: 502
means Access is in front and the origin is down, not that Access is gone. Both
readings are stated there now, and nothing yet watches the origin.

**The fifteen-second claim is still unclaimed, for a different reason.** The
entry above says it cannot be measured until the ingress exists. The ingress
exists. It is unclaimed now because only the owner can get through Access to file
from a phone, which is the one clause of milestone 2c that this repository cannot
close on its own.

What did get measured, with its method, is the loopback leg: ten filings by
`curl` against a `--export` server, `%{time_total}` each, median 1.71 ms and
worst 2.10 ms. That replaces a bare **20 ms** which named no instrument and was
cited to a record whose Evidence field is empty.

## 2026-08-21 — decisions taken while building milestone 3

### Injection belongs to the milestone that owns sessions

Milestone 3's done-when ended "and the answer is injected back into the session
that raised it". Nothing in milestone 3 can do that. [The decision that sessions
are supervised through tmux](#sessions-are-supervised-through-tmux) fixes that
Mustur never attaches to a session it did not start — the arrow only points the
other way — and the adapter that starts sessions arrives at milestone 4.

So the clause required milestone 4's machinery to sit inside milestone 3. That
is a conflict between [Plan.md](Plan.md)'s milestone ordering and this file, and
[workflow.md](workflow.md) says a conflict between them is a bug to report
rather than a thing to pick. It went to the owner as a prompt.

**Their answer: split it.** Milestone 3 is the block and the queue. Delivering
an answer back into the raising session moves to milestone 4, beside the
adapter that makes it possible. Both rows in Plan.md now say so.

What this costs is worth naming: until milestone 4, an answer sits in the store
and on the queue page, and the agent that asked has to be told by whoever is
running it. The gate does not depend on the answer arriving — it turns on the
question having been *asked* — so nothing is blocked by the gap. What is lost is
the round trip, not the enforcement.

The alternative the owner did not take was pulling the adapter into milestone 3,
which honours the sentence as written and takes the core of milestone 4 with it.
A third option, answering through the next `mustur_route` call, was offered and
not taken; it would have kept the round trip without an adapter, at the price of
the answer waiting for the next session rather than reaching the blocked one.

### A question is its own kind, and only some become decisions

An open question needed somewhere to live. The owner's first instinct was a
status field on decision records — questions become decisions, so let the record
start open and close answered.

Argued against, and the owner took the argument. Three things break it. Some
answers are instructions rather than decisions: two of the four prompts raised
during the milestone 2c review were about what to do next, and neither is a
durable statement about why the project is the way it is. Some questions never
resolve at all — withdrawn, overtaken, or never answered because the owner was
away — and a decision record that never becomes a decision is a contradiction.
And this file is append-only, by [workflow.md](workflow.md)'s standing rules: a status flipping from
open to answered is an edit to the one kind that forbids edits. Findings are
amended freely, so the pattern exists everywhere except the one place this
would have put it.

So `MUS-Q-…`, exported to `records/questions.md`, carrying open, surfaced and
answered. When an answer is a real decision, a decision entry is appended citing
the question. When it was only an instruction, nothing is, and the question
still says what was asked and what came back.

The cost is a role letter and the export, verify and adoption surface that comes
with it. There is [a queue line](queue.md) asking whether Mustur should be
adding record kinds at all, and this is the second one to arrive without that
question being settled.

### The gate turns on being asked, not on being answered

`make check` fails while an open question has never been surfaced as a prompt.
It does not fail on a question that was surfaced and is still waiting.

The distinction is the whole design. An owner who is asleep, or on a plane, must
not stop the work — and a gate that waited for answers would teach whoever hit
it to stop asking. What is being enforced is that the question reached the
surface where questions get seen, which is the failure the milestone is named
after: three open decisions written into a pull request body, with options and
consequences, that the owner correctly called useless.

A missing store **skips and says so**. It must never pass: "there was no store
to read" and "no question was buried" are different facts, and reporting the
second when the first is true is the substitution `DL-03` already made once in
this repository, in CI, on a shallow clone.

## 2026-08-21 — what the milestone 3 review changed

### The gate reads the tree, not the store

`make check`'s question gate read the SQLite store. That was wrong in a way
worse than not having the gate at all.

[workflow.md](workflow.md) requires every gate to run offline against the
working tree. The store is machine-local, so on a clone and in CI the check
could only skip — while [CLAUDE.md](CLAUDE.md) told every session, without
qualification, that work could not be reported complete around an open question.
A rule stated absolutely and enforced on one machine is worse than one stated
conditionally, because nobody knows to check.

It was also unsound where it did run. The gate could not tell *no store* from
*no buried question*: `openStore` creates a store that is not there, so a gate
pointed at a fresh path exited 0. The Makefile's file test was the only thing
that could have caught that, and it probed `${MUSTUR_DB:-$HOME/…}` while the
binary resolved `MUSTUR_DB` → `$XDG_DATA_HOME/mustur/…` → `$HOME/…`. With
`XDG_DATA_HOME` set, the guard found the real store, the binary made an empty
one, and the gate printed **ok**. Two reviewers reproduced it independently. A
zero-byte file at the path did the same.

The entry above claiming a missing store "skips and says so; it must never pass"
stays as written and is corrected here: it was true of the branch that never
ran, and the skip it describes exited 0, which at the level that decides whether
`make check` is green is not distinguishable from passing.

Against `records/questions.md` none of it arises. There is nothing to skip — an
absent or empty file is the tree saying there are no questions, which is a fact
rather than a gap — and CI, a clone and the reviewer all read the same file.

### An answer is required when the work depends on it

The rule was built as: surfacing is enough, and an unanswered question never
blocks. The owner ratified it with a qualification that changes the design —
being asked is enough **"as long as the work it is doing doesn't depend on the
question's answer"**.

So a question can be marked `--needed`, and the gate will not pass on surfacing
alone for those. Reporting work complete that turned on an answer nobody gave is
the same lie as never having asked; the remedy is the one
[workflow.md](workflow.md) already gives, which is to do everything independent
of the answer first and leave the dependent part unreported.

This corrects the entry above, which recorded the unqualified rule as settled.
It was also a decision the builder took and wrote down without asking — inside
the milestone whose entire subject is not doing that. It has since been put as a
prompt and is recorded as `MUS-Q-0005`.

### A question may be withdrawn by its raiser, never answered by them

`mustur answer` took any actor, so the agent that raised a question could close
it and the gate would stop seeing it: the whole enforcement walked around in one
command. The owner's answer, `MUS-Q-0007`: refuse self-answer, allow
self-withdraw. Withdrawing your own question is honest — it is overtaken, or no
longer worth asking, and the record goes on saying it was asked. Supplying your
own answer is not.

The raiser is recorded on the question at `ask` time for this reason.

### The decision queue's banner is interim, and MUS-D-0041 still stands

[MUS-D-0041](#the-phone-bar-has-four-tabs) rejected a decision queue reached
from a banner, because a banner on another screen can be scrolled past and a
fixed place the eye already knows to check is worth more. Milestone 3 shipped a
banner on the intake box and gave as its reason the argument that decision
overruled.

The owner's answer, `MUS-Q-0006`: fine as an interim, fix the rationale. The tab
bar arrives with the session list at milestone 4 and the banner moves then. What
is corrected is the claim, not the code: the comment no longer argues for a
banner, it says the banner is what exists until the thing MUS-D-0041 chose does.

### The tool call has to know every kind, and now cannot forget one

Adding the `question` role letter silently broke a claim milestone 2 shipped:
`mustur_route` describes its own reply as "an index of every record", and the
kind list it iterated was written out by hand, so questions were absent from
every default index. The mandated call — the one thing every session makes
before anything else, and the obvious place to learn that a question is open —
was the single surface that could not see them.

The list is now derived from `ident.Roles`. The one place that cannot be, the
JSON schema on a struct tag, is asserted against `ident.KindNames` by a test, so
the next role letter cannot repeat this quietly.

## 2026-08-21 — the decision queue is rebuilt from its artboard

### An answer is a choice between options, not a text box

The queue shipped at milestone 3 offered a text field. Its artboard in
[the published plan](https://plan.agent-native.com/plans/plan-4827b50a72674a22)
offers a short list of options, each with one line saying what it costs, one of
them marked recommended, and the paragraph behind each shown only when asked
for.

The drawing is right and the reason is worth stating plainly: **a text box makes
the owner reconstruct the options the asker already had.** An agent blocked on a
decision has just finished weighing the alternatives — that is why it is
blocked — and throwing them away at the surface means the owner does the work
twice. It is also the shape every decision in this repository has actually
arrived in, so the queue now matches the thing it is a queue for.

Free text stays, beneath the options, and beats a chosen one when both are sent.
The case a list of options is worst at is the owner wanting to say something the
list does not contain, and that must never be the case the surface refuses.

Options are stored as repeated fields on the question, `Label :: one line ::
the paragraph`. The separator is not a pipe, because a pipe would break the
table the export renders these into.

### The queue was rebuilt because the plan was routed around, not because it was wrong

The surface met its brief. It was rebuilt because it was written *from* the
brief, when a published plan with an artboard for it already existed — the same
route intake took, and the thing publishing the plan was meant to stop.

The owner's answer on MUS-Q-0010: "Redesign it from the plan's artboard. The
built queue met its brief, but a plan agents route around is not a plan."

Recorded because the sequence repeated after being written down once already.
[docs/ui-surfaces.md](docs/ui-surfaces.md) said, in its own first paragraph,
that intake had been built this way and should not have been — and the next
surface was built the same way regardless. **The record was not the safeguard.**
What would be is nobody having built the fifth, sixth and seventh surfaces yet.

### The tab bar carries only the surfaces that exist

The drawing has four tabs: Sessions, Decisions, Intake, Records. Two of those
surfaces are not built, and a tab pointing at one would be an unbuilt capability
described as existing, which [workflow.md](workflow.md) gates against. The bar
renders the two that exist and grows as the others arrive.

The count is spelled out rather than shown as a badge, which is the drawing's
own note: a badge holding one character reads as an unexplained dot at this
size.

## 2026-08-21 — what the queue rebuild's review changed

### The tab bar is MUS-D-0041's, built two tabs at a time

The entry above, "The tab bar carries only the surfaces that exist", is correct
about the code and wrong about its own authority. It attributed four tabs to the
drawing. Four tabs is **MUS-D-0041**, which is the owner's, taken *against* a
recommendation of three — so reducing the bar to two overrode an owner decision,
and it was written down instead of asked.

Put to the owner on MUS-Q-0012. Their answer: **two now, growing to four.**
MUS-D-0041 stands unchanged; the bar renders the surfaces that exist and gains
tabs as the rest arrive. This entry is the correction the earlier one should
have been.

It also corrects [MUS-D-0053](#the-decision-queues-banner-is-interim-and-mus-d-0041-still-stands),
which says "the tab bar arrives with the session list at milestone 4 and the
banner moves then". A bar exists at milestone 3, on the queue. The banner has
not moved and still stands, because intake has no bar of its own — so the half
of that sentence about the banner holds and the half about milestone 4 does not.
Both entries stay as written; this one is where they are corrected.

### The stack table gains a named exception rather than losing its rule

`Plan.md`'s stack table said the human interface is server-rendered HTML with no
per-project client state. That row is on `main`. MUS-Q-0008's answer — WebSocket
for the session surface — reverses it for one surface, prospectively.

It went to the owner as a reopening, because a falsified claim of a shipped
milestone is scheduling and scheduling is theirs. Their answer on MUS-Q-0011:
**correct the row forward, no reopening.** The row now reads server-rendered by
default with a client layer only where a surface streams, and names the
exception rather than dropping the rule. Milestone 2 stays passed.

[MUS-F-0019](records/findings.md) cites the old row as its evidence for shipping
intake's destination row differently from its approved artboard. That reasoning
survives: intake does not stream, so the default still governs it.

### Milestone 4 is two milestones

MUS-Q-0009's answer split it: **4a** is the adapter, tmux supervision and the
answer delivered back into the session that raised it; **4b** is streaming a
session to a browser tab. 4a is unblocked and carries the clause milestone 3
handed over. 4b is where the client-layer exception above is spent.

Recorded here because the answer landed two commits ago and changed nothing —
`Plan.md` and the milestone records went on describing one undivided milestone,
which is the same Plan-versus-records drift MUS-Q-0001 was raised about and
MUS-D-0046 was supposed to have taught.

### Three timestamps were typed rather than read

MUS-Q-0008, 0009 and 0010 recorded `Surfaced 05:40` and `Answered 05:46`. The
export that would have contained them ran at 09:25 and did not. Those times were
passed to `--at` because the flag accepts them, not because anything was
measured.

That is the no-unmeasured-number gate applied to a clock, and it is worse than
an unmeasured number because a timestamp reads as a reading by construction. The
dates are right and the minutes were invented. Recorded rather than quietly
adjusted, because a corrected fabrication and a real measurement look identical
afterwards.
