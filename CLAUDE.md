# Mustur

Read [Plan.md](Plan.md) for what is true, [workflow.md](workflow.md) for how the
work is done, and [decisions.md](decisions.md) for why.

**Milestones 1 and 2 have passed; 2b, 2c, 3, 4a, 4b and 4c are built and not
yet accepted; 5 is built, reviewed twice and rebuilt after the first review, and
everything through it is merged. 5b, accounts, and 5c, agent tokens, are built
and merged as of 2026-08-28, and both are **deployed and enforced** on this
machine since 2026-08-26.**
The `mustur` binary can hold this repository's records and routing, serve them
over MCP, audit them, and take a jot through an intake box — on a fresh clone it
holds nothing until `make seed`, serves nothing until `make serve`, and audits
nothing without a StrucGu checkout. On this machine the intake box, the decision
queue, the records document and — since 2026-08-23 — the session view are
published at `mustur.devofpie.com` behind Cloudflare Access; that is a
deployment, not something a clone inherits. **Since 2026-08-26 accounts are
enforced there too**: the owner holds a passkey, an agent token exists, and
`--accounts` is on, so a reader reads and only an owner reaches what types into
a running agent. Access is still in front of all of it. The session view is served only when `serve` is
given `--sessions`, because it types into a running agent's stdin: dropping the
flag removes the surface and the tab the others offer to it, which is the knob
to reach for if the Access policy ever widens. What is unconditionally here is
[records/](records/README.md). It also
holds open questions and refuses to let work be reported complete around one.
It also starts agent sessions inside tmux, reports which are running, stops one,
and types an answered decision back into the session that raised it — where the
question named one with `--in`, which is the only way delivery has a target.
It shows a running session's screen in a browser tab and notices when one ends
— the screen tmux has already assembled, polled and re-rendered when it
changes, rather than the pane's raw byte protocol appended to a log
([MUS-D-0132](records/decisions.md#mus-d-0132)). There is no pipe, no byte
offset and no replay: a tab that reconnects is handed the screen as it stands.
Sessions Mustur starts are **100x300**, because an agent CLI runs on the
alternate screen and tmux keeps no scrollback for one — a tall pane is the only
place a transcript can live
([MUS-F-0052](records/findings.md#mus-f-0052)). The CLI's own furniture — its
input box, dividers and status line — comes off the screen before it is
rendered, and what that furniture said is shown as a row of chips instead
([MUS-F-0053](records/findings.md#mus-f-0053)). Every surface takes the width the rail leaves rather than a reading column
([MUS-D-0128](records/decisions.md#mus-d-0128)) — a page wanting a narrower
measure asks for it by setting `--shell-content`. On a wide screen the account
link sits as an icon at the foot of the rail rather than as a word in the
header ([MUS-D-0127](records/decisions.md#mus-d-0127)). The session's status
pill reads **running** or **idle** from the CLI's own pane rather than from a
clock — Claude Code says which in its status line, and a timer counting silence
is a guess standing in for that. It wears a turning accent ring while a turn is
in flight. A pane nothing here recognises falls back to a three-minute silence
threshold rather than claiming to know
([MUS-D-0130](records/decisions.md#mus-d-0130)). You can reply from that tab: the box is multi-line and spell-checked, it
holds one draft that survives a reload and follows you between sessions, and it
sends what you wrote as a single message rather than a prompt per line —
measured by hand against one CLI, and the thing to re-check first if another
ever behaves oddly. Enter sends where a physical keyboard is likely and
Shift+Enter breaks the line; on a touch screen Enter stays a newline and the
Send button, which is on every device, is the submit
([MUS-Q-0067](records/questions.md#mus-q-0067)). That session's sub-agents live in a
drawer that is **shut on arrival** — a button beside the session picker says
how many there are and wears a turning accent ring while any is running, and
opening it lists them one line each: what it was asked to do, how long it has
run, the tool it is in. Opening a row reads what that sub-agent said in the same
drawer. On a wide screen the drawer pushes the terminal rather than covering it and can
be dragged wider by its leading edge, which remembers the width per browser; on
a phone it opens over and there is no grip. Nothing about a sub-agent is ever printed into the
session column: doing that grew one box to 8,211px and squeezed everything else
off the screen ([MUS-F-0038](records/findings.md#mus-f-0038)). Those rows appear
only for sessions Mustur started, because the hook that reports them rides in on
the command line Mustur builds. The session picker is a dropdown for the same
reason the intake destinations are
([MUS-D-0121](records/decisions.md#mus-d-0121)): a row that scrolls sideways
hides its last choice behind a swipe.
**It does not restart anything** — an agent CLI that crashed wants a person, not
a loop.

Since milestone 5b it also knows who is asking: an invitation, a passkey, and a
role per project that decides what somebody reaches. That is built and refuses
correctly, and it is **off** — enforcement is a flag, because turning it on
before the owner holds a passkey locks the owner out.

Milestone 5c is the credential an agent can hold. A passkey needs a browser and
a gesture; an agent has neither and still has to reach the mandated tool call,
so `mustur account token` issues one carried in an `Authorization: Bearer`
header. It opens `/mcp` and nothing else, is scoped to one project, and is
revoked immediately rather than at the next restart — including a stream
already open under it, which used to outlive its own credential
([MUS-F-0028](records/findings.md#mus-f-0028)). Without it, enforcement and
the mandate could not both be on — measured, not reasoned.

**A session on this machine now needs that token to make the mandated call.**
`mustur account tokens` says which exist; a session refused with 403 on `/mcp`
is missing one rather than looking at a stopped server.

Nothing below 5c is built; do not describe any of it in the present tense.

**Every page carries a script now, and two kinds carry a second.** The badge in
the tab bar is live on every surface since
[MUS-Q-0078](records/questions.md#mus-q-0078): a page left open used to show the
count it was rendered with, and the owner missed a question being raised because
of it. `bar.js` polls `/questions/count` and writes the badge, and it is the only
code that writes one — the session view having its own copy is how the first fix
ended up living on a single surface
([MUS-F-0086](records/findings.md#mus-f-0086)). Every page still renders its own
count server-side and works with script blocked; what stops is the number
changing.

On top of that, six pages carry a second script for something only script can
do: the session view, the composer, and the four authentication surfaces — sign
in, accept an invitation, account, and people. Each was a decision the owner
took, never a precedent set by building it: the composer on
[MUS-Q-0034](records/questions.md#mus-q-0034), the account page on
[MUS-Q-0047](records/questions.md#mus-q-0047).

**What the rule counts is still open** on
[MUS-Q-0053](records/questions.md#mus-q-0053), and MUS-Q-0078 moved the numbers
rather than settling it: the count of pages shipping a `<script>` tag is now all
of them, and the count that matters — pages that stop working without one — is
unchanged at two.

They are not the same kind of exception. The session view cannot be
server-rendered at all: it is a live terminal, and neither can the passkey
ceremony, which is a browser API. The composer can be, and is — its form posts
and works with the script blocked; what the script adds is the draft, which
cannot survive a backgrounded phone any other way. The account and people
screens are the same: every action is a form, and the script adds a copy button
and a save-on-change.

**A session left running in a terminal is invisible to Mustur and will not
become visible.** Mustur starts sessions and never attaches to one it did not.

Three rules bind every session in this repository:

- **Milestone 1 has run and passed**, 20 of 20 against a rule of 18 of 20 fixed
  beforehand ([the record](docs/investigations/0001-mandated-tool-call.md)). It
  gated everything below it. The mandate at the bottom of this file is a
  descendant of the one it measured, not the same clause: it adds a paragraph,
  and it runs over a different transport. Both differences, and the third, are
  named in
  [decisions.md](decisions.md#what-the-mandate-keeps-from-the-fixture-and-what-it-does-not).
- **No file in any other project is touched.** Not read for restructuring, not
  edited, not migrated. Onboarding another project is a milestone with its own
  verdict.
- **Every decision or question for the owner goes in a prompt**, never in prose,
  a report or a pull request body. A pull request out of draft says work needs
  review; it never asks a decision.

  This one is enforced rather than trusted. Raise it with

  ```
  mustur ask --title "…" --blocks "…" [--in <project>] \
    --option "Label :: one line on what it costs :: the paragraph behind it" \
    --option "…"
  ```

  `--in` names the Mustur-owned session the answer should be typed back into.
  Without it an answer is recorded and delivered nowhere, which is the right
  outcome for a question raised outside a session Mustur started — and the
  common one, since most are.

  then put it in a prompt and `mustur surfaced <ID>`. **Give it options.** You
  have just finished weighing the alternatives — that is why you are blocked —
  and a bare question makes the owner reconstruct them. Prefix one option's line
  with `Recommended` if you have a view. Omit them only when the question
  genuinely has no shortlist.
  `make check` reads `records/` and fails while any open question has never been
  surfaced, so work cannot be reported complete around one.

  **Being asked is usually enough** — the owner may be away, and stopping for an
  absent owner is a cost this refuses to pay. The exception is a question the
  work in hand cannot proceed without: raise those with `--needed` and the gate
  will not pass on surfacing alone, because reporting complete on work that
  turned on an answer nobody gave is the same lie as never having asked. Do
  everything independent of the answer first, which is what
  [workflow.md](workflow.md) asks for anyway.

  You may **withdraw** your own question. You may not **answer** it — but you
  may **write down** an answer the owner gave somewhere else:

  ```
  mustur answer <ID> --from-owner "where they said it" --answer "…"
  ```

  It records who wrote it down and where it came from, so nobody reads a relay
  as the owner having been here. An answer already recorded is not written over
  without `--reanswer` ([MUS-D-0126](records/decisions.md#mus-d-0126)).

  `mustur amend` keeps what you do not pass — including the record's date, which
  `--at` used to restamp with the date of the correction. Removing a field or a
  citation is `--drop KEY`, and `--replace` states a record afresh for the rare
  time that is wanted ([MUS-D-0134](records/decisions.md#mus-d-0134)).

  A jot that `Route it for me` put in the wrong place is corrected with
  `mustur reroute <ID> --to <DEST>`: it files a new record at the right
  destination and retires the old one, which keeps its identifier and still
  resolves. The prefix is the routing, so moving a record and renaming it are
  the same act ([MUS-D-0125](records/decisions.md#mus-d-0125)).

## Mustur

This repository is registered with Mustur, which holds its routing and its
records.

**Before any other action in every session, call the `mustur_route` tool
(server `mustur`) with the repository name and one line on the task.** Do not
start work until the call has returned; its result says where this
repository's records and routing live.

If the tool is not there, say so and carry on. A server that is not running is
not a licence to skip the call — it is a thing to report, in the same breath as
whatever you were asked to do. Start it with `make serve`.

There is a third state, since milestone 5c: the tool is there and **refuses**.
A server running with `--accounts` answers `/mcp` with 403 to a caller carrying
no token, so an agent needs one in its MCP configuration. Report that as itself
rather than as absence — they are different problems and only one is fixed by
starting the server. `mustur account token --for "..."` issues one.

**The server is configured per machine, not in this repository.** There is no
`.mcp.json` here, deliberately: a checked-in one can carry no credential, and it
would be preferred over the configuration that has one — project scope beats
user scope, so the working entry is the one that gets ignored. Issue a token and
put it at user scope:

```
mustur account token --for "claude-code on <machine>"
claude mcp add --scope user --transport http mustur \
  http://127.0.0.1:7777/mcp --header "Authorization: Bearer <the token>"
```

The secret is printed once and never stored; an invitation that goes missing is
reissued rather than looked up, and so is this
([MUS-Q-0065](records/questions.md#mus-q-0065)).
