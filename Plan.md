# Mustur — Project Plan

Scope contract and specification. States **what** is true, not why.

| | |
| --- | --- |
| Rationale for every decision | [decisions.md](decisions.md) |
| How the work is done | [workflow.md](workflow.md) |
| Came from | [DevOfPie/IdeaWarehouse `ideas/agent-workflow-web-platform.md`](https://github.com/DevOfPie/IdeaWarehouse/blob/main/ideas/agent-workflow-web-platform.md) |
| Last updated | 2026-09-10 |

**Core rule:** Mustur holds only what no single machine can hold, and reaches an
agent by being called rather than by being available.

## Vision

One authenticated web surface over agent work that happens on machines the owner
already owns. Mustur does not reimplement an agent and never attaches to a
session it did not start. It starts and keeps sessions itself, through a small
adapter on each machine; it is the system of record for the milestones, findings
and decisions of the projects that have moved into it; it holds the routing that
says where work goes and which machine holds it; and it delivers all of that into
a session by being **called**, from a thin repo-local file that mandates the call.
It serves one solo developer running several agents across roughly eight
repositories from a laptop on the home LAN and a phone off it, plus the occasional
collaborator invited in to look at one thing without cloning anything.

Mustur is its own first project. Its records are kept in the same shape the other
projects use, so that the transition is exercised on the one project that can
afford to get it wrong.

## Principles

1. **Inject, never offer.** A capability that depends on an agent choosing to call
   it is not shipped. Measured: 0 memory operations in 114 turns against a
   pre-seeded store.
2. **Only what is not local.** If a repo-local file on one machine already does
   it, Mustur does not. The repository keeps project detail and the mandate;
   Mustur keeps what crosses a machine or a person.
3. **The client stays flat as sessions grow.** One browser tab against N
   server-side sessions. A feature that costs per-project client state is rejected
   on that ground alone.
4. **A decision reaches the owner as a prompt or it has not been raised.** Prose
   asking a question is not a question. Mustur blocks on unsurfaced decisions and
   routes the answer back rather than trusting anyone to remember.
5. **Never embed; link out only when it earns it.** A link opens in a new tab and
   never replaces the one the owner is in. It is permitted only where linking is
   necessary because no reasonable alternative exists, or measurably better in
   performance or function than integrating.
6. **Small enough to be wrong.** The owner expects significant ongoing change
   discoverable only through usage, so every module ships at the smallest size
   that can be used daily and every boundary is provisional.

---

## Stack

Only what is actually decided. "Undecided" is a legitimate entry and is more
useful than a guess that later reads as a commitment.

| Layer | Choice |
| --- | --- |
| Language | **Go.** One long-lived process on the VM under systemd, supervising child sessions; static binary; official MCP SDK mounts beside the server-rendered routes. |
| Storage | **SQLite, with a deterministic markdown export** as the audit surface and backup; insert-only enforced by trigger and tool layer; history as an immutable event log. Records are addressable by identifier from the moment they are written, and routing is authored in exactly one place rather than duplicated per machine. |
| Record shape | **StrucGu's module roles.** Four are record kinds — `decision-log`, `findings-queue`, `investigations`, `work-units`. `triage-rule` describes a document rather than a set of records and is not one; that correction is [in the log](decisions.md#four-of-strucgus-five-roles-are-implemented-and-the-repository-adopts-five-modules). Already specified with fixtures and an audit vocabulary, and already declared by four repositories. |
| Agent interface | MCP over HTTP, registered **per machine at user scope** with `claude mcp add --scope user`, and the mandate clause committed to the checkout. This row said the opposite until 2026-09-10 — registered per repository, so the trigger is committed rather than installed — and a tracked `.mcp.json` was removed on 2026-09-02 because it is the one thing that cannot carry a credential and is preferred over the configuration that can ([MUS-Q-0065](records/questions.md#mus-q-0065), [MUS-F-0063](records/findings.md#mus-f-0063)). What is committed is the clause; what is installed is the server and its token. |
| Agent transport | A per-machine adapter that starts and supervises long-lived sessions **inside tmux**, shelling out to the configured CLI. Sessions survive adapter restarts and stay attachable from a terminal; Mustur still never attaches to a session it did not start. They do **not** survive a reboot — nothing does — so what is kept across one is a note of what was launched and which conversation it was having, and a button that starts it again ([MUS-D-0149](records/decisions.md#mus-d-0149)). Vendor neutrality is a designed boundary **except where a capability belongs to one vendor**: sub-agent visibility and, from milestone 8, the gate that holds a tool call in front of the owner both ride on Claude Code's own lifecycle hooks, so `Start` appends them to a command whose program is that CLI and leaves anything else exactly as given, showing no rows rather than failing to start. The owner named the boundary that way on [MUS-Q-0028](records/questions.md#mus-q-0028) rather than let it be crossed quietly. Only Claude Code is proven. |
| Human interface | **Server-rendered HTML by default, with a client layer only where a surface needs one.** No per-project **server** state per client. **Every page that renders the bar carries a script now, and every page works without it but two.** `bar.js` polls the decision count and writes the badge, on the six surfaces that draw the bar, since [MUS-Q-0078](records/questions.md#mus-q-0078) — until 2026-09-10 this row said intake and the decision queue carry no script at all, which stopped being true on 2026-09-04. Sign-in and accepting an invitation draw no bar and carry no `bar.js`; what they carry is the ceremony script, and the page shown when an invitation has already been used carries nothing at all. Every page still renders its own count server-side; what stops without script is the number changing. Six surfaces carry a script for something only script can do — the session view, the composer and the four authentication surfaces, and on two of those four it is the only script on the page rather than a second — and two of the six stop working without it: the session view, which is a live terminal, and the passkey ceremony, which is a browser API. The composer was taken deliberately on [MUS-Q-0034](records/questions.md#mus-q-0034) because a draft cannot survive a backgrounded phone; the account page on [MUS-Q-0047](records/questions.md#mus-q-0047), because adding a passkey had had a page of its own that held a heading and one button. Both post their forms and work with the script blocked. What the count should measure is open on [MUS-Q-0053](records/questions.md#mus-q-0053), and [MUS-Q-0078](records/questions.md#mus-q-0078) moved the numbers rather than settling it. The session view's client layer holds one socket and the screen as it stands: there is no byte offset, no buffer and no replay since [MUS-D-0132](records/decisions.md#mus-d-0132), and this row described an offset until 2026-09-10. `/sessions` redirects into the session view whenever anything is running, so the only unscripted rendering of it is the empty state — and the start page itself when it is asked for by name, which is how a session is started and how a lost one is offered back ([MUS-D-0146](records/decisions.md#mus-d-0146), [MUS-D-0150](records/decisions.md#mus-d-0150)). The exception is the session view at milestone 4b, which streams a live session over a WebSocket and cannot be server-rendered; the owner took that on [MUS-Q-0008](records/questions.md#mus-q-0008) and corrected this row forward rather than reopening milestone 2, on [MUS-Q-0011](records/questions.md#mus-q-0011). The exception is named so the rule is not quietly dropped. |
| Deployment | The VM already running `cloudflared` 2026.8.2. **One new ingress rule on the existing tunnel — never a second tunnel** (cloudflare/cloudflared#59). That tunnel is token-managed, so its rules live in Cloudflare rather than on disk, and it is LinkCtrl's — both read on 2026-08-20 and written up in [docs/ingress.md](docs/ingress.md). |
| Identity | Cloudflare Access at the edge **and, from milestone 5b, accounts of Mustur's own**: invite-only, passkeys, and a role per project. Access is kept in front while those are built and comes off when the owner judges Mustur ready to stand alone ([MUS-Q-0039](records/questions.md#mus-q-0039)) — until then a stranger never reaches the origin, which is what makes building the second gate safe. No client on the phone; the free tier is 50 users. |

## Scope

Authoritative. Where this table and prose elsewhere disagree, this table wins.

| Capability | In v1 | Later | Never |
| --- | --- | --- | --- |
| Routing registry — repositories, machines, cross-repo projects | yes | | |
| System of record for milestones, findings and decisions, addressable by identifier | yes | | |
| Records and routing behind one repo-scoped tool call | yes | | |
| Repo-local injection kit — a mandate clause in `CLAUDE.md`, with the server registered at user scope | yes | | |
| Decision enforcement — block on unsurfaced decisions, route the answer back | yes | | |
| Intake — capture from any device into Mustur's own `findings-queue`, with routing hints | yes | | |
| Conformance audit over the records Mustur owns, against StrucGu's check vocabulary | yes | | |
| Conformance audit over another repository's declared files | | at that repository's onboarding | |
| Composer, multi-line and spell-checked, usable from a phone | yes | | |
| Long-lived sessions owned by Mustur, via a per-machine adapter | yes | | |
| Read access for a second person, by Access policy | yes | | |
| A second agent CLI fitted to the adapter | | yes | |
| Onboarding a second project, and taking over its records | | yes, with its own verdict | |
| Repository and commit activity | | yes | |
| Reaching served demos behind the same door | | yes | |
| Usage pacing aggregated across machines | | pending triage of its jot | |
| IdeaWarehouse's gates as an enforced module | | pending triage of its jot | |
| Reading, restructuring or writing any other project's files | | | never, until a project is deliberately onboarded |
| Attaching to a session Mustur did not start | | | never |
| Reimplementing an agent, or depending on a private interface | | | never |
| Embedding any backend in a frame | | | never |
| A hosted product for anyone outside the owner's invitees | | | never |

The two "pending triage" rows are jots naming Mustur as their host. They are
listed so the boundary is visible, **not** because either has been accepted;
triage decides them in IdeaWarehouse.

## Non-goals

What this deliberately will not do, and why. Omitting this means re-litigating
the same expansion in three months.

| Not doing | Because |
| --- | --- |
| Attaching to a session already running in a terminal | No documented interface exposes another process's session, so it would mean depending on a private one. Mustur converses only with sessions it started, and a session left in tmux is not visible in Mustur and will not become visible. |
| Touching another project's files before it is onboarded | The owner's instruction, and a sound one: a half-built router that has already edited eight repositories is worse than no router. Onboarding is a milestone with its own verdict. |
| ID expansion as a rendering trick over someone else's prose | It was retrofitting addresses onto text that has none. The numbers this row used to give — 10 of 247 decision identifiers under a heading — were stale by a factor of twenty-seven on the count and fifteen on the proportion: 271 of LinkCtrl's 444 identifiers are addressable today, against the 10 of 247 this row claimed, because it adopted a per-decision heading at D16 ([MUS-F-0118](records/findings.md#mus-f-0118)). Where the row is right, and by more than it claimed, is findings: all 381 rows in that repository's deferred-findings file are table rows and none is addressable at all. Records Mustur owns are addressable when written. |
| Embedding backends behind one origin | Documented to break back and forward, deep links and iOS scrolling at specification level; the identity provider refuses framing outright. |
| A second git repository for documentation | Already run as `TradeShop-Support` and found unsatisfying: per-session rediscovery, cross-repo authentication, and a 13,595-byte file that exists only to explain the split. |
| A gate on plan usage | Subscription plan usage is not readable programmatically, so a gate cannot be built and claiming one would be false. |
| Scoring, ranking or dashboards over records | Fake precision, and the same non-goal IdeaWarehouse already holds. |
| Serving anyone the owner has not invited | One maintainer, no support surface, and the category's graveyard is full of hosted layers that died funding themselves. |

---

## Milestones

A sketch, not a schedule. The point is to prove the idea decomposes at all —
being unable to break it into ordered, individually shippable pieces is stronger
evidence against feasibility than any effort estimate.

Each milestone should be independently completable and leave the project working.

| # | Milestone | Done when |
| --- | --- | --- |
| 1 | The delivery bet is tested | The smallest disproof has run in two throwaway repositories with its decision rule fixed beforehand, and the result is recorded either way. Nothing below starts until it passes. |
| 2 | Records and routing, behind one call | Mustur holds its own records in StrucGu's module roles, addressable by identifier, plus its own routing record, and one repo-scoped tool call returns them. Mustur's own `.claude` carries the mandate. This clause is kept as milestone 2 was measured against it; the mandate has since moved to the bottom of [CLAUDE.md](CLAUDE.md) and `/.claude/` is ignored, so a reader checking the tree today should look there ([MUS-Q-0065](records/questions.md#mus-q-0065)). |
| 2b | The audit StrucGu never shipped | Mustur checks its own records against StrucGu's check vocabulary and emits an audit in the specified form, with fixtures proving the checks detect something. Records and audit ship together, because auditing records you own is far cheaper than auditing files you do not. |
| 2c | Intake | A jot from a phone lands in Mustur's `findings-queue` in seconds, carries a routing hint where one is obvious, and defaults to the idea inbox where it is not. |
| 3 | Decisions cannot be buried | An agent working Mustur cannot report work complete while an open decision has never been surfaced as a prompt, and the decision lands in a queue the owner answers from any device. The answer reaching the session that raised it moved to milestone 4, which is where the machinery that can reach a session arrives; the owner decided that split on a prompt, and [decisions.md](decisions.md#injection-belongs-to-the-milestone-that-owns-sessions) records why. |
| 4a | Sessions Mustur owns | The per-machine adapter starts a long-lived session per project inside tmux, reports which are running, stops one, and refuses to touch any session it did not start. An answered decision is delivered back into the session that raised it, which is the clause milestone 3 could not honour without this. Supervision and surviving a dropped connection moved to 4b on [MUS-Q-0015](records/questions.md#mus-q-0015), because an earlier version of this row narrowed both without asking — the scope contract being edited by the thing it measures. |
| 4b | A session in a browser tab | Output streams to a browser tab over a WebSocket, and **survives a dropped phone connection**. One tab, several sessions. The adapter **supervises** what it started: a session that dies is noticed and said so, rather than discovered. Both clauses moved here from 4a on [MUS-Q-0015](records/questions.md#mus-q-0015) — a dropped connection needs something connected, and supervision without anything watching the output is a restart loop. This is the surface the stack table's client-layer exception is for, and the only place in v1 where server-rendered HTML is not enough. **The connection carries an agent's output out and the owner's keystrokes in**, so the WebSocket refuses any origin but its own — Access authenticates the person and does nothing about a socket opened by a page they happened to visit. The composer is embedded and always writable: the owner took that on [MUS-Q-0018](records/questions.md#mus-q-0018), which makes the origin check and the Access policy's scope the only things between a stranger and an agent's input rather than one layer of two. Sub-agents are **not** here — they moved to 4c on [MUS-Q-0017](records/questions.md#mus-q-0017). |
| 4c | Sub-agents are visible | A session that spawns sub-agents shows them as their own rows — which are running, for how long, and one readable without losing the parent. **It started by establishing whether that was possible at all**, with the decision rule committed before any evidence was looked at so that *cannot be done* stayed a permitted verdict: [investigation 0002](docs/investigations/0002-sub-agent-visibility.md). It can be done, and not the way this table expected — the adapter cannot place a sub-agent anywhere, because a sub-agent is a call inside the CLI's one process. The route is the CLI's lifecycle hooks, which leave the pane exactly as 4b built it. Mustur installs the hook per session on the command line it already builds, so nothing of the owner's is modified ([MUS-Q-0024](records/questions.md#mus-q-0024)), and a row shows only what a documented interface carries: its task, its age, the tool in flight, and its output once it ends ([MUS-Q-0025](records/questions.md#mus-q-0025)). Split out of 4b on [MUS-Q-0017](records/questions.md#mus-q-0017). |
| 5 | Composition | The owner composes multi-line, spell-checked text from the phone, off the home network, without a terminal, and it reaches the intended session. |
| 5b | Accounts | Mustur knows who is asking. A person is invited, registers a passkey, signs in, and is recognised on their next visit from their own device; a passkey lost with a device can be replaced without losing the account. An account carries a role **per project**, and a role that only reads cannot reach the surfaces that type into a running agent. **Cloudflare Access stays in front throughout** and comes off when the owner judges Mustur ready to stand on its own, which is a separate act and not part of this milestone's verdict. Raised on [MUS-Q-0039](records/questions.md#mus-q-0039) after the builder proposed something smaller. |
| 5c | An agent carries a token | An agent reaches the mandated tool call while accounts are enforced. A passkey is a person's credential and an agent has no browser, so Mustur issues a token an agent carries in a header, with its own lifetime and its own revocation. Until this exists, `--accounts` and the tool call cannot both be on — which the builder found by measuring, not by reading — and `--accounts` stays off. Raised on [MUS-Q-0051](records/questions.md#mus-q-0051), where the owner took this over exempting `/mcp` inside the guard, because leaning on Cloudflare Access is what standing alone means not doing. |
| 6 | A second person | Someone who is not the owner signs in and reads a project's routing and records from their own device, without a clone and without a machine. Said *through Access* until milestone 5b was raised; whichever gates are in front on the day, the clause is that a second person gets in and reads. **Its reading surface already exists**, built inside 5b's branch — a scope decision announced in a commit message rather than asked, and left standing on [MUS-Q-0054](records/questions.md#mus-q-0054) rather than undone. What remains is the person. |
| 7 | A second project moves in | Its own verdict, not assumed here. The first project onboarded proves the transition the record shape was chosen to test. |
| 8 | A tool call is answered without the screen | A tool call the session is about to make reaches the surface because Mustur was **told** about it — the tool, its input and the session — rather than because a parser read a dialog off the pane, and the owner's answer resolves it. **Which calls** is Mustur's own rule and not the CLI's: nothing tells Mustur which calls the CLI would have drawn a dialog for ([MUS-F-0122](records/findings.md#mus-f-0122)), so a named set of tools in a permission mode where the CLI prompts is what is held ([MUS-D-0153](records/decisions.md#mus-d-0153), [MUS-D-0154](records/decisions.md#mus-d-0154)). This row said *a permission dialog the CLI would draw* until 2026-09-10, which is neither what was built nor what could be. The route is the CLI's own lifecycle hooks, injected on the command line `Start` already builds, so nothing of the owner's configuration is touched and the session stays a tmux terminal a person can attach to. **It was gated on an investigation whose rule was committed before any evidence was looked at** ([0003](docs/investigations/0003-dialog-without-the-screen.md)), and the row stayed out of this table until that returned: `PreToolUse` carries all three properties on five consecutive trials, `PermissionRequest` fires with a full payload and its decision is ignored, and a hook nobody answers times out into the CLI drawing its own dialog. **So the pane parser is not retired** — it is the fallback, and two paths to one dialog exist for as long as both do. Bounded by the owner's answer on [MUS-Q-0094](records/questions.md#mus-q-0094) to the hooks and this vendor: no interface per vendor, which is [MUS-Q-0085](records/questions.md#mus-q-0085)'s option 1 and stays unbuilt rather than foreclosed. The picker half of the parser is not in scope either — the model picker, the effort cycler and the toggles of [MUS-F-0101](records/findings.md#mus-f-0101) are drawn on the owner's own keypress and fire no hook. **It does nothing in a mode the CLI never prompts in**, which on this machine is every session started from the surface: the owner's default is `auto`, and on [MUS-Q-0100](records/questions.md#mus-q-0100) they chose to leave it that way rather than have Mustur override a setting they made on purpose ([MUS-D-0158](records/decisions.md#mus-d-0158)). |

## Known limitations

Accepted before building, each with its consequence.

| Limitation | Consequence |
| --- | --- |
| The requirements are only discoverable by use — the owner said so explicitly | The plan will change materially after first use. Every boundary above is provisional, and a boundary defended on the grounds that it is written here is being defended wrongly. |
| Mustur's sessions are not the terminal's sessions | Work left running in tmux is invisible in Mustur, permanently and by design. If the owner's mental model is "open Mustur, continue the session I left", that model is wrong and will be discovered in week one. |
| Vendor neutrality is designed, not proven | Only Claude Code is fitted to the adapter. A second CLI may not share the same session lifecycle, and the boundary can only be validated by fitting one. |
| The composer is the largest and least certain estimate | Five to eight days is for something usable daily, not finished. It is simultaneously the piece most likely to be abandoned and the piece without which the rest is a registry with a web page. |
| Delivery depends on how one vendor's client loads repo-local files | An upstream change to context-file or project-scoped MCP behaviour breaks injection, and nothing inside Mustur can compensate. |
| Repo scoping is available but not enforced | A personal skill at `~/.claude/skills/` still fires in every repository. Mustur can hold the registry; it cannot stop someone installing a global skill that overrides it. |
| The gate is off wherever the permission mode says do not ask | Milestone 8 holds a tool call only in a mode where the CLI would have prompted, and this machine's default is `auto`. So the capability is built, measured and deployed, and does nothing in the ordinary use of the deployment it runs on. Accepted on [MUS-Q-0100](records/questions.md#mus-q-0100) rather than worked around: a mode that says do not ask is the clearest instruction there is, and overriding it would make Mustur the permission system by accident. Widening the gate later is that question answered differently |
| Mustur is its own first project | Requirements surfaced on a project that had no production code and no build until its second milestone, and that still has no users but one. That is unlike every project meant to follow. The transition milestone exists because of this, and cannot fully compensate for it. |
| Records served by tool call are not measured against records in the repository | If retrieval degrades, the packaging win is bought with task success and nothing here would detect the trade. |
| Plan usage is not readable programmatically | Any usage module estimates rather than measures and can never gate. Its only advantage over the built-in estimate is aggregating across machines. |
| Cloudflare holds identity and the edge | An outage there takes the whole surface down, and there is no local fallback path by design. |
| Mustur becomes the runner StrucGu deliberately did not ship | Four repositories' adoption records start depending on a service one person maintains. The blast radius of abandoning Mustur grows from "Mustur" to "every repository that declared a module". |
| StrucGu could later ship its own runner | There would then be two implementations of one spec, and Mustur's would be the unofficial one. Nothing prevents this and no agreement covers it. |
| LinkCtrl declares no StrucGu adoption | It is the largest and most active corpus and its `M`/`F`/`D` shape predates the spec — StrucGu was built from the structure LinkCtrl started and the transition never happened. Onboarding it means a mapping exercise, expected and assigned to that repository's agents when the time comes, not a lift-and-shift. |
| One maintainer, and this is tooling for the work rather than the work | The most likely failure is not being wrong but being finished at 70% and abandoned, with sessions and records then depending on a half-built service. |

## Success criteria

How you will know this worked. Written now, while you can still be honest about
it, and specific enough that failing them is unambiguous.

1. A project becomes routable by editing **one record in Mustur**, with no change
   to any global file on any machine, and a session started in it names the right
   target without being told.
2. A decision raised by an agent reaches the owner as a **prompt on whatever
   device they are holding**, and no piece of work is reported complete with an
   unsurfaced decision behind it.
3. The owner composes and sends multi-line, spell-checked text from the phone, off
   the home network, without a terminal, and it reaches the intended session.
4. A record written six months earlier is retrieved by its identifier, by an agent
   and by a human, without either of them knowing which file it lives in.
5. A second person reviews a design decision without cloning anything and without
   being given access to a machine.
6. Client cost does not grow with concurrency: one browser tab at eight live
   sessions, on a laptop with 1.5 GB free.
7. Mustur audits its own records against StrucGu's check vocabulary and the run
   is reproducible by someone who does not trust it, with fixtures showing the
   checks fail when they should.
8. A jot made from the phone is in Mustur's queue in under fifteen seconds and
   never required a decision to file.
9. No file in any project other than Mustur has been modified before its
   onboarding milestone is deliberately started.

---

## Build status

**Milestones 1 and 2 have passed. 2b, 2c and 3 are built and reviewed; 2c is now
reachable, gated, and proven from the owner's phone, so nothing in it is
outstanding. None of 2b, 2c or 3 is accepted. 4a, 4b, 4c and 5 are built and
reviewed — 5 twice, and rebuilt in between — and none of them is accepted
either. **5b was raised on 2026-08-24 and inserted before 6; 5c came out of 5b's
own review and was inserted before it. Both are built, reviewed and merged —
pull requests 30 and 31, on 2026-08-28 — and since 2026-08-26 both are deployed
and enforced on the owner's machine. Everything through 5c is merged. Nothing
below 5c is merged.** Until 2026-09-10 this paragraph said neither was merged,
thirteen days after they were. **Milestone 8 is built and not merged**: it was
raised on [MUS-Q-0094](records/questions.md#mus-q-0094) once
[investigation 0003](docs/investigations/0003-dialog-without-the-screen.md)
returned, and is the first row below 5c that anything has been built for.

| # | State | Evidence |
| --- | --- | --- |
| 1 | passed 2026-08-19, 20 of 20 against a rule of 18 of 20 | [the investigation](docs/investigations/0001-mandated-tool-call.md) |
| 2 | passed 2026-08-20, reviewed by three agents that did not build it and every finding dispositioned | [the records](records/README.md), and the `mustur_route` tool they are served by |
| 2b | built and reviewed 2026-08-20; awaiting acceptance | `make audit` over this repository, and 344 expected states across 37 of StrucGu's fixture trees |
| 2c | built and reviewed 2026-08-20. Reachable and gated 2026-08-21: `mustur.devofpie.com` behind Cloudflare Access, answered by a service that starts at boot. **Proven end to end 2026-08-22** by the owner, from a phone, through Access — which nobody else could do | Three jots from the owner's phone, all routed correctly and all in `records/findings.md`: `MUS-F-0023` and `MUS-F-0024` name Mustur and went to `DevOfPie/Mustur`; `MUS-F-0025` names nothing and went to the idea inbox, which is the clause's own default case. `MUS-F-0024` is a real defect the owner noticed while filing. Earlier: `MUS-F-0022` through the running service, and ten filings on loopback at a median of 1.71 ms, worst 2.10 ms, method in [docs/ingress.md](docs/ingress.md) |
| 3 | built and reviewed 2026-08-21, split by the owner so that injection moves to milestone 4. An open question blocks `make check` when it was never surfaced, or when it is marked as one the work depends on; the queue is answerable from a phone | `records/questions.md`, `make questions` over it, and the questions this repository has raised — including the four carrying `Blocks: MUS-M-0005`, which stopped this milestone's own build until they were surfaced. The count is in [records/README.md](records/README.md) rather than restated here, because restating it is how it went stale twice |
| 4a | built and reviewed 2026-08-21, awaiting acceptance. The adapter starts a session per project inside tmux, lists and stops only sessions it started, and carries an answered decision back into the one that raised it | Against real tmux: a session named `mustur/zzfake` created by hand was invisible to `list` and refused by `stop`, while a session Mustur started in the same server was listed; a command exiting immediately is reported as such rather than as started; a missing `--dir` is refused rather than silently becoming `$HOME`; an answer reached a live session's input and an answer whose session had gone was recorded with the reason. Method in [records/work-units/MUS-W-0016.md](records/work-units/MUS-W-0016.md) |
| 4b | built and reviewed 2026-08-22, awaiting acceptance. A session's output streams to a browser tab over a WebSocket; a dropped connection reconnects and is handed the screen as it stands; a session that ends says so; one socket at a time, and switching sessions is a page navigation rather than a second connection. **Replay is how this shipped and is not how it works now**: [MUS-D-0132](records/decisions.md#mus-d-0132) replaced the byte stream with whole screens on 2026-09-01, and the pipe, the buffer, the offset and the gap message went with it | Against real tmux and the running server: a cross-origin handshake carrying a valid session is refused **403**, and an origin-less one with it; a same-origin socket received the pane's backlog and then live output; text typed over the socket arrived in the session's stdin; a reconnect replayed the gap with every line appearing exactly once, which is what the evidence said when the socket carried bytes; a stopped session refuses the socket **404** and its page says Mustur did not start it. Method in [records/work-units/MUS-W-0017.md](records/work-units/MUS-W-0017.md) |
| 4c | built and reviewed 2026-08-22, awaiting acceptance. A session's sub-agents appear as their own rows — above the output when this shipped, and in a drawer shut on arrival since [MUS-F-0038](records/findings.md#mus-f-0038) grew one to 8,211px on 2026-08-26 ([MUS-D-0123](records/decisions.md#mus-d-0123)): what each was asked to do, how long it has run, the tool it is in, and what it said when it finished. The hook that makes this possible is installed per session and persists nothing | Against the real CLI in a real tmux pane: a session started through the adapter carried the hook, two sub-agents launched into it appeared as two rows with their own tasks and their own final messages, and the same run produced two stops for work the hook never saw start — which make no rows, and now have a regression test. The parser is tested against payloads the CLI actually emitted, kept as a fixture. Method in [records/work-units/MUS-W-0018.md](records/work-units/MUS-W-0018.md); the investigation that preceded the build, and the harness that reproduces its numbers without a CLI, are in [docs/investigations/0002-sub-agent-visibility.md](docs/investigations/0002-sub-agent-visibility.md) |
| 5 | built and reviewed 2026-08-24, rebuilt after the review, and reviewed a second time the same day; awaiting acceptance. Surface 1 is its own screen: the box first, the destination row beneath it defaulting to the last active session, and the idea inbox among the routes so a jot and a message to an agent are the same gesture. Multi-line, spell-checked, one draft kept as it is typed. **The first build was a box inside the session view and had this row rewritten to match it** — the scope contract edited by the thing measuring it, which is why this row is the owner's sentence again | Real tmux and the real CLI. Multi-line left **the composer** — a form post, not a socket — and arrived in a real session's input, in order, with its newlines. An earlier version of this column credited that to the session view's WebSocket test, and the composer's own delivery was exercised by nothing until a second review said so. That a bracketed paste reaches an agent's composer as *one* message rather than one prompt per line was measured by hand — four lines, one Enter, and the agent answered from all four — and no test here proves it, because the panes hold `cat` and `cat` is happy with newlines either way. Method, and what is still the owner's to confirm, in [records/work-units/MUS-W-0019.md](records/work-units/MUS-W-0019.md) |
| 5b | built and reviewed 2026-08-25, awaiting acceptance. An invitation is a one-time link; redeeming it registers a passkey and signs the person in; signing back in needs nothing typed. A role is per project, and a reader cannot reach the surfaces that type into a running agent. **Deployed and enforced 2026-08-26**, in the only order that works: origin, first owner made on the machine, a passkey registered and seen to sign in, an agent token, then enforcement. Cloudflare Access stays in front throughout | The ceremony itself, against a virtual authenticator written for the purpose — invited, registered, signed out, recognised; a replayed assertion refused; a passkey signed for another relying party refused, with a control run differing only in that field; a lost device replaced onto the same account with its role intact. The guard measured on a running server: a reader 403 on `/sessions`, `/sessions/MUS/ws`, `/compose` and every write, 200 on the reading surfaces. What no test proves is a real authenticator — a phone, a fingerprint, a browser — which is the owner's to try, as 2c's last sentence was. Method in [records/work-units/MUS-W-0020.md](records/work-units/MUS-W-0020.md) |
| 5c | built and reviewed 2026-08-25, awaiting acceptance. `mustur account token` issues a credential an agent carries in a header. It opens the tool call and nothing else, is scoped to one project, revoking it takes effect on the running server, and `--expires` is optional with no expiry by default ([MUS-Q-0055](records/questions.md#mus-q-0055)) | Against a live server with `--accounts` enforced: `POST /mcp` with no credential **403**, with a token **200**, and a full handshake — initialize, initialized, `tools/call` — returning this repository's routing. `GET /records` with the same token **303** to sign-in rather than through. Then, without restarting, `mustur account revoke` and the same token **403** on the same process. Both negative tests mutation-checked; the scope test first passed for the wrong reason, following a redirect to a public page. Through the CLI: a token with no expiry, one good for 720h and one already expired, each read back correctly by `mustur account tokens` and answered **200**, **200** and **403** by a live server. Method in [records/work-units/MUS-W-0021.md](records/work-units/MUS-W-0021.md) |
| 8 | built and reviewed 2026-09-10 by three agents that did not build it, awaiting acceptance. A tool call in the gate reaches the session view as something Mustur was told — the tool, its input, and the whole call behind a disclosure — and the owner's press returns a decision the CLI honours. Nothing is allowed without a press; a denial names who refused it and reaches the agent verbatim; an unanswered call times out into the dialog the CLI would have drawn | Against the payloads investigation 0003 captured from a real CLI, including the quiet one where `PreToolUse` fires for a call nobody would have been asked about and the gate has to decline it. Against real tmux and a real socket: a call raised before the tab opens arrives on the hello frame, a press reaches a process blocked exactly as the hook blocks, a press for a call nothing holds is refused out loud, and a call whose hook was killed clears within a tick rather than covering the CLI's own dialog. The first was mutation-checked. **And against the real CLI, started by Mustur's own adapter rather than by the investigation's harness**: an allowed call ran with no dialog ever drawn, a denied one did not run and the agent said so in its own words, the pane took typed input after both, and a second client attached over its own socket and typed into it. Method in [records/work-units/MUS-W-0023.md](records/work-units/MUS-W-0023.md). The reviewers found six defects and a clause this row used to overstate; what each of them turned into is in pull request 43. **Seen working only in a session started in a prompting mode**: the owner's own default is `auto` and the gate stands down there, which they found in the first five minutes of trying it and chose to keep ([MUS-F-0129](records/findings.md#mus-f-0129), [MUS-D-0158](records/decisions.md#mus-d-0158)). **Proven from the owner's phone on 2026-09-11**, which is the one clause no test here could reach: they sent a prompt from `mustur.devofpie.com`, the pop-up arrived naming the tool and the command, and they pressed it — and the pane carries the other half, because the command ran and no dialog was ever drawn on it, so the press resolved the call rather than the timeout. At Claude Code 2.1.268, a version none of the measurements above used |
| 6 | not met, and closer than the table above says. The invitation, the ceremony, the role and the reading surfaces exist, a second account holds a passkey and a reader role on MUS, and since 2026-09-10 the tab bar no longer offers that reader the one page that refuses them ([MUS-F-0126](records/findings.md#mus-f-0126)). **What is missing is the reading**: as of 2026-09-10 that person has not looked through Mustur ([MUS-D-0157](records/decisions.md#mus-d-0157)) | The verdict cannot come from the store: nothing here records a page being read, because the server keeps no request log. It comes from the owner's word or that person's, the way 2c's last clause came from the owner filing a jot from their own phone |
| 7 onwards | not started. Milestone 6's reading surface arrived early, inside 5b's branch — a scope decision taken in a commit message rather than asked, and left standing on [MUS-Q-0054](records/questions.md#mus-q-0054) | |

**Since 5c, and discharging no milestone above.** Milestone 8 is the exception
and has a row of its own above; the rest belongs to no milestone this file names,
so it is listed here rather than left to make the table read as the whole of what
is built: the
sub-agent drawer and the session dropdown
([MUS-D-0123](records/decisions.md#mus-d-0123), 2026-08-26); the decision badge
going live on every surface ([MUS-D-0145](records/decisions.md#mus-d-0145),
2026-09-04); the CLI's own dialogs read off the pane and offered as controls, as
a pop up and a key row ([MUS-D-0141](records/decisions.md#mus-d-0141) through
[MUS-D-0144](records/decisions.md#mus-d-0144), and
[MUS-W-0022](records/work-units/MUS-W-0022.md), all 2026-09-04, with the shapes
it grew to handle landing through 2026-09-07);
`make deploy` ([MUS-F-0092](records/findings.md#mus-f-0092), 2026-09-05);
starting a session from the surface
([MUS-D-0146](records/decisions.md#mus-d-0146), 2026-09-06); stopping one from
its own page ([MUS-D-0147](records/decisions.md#mus-d-0147), 2026-09-07);
sessions written down and offered back after a reboot
([MUS-D-0149](records/decisions.md#mus-d-0149),
2026-09-07, and
[MUS-D-0150](records/decisions.md#mus-d-0150), 2026-09-08); the tmux server in a
scope of its own, so a deploy stops ending every session
([MUS-D-0151](records/decisions.md#mus-d-0151), 2026-09-08); and Enter sending in
the session box, with spelling left to the browser's underlines
([MUS-D-0148](records/decisions.md#mus-d-0148), 2026-09-07).

Whether the dialog work grows into a milestone of its own — the structured
channel, rather than the pane — was
[MUS-Q-0094](records/questions.md#mus-q-0094)'s to answer, and
[investigation 0003](docs/investigations/0003-dialog-without-the-screen.md) is
the evidence it turned on. It was answered on 2026-09-10 — the hooks, this
vendor, no module boundary — and milestone 8 is that answer's row.

*Passed* is a verdict acceptance makes, and this file does not make it early —
the same reason its own note below says the status change in IdeaWarehouse was
never this file's to claim.

What exists is one binary. It holds this project's records and its routing in a
SQLite store that only accepts inserts, exports them to
[records/](records/README.md) so a reader who does not run it can check them,
serves them to a session through one tool call that the clause at the bottom of
[CLAUDE.md](CLAUDE.md) mandates, audits this repository against the StrucGu
modules it declares, and takes a jot into its own findings queue through one
box, which a tunnel and a Cloudflare Access application now publish at
`mustur.devofpie.com`. It also holds the questions it owes the owner, refuses to
let work be reported complete around an unsurfaced one, and serves a queue those
are answered from. **It starts sessions inside tmux and owns them**: it lists
what is running, stops one, shows a session's screen in a browser tab and types
into it, shows the sub-agents that session spawned, reads the CLI's own dialogs
off the pane and offers them as controls, and writes down what a reboot took so a
person can start it again. It knows who is asking — an invitation, a passkey and
a role per project — and issues a token an agent carries to reach the tool call.
Until 2026-09-10 this paragraph ended *there is no adapter, no session Mustur
owns, and no second project*, two-thirds of which milestones 4a, 4b and 4c had
built and this file's own table already recorded. There is still no second
project.

This file began as the plan seed from
[agent-workflow-web-platform](https://github.com/DevOfPie/IdeaWarehouse/blob/main/ideas/agent-workflow-web-platform.md),
lifted here on 2026-08-19 so planning and wireframing could start in this
repository. The status change in IdeaWarehouse is a verdict and is not this
file's to claim.

**Effort figures live in the idea file, not here, and they run long.** The
owner's calibration, given 2026-08-19, is that estimates produced by an agent are
significantly longer than the work actually takes. Treat the sixteen-to-twenty-one
focused days recorded there as an upper bound rather than a plan, and do not use
it to decide scope.
