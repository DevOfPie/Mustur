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
| [tmux is the source of truth for what is running](#tmux-is-the-source-of-truth-for-what-is-running) | No mirror, and no reconstruction when the server dies |
| [Mustur can type into a session it started](#mustur-can-type-into-a-session-it-started) | The capability named, and the three limits on it |
| [A project name may not address a window or a pane](#a-project-name-may-not-address-a-window-or-a-pane) | tmux reads : and . as target separators |
| [An undelivered answer is still an answer](#an-undelivered-answer-is-still-an-answer) | Delivery never fails the answer |
| [There is no `session attach`](#there-is-no-session-attach) | A verb there would suggest the arrow points both ways |
| [Ownership is provenance, not a name](#ownership-is-provenance-not-a-name) | A review adopted Mustur's own sessions with one tmux command |
| [The word was "supervises" and the thing was not built](#the-word-was-supervises-and-the-thing-was-not-built) | Asserted in three places, implemented in none |
| [An answer that reached the server is the owner's](#an-answer-that-reached-the-server-is-the-owners) | A dropped connection was losing the answer entirely |
| [The session that raised a question and the session an answer can reach are different things](#the-session-that-raised-a-question-and-the-session-an-answer-can-reach-are-different-things) | A field redefined under records already written |
| [There is no `session send`](#there-is-no-session-send) | It made "the only caller is the answer path" false |
| [The unit cannot have a private /tmp](#the-unit-cannot-have-a-private-tmp) | tmux's socket lives in /tmp, so the feature shipped inert |
| [v1 has eight surfaces, and the eighth was found by trying to build it](#v1-has-eight-surfaces-and-the-eighth-was-found-by-trying-to-build-it) | The session list is not a session's output |
| [The standing instruction is what stopped 4b starting](#the-standing-instruction-is-what-stopped-4b-starting) | Deleted by one commit, restored by a review, earned it at once |
| [Sub-agents are their own milestone, and it starts by finding out whether it is possible](#sub-agents-are-their-own-milestone-and-it-starts-by-finding-out-whether-it-is-possible) | A verdict of "cannot be done" is a real outcome |
| [The session composer is always writable](#the-session-composer-is-always-writable) | And what that puts on the origin check |
| [A session's exit is an event, not a record](#a-sessions-exit-is-an-event-not-a-record) | The line already drawn around session output |
| [A surface reachable only when it has something to say is not reachable](#a-surface-reachable-only-when-it-has-something-to-say-is-not-reachable) | The owner found it by loading the site |
| [The reader lingers after the last viewer leaves](#the-reader-lingers-after-the-last-viewer-leaves) | Otherwise a dropped phone loses the session's continuity |
| [The buffer is seeded from the pane, not only from the pipe](#the-buffer-is-seeded-from-the-pane-not-only-from-the-pipe) | pipe-pane carries nothing that happened before it |
| [A WebSocket library rather than hand-rolled framing](#a-websocket-library-rather-than-hand-rolled-framing) | Not on the path that types into an agent |
| [The origin check refuses a handshake with no Origin at all](#the-origin-check-refuses-a-handshake-with-no-origin-at-all) | It is the control, not hardening |
| [The answer path is no longer the only caller, and the entries saying it is stay put](#the-answer-path-is-no-longer-the-only-caller-and-the-entries-saying-it-is-stay-put) | One of MUS-D-0063's three limits survives |
| [The idle timeout was a lie about the session](#the-idle-timeout-was-a-lie-about-the-session) | Thirty minutes of a live session read as ended |
| [A viewer that falls behind is disconnected, not quietly starved](#a-viewer-that-falls-behind-is-disconnected-not-quietly-starved) | Skipping left it 8 MB behind with no notice |
| [The bar grows in three templates, not one](#the-bar-grows-in-three-templates-not-one) | Three different bars in one binary |
| [The exit is logged, and supervision only runs while somebody is watching](#the-exit-is-logged-and-supervision-only-runs-while-somebody-is-watching) | A session nobody opened is not being watched |
| [The investigation was registered before it was run](#the-investigation-was-registered-before-it-was-run) | Why the rule preceding the finding is the evidence |
| [The route is lifecycle hooks, and the pane is untouched](#the-route-is-lifecycle-hooks-and-the-pane-is-untouched) | How a sub-agent is found, and what it costs nothing |
| [The hook is passed per session, and persists nowhere — MUS-Q-0024](#the-hook-is-passed-per-session-and-persists-nowhere--mus-q-0024) | Where the hook lives, and what it never writes to |
| [A row shows what is documented — MUS-Q-0025](#a-row-shows-what-is-documented--mus-q-0025) | What a sub-agent row carries, and what it declines to |
| [The gate that failed again, and what it cost this time](#the-gate-that-failed-again-and-what-it-cost-this-time) | Five decisions taken in prose, and the one that mislabelled a row |
| [Three defects the fake runner could never have shown](#three-defects-the-fake-runner-could-never-have-shown) | Rows outliving their session, a tool never left, a launch queued twice |
| [The records were the fiction, not the prose](#the-records-were-the-fiction-not-the-prose) | A generated export hand-edited, with every gate still green |
| [A third invented citation, in the third place the verifier cannot look](#a-third-invented-citation-in-the-third-place-the-verifier-cannot-look) | Identifiers asserted in Go comments, where nothing checks them |
| [Evidence a reader can reach](#evidence-a-reader-can-reach) | The harness that reproduces the investigation's numbers |
| [Live rows, against the recommendation](#live-rows-against-the-recommendation) | Sub-agent rows pushed down the socket, and what that widened |
| [What changed](#what-changed) | A routing record naming its own identifier prefix |
| [Narrow on purpose](#narrow-on-purpose) | One target now, and why generalising waits for milestone 7 |
| [`MUS-F-0025` keeps its name](#mus-f-0025-keeps-its-name) | Why a misleading prefix is cheaper than an exception to permanence |
| [The design question was answered, and this repository went on calling it open](#the-design-question-was-answered-and-this-repository-went-on-calling-it-open) | Thought first, destination second, and three days of staleness |
| [One draft, not one per session](#one-draft-not-one-per-session) | Why the draft follows the owner rather than the page |
| [It is not a fourth tab, and not a second scripted page](#it-is-not-a-fourth-tab-and-not-a-second-scripted-page) | Where the composer lives — superseded by MUS-Q-0034 |
| [A newline typed into a terminal is Enter](#a-newline-typed-into-a-terminal-is-enter) | Why multi-line goes as a bracketed paste |
| [An hour lost to `timeout`](#an-hour-lost-to-timeout) | A test fixture that starved its own pane on SIGTTIN |
| [The second review pass, and two of its findings](#the-second-review-pass-and-two-of-its-findings) | A correction is a claim, and nothing gates claims |
| [Timestamps that could not be true](#timestamps-that-could-not-be-true) | Answers stamped before their questions existed |
| [Amend replaces, and I have now been caught by that twice](#amend-replaces-and-i-have-now-been-caught-by-that-twice) | A record re-dated and stripped of provenance by its own correction |
| [Who may read the records is not itself a record](#who-may-read-the-records-is-not-itself-a-record) | Why accounts live beside the log rather than in it |
| [The gate is a flag, and the flag is off](#the-gate-is-a-flag-and-the-flag-is-off) | Enforcement turned on before a passkey exists locks the owner out |
| [The ceremony needed a client, not a browser](#the-ceremony-needed-a-client-not-a-browser) | A virtual authenticator, and the test that was passing for the wrong reason |
| [A drawing found a bug no test would have](#a-drawing-found-a-bug-no-test-would-have) | Discoverable credentials, caught by a comment on a picture |
| [The count that nobody drew](#the-count-that-nobody-drew) | Two reviewers, two wrong replacements, one structural cause |
| [Four surfaces built before they were drawn, and the pattern has a number now](#four-surfaces-built-before-they-were-drawn-and-the-pattern-has-a-number-now) | Seven instances, a rising rate, and MUS-F-0027 |
| [What the review caught in the code](#what-the-review-caught-in-the-code) | Three fixed, one disputed with the measurement |
| [A token is not an account, and that is the point](#a-token-is-not-an-account-and-that-is-the-point) | Scope is what makes a weaker secret acceptable |
| [No expiry, deliberately](#no-expiry-deliberately) | A credential that stops at 3am is an outage, not a control |
| [The guard trusts one thing about the tool surface](#the-guard-trusts-one-thing-about-the-tool-surface) | Why a second MCP tool fails a test in another package |
| [The id is hex because it is typed at a shell](#the-id-is-hex-because-it-is-typed-at-a-shell) | About one id in 64 would have been unrevokable |
| [A test that passed for the wrong reason, again](#a-test-that-passed-for-the-wrong-reason-again) | Twice in two milestones; mutation found both |
| [A fourth difference between the mandate and what milestone 1 scored](#2026-08-25--a-fourth-difference-between-the-mandate-and-what-milestone-1-scored) | The entry has now been wrong about its own completeness twice |
| [A token's lifetime, handed back to the owner](#2026-08-25--a-tokens-lifetime-handed-back-to-the-owner) | An answered decision overridden by the party who asked it |
| [The owner found the bug the whole suite agreed with](#2026-08-26--the-owner-found-the-bug-the-whole-suite-agreed-with) | No synced passkey could sign in; the test double shared the mistake |
| [Building the bar found what the code had already decided](#2026-08-26--building-the-bar-found-what-the-code-had-already-decided) | The stylesheet and the script were both written for a shell nobody built |
| [A browser, and the two things it found](#2026-08-26--a-browser-and-the-two-things-it-found) | Measuring the wrong thing comes with confidence attached |
| [A picture, and the thing about it that travels](#2026-08-26--a-picture-and-the-thing-about-it-that-travels) | The export is public; the description goes, the pixels stay |
| [A place to put something that was never meant to be kept](#2026-08-26--a-place-to-put-something-that-was-never-meant-to-be-kept) | A test filing should not advance a counter |
| [The log below this line is generated](#2026-09-03--the-log-below-this-line-is-generated) | From MUS-D-0121 the entries are rendered from the store |

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

## 2026-08-21 — decisions taken while building milestone 4a

### tmux is the source of truth for what is running

tmux already knows which sessions exist, which are alive and what they last
printed, and it knows it a second before any mirror would. So nothing in the
adapter keeps a table of sessions: listing is a live query, and the store holds
only what outlives a session — which project it was for, and any question it
raised.

The owner's call on MUS-Q-0013, against mirroring into the store and against a
third store built for live state. Mirroring means two sources that can disagree,
and an insert-only log gaining a row every time a session changes state is a lot
of events for a fact that is true for one second.

**The cost, accepted rather than discovered:** when the tmux server dies, every
session dies with it and Mustur knows nothing about what was running. There is
no reconstruction, because there was never a second copy. What survives is what
was already a record.

### Mustur can type into a session it started

An answered decision reaches the session that raised it by `tmux send-keys` —
the adapter types it in, and at the far end it is indistinguishable from the
owner having typed it. The owner's call on MUS-Q-0014, against landing the
answer in a file for the next mandated call to pick up, which they had already
declined once because it delivers to the next session rather than the blocked
one.

**This is a capability worth naming plainly rather than discovering later:
Mustur can put words into an agent's input.** Three things follow, and all three
are enforced rather than documented:

- It only ever types into a session it started. `Send` refuses anything without
  the `mustur/` prefix, which is the same rule as never attaching.
- The text says what it is: *"The owner answered MUS-Q-0001: …"*. An agent that
  could not tell an answer from a fresh instruction would treat every answer as
  a new task, and nothing should read Mustur as the author of a decision it only
  carried.
- The only caller is the answer path.

The text is sent with `-l`, literally, so an answer containing something tmux
would otherwise read as a key name arrives as characters. `Enter` is a separate
call, or it would be typed as a word.

### A project name may not address a window or a pane

tmux reads `:` and `.` as target separators, so a project called `Mustur:0`
would address a window rather than a session — and `send-keys -t` would deliver
somewhere nobody chose. Project names are letters, digits, dash and underscore,
refused at the boundary rather than escaped.

### An undelivered answer is still an answer

Delivery never fails the answer. If the session has gone, or tmux is not there,
or the pane died mid-write, the answer is recorded anyway and the record says
what happened to the delivery.

The alternative — refusing to record an answer that could not be carried —
would lose the one thing that was not recoverable, in order to keep the record
tidy about the one thing that was. And a silent failure would leave an agent
blocked on an answer that exists, which is the failure this whole milestone is
named after, one layer along.

### There is no `session attach`

A person wanting to watch a session Mustur started runs `tmux attach -t
mustur/<project>`, which works and always did. There is no verb for it here,
because a verb here would suggest the arrow points both ways. It does not:
Mustur starts sessions and never attaches to one it did not start.

## 2026-08-21 — what the milestone 4a review changed

### Ownership is provenance, not a name

The adapter filtered on the `mustur/` name prefix, and
[the entry above](#mustur-can-type-into-a-session-it-started) called that
"enforced rather than documented". A review broke it in one command: `tmux
new-session -s mustur/anything` by hand, and Mustur listed it, typed into it and
killed it. A name is something anyone can write.

`Start` now sets a tmux user option on the session it creates, and `List`
returns only sessions carrying it. Everything else goes through that filter. The
prefix stays for legibility — a person running `tmux ls` can see which sessions
are Mustur's — and is no longer what the rule rests on.

Verified the way the review broke it: a hand-made `mustur/zzfake` is invisible to
`list` and refused by `stop`, while a session Mustur started in the same tmux
server is listed. `tmux ls` shows both; Mustur shows one.

### The word was "supervises" and the thing was not built

Three places said the adapter supervises sessions. Nothing did: no restart, no
health check, no output capture, no notification when a session dies. The word
is removed from every claim about what exists.

What `Start` gained instead is narrower and true: it checks the session is still
there before reporting success. tmux reports success whether or not the command
survived, so an agent CLI crashing on startup used to read as a started session.
One check was not enough — tmux does not reap synchronously, and an immediate
exit was still listed for a moment — so it watches for a short window. That
catches a command that dies at once, which is the common failure. **It is not
supervision, and a CLI that crashes a second later is still reported as
started.**

Supervision moved to 4b on MUS-Q-0015, with surviving a dropped connection,
because supervision without anything watching the output is a restart loop.

### An answer that reached the server is the owner's

The web answer path used the request's context for both the delivery and the
write. A review reproduced what that costs: a client that disconnects while tmux
is being shelled out to cancels the write as well, and the answer is lost
entirely — the question still open, no answer, no reason. **A phone on a flaky
link is the scenario this milestone names.**

The write is detached from the request now, and the delivery carries its own
timeout so an unresponsive tmux cannot hold the answer unwritten. This is what
MUS-D-0065 meant and did not achieve.

### The session that raised a question and the session an answer can reach are different things

Milestone 4a redefined the `Session` field to mean the project, so delivery could
target it. Seven records already carried a Claude Code session id under that
name, and the redefinition made every one of them wrong — silently, because a
Claude session id passes the project-name check and was handed to tmux as a
project rather than refused.

`Session` keeps its original meaning and its original records. `Session project`
is the new field delivery targets, set by `mustur ask --in`. Redefining a field
under records already written is a rewrite of the past disguised as a comment
change.

### There is no `session send`

An operator verb taking arbitrary text made "the only caller is the answer path"
false in the same commit that claimed it. It is removed. Typing into an agent's
input is a capability the answer path needs and nothing else does, and a person
who genuinely wants to has `tmux send-keys` — as themselves, rather than as
Mustur.

### The unit cannot have a private /tmp

`PrivateTmp=yes` gives the service its own `/tmp`, and tmux's socket is
`/tmp/tmux-$UID`. Under that flag the service cannot reach the owner's tmux
server at all, so every answer filed from the phone would have recorded "not
delivered" while the code looked correct — the milestone's headline capability
inert exactly where it ships.

`PrivateTmp=no`. A hardening flag that silently removes the feature is worse
than the hardening is worth, and the rest of the confinement stays.

## 2026-08-21 — the eighth surface

### v1 has eight surfaces, and the eighth was found by trying to build it

[docs/ui-surfaces.md](docs/ui-surfaces.md) listed seven and the published plan
draws artboards for those seven. Milestone 4b streams a running session's output
to a browser tab, and none of the seven is that — the session **list** says
which sessions exist; this says what one of them is saying. Two different
screens with two different jobs, and only one of them had ever been named.

That is [Plan.md](Plan.md) and `docs/ui-surfaces.md` disagreeing about how many
surfaces v1 has, which [workflow.md](workflow.md) makes a bug to report rather
than pick. It went to the owner on `MUS-Q-0016`, and their answer was to draw it
before building it: [plan-6009f123020a4f58](https://plan.agent-native.com/plans/plan-6009f123020a4f58).

The gap was invisible from inside the plan. It only appeared when a milestone
tried to build against the surface and found nothing there, which is worth
noticing about how the surfaces were enumerated in the first place: the list was
written from the phone bar's tabs, and a session's output is not a tab.

### The standing instruction is what stopped 4b starting

The owner's answer after intake was that a plan is published for every remaining
surface before any more are built. An earlier commit on this stack deleted that
sentence from `docs/ui-surfaces.md` while rewriting the file, and a review caught
it and put it back.

It earned its restoration immediately. It is the rule that stopped milestone 4b
being written from a brief the way intake and the decision queue both were —
the third time would have been the one where the pattern stopped being an
accident.

## 2026-08-21 — the session surface's last three questions

### Sub-agents are their own milestone, and it starts by finding out whether it is possible

Mustur shells out to a CLI and reads one pane. A session that spawns three
reviewers is three agents writing into that pane, unlabelled and interleaved
with the parent — so every option for *showing* sub-agents was really an option
for *finding out about them*, and none of the three was cheap.

The owner's answer on `MUS-Q-0017`: ship 4b without them, and make sub-agents
milestone 4c.

What makes that the right shape rather than a deferral is the verdict it
permits. 4c's first unit is establishing whether the CLI will let the adapter
place a sub-agent where it can be read separately. **If it will not, the
milestone's verdict is that this cannot be done** — which is a real outcome, and
one the alternative could not reach. Parsing the pane for markers would have
produced a sub-agent list that was sometimes wrong, and a wrong status reads as
a fact once it is on a screen. That is the same failure as inferring
waiting-for-input, which the owner declined on `MUS-Q-0005`.

### The session composer is always writable

Read and write share one connection. The alternative offered was read-only until
armed per session, so that a tab left open in a pocket could not type.

The owner's answer on `MUS-Q-0018`: always writable, as drawn. The embedded
composer keeps its one tap from watching a session to answering it, which is the
whole argument for embedding it.

**What that costs, stated rather than left implicit.** The WebSocket origin
check and the Access policy's scope are now the *only* things between a stranger
and an agent's input; there is no second layer behind them. Two consequences
follow, and both are heavier than they were an hour ago:

- The origin check is not a hardening detail. It is the control, and it is
  tested first in 4b's verification.
- Confirming the Access policy admits the owner and nobody wider is now
  urgent rather than tidy. It has been outstanding since the ingress went up.

### A session's exit is an event, not a record

Supervision notices an exit and reports it on the surface and in the service
log. Nothing goes into `records/`.

The owner's answer on `MUS-Q-0019`, and it follows the line already drawn around
session output: not addressable, not exported, not a record. An exit is the same
kind of thing. The alternative — a finding for every non-zero exit — fills the
findings queue with noise the first time something is flaky, and a records tree
nobody reads is worse than one that is missing something.

## 2026-08-21 — the bar reaches intake

### A surface reachable only when it has something to say is not reachable

The decision queue's only route from intake was the banner, which renders when
something is open. With every question answered the banner was absent, so
loading `mustur.devofpie.com` gave the intake box and no way to reach anything
else — the queue was reachable from intake exactly when it had something to say,
and invisible when it did not.

The owner found it by loading the site. It had been recorded as a queue line
during the milestone 4a review and not acted on, which is worth noting: a defect
written down is not a defect fixed, and this is the second time on this stack
that a record was mistaken for a safeguard.

The fix is [MUS-D-0041](#the-phone-bar-has-four-tabs) applied where it had not
been yet. The bar carries the surfaces that are built and grows as the rest
arrive, exactly as it does on the queue. The banner stays beside it rather than
being replaced: the bar is the fixed place the eye knows to check, and the
banner is what makes an open decision impossible to miss on whichever surface
was opened. That is the pair MUS-D-0041 argued for, not an alternative to it.

## 2026-08-22 — decisions taken while building milestone 4b

### The reader lingers after the last viewer leaves

The plan said `pipe-pane` is opened when the first viewer arrives and closed
when the last one leaves. Building it showed that is wrong for the case the
milestone exists for.

One owner with one phone **is** the last viewer. Closing the reader when they
disconnect throws away the buffer and the byte offsets, so reconnecting a second
later resumes from zero with nothing to replay — and "survives a dropped phone
connection" is precisely the clause that would not work.

The reader now stays open for two minutes after the last viewer leaves. A
reconnect inside that window is continuous; after it, the session is still
running and the next viewer starts from what the pane already holds.

### The buffer is seeded from the pane, not only from the pipe

`pipe-pane` carries output produced after it is enabled and nothing before it.
Without a seed, the first viewer of a session that has been running for an hour
opens an empty screen and waits for the next line — which is not "2,140 earlier
lines", it is a blank page that reads as a hung session.

`capture-pane` supplies the scrollback tmux is already keeping, taken once
before the first byte off the pipe so the buffer reads in order. Found by
writing a test that watched a session which had already finished printing, and
seeing nothing arrive.

### A WebSocket library rather than hand-rolled framing

`coder/websocket`: pure Go, no transitive dependencies, so the static binary
stays static.

The alternative was implementing RFC 6455 here — masking, fragmentation, close
handshakes, ping/pong. On the one path in this project that carries keystrokes
into an agent, a vetted implementation beats a hand-written one, and that is the
whole argument. The owner named the connection's security a first-order concern
on the same day; hand-rolling the framing under it would have been the wrong
reading of that.

### The origin check refuses a handshake with no Origin at all

Browsers always send `Origin` on a WebSocket handshake, so its absence means the
client is not a browser — and a non-browser client has no business on the path
that types into an agent. Refusing is the strict reading and this is the place
to take it: a socket that reaches a running agent is not somewhere to be
generous with clients that decline to identify themselves.

The check compares the origin's host to the request's own. Cloudflare Access
authenticates the *person*; it says nothing about which page opened the socket,
and browsers exempt WebSockets from the same-origin policy while still sending
cookies with the handshake. Without this check, a page the owner merely visited
could open a socket here on their authenticated session and type into an agent.
It is the control, not hardening, and it is the first thing the verification
list tests.

## 2026-08-22 — what the milestone 4b review changed

### The answer path is no longer the only caller, and the entries saying it is stay put

[MUS-D-0063](#mustur-can-type-into-a-session-it-started) named three limits on
typing into an agent and called them "enforced rather than documented": the
ownership option, the self-describing text, and the answer path being the only
caller. [MUS-D-0070](#there-is-no-session-send) removed an operator verb
specifically to keep the third true.

The session composer is a second caller. It takes arbitrary text and sends it
with no *"The owner answered …"* framing, because it is not answering anything.

The capability is the owner's, taken on `MUS-Q-0018` with the trade named. What
was missing is this entry: both of those are append-only and neither could be
edited, so the correction is here. **One of the three limits survives** — the
ownership option — and it is the one enforced in code. The other two are now
true of the answer path and not of the surface.

### The idle timeout was a lie about the session

Thirty minutes after a tab opened, the socket sent `ended` and closed, whatever
the session was doing; the timer was created once and never reset. The client
then said *"Nothing is running"* about a session that was, disabled the
composer, and did not reconnect.

It resets on activity now — output or typing — and a genuinely idle socket gets
an ordinary close, which the client treats as a disconnect. `ended` means the
session ended.

That also repairs MUS-D-0075's stated reason for an always-writable composer:
*"a tab left open in a pocket could not type"*. After thirty minutes it could
not, which was not the argument anybody made.

### A viewer that falls behind is disconnected, not quietly starved

The fan-out skipped a viewer whose buffer was full. Go discards the *new* item,
so that viewer kept receiving a contiguous but ever-staler prefix — no sequence
jump, nothing to notice. A review measured one 8 MB behind with zero holes and
zero notice, under a comment claiming "the viewer will resume from its own
sequence", which nothing implemented.

Closing the channel ends that socket and the client reconnects from the offset
it actually reached, which is what the comment always claimed. A stalled tab
still cannot hold up the reader or the other viewers.

### The bar grows in three templates, not one

`MUS-D-0057` says the bar "renders the built ones and grows as the rest arrive".
Sessions arrived, the session page rendered three tabs, and the other two
surfaces went on rendering two — three different bars in one binary, and the
promise was made in the file that did not keep it.

A promise that has to be kept in three templates at once needs a test that reads
all three, and now has one. This is the same shape as the queue being reachable
from intake only when it had something to say: a rule stated in one place and
implemented in one place, while the rule was about every place.

### The exit is logged, and supervision only runs while somebody is watching

`stream.go` claimed an exit "is reported on the surface and in the log" before
anything logged anything. It does now.

The limitation underneath is worth stating rather than fixing quietly: the
reader is started by a viewer, so **a session nobody has looked at is not being
watched**. An exit is noticed when a tab is open on it, or not at all. Watching
every owned session would mean polling tmux continuously for sessions nobody is
reading, and that trade has not been made.

## 2026-08-22 — sub-agents can be seen, and how

### The investigation was registered before it was run

Milestone 4c began with a question rather than a build:
[can Mustur see a sub-agent at all](docs/investigations/0002-sub-agent-visibility.md).
The routes to try, the three properties an answer had to carry — enumerable,
separable, terminal — and the rule for reaching *cannot be done* were committed
in one commit, and the finding in the next. The history is the evidence that the
rule was not written to fit the result, which is the same discipline
[milestone 1's disproof](docs/investigations/0001-mandated-tool-call.md) rested
on.

The rule was worth having for one reason in particular. A session's sub-agents
are already on disk, in a `subagents/` directory holding a transcript and a
metadata file for each — enumerable, separable and readable, and it would have
worked today. It is undocumented, so it is written up as a finding and not used.
Without a rule fixed beforehand, that directory is exactly what a builder in a
hurry adopts.

### The route is lifecycle hooks, and the pane is untouched

`SubagentStart` and `SubagentStop` are documented hooks carrying `agent_id` and
`agent_type`, with the final message and the per-agent transcript path on stop. A
tool-use hook carries `agent_id` when the call happens inside a sub-agent and
omits it in the main conversation, so a sub-agent's activity is attributed by an
identifier the CLI supplies rather than by reading its prose — which is what
[route D was ruled out for](docs/investigations/0002-sub-agent-visibility.md),
and what the owner declined an inferred status for on `MUS-Q-0005`.

The important property is what it does *not* change. Everything milestones 4a
and 4b built stands: the session is still a tmux pane, still typed into with
`send-keys`, still read with `pipe-pane`, still attachable from a terminal.

The alternative would have cost exactly that. `--output-format stream-json` with
`--forward-subagent-text` carries all three properties and is fully documented,
and a session held open on `--input-format stream-json` accepts further messages,
so it is a real session rather than a one-shot. It requires `-p`, and `-p` is not
a terminal. Adopting it would replace the pane with a JSON harness — a different
product, not a way of showing sub-agents in this one. It is recorded rather than
adopted, so it is findable if the pane is ever given up for other reasons.

### The hook is passed per session, and persists nowhere — MUS-Q-0024

A hook is executable configuration running inside the agent's own session, so
where it is installed was the owner's decision and not the builder's.

Mustur starts the session, so it passes the hook as a `--settings` JSON string on
the command line it already builds. Nothing is written to `~/.claude` and nothing
into the checkout the session runs in. The cost, accepted: a session started by
hand carries no hook and shows no sub-agents. Mustur reports on what Mustur
started, which is the same line the
[ownership option](decisions.md#2026-08-21--the-session-surfaces-last-three-questions)
already draws.

### A row shows what is documented — MUS-Q-0025

What a sub-agent was asked to do, how long it has been running, what it is doing
now, and its output once it finishes. Full prose while it is still running would
mean deriving the per-agent transcript path, and the owner declined to make that
exception. Reading a sub-agent mid-flight means opening the parent pane, which is
still there.

## 2026-08-22 — what the milestone 4c review changed

Three reviewers, none given the pull request or any report. They agreed on
something the builder had not noticed: **this milestone's own documents claimed
things its tree did not support.** The investigation named captured files that
were not committed, the code cited a decision that did not exist, and the plan
recorded a review that had not happened.

### The gate that failed again, and what it cost this time

Five decisions were taken in prose. Two of the five were not small.

**Pairing a task to a sub-agent by order is an inference**, and it went in
without being asked — in a package whose own header says rows are *"attributed
by an identifier the CLI supplies, never by reading the pane and guessing"*. A
reviewer reproduced the failure: an `Agent` call that never produced a sub-agent
left its description in the queue for the next one, rendering a row that read
**"DENIED call, never ran"**. The owner bounded it on
[MUS-Q-0026](records/questions.md#mus-q-0026) rather than dropping labels, and
was told plainly that a bound narrows the failure without closing it.

The number that bound it is the part worth keeping. "A few seconds" was the
instinct, and measuring the captures says a few seconds is wrong: the nine
launch-to-start pairs run from 1.563s to **5.985s**, because three sub-agents
launched together are spawned in sequence and the last waits for the first two.
A two-second window would have stripped the label off rows that were right —
the same failure through the other door. Thirty seconds, and a test that fails
if anyone shrinks it below the slowest pair actually observed.

**Rejecting the structured-output route** was a product judgement argued down in
prose. Both routes cleared the investigation's rule; choosing hooks over a JSON
harness is what keeps a Mustur session a terminal you can attach to. Ratified on
[MUS-Q-0027](records/questions.md#mus-q-0027), and it should have been asked
before it was built on.

### Three defects the fake runner could never have shown

**Rows outlived their session.** The log was keyed on the project and nothing
cleared it, so stopping a session and starting another showed the old one's
rows — one still pilled *running*, ageing forever, for a process dead before the
page existed. That is the condition the investigation's own rule named as
disqualifying, arriving in the implementation of the route that passed it.
`Start` now forgets them.

**A row claimed to be in a tool it had left.** Only the start of a tool call was
hooked, so `Doing` was the last tool forever, while a comment on the surface
claimed a sub-agent between calls "is thinking, and says so". The comment
described behaviour the code did not have; `PostToolUse` is the other half, and
it costs a second short-lived process per tool call.

Verifying that hook before relying on it caught a third defect that had not
shipped: the parent's own `PostToolUse` for its `Agent` call carries no
`agent_id` either, so without checking the event name it landed as a second
launch — every task description queued twice.

### The records were the fiction, not the prose

`records/` is generated from the store and says so in its own header. The 4c
questions were written **into the export by hand**, so the file said 23 records
over a file holding 25, the identifier index stopped at `MUS-Q-0023`, the
decisions had no `MUS-D` at all, and investigation 0002 existed on disk with no
`MUS-I`. `make check` was green throughout, because every gate reads the export
and the export was internally consistent.

Fixed the only way that is not another fiction: through `mustur ask`, `surfaced`
and `answer`, which is when the store refused to let the asker answer their own
question — the gate built on
[MUS-Q-0007](records/questions.md#mus-q-0007) — *refuse self-answer, allow
self-withdraw* — doing exactly its job to the person who built it. The answers are recorded as the owner's because they are.

### A third invented citation, in the third place the verifier cannot look

`subagents.go` cited `MUS-D-0087` when the tree ended at `MUS-D-0086`. All three
of this session's invented citations have been in Go comments, and `mustur
verify` reads `records/`. The pattern is now specific enough to name: **prose in
`.go` files is the one place identifiers are asserted and never checked.**
Whatever fixes it is not another careful author.

### Evidence a reader can reach

The investigation claimed runs "reproduced by the commands quoted below" and
quoted no commands, over captures that were not in the tree — the same complaint
`queue.md` already carries against 4a. [0002-harness/](docs/investigations/0002-harness/)
is the correction, following what 0001 did: the recorder, the runners, the
scorer, and the captured output. `score.py` re-derives the pairing result and
`count.py` the coverage, both from committed files, with no CLI and no network.

4c also had no work unit, so the plan cited the investigation as its method — a
document written two hours before the build and containing none of it.
[MUS-W-0018](records/work-units/MUS-W-0018.md) is the method.

### Live rows, against the recommendation

The rows were server-rendered like the rest of the surface, so a sub-agent's age
and current tool froze until reload, on the page that already holds a socket.
The owner chose to push them down it
([MUS-Q-0029](records/questions.md#mus-q-0029)) against the recommendation to
leave them static.

Kept as small as that can be: the server polls the hook's log on a two-second
ticker, skips the parse entirely when the file's size and modification time have
not moved, and sends a frame only when the rows differ. Ages are not sent —
each row carries its stamps and the client counts, so a running sub-agent's
clock moves without a frame per second to move it. Tested against real tmux
with a real handshake, because this is the one thing the client layer now models
that is not the terminal.

## 2026-08-22 — a record's prefix says which project it belongs to

The owner filed three jots from a phone to prove milestone 2c, and one of them
routed correctly to the idea inbox while reading as though it had not. It was
called `MUS-F-0025`, which is indistinguishable at a glance from a record about
Mustur itself.

Nothing was mis-routed. The point stands anyway, and it is the owner's: **the
MUS tag should mean the Mustur project, not the store that happens to hold the
record.** A prefix that is the same for everything is a prefix that cannot be
scanned, and every jot to the idea inbox had to be opened to find out where it
had gone.

The identifier scheme already anticipated this — three upper-case letters,
chosen "so a second project onboarded later cannot collide with this one". What
was wrong is that intake stamped the serving project's prefix at creation, some
lines before it knew where the jot was going.

### What changed

A routing record names its own prefix in a field, the way it already names
itself the intake default. The idea inbox's is **IDW**, for IdeaWarehouse, which
is where these go once that project is onboarded at milestone 7 — so the prefix
names the destination and a record keeps its identifier when it finally arrives,
rather than being renamed into it.

A routing record whose prefix is not three upper-case letters is ignored and the
store's used instead. Wrong in a way somebody can see beats an identifier the
scheme cannot parse.

### Narrow on purpose

Only the one routing target that exists carries a prefix. Generalising it to
every target is milestone 7's work, where a second project's cases are real
rather than imagined, and doing it now would front-run those decisions with one
example to test against. The owner chose the narrow cut on
[MUS-Q-0030](records/questions.md#mus-q-0030).

### `MUS-F-0025` keeps its name

The `ident` package's rule is that identifiers are permanent, because the store
is insert-only and a scheme that allows renaming is one that allows a citation
to rot. `MUS-F-0025` is cited in `Plan.md`, `docs/ingress.md`, this file and a
pull request comment.

So it stays, as the last record filed under the old scheme, and the owner said
why that costs nothing: it was a test, and its information is used up by the
finding it already produced.

## 2026-08-23 — the composer, and a newline that would have submitted

Milestone 5's sentence is *multi-line, spell-checked text from the phone, off
the home network, without a terminal, reaching the intended session.* Four of
those were already true at 4b. The two that were not are the two that make it a
composer rather than a chat box.

### The design question was answered, and this repository went on calling it open

`docs/ui-surfaces.md` listed *"whether a message is composed against a chosen
session, or composed first and routed second"* as an open question for design.
The owner answered it on 2026-08-20, in the plan, and the annotation on the
composer artboard is the decision: **thought first, destination second, with the
last-active session as the default**, and *"the draft indicator is the whole
reason this is not a chat box."*

Three days of that being stale is a small thing on its own and the same shape as
the larger one the reviews keep finding: a claim true in the file that makes it
and false against the thing it names.

### One draft, not one per session

What is being written is a thought; which session it goes to is a separate
choice that can change *after* it is written. That is what thought-first means,
and it decides the implementation: the draft is kept under one key, not one per
project, because a per-project key loses it at exactly the moment the design
exists to protect — the owner deciding mid-sentence that this belongs somewhere
else.

It is written on every keystroke rather than on unload, because a backgrounded
phone need never deliver another event, and the clause is that a draft survives
being backgrounded. Where the browser refuses to store it at all, typing still
works and nothing is kept.

### It is not a fourth tab, and not a second scripted page

The four tabs are Sessions, Decisions, Intake and Records (MUS-D-0041). A
composer needs a client layer, and the session view is the only surface that
carries one — a second is a decision rather than a consequence of building this.
So the composer is the session view's, made real: a textarea where an input was.

### A newline typed into a terminal is Enter

The part that would have shipped broken. Enter in an agent's composer submits,
so the obvious path turns a three-line draft into three prompts and a fourth
empty one.

**Measured**, before choosing: `send-keys -l` with embedded newlines *does*
land every line in Claude Code's composer, and a bracketed paste does too and
arrives as one message. Both work.

**Asserted**, and this is the part that actually chose the mechanism, so it is
labelled rather than blended in: that the TUI reads one burst as a paste, that
this is the CLI inferring intent from timing, and that a write arriving split
would submit halfway through. **No split write was ever observed.** The paste is
used because it states that it is text instead of leaving it to be inferred —
an argument from reliability, not from a failure to work — and it drops its
buffer afterwards so a draft does not sit in tmux's paste stack.

Single-line still goes as keystrokes: that is 4a's answer-delivery path, and it
has shipped behaviour behind it that this had no reason to disturb. That claim
was itself too narrow and `MUS-D-0098` carries the correction: nothing limits an
answer to one line.

**No test in this tree proves the clause that matters.** That a paste arrives as
one message rather than one prompt per line was measured by hand against the
real CLI — four lines, one Enter, and the agent answered from all four. The
real-tmux tests hold `cat`, and `cat` is happy with newlines either way, so they
prove the bytes arrive and not which mechanism carried them. The tests say so in
their own comments rather than leaving a reader to find out.

### An hour lost to `timeout`

Worth writing down because it will happen again. A test fixture ran the pane as
`timeout 20 cat > file` so the program would exit on its own. `timeout` puts its
child in a new process group without the terminal, so `cat` was stopped on
SIGTTIN the moment it read, and received nothing at all — a green mechanism
looking like a broken one. Plain `cat`, with the hub's cleanup registered before
the session's so it runs after it, was the fix.

## 2026-08-24 — correcting the entry above, and what the second review found

The entry *"A newline typed into a terminal is Enter"* above says: **"The owner
answered it on 2026-08-20, in the plan."** That is false. The composer decision
is `MUS-D-0013`, taken on **2026-08-19** at the evening design review against
the published wireframes — a day before that plan existed, and not among the
four questions it settled. The correction is appended rather than edited in,
because that is the rule; a reader arriving at the entry above should arrive at
this one next.

Getting the attribution wrong is what let two of `MUS-D-0013`'s three clauses go
unbuilt without anyone noticing.

### The second review pass, and two of its findings

Three fresh reviewers read the rebuild. Two findings are worth keeping here
because they are the *first* pass's findings reproduced inside the corrections
written to fix them:

- The record rewritten to stop crediting a test with assertions it did not make
  credited **a different test with a different surface's proof**: the composer
  has no socket, and "multi-line left the composer over a real WebSocket"
  described the session view's reply box. The composer's own delivery into a
  live session was exercised by nothing. It has a test now.
- "Both say so in their own comments" — only one did.

The lesson is not that the corrections were careless. It is that **a correction
is a claim**, and this repository has no gate that reads claims.

### Timestamps that could not be true

Every question raised here up to today records an answer stamped **before the
question existed**: `MUS-Q-0034` was created at 09:53 and says it was answered
at 09:00, because the times were typed by hand from a conversation that had
already happened. `make check` could not see it — the gate requires the
`Surfaced` field to be non-empty and nothing else — so the record this
repository leans on hardest was not evidence of the thing it is kept for.

`surfaced` and `answer` now stamp the clock and refuse a `--at` in the past
rather than silently overriding it (`MUS-D-0101`). The records already carrying
impossible times stay as they are, with this as their correction.

### Amend replaces, and I have now been caught by that twice

`MUS-F-0002` was amended to note it had been overtaken, and the amend **moved a
finding noticed on 2026-08-19 to the 24th and dropped its provenance rows** —
in the export, which `MUS-D-0024` designates as the surface a reader checks
without the binary. The same trap took `MUS-M-0010` earlier in this session.
Both are restored. `amend` takes what it is given and writes exactly that; the
habit it needs is reading the record first, which is not a thing a comment can
enforce.

## 2026-08-25 — accounts of Mustur's own, and what the review found

### Who may read the records is not itself a record

`internal/account` lives beside the record log rather than in it. The log is
insert-only, exported to markdown and read by strangers; an account is mutable,
private, and gets deleted. They share one SQLite file because two files would be
two things to back up and one of them would be forgotten, and they share nothing
else: `mustur export` renders records and has never seen an account table.

The credential is a passkey, and losing the device is not losing the account —
the owner's clause on `MUS-Q-0041`. An address that already has an account
reuses its identifier when it redeems a second invitation, which is what makes a
replacement passkey land on the account it is replacing a key for rather than on
a second account with the same email. That is the whole recovery story and it is
one line of SQL, which is the only reason it is trustworthy.

### The gate is a flag, and the flag is off

Turning enforcement on before the owner holds a passkey locks the owner out of
their own running service. So `--accounts` is off by default and the deployment
does not pass it, the same shape as `--sessions`. Cloudflare Access stays in
front throughout; taking it off is the owner's judgement, reserved on
`MUS-Q-0039` and not part of this milestone's verdict.

### The ceremony needed a client, not a browser

Every test of authentication here started *after* the passkey — invite, redeem,
mint a session cookie by hand — and a comment in `guard_test.go` said why: the
ceremony needs a browser. It does not. It needs a correct client of the
protocol, which is 224 lines of ES256 and CBOR, and with one the whole clause
runs: invited, registered, signed out, recognised.

That mattered more than the coverage. One of the tests it made possible was
**passing for the wrong reason** — a cross-account ceremony was being refused by
the library's session check, not by Mustur's own handle check, and deleting that
check left the test green. A true outcome proving nothing about the line under
test is the failure mode this whole file exists to catch, and it was found by
mutating the code rather than by reading it.

### A drawing found a bug no test would have

`MUS-F-0026`: registration never required a discoverable credential. An
authenticator could have made one this server can never ask for — an account
that exists, holds a passkey, and cannot be signed into. It was found by a
comment on a *picture*, during a plan review, by somebody looking at a screen
that said "choose a passkey" and asking which passkeys.

### The count that nobody drew

Two of three reviewers independently found that "two surfaces carry script" was
false. Both then got the replacement wrong, and so had the builder: one said
five, one said six, the code said four. Six is right, and the reason is
structural rather than arithmetic — `authTmpl` serves sign-in *and* the
invitation page, `accountTmpl` serves the account *and* people screens, so each
"one more scripted surface" was two. A count asserted rather than enumerated is
a count nobody has checked. What the rule should measure at all is
`MUS-Q-0053`.

### Four surfaces built before they were drawn, and the pattern has a number now

`docs/ui-surfaces.md` exists to stop exactly this and has now failed seven
times, four of them in this one milestone. Recording each instance has not
reduced the rate; it has gone up. `MUS-F-0027` records the pattern as a defect
with an identifier rather than as four more paragraphs in the file that already
says not to.

The surfaces were drawn afterwards under `MUS-Q-0043` and rebuilt from what
three rounds of review settled. That remediates the instance. It does not
remediate the pattern, and saying otherwise would be the fifth time.

### What the review caught in the code

A disabled account could be re-invited: the invitation was spent, a passkey
stored, a cookie issued and immediately refused by the guard, and the person
dropped back at sign-in knowing nothing. `RemoveCredential` counted before it
checked ownership, so removing somebody else's passkey answered "that is your
only passkey" — true about an account that was not theirs. The ceremony script
loaded on two pages with no ceremony.

One finding was disputed and is worth recording as disputed: foreign keys were
reported as enforcing nothing because no pragma sets them. They enforce. Measured
with no pragma anywhere in the tree, a grant to a nonexistent account fails with
`SQLITE_CONSTRAINT_FOREIGNKEY`, because modernc's driver switches them on. The
pragma was added anyway, so the guarantee belongs to this schema rather than to a
driver default, and the test stays — the property is worth pinning wherever it
comes from.

## 2026-08-25 — the credential an agent can hold

### A token is not an account, and that is the point

A passkey needs a browser, an authenticator and a gesture. An agent has none of
the three and still has to reach the mandated tool call, so milestone 5c gives it
a token carried in an `Authorization: Bearer` header.

The temptation was to make it an account with a funny credential. It is not one:
no email, no passkey, no session, and the guard consults it on exactly one path.
That is not tidiness. A token lives in a systemd unit or a process's
environment, which is a materially weaker place than a device's secure element,
and **scope is what makes the weaker secret acceptable**. An agent token opens
`/mcp` for one project and cannot read a record, open a session, or sign in.
Folding it into `account` would have made a leaked token a way into the browser
surfaces instead of into the one call it exists for.

### No expiry, deliberately

An invitation expires because it is a one-time link in transit. A session
expires because a browser is borrowed. An agent token is **configuration**, and
a credential that stops working at 3am without anybody having decided that is an
outage, not a security control. Revocation is the control, it is a row read on
every call rather than a cache, and it takes effect on the running server —
measured, not assumed.

Revocation is a timestamp rather than a delete, so a listing can still say the
token existed and when it stopped. A row that vanished cannot tell anybody
investigating anything.

### The guard trusts one thing about the tool surface

An agent token is let past `/mcp` with **no write check**, because an MCP call is
a POST by method and a read by nature — the surface serves one tool and it
reads. That is a real assumption, and a comment saying so is a promise nobody
checks. `internal/mcpsrv`'s own test asserts exactly one tool named
`mustur_route` and now says why, so a second tool fails there and names the
guard as the thing to revisit.

### The id is hex because it is typed at a shell

The first version made ids base64url, like every other secret here. One in
sixty-four would have begun with `-`, which `flag` parses as a flag — so
`mustur account revoke <id>` would have failed on the exact id the tool itself
had just printed. Found by issuing one and reading the output, which is the
cheapest test there is and the one most easily skipped.

The first version of this paragraph said **one in thirty-two**, and a review
caught it: the alphabet is 64 symbols, not 32. Measured afterwards over 200,000
draws — 3,072 leading hyphens, 1 in 65.1. The entry congratulating itself for
finding a bug by running the code had a number in it that nobody had counted,
which is `workflow.md`'s own gate failing inside the entry that celebrates
having met it.

### A test that passed for the wrong reason, again

The scope test — "a token opens the tool call and nothing else" — passed while
asserting nothing. Its client followed the guard's `303` to `/signin`, landed on
a public page, and reported `200`; the test looked for "not 200" and was
satisfied by a redirect it had already followed. It now follows no redirects and
asserts `303` or `403`, and both negative tests here were checked by breaking
the code to see them fail.

That is the second time in two milestones. The first was 5b's cross-account
ceremony test, refused by the library rather than by the check under test. Both
were found by mutation, neither by reading, and the habit is now the point:
**a negative test is not evidence until it has been seen to fail.**

## 2026-08-25 — a fourth difference between the mandate and what milestone 1 scored

`Three things differ from what was scored, and the entry exists to name all
three` — and that entry's own history is that an earlier draft named two and a
review found the third. A review has now found a fourth, which makes the pattern
more interesting than the item.

**The tool call can require a credential.** Milestone 1's fixture registered a
stub over stdio, where the client launches the server and the tool is therefore
always present. What ships is HTTP on loopback — difference 3 — and since
milestone 5c that transport can be gated: with `--accounts` on, `/mcp` refuses a
caller carrying no token. An agent honouring the clause at the bottom of
`CLAUDE.md` now needs a secret in its MCP configuration under one supported flag
setting, which the disproof never measured.

It bites only under `--accounts`, which is off by default and which
`deploy/mustur.service` does not pass, so nothing running today contradicts the
original entry. The claim is that the enumeration is incomplete for the shipped
binary, not that the deployment falsifies it.

**And a third state the instruction does not name.** `CLAUDE.md` tells a session
what to do when the tool "is not there": say so and carry on. Under `--accounts`
with no token the tool *is* there and answers 403 — neither absent nor working.
That sentence now names the third case, because a session meeting it would
otherwise report the one thing that is not true.

So the count is four, and the more useful record is that this entry has been
wrong about its own completeness twice, both times found by somebody who did not
write it.

## 2026-08-25 — a token's lifetime, handed back to the owner

The owner's answer on `MUS-Q-0051` said a token has "its own lifetime and its
own revocation". I built revocation, built no lifetime, and wrote the argument
for that into this file under a heading that congratulated itself for it.

`workflow.md` is explicit: Plan.md wins on *what*, and a conflict between it and
the build "is a bug — report it, do not pick." I picked, and a review caught it.

The argument was not wrong. An invitation expires because it is a link in
transit; a session expires because a browser is borrowed; an agent token is
configuration, and a credential that stops at 3am because a timer ran out, with
nobody having decided so, is an outage rather than a control — and the agent it
stops is the one that reads these records.

But it is an argument to *put*, not to record and proceed on. Asked as
`MUS-Q-0055`, and the answer was the middle shape neither of us had written
down: **`--expires` is optional and zero means never.** The deployment's token
lasts until revoked; a token for a single job or somebody else's machine can be
given a lifetime. The cost, accepted rather than hidden, is that a token now has
two ways to stop working — so the listing names which one, revoked beating
expired because a revocation is a decision and an expiry is a date arriving.

`ByToken` tells a caller neither, for the same reason `ErrNoInvite` says nothing
about why.

**The shape of the mistake is what matters.** This was not a decision taken
without asking; it was an *answered* one overridden by the party who had asked
it, and the override was written down carefully enough to look like diligence.
The tell was a heading that argued rather than recorded. When an entry here
starts persuading, the thing it is persuading about probably belongs in a
prompt.

## 2026-08-26 — the owner found the bug the whole suite agreed with

The owner registered a passkey on a phone, through Bitwarden, then tried to sign
in from a laptop and was told the passkey was not recognised.

Registration had worked — the account existed with one credential, a sixteen-byte
id and a seventy-seven byte ES256 key. **Sign-in could never have worked**, for
that credential or any other like it.

### What was wrong

WebAuthn asks a relying party to notice if a credential's backup-eligible flag
changes between registration and use. `go-webauthn` enforces it at login.
Mustur stored no flags at all, so the credential it rebuilt to check the
assertion claimed `BE=0` against an assertion carrying `BE=1`, and the library
refused: *Backup Eligible flag inconsistency detected during login validation*.

Every synced credential manager sets `BE=1` — Bitwarden, iCloud Keychain,
Google Password Manager, 1Password. So **every passkey a person would
realistically use could be registered and then never used.** Only a credential
welded to hardware would have worked, and only because its `BE=0` happened to
match a flag that was never written.

### Why three reviewers and a mutation-checked suite missed it

The virtual authenticator built for milestone 5b modelled a hardware key. Its
`BE=0` agreed with the server's missing flag, and agreement reads exactly like
correctness. Nothing in the review was careless; the double was a correct client
of the protocol and every test it ran passed truthfully.

**A double is a claim about the world**, and this one — that an authenticator
looks like a YubiKey — was never examined, because it was made in passing while
building something else. It is now a synced credential by default, with the
hardware case named as the exception, and both are tested. Dropping the stored
flag fails the synced case and leaves the hardware one green, which is the shape
of the original bug.

The rule worth keeping: when a double and the code under test share an
assumption, the test proves the assumption is *shared*, not that it is *right*.
Look wherever the double was written by the same hand, in the same sitting, as
the thing it tests.

### And the diagnosis was harder than it needed to be

A page that distinguished "no such credential" from "bad signature" would be an
oracle, so a browser gets one sentence. That was a decision and it stands.

Telling the *operator* nothing was not a decision, it was an omission. There was
no log line at all, and the cause had to be reconstructed from a ceremony table
that happened to retain abandoned rows and from reading the library's source.
Two different audiences had been treated as one. Every refusal in the
authentication path now logs the check that failed while the browser keeps its
single sentence.

### The store had to be migrated, which it had never needed before

`CREATE TABLE IF NOT EXISTS` builds a missing table and says nothing about one
that exists with the wrong shape. Two columns had to reach a store that already
existed, and that store was the owner's live one. `store.Open` now adds missing
columns from a list in the source — added, never dropped or retyped, each with a
default. The record tables are not in that list and are not expected to be:
their shape is the export's contract, and changing one is a decision rather than
a migration.

### What it cost the owner

One passkey, deleted rather than guessed at. The stored credential carried no
flags, so no honest value could be backfilled for a security-relevant field —
and leaving it in place would have made the authenticator refuse to create a
replacement, since a registration excludes credentials the site already holds.

## 2026-08-26 — building the bar found what the code had already decided

The bar was drawn before it was built, which is the thing `docs/ui-surfaces.md`
exists to make happen and has watched fail seven times. Building it turned up
three things the drawing could not have.

### Both halves were already written for a shell nobody built

The session view's stylesheet said `#out { flex: 1 }` and
`nav { margin-top: auto }` — an app shell with a scrolling pane and a bar on the
bottom edge. It set `min-height: 100vh`, a floor with no ceiling, and gave
`#out` no overflow, so the column grew with the output and carried the bar away.

The script turned out to be the same story. `atBottom()` measures
`out.scrollTop` against `out.scrollHeight` and always has — but a pane with no
overflow never scrolls, so `scrollTop` stayed 0, the comparison always answered
true, and the follow-the-tail logic was inert. **The plan named this as the
half with real behaviour in it and estimated it wrong**: nothing needed writing,
because it had already been written for the shell the CSS described. Both halves
were waiting on three declarations.

### The rail is positioned, not placed

Grid was the tidier way to put a last-in-source `<nav>` into the first column,
and it is wrong here. Making `body` a grid breaks the session view: `flex: 1`
means nothing to a grid item, so the output pane stops filling the page and the
composer loses the bottom edge. Taking the rail out of flow with
`position: fixed` leaves every surface's internal layout exactly as it was,
which is what a navigation change should cost.

### The drift was worse than the count

The plan said five templates carried their own copy of the bar. It was six, and
they had already diverged: `intake` drew a 1px border where the rest drew
1.4px, three surfaces made the page a full-height column and three did not, and
`accountpage` had lost the rule that marks the current tab. That is the same
drift that put a different bar on the records surface a day earlier.

`shell.go` holds the shared half now, and `TestNoTemplateDeclaresItsOwnNavRules`
reads the source rather than the output — because a second copy appears in the
source, and that is the test that would have caught this the first time.

### And one more test that passed for the wrong reason

The assertion that `#out` scrolls looked for `overflow-y: auto` anywhere in
`sessions.go` and matched the sub-agent box, which has carried that declaration
all along. A true substring, proving nothing about the pane under test. It reads
the `#out` rule itself now.

That is the third time in three milestones. The habit is holding — each one was
found by breaking the code and watching the test fail — but the rate is not
falling, and the common shape is worth naming: **every one of them asserted
something true about the file rather than about the thing.**

## 2026-08-26 — a browser, and the two things it found

Two CSS defects in a row reached the owner because nothing here could see a
rendered page, and a third was about to be guessed at. A headless browser is
installed now, and the difference was immediate.

**It found the desktop overlap in one measurement.** The rail was `width: 13rem`
with no `box-sizing`, so its real width was 13rem plus a rem of padding plus a
border — about 14rem — over a column whose left margin was 13rem. The rail width
and the gutter are named values now and the content's margin is computed from
them, so the two cannot drift apart.

**It found the records overflow the owner had described sideways.** They
reported the records tab as wider than the others, with only three of four tabs
reachable. The page was 601px wide on a 390px screen: a field row is a flex line
whose value will not shrink below its content unless told it may, and
`min-width: 0` is the telling. The bar is fixed to the viewport, but a page
wider than the viewport is one a phone lets you pan, and the bar pans with it.
`MUS-F-0033`.

**And it caught the builder inventing a defect.** The first clearance
measurement said the records page had text underneath the bar. It does not:
Chromium keeps layout boxes for a *closed* `<details>` under
`content-visibility: hidden`, so `getBoundingClientRect` reports content that is
never painted. `checkVisibility()` says 35px of clearance. Measuring the wrong
thing is not better than not measuring; it is worse, because it comes with
confidence.

### What it could not find

The session view's lower half walking off a phone was not reproducible at any of
five viewport sizes. The fix is not aimed at the cause, because the cause was
never isolated — it is the shape the owner asked for: the quiet timer and the
composer are a block fixed to the viewport, which has no flow position to be
pushed out of, and the output runs behind them. Robustness in place of a
diagnosis, said plainly rather than dressed up as one (`MUS-F-0034`).

### The habit that is doing the work

`min-width: 0` appears in all three of this week's layout fixes — the output
pane that would not scroll, the field row that would not wrap, the flex child in
each case expanding to its content because nothing said it could be smaller.
Worth knowing as a shape rather than rediscovering three times.

## 2026-08-26 — a picture, and the thing about it that travels

The owner tried to report a layout defect with a screenshot, found the intake
box takes text only, and asked for images.

The obvious implementation was the wrong one. `records/` is committed and
`github.com/DevOfPie/Mustur` is **public**, so a screenshot written beside the
records would have published whatever was on the screen — agent output, record
prose, an email address — permanently, and past any later deletion. That is a
privacy decision wearing the costume of a storage decision, and it went to the
owner as one.

**The answer was none of the three options offered:** an agent's summary of what
the image shows may be exported, as long as it carries nothing unnecessary,
while the image itself stays private. A reader with only the clone still learns
what the picture showed. Nothing is published that did not need to be. It is a
better shape than any of the alternatives that were put up, which is the second
time this week the owner has improved on the menu rather than picking from it.

### What is deliberately not stored

No filename. A filename is the sender's text and carries a date, a device and
often the content of the picture; nothing here needs one, and the identifier is
the handle. The media type is sniffed from the bytes rather than believed from
the request, because a caller's `Content-Type` is a claim about a file the
caller also chose. SVG is refused outright: it is XML that can carry script and
would run on this origin the moment somebody opened it.

### The comment that was not true yet

The handler read the picture before writing the record, under a comment saying
that a refused picture would not leave a jot behind claiming to have one. The
validation was in `Attach`, which runs *after* the record exists, so every
refused picture left a jot. The test written alongside it said so on the first
run.

That is the shape to keep noticing: a comment describing an intention the code
next to it does not implement. `margin-top: auto` was the same thing, and so was
`#out`'s scroll. Three this week, all found by something that ran rather than by
reading.

## 2026-08-26 — a place to put something that was never meant to be kept

Testing the picture upload twice cost two permanent identifiers in the idea
warehouse. `IDW-F-0002` and `IDW-F-0003` both say "test" in their own titles and
both will be in the records forever, because an identifier here never comes back
and the log only ever grows. The owner's framing was exact: **a test filing
should not advance a counter.**

### The obvious shape was the wrong one

A record with an expiry. It is wrong because the log is insert-only and the
exported tree is the surface a reader checks without running the binary
(`MUS-D-0024`); a record that later vanishes puts an exception under both, and
an exception is what the next one argues from.

The clarification dissolved the problem rather than answering it. If the point
is the counter, then a scratch filing simply **is not a record** — and nothing
about insert-only is threatened by something that never enters the log. It takes
no identifier, is never exported, is never counted, and cannot be cited: its id
is deliberately unlike an identifier so that nothing can try.

### The restart that was meant

"Or until a restart" went into `store.Open` first, which meant every `mustur
list` and every `mustur get` wiped the pad. The first end-to-end run lost a
filing to the command that went looking for it — a unit test never saw it,
because a unit test holds one store open. A serving process starting is the
event the owner described, and that is where the sweep lives now.

### And an upper-case habit that broke a sweep

Attachments upper-cased the record id they were filed against, on the reasonable
grounds that record identifiers are upper-case. A scratch id is not one. Stored
shouting, it no longer matched the sweep's subquery, and the picture outlived the
note it belonged to — caught by the test written in the same sitting.

The store had been second-guessing the case of an identifier it was handed. It
does not any more. Three bugs this week from a rule applied one step past where
it was true: the filename that was stripped while EXIF was kept, the `9rem` cap
that belonged to a different element, and this.

## 2026-09-03 — the log below this line is generated

This file fell seventeen decisions behind the store, and nobody noticed until
the owner went looking for one and did not find it
([MUS-F-0073](records/findings.md#mus-f-0073)). The last entry written by hand
covers `MUS-D-0120`; `MUS-D-0121` through `MUS-D-0137` existed only in the
store and in the export. Every gate passed throughout, because nothing compares
the two.

Everything above this section is prose written at the time and stays exactly as
it is. From `MUS-D-0121` the entries below are rendered from the store by `make
export`, in the record's own shape.

**The two halves read differently on purpose.** An entry above is an argument —
a narrative carrying its own corrections and retractions — written when a
decision earns one. An entry below is the decision as the store holds it: the
claim, the reasoning, the fields. A decision that deserves an essay still gets
one written above; nothing below is written by hand, and editing it there is
undone by the next export.

**Nothing yet fails when this file is stale.** `make export` is what keeps it
current, which is the standing `records/` has had since
[MUS-F-0011](records/findings.md#mus-f-0011) — no gate detects the export
drifting from the store. This file now sits under that open finding rather than
beside it.

<!-- mustur:generated from=MUS-D-0121 -->

### MUS-D-0121

**Destinations are a grouped list, and the kind is what tells two of them apart**

decision · 2026-08-26

asked by: MUS-F-0036

holds: MUS-D-0041

The owner asked why 'DevOfPie/Mustur' and 'Mustur' both appear as destinations and said the pair is confusing on its own. They are two different kinds of routing record: MUS-R-0001 is the repository — a remote, a checkout path, a contract file — and MUS-P-0001 is the project that contains it, which carries the MUS prefix. Filed to either, a jot comes out as MUS-F-####; only the cited destination differs, because a repository record names no prefix and falls back to the store's own. So today they are nearly the same choice, and the row gave no way to tell why there were two. Neither is removed. The distinction earns its keep the day a project has two repositories, or a jot is about the checkout rather than the work, and deleting a destination to tidy a control would be solving the wrong half. What was missing was the kind, so the kind is now the heading: Projects, then Repositories, then Machines, projects first because a jot usually belongs to one. The control itself is a list rather than a row of chips, which the owner asked to try. The chips were one line that scrolled sideways, on the reasoning that a clipped name is a wrong destination picked by accident, and they produced exactly that — MUS-F-0036, six choices in a row that could show four. A native select has no hidden end, becomes the system picker on a phone, takes type-ahead on a desktop, and is still a form control, so this surface still works with script blocked. Searching the list is noted for later rather than built: with six destinations a search box would be furniture.

### MUS-D-0122

**A sub-agent's output is read in a sheet over the session, not in the list and not on its own page**

decision · 2026-08-26

answers: MUS-Q-0056

fixes: MUS-F-0038

Three shapes were drawn and put to the owner on MUS-Q-0056: its own page at /sessions/{project}/agent/{id}, a details element opening in place, or a sheet over the session. The recommendation was the page, on the grounds that it is the only shape with a URL you can send somebody. The owner chose the sheet, and the reason it is the better answer is the one the recommendation undersold: opening a page closes the socket. The byte offset the stream resumes from lives in the page's own script, so navigating away and back re-seeds from capture-pane rather than resuming — a cost paid on every open and every back, for an address nobody had asked to send anywhere.

The sheet does not add a scripted surface. Six ship a script tag and the session view is one of them; it is also one of the only two that stop working without it, because it is a live terminal and cannot be server-rendered. So the count MUS-Q-0053 leaves open is untouched either way, and no seventh decision is being taken here. What the sheet adds is a second thing that page's client layer holds state for, which is a maintenance cost rather than a decision.

Everything the sheet shows is read back out of the row it was opened from, rather than from the frame the socket last sent. That is one code path for the server's first paint and for every rebuild after it, and it means a tap before the first frame is answered the same as one after it — the rows are server-rendered, so the alternative would have left the first few seconds of every page load unable to answer a tap.

Corrected in place: this first said the session view was one of two exceptions the composer's question and MUS-Q-0053 had named. It is one of six surfaces carrying script, and MUS-Q-0053 is the open question about what the rule counts rather than a decision granting an exception.

### MUS-D-0123

**The sub-agent list lives in a drawer that is shut by default, and the session strip is a dropdown**

decision · 2026-08-26

answers: MUS-Q-0057

fixes: MUS-F-0038

retires: MUS-D-0122

follows: MUS-D-0121

The sub-agent list moved off the session column and into a drawer that is shut on arrival, opened by a button pinned beside the session picker. Answered on MUS-Q-0057, every part chosen by the owner:

The session strip became a dropdown. That is MUS-D-0121's answer to the identical problem on the intake row — a row that scrolls sideways hides its last choice behind a swipe with nothing on screen saying so — applied to the place the same defect had reappeared. The form is real: without script the button submits it and /sessions turns the query into a path.

The drawer pushes on a laptop and opens over on a phone, against the recommendation of one behaviour everywhere. It buys what a sidebar is for over a sheet: the terminal and the list at once. The cost was named before it was built and had to be handled deliberately — the composer is placed by --shell-dock-left and --shell-dock-width rather than by flow, so it does not narrow with the content and would slide under the drawer. Both take the same min() expression, which turns out to mean nothing moves at all on a wide screen: at 1366px the reading column is 736px with 406px already empty beside it.

Output is read inside the drawer rather than in the sheet built the day before, so there is one surface for sub-agents instead of two. That retires MUS-D-0122.

The badge counts what is running and falls back to the total, because a count of only the active ones goes blank the moment they all finish — which is when their reports are worth reading, and with the drawer shut nothing else says they exist. While anything runs the whole button wears a rotating accent ring with two highlights on opposing sides: painted once and turned with the rotate property rather than by animating a gradient angle, which would repaint the whole gradient every frame. Constant speed, because a rotation that eases reads as a stutter. Removed rather than paused under prefers-reduced-motion, with the accent colour carrying the state on its own.

| Field | Value |
| --- | --- |
| Where | internal/web/sessions.go, internal/web/assets/session.js |
| Evidence | Measured against the owner's own Demo session log served under four sessions, in Chrome and Firefox at 390x844, 1000x800 and 1366x768. Shut on arrival with the old box gone entirely, the terminal holds 618-654px. At 1366 the drawer opens into space that was already empty: the composer stays 224..960 and the panel sits 1094..1366, so nothing moves. At 1000, where that space runs out, the reading column narrows from 736 to 488 and the composer narrows with it — dock 224..712 against a panel at 728, no overlap. On a phone it overlays with the veil painted. Crossing the breakpoint with it open behaves in both directions, and shutting it restores dock 224..960 and a 736px body. The ring: a conic gradient with two bright stops at 40deg and 220deg — exactly 180 apart, same accent colour, transparent stops either side of each so there is no hard edge — animated 'turn 3s linear infinite' on the rotate property, with a two-layer halo and nothing clipping it. Under prefers-reduced-motion the animation computes to none while the ring stays painted and the badge stays accent-coloured. A sub-agent starting mid-watch took the badge from the total 3 to the running 1, lit the ring and added a row without a reload. Reading a 7,267-character message inside the drawer at 390px: all of it, scrolling in 334px. Escape steps out of the reading pane first and closes the drawer second. |

### MUS-D-0124

**A control whose presence depends on scripting is decided by the browser, not by our script**

decision · 2026-08-26

fixes: MUS-F-0046

amends: MUS-D-0123

fixes: MUS-F-0044

The session picker's submit button is rendered inside a noscript element rather than rendered always and hidden by the script.

The owner asked for it to appear only when scripting is disabled, having first asked for it to be small and beside the dropdown rather than large and beneath it. noscript delivers the first exactly and makes the second moot for anyone with script.

The reason to prefer it over a hidden attribute is not tidiness. Anything the server renders and the script removes has two states that must agree, and they are delivered separately: the markup is one response and the script is another. When they disagree — a stale page, a blocked asset, a script that never runs — the failure is a control appearing that nobody expected, which is what happened. noscript is resolved by the browser at parse time from a single fact it already knows, so there is no second state to keep in step.

The general rule this leaves behind: if a control's presence depends on scripting, let the browser decide it, not our script. And a bare element selector is a rule about every element of that kind that will ever exist on the surface — the composer's form rule silently reshaped a form written months later, which is worth remembering before writing the next one.

### MUS-D-0125

**A mis-routed record is corrected by filing a new one and retiring the old, which keeps its identifier**

decision · 2026-08-26

answers: MUS-Q-0058

fixes: MUS-F-0044

first used on: IDW-F-0004

The obvious correction — a --to flag on amend — is not available, and the reason is the whole design.

The identifier is the routing. IDW-F-0004 is called IDW because it went to the idea inbox; the prefix is derived from the destination at the moment of filing. Moving a record and renaming it are therefore the same act, and identifiers are permanent.

Asked which promise gives way (MUS-Q-0058), the owner chose neither. 'mustur reroute ID --to DEST' files a new record at the right destination and retires the old one in place: it keeps its identifier, still resolves, carries Superseded by, and stops making a claim. Every citation that already exists — in a commit message, a decision, a comment nobody can search — keeps working, and no prefix ever lies about where its record lives.

The cost was chosen with open eyes: a stub is left in the wrong project's list, and its counter goes up rather than down.

Two things the implementation had to get right. It files through intake rather than writing the record itself, so the destination, the prefix and the field shape all come from the code that files everything else. And it carries the old record's title, body, date and fields across, keeping only intake's routing decision — without that, rerouting re-derived the title from the body and reset the status, quietly undoing every amendment made since the jot was filed.

| Field | Value |
| --- | --- |
| Where | cmd/mustur/reroute.go |

### MUS-D-0126

**An agent may write down an answer the owner gave elsewhere, and must say where**

decision · 2026-08-26

answers: MUS-Q-0059

fixes: MUS-F-0045

The rule stands: the asker may withdraw its own question and may not answer it, because a gate that can be closed by the thing it is holding is not a gate. What the owner allowed on MUS-Q-0059 is narrower.

'mustur answer --from-owner' takes where the answer was given, not a bare yes, and writes a Relayed field naming who wrote it down and where it came from. An unattributed relay would be worse than none, because it would read exactly like the owner having answered here. Nothing can verify the claim — on a single-tenant machine nothing could — so what it does instead is make the claim explicit, and disbelievable.

This closes MUS-F-0045: a question answered in a prompt, a plan or a conversation no longer sits open in the queue until the owner answers it a second time.

It also arrived with a hazard, which cost a record before it was caught. Nothing stopped an answer being written over an answer. MUS-Q-0056 had been answered by the owner through the queue; a relay written over it replaced their words, moved the timestamp four hours, and added a Relayed line claiming they had answered somewhere they had not. It was restored from the event log, which is the only reason this is a story rather than a loss. 'answer' now refuses a question that already carries one unless --reanswer is passed, and prints what is there. An answered question is the thing everything downstream was allowed to proceed on, and it should be hard to change by accident.

| Field | Value |
| --- | --- |
| Where | cmd/mustur/questions.go, internal/question/question.go |

### MUS-D-0127

**On a wide screen the account link is an icon at the foot of the rail, not a word in the header**

decision · 2026-08-27

narrows: MUS-Q-0052

follows: MUS-D-0124

Asked for by the owner. MUS-Q-0052 put the account link in the header so the bar would stay four tabs, and that reasoning holds for the bar and not for the rail: below the breakpoint a fifth entry would squeeze the four that say where you are, and above it the rail is a column with a free bottom edge.

So the same link is rendered twice and the stylesheet chooses — words in the header below 60rem, an icon at the foot of the rail above it. margin-top: auto is what sinks it, so nothing names a position or a height.

An icon here and words there is not an inconsistency: the rail has room for a glyph to stand alone and the bar does not. The label is on the element rather than beside it, so it reads the same to a screen reader in both.

Rendered twice and chosen by a media query, not rendered once and moved by script. A control the server draws and the script places is one that can be misplaced when the script is stale, which is MUS-D-0124's rule and the reason the picker's button lives in a noscript.

| Field | Value |
| --- | --- |
| Where | internal/web/shell.go, and the six surfaces that carry a nav |
| Evidence | At 1366x768 the rail entry sits at y=720 of a 768px rail, 191x36, and the header link is not painted. At 390x844 the rail entry is not painted and the header link is. Chrome and Firefox. |

### MUS-D-0128

**Every surface takes the width the rail leaves; a page that wants a narrower measure asks for it**

decision · 2026-08-27

Reported by the owner twice: first that the desktop UI did not use the full width, then that fixing it on the session view alone had left every other page the same.

The reading column was 46rem by default and 40rem on three surfaces, which is the right instinct for prose applied to everything. On a 1366px laptop that made every page 736px wide with 406px of nothing beside it, whatever was on it — a terminal, a table of people, a queue of questions, a form.

The default is now --shell-full: what remains after the rail and a gutter each side. A surface whose content genuinely wants a narrower measure can still say so by setting --shell-content; what has changed is that narrow is no longer assumed.

The value is a custom property rather than a max-width because the composer's width and the sub-agent drawer's push are both derived from it. Overriding max-width alone would widen a page and leave its docked parts at the old measure underneath.

One surface needed more than the default. Intake carries its own padding and had no box-sizing, so the padding landed outside the cap and it was the only page reaching the right edge while the rest kept a gutter.

| Field | Value |
| --- | --- |
| Where | internal/web/shell.go, and the surfaces that capped themselves |
| Evidence | At 1366x768 every surface now measures 1126px from 224 to 1350, with the rail ending at 208 and 16px spare on the right: sessions, records, decisions, intake and compose alike. Before: 736px on most and 640px on the three capped at 40rem. Chrome and Firefox. |

### MUS-D-0129

**The session view's live strip is gone; the pill beside the project name already said it**

decision · 2026-08-27

Reported by the owner as providing nothing useful.

It was a full-width band above the output reading 'live', while the pill beside the project name in the same header read 'running'. Two places saying one thing, and the larger one saying the less precise version.

The strip carried three other states — connecting, reconnecting, and session ended with a time. The pill already carried the first three. The time a session ended was the only thing the strip alone knew, and it has moved into the pill.

What is left is a header row and a picker row above the output, where there were three.

| Field | Value |
| --- | --- |
| Where | internal/web/sessions.go, internal/web/assets/session.js |
| Evidence | No .strip rule and no element remains; the header pill reads running, reconnecting, or ended with the time. A test fails if the strip returns. |

### MUS-D-0130

**Running or idle is read from the CLI's own pane, and the silence timer is what happens when it cannot be**

decision · 2026-08-27

needs: MUS-F-0042

follows: MUS-D-0129

The first version of this used a three-minute silence threshold. The owner pointed out that the CLI already says which it is: tmux carries the working line Claude Code prints, and a timer counting bytes is a guess standing in for a fact that is right there.

The guess is worse in both directions. A tool call that prints nothing for two minutes is working and would have been called idle; a session that finished four seconds ago is not working and would have been called running for another three minutes. Neither is knowable from the byte stream, and both are stated plainly at the bottom of the pane.

So Adapter.Doing captures the last dozen rows and reads them. 'esc to interrupt' appears in the status line for exactly as long as a turn is in flight; the input caret is drawn when the CLI wants a person. Working is checked first, because the input box is drawn during a turn as well and looking for the caret first would call every working session idle.

It is one CLI's strings and it says so. Anything else reads as unknown, and unknown falls back to the silence timer rather than asserting idle — degrading to the old guess is the right failure, where making a claim about a CLI nobody has read would not be. The threshold stays for that path.

One capture per socket tick, which is every two seconds and only while somebody is watching: the socket loop is the whole of when it runs. The ring on the pill follows the same flag, so it turns while a turn is in flight and stops when it ends.

| Field | Value |
| --- | --- |
| Where | internal/session/session.go, internal/web/sessions.go, internal/web/assets/session.js |
| Evidence | Against a real Claude Code session in tmux: at the prompt after six minutes of silence the pill reads idle with no ring; a turn started by typing into it takes the pill to running with the ring turning; the turn ending returns it to idle with the counter reading 'quiet 1s' — one second, where the timer would have said running for three more minutes. Four unit cases cover mid-turn, waiting, an unrecognised pane and a failed capture. |

### MUS-D-0131

**The four tabs are drawings in the bar and drawings with words in the rail, built in CSS**

decision · 2026-08-27

extends: MUS-D-0127

worked around: MUS-F-0048

Asked for by the owner: replace the text tabs on mobile with icons, and show icon then word on desktop. Chosen from drawings rather than descriptions, over four rounds.

The set is stroked outlines on one 22px box with one 1.7px border, so the five read as a set. A prompt for Sessions, a question mark in a circle for Decisions, a speech bubble for Intake, a page with lines for Records, and a head and shoulders for the account entry at the foot of the rail. One deliberate exception to the shared box: the bubble is 22 wide and 15 tall, because a speech bubble drawn in a square is a rounded rectangle.

Drawn in CSS rather than SVG. That began as a constraint of the tool the owner reviews these in, which refuses svg in every block it has — confirmed by publishing a bare circle on its own and having it refused. It is the better build regardless: nothing embedded, no viewBox to keep in step with a stroke width, and currentColor on a border inherits the theme with no dark-mode branch.

Intake took every one of the four rounds and each correction was the owner's. First an arrow into a tray, with the head sitting in the tray's mouth. Then seven alternatives, and the objection killed all seven rather than two: a downward arrow is the download glyph and an envelope is something that arrives, where this page is the opposite of both — you write a thought and post it. Then six from different directions, of which the bubble was chosen and immediately sent back, because it was drawn with a skewed square tail filled white, which is a white block on a dark page. Then the bubble again, because a rounded rectangle with one square corner standing in for a tail still read as a box.

It is now an ellipse with a tail and three dots: no straight edge anywhere, which is the difference between a shape that suggests a bubble and one that is a bubble. The ellipse is the element's own border rather than a pseudo-element, which leaves both pseudos for the tail and the dots and keeps the markup a single empty tag.

The dots earn their place. Side by side, a plain rounded shape in the bar reads as a box beside the document icon; with three dots it reads as a message. In a bar with no words that difference is the whole job.

The word leaves the screen and not the page: every tab keeps its span, hidden below 60rem and shown in the rail, and every tab names itself in aria-label. The count of waiting decisions stays visible in the bar where the word does not — how many are waiting is the one thing on that row worth reading at a glance.

| Field | Value |
| --- | --- |
| Where | internal/web/shell.go, and the six surfaces that carry a nav |
| Evidence | Chrome and Firefox, light and dark. The bar at 390px: four cells of 98px, icons 22x22 with the bubble 22x15, 45px tall, no word painted and all four still in the markup, five aria-labels, and no icon carrying a colour of its own in either theme. The rail at 1366px: four words painted, every icon on the same 21px left edge and every word on 52px, rows 40px, and the account entry sharing that edge at the foot. |

### MUS-D-0132

**The session view renders frames from capture-pane, and the byte stream is gone**

decision · 2026-08-27

answers: MUS-Q-0060

fixes: MUS-F-0049

and: MUS-F-0031

retires: MUS-Q-0021

The session view reads the screen tmux has already assembled, on a timer, and sends a frame when it changes. Chosen by the owner on MUS-Q-0060 after MUS-F-0049 established that the old model was interpreting a screen-painting protocol as a log.

What went: pipe-pane, the 256KB buffer, the byte offset a viewer resumed from, the replay flag, the gap message, and the whole stream.go. A viewer that reconnects is sent the current screen, which is the whole of what resuming means when the unit is a frame. MUS-Q-0021's buffer answered a question this no longer asks.

What arrived: internal/ansi, which turns the captured SGR into HTML. It renders what it understands and drops what it does not, because printing an escape is the defect being fixed, and every character of the pane is escaped on the way through — the pane's contents are somebody else's output and must never be markup. White and black fall through to the page's own colour rather than the terminal's, because a page that is light for one reader and dark for the next cannot use either extreme.

Three things fall out of it. There is no pipe, which takes MUS-F-0030 and MUS-F-0043 with it — a service that could not be stopped while piping, and a dead Mustur that held its listening port for as long as its pipe ran. The agent's working state comes free, read from the capture the poller already has rather than from a second one every two seconds. And the test suite lost about forty seconds, because nothing waits on a pipe any more.

What it costs: one capture per watched project per tick, about two and a half a second while somebody is looking and nothing when nobody is. Frames carry 120 lines of history — measured at about 7KB against 1.2KB for the visible screen alone and 42KB for the whole of it — and only when the screen has actually changed.

| Field | Value |
| --- | --- |
| Where | internal/ansi, internal/session/screen.go, internal/web/sessions.go, internal/web/assets/session.js |
| Evidence | Against a live Claude Code session in Chrome and Firefox: zero escape codes on the page where the old stream left them as literal text, colour rendered as spans, box drawing intact, monospaced, no sideways overflow at 390px or 1366px, and no page errors. A turn started by typing into the pane took the pill from idle to running and back within 1.5s of it ending. |

### MUS-D-0133

**A built surface names the path it serves, and the gate reads that line rather than trusting it**

decision · 2026-08-28

answers: MUS-Q-0061

gates: MUS-F-0027

docs/ui-surfaces.md exists to stop a surface being designed in a Go template and shown to the owner afterwards. It asked for that in prose and was ignored seven times, twice after the owner had answered on the same subject, with the rate going up rather than down (MUS-F-0027). Its own diagnosis was that a record read after the fact is not a safeguard. The owner's answer was to make the gate enforce it (MUS-Q-0061).

Each built surface now carries a **Serves** line naming its path, and `make surfaces` reads two things that can disagree: the routes internal/web actually registers, and the paths that file claims. A page served with no surface naming it fails, and so does a surface naming a path nothing serves — without the second half the gate is satisfied by writing a path down, which is the original failure wearing the opposite costume.

The routes are read with go/ast rather than a regular expression, because a pattern over source also finds a string in a comment or a fixture. A GET is a page; a POST is something a surface does. Three exclusions are named with their reasons — a script, a socket, and image bytes — and each has to match at least one live route, so an exclusion that stops matching fails as stale rather than sitting there as a hole in the shape of a rule.

What it cannot see, written down rather than left to be discovered: a surface is recognised by an explicit GET method, so a page mounted method-less — as cmd/mustur/main.go mounts the intake fallback, /mcp and /healthz — is invisible to it, as is anything served outside internal/web.

The cost is the one worth stating plainly, because it is the reason this had not been built: it will block a commit at an inconvenient moment, and somebody will want to add a surface faster than they can draw one.

| Field | Value |
| --- | --- |
| Where | internal/web/surfaces_test.go, Makefile, docs/ui-surfaces.md |

### MUS-D-0134

**An amendment keeps what it does not mention, and removing something is a thing you type**

decision · 2026-08-28

answers: MUS-Q-0063

fixes: MUS-F-0055

over: MUS-D-0126

`amend` used to state a record afresh: anything the caller did not restate was dropped. The reasoning was written into the code and is not silly — carrying fields forward silently would make `amend --title` keep data the writer never saw, and the log holds every earlier version anyway. It lost to what actually happened. The worry was about a writer surprised by what survived; the writers were surprised by what did not, fifteen times (MUS-F-0055).

Merge, then, chosen by the owner on MUS-Q-0063 over three alternatives. What is passed replaces its counterpart. What is not passed survives. `--drop KEY` removes a field by name or a citation by its label or its identifier, and `--replace` still states a record afresh for the rare time that is wanted.

The alternative closest to this tree's habits was to refuse — the shape MUS-D-0126 gave `--reanswer`. It was not taken, and the reason generalises: a guard that fires on every ordinary correction teaches everybody to pass the override without reading it, at which point the guard is decoration and the trap is back. A default that is safe needs nobody to remember anything.

Two details that are not incidental. A field passed again keeps the position it already had, because fields render in order and a correction that reorders them is a diff nobody asked for. And a citation is identified by its label *and* its target, not by its label alone, because a record can be found in two work units and cite both under the same word — keying on the label would silently collapse them.

It also closes a smaller trap in the same command: `--at` defaulted to today, so every correction restamped the record with the date of the correction rather than the date its content was true. An amendment that keeps what it did not mention keeps the date too.

| Field | Value |
| --- | --- |
| Where | cmd/mustur/main.go, cmd/mustur/merge_test.go |

### MUS-D-0135

**GitHub's anchor rule has one implementation, and the shell gate asks for it**

decision · 2026-08-29

answers: MUS-Q-0064

fixes: MUS-F-0062

and: MUS-F-0060

same shape as: MUS-F-0054

`scripts/check-links.sh` worked out a heading's anchor for itself, and so did `internal/audit/markdown.go`. On the same day they were wrong in opposite directions: the audit refused a correct anchor because it stripped a heading's backticks before reading it (MUS-F-0060), and the script accepted an anchor that does not exist because it read a `#` inside a code fence as a heading (MUS-F-0062). Both are the same defect MUS-F-0054 already recorded — two implementations of one thing, one of them approximate — and that one was fixed by deleting the second.

So: one implementation, in Go, chosen by the owner on MUS-Q-0064. `audit.Anchors` is the rule, `mustur anchors FILE...` exposes it, and the script asks rather than deriving.

The cost was named before it was paid and is not small: this gate now needs a tree that builds. It used to run on a checkout somebody was only reading. Four of the nine gates already build Go, so `make check` is unchanged — but `check-links.sh` on its own is no longer a shell script somebody can run anywhere.

Two details that keep it honest. The anchors are read once for every file the commit will contain rather than once per link, because there are thousands of links and a process each would make the gate unusable. And a link pointing at a document that enumeration did not cover — one that is ignored, or outside the tree — is read on demand rather than treated as having no headings, which would be the gate reporting on a file it never opened.

The alternative not taken was a shared fixture both implementations are run against. It keeps the shell gate independent, and it does not stop both being wrong in the same way — which is the failure a single implementation also has, at a fraction of the machinery.

| Field | Value |
| --- | --- |
| Where | internal/audit/markdown.go, cmd/mustur/anchors.go, scripts/check-links.sh |

### MUS-D-0136

**A pull request diff shows the code and two documents; the rest of the markdown folds away**

decision · 2026-09-03

answers: MUS-Q-0066

Markdown dominates a change here and most of it is not prose somebody wrote for that change. `records/` is rendered from the store by `make export`, so a diff of it is a diff of a rendering whose source of truth is the insert-only log. In one recent change it was 487 lines against 47 of hand-written prose and 1,329 of code, and it is the first thing a reviewer scrolls past.

`.gitattributes` now marks `*.md` as `linguist-generated`, which collapses a file in the pull request diff — hidden by default, one click to expand. The owner exempted two: CLAUDE.md and README.md, which are how somebody arriving works out what this is (MUS-Q-0066). Later rules win in that file, so the exemptions are written after the rule they carve out of.

The cost is named rather than left to be discovered. decisions.md, Plan.md and workflow.md are hand-written, and they fold away with the rest — a change to the contract is now one click from a reviewer rather than in front of them. That was put to the owner as the risk of this option and chosen anyway. Nothing is hidden irreversibly: the files are committed, exported and gated exactly as before, and `make check` reads them rather than the diff.

It also drops this markdown from the repository's language statistics, which is the more honest figure for a tree whose code is Go. What has not been checked is whether the same mark affects GitHub code search; the collapse and the statistics are the two effects this is relied on for.

One consequence to know before it surprises somebody: the exemptions are anchored to the root, so `ci/README.md` and `records/README.md` fold away with everything else. The second is generated; the first is a small hand-written file about the CI gate, and unfolding it is one line here.

| Field | Value |
| --- | --- |
| Where | .gitattributes |
| Evidence | `git check-attr linguist-generated` over eleven files: false for CLAUDE.md and README.md, true for decisions.md, Plan.md, workflow.md, queue.md, docs/ui-surfaces.md, ci/README.md and the records tree. |

### MUS-D-0137

**An answer keeps its choice and gains a note**

decision · 2026-09-03

answers: MUS-Q-0068

supersedes one clause of: MUS-D-0055

raised by: MUS-F-0071

MUS-D-0055 made an answer a choice between options and put free text beneath them, beating a choice when both were sent. The reasoning for the override still holds: the owner wanting to say something the list does not contain is the case a list is worst at. What it did not anticipate is the owner wanting to say something *about* an option — and the only way through the surface was to retype the option's label into the box and append the remark, after which the record said what was typed and never which option it named.

The owner chose to keep the choice. A chosen option is still the answer, verbatim and matchable back to the option it names; the box beside it becomes a note on that choice, kept in its own field. Free text with no choice is unchanged and is still the answer itself, which is the clause of MUS-D-0055 that this leaves standing.

A session waiting on the answer is told both, joined into one sentence: an agent handed only the label would act on an option the owner had qualified.

The two alternatives were weighed and refused. Folding the note into the answer string is the smallest change and destroys the property that made options worth having. Prefilling the box with the chosen label fixes the retyping the owner named and not the thing underneath it — the record still ends up saying only what was written.

| Field | Value |
| --- | --- |
| Stored as | Answer keeps the option's label; Note holds the remark. No note is written when there is none, so a plain answer renders exactly as it did |

### MUS-D-0138

**The decision log's tail is generated, and the marker carries its own boundary**

decision · 2026-09-03

answers: MUS-Q-0069

raised by: MUS-F-0073

decisions.md fell seventeen decisions behind the store — MUS-D-0121 through MUS-D-0137 — and every gate passed throughout, because nothing compares the two (MUS-F-0073). The owner found it by looking for a decision and not finding it.

Three ways out were offered: hand the file over to a pointer and add a gate, backfill seventeen essays, or generate the tail. The owner chose to generate it.

So the file keeps its prose, its index and its append-only rule down to a marker, and everything below the marker is rendered from the store by `make export`. The two halves read differently on purpose: above is an argument with its corrections and retractions inside it, written when a decision earns one; below is the decision as the store holds it. A decision that deserves an essay still gets one written above.

**The marker carries the boundary**, `<!-- mustur:generated from=MUS-D-0121 -->`, rather than the code holding a constant. The hand-written half stopped where it stopped for reasons that are in the document, and a number in Go would be a second place to keep them in step. A file with no marker, or a marker naming no boundary, is refused with the file untouched — the alternative is guessing where the prose ends, and guessing wrong deletes it.

**It is not part of `export.Write`, and the flag is off by default.** Write owns a directory it may prune; this edits one file it must not otherwise touch. The running service exports `records/` from a systemd unit whose filesystem is read-only everywhere else, so `--decisions` is passed by the Makefile and by nothing else. A daemon that could write the checkout's root is a different thing from a daemon that renders its own export directory.

Nothing yet fails when the file is stale. `make export` is what keeps it current, which is the standing `records/` has had since MUS-F-0011 — this file now sits under that open finding rather than beside it.

| Field | Value |
| --- | --- |
| Marker | <!-- mustur:generated from=MUS-D-0121 --> in decisions.md; everything below it is replaced on every export |
| Run by | make export, which passes --decisions decisions.md; the service does not |

### MUS-D-0139

**Nine questions closed on a prompt's word are reopened, and the gate holds them open**

decision · 2026-09-03

answers: MUS-Q-0070

raised by: MUS-F-0074

the rule the relays were written under: MUS-D-0126

MUS-F-0074 found the prompt returning an option the owner had not chosen: five calls in one session, each returning the first and recommended option, and the owner confirmed they had answered none of them. Ten questions in this store carry a `Relayed` field; nine of them — MUS-Q-0058 through MUS-Q-0066 — cite a prompt as where the owner gave the answer. Each closed a question. Each drove behaviour that shipped.

Three ways forward were offered: review the nine in one pass, let them stand until something contradicts one, or reopen all nine. The owner chose to reopen all nine, which was named as the safest and the most expensive.

So each of the nine is open again and marked as one the work cannot proceed without, which is what puts it in front of `make check` rather than leaving it to be waited out. The gate is red until the owner answers them, deliberately: that is the difference between reopening a question and noting that it might be wrong.

**Nothing was deleted and nothing was reversed.** The relayed text is kept verbatim on each record as `Unconfirmed answer`, beside when it was recorded and by whom, so a reader can see what was acted on. No code was changed and no shipped behaviour was rolled back — a decision that turns out to have been right will be answered the same way, and one that does not is a defect with a record already pointing at it.

MUS-Q-0057 is not among them. Its answer was relayed from a question form on a published plan rather than from a prompt, which is a surface the owner used directly.

| Field | Value |
| --- | --- |
| Reopened | MUS-Q-0058, MUS-Q-0059, MUS-Q-0060, MUS-Q-0061, MUS-Q-0062, MUS-Q-0063, MUS-Q-0064, MUS-Q-0065, MUS-Q-0066 |
| Kept on each | Unconfirmed answer, Unconfirmed because, Originally recorded |

### MUS-D-0140

**The Answer button dims in CSS, so nothing is retired and no script arrives**

decision · 2026-09-03

answers: MUS-Q-0071

raised by: MUS-F-0075

the clause it does not retire: MUS-D-0055

MUS-Q-0071 offered three ways to make Answer unavailable until there is something to answer with, and the owner took none of them: *can we enable the button for a chosen option or text in the box?* That is option three's behaviour without option three's price, and it turns out to cost nothing at all.

`:has()` asks the form whether any radio is checked; `:placeholder-shown` asks whether the box is empty. Both are live, both are CSS, and each question is its own form so the rule scopes to one question rather than to the page. A question offering no options has no radio to check and so turns on text alone, which is what it should do.

So MUS-D-0055's surviving clause stands — text with no choice still answers a question that offers options — and the decision queue is still a page with no script on it. Neither of the costs the menu asked the owner to accept is paid.

**It is an affordance and not a guard, and the record should say so rather than let a reader assume otherwise.** `pointer-events: none` stops a pointer and not a keyboard, and a browser without `:has()` applies none of it and gets the button as it was. The refusal that actually holds is the server's, which has been there since the surface was built and is unchanged. Withdraw is untouched on purpose: closing a question with no answer is the one thing that has to work when nothing is chosen.

This is the third time the owner has improved on the menu rather than picking from it.

| Field | Value |
| --- | --- |
| How | form:not(:has(input[type=radio]:checked)):not(:has(textarea:not(:placeholder-shown))) button.primary |
| What still refuses | the server, unchanged; this is what the button looks like, not what the form accepts |

### MUS-D-0141

**The session view sends seven keys, and Escape is the one it was built for**

decision · 2026-09-04

answers: MUS-Q-0072

what it is for: MUS-Q-0073

raised by: MUS-F-0080

the decision it excepts: MUS-D-0096

Everything reaching a pane was a line of text followed by Enter, because `Send` refuses empty text and MUS-D-0096 made a message the unit. A pane asking for a keypress rather than a sentence was visible and unreachable (MUS-F-0080). The owner chose a small row of keys above the composer over sending nothing and over becoming a full keyboard.

**Escape, Enter, the four arrows, Ctrl-C.** Exactly seven, in an allowlist mapping a name the browser sends to a name tmux understands. Not a pass-through: this package shells out to tmux with the caller's string as an argument, and `send-keys` reads names like `C-c` out of it, so a browser naming its own key would be a browser choosing what tmux does to a pane. The next key wanted is a line in that map and an argument for what it is for.

**What it is actually for is MUS-Q-0073.** That question was asked as *does stopping a session get a control*, which was the wrong question — the owner meant interrupting an agent mid-turn to correct it, having noticed it misreading them, which in the terminal is Escape. So the row is not a convenience beside the real fix; it is the fix, and Escape is its first button.

**`SendKey` is separate from `Send` rather than a mode of it.** Send's whole argument is that a message is text and goes in as a paste that says so; keeping them apart is what stops *send this text* quietly growing a way to press Ctrl-C. No Enter follows a key — Send types a line and submits it, and this presses what it was asked for and nothing else, because a stray Enter would answer a dialog the owner had only meant to look at.

**Keys are paced at 60ms where a message is paced at 250ms.** Nobody writes two sentences in a quarter second and the limit is there to stop a pane being flooded with prose; moving four rows down a list at one press per 250ms is the latency that makes a control feel broken. They have separate budgets, so holding an arrow does not spend the composer's.

The row sits outside the form. A button inside a form submits it, which is the defect this row exists to be the opposite of. Ctrl-C sits apart from the keys that move around inside a dialog, and carries no tick: a row of seven buttons that each need confirming is not a row anybody would use.

| Field | Value |
| --- | --- |
| The seven | escape, enter, up, down, left, right, cancel (C-c) |
| Measured | Against real tmux with 'cat -v' as the probe, which prints control characters rather than acting on them: Escape arrives as ^[ and Up as ^[[A, on one line, so nothing was appended to either |
