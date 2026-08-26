# Mustur

Read [Plan.md](Plan.md) for what is true, [workflow.md](workflow.md) for how the
work is done, and [decisions.md](decisions.md) for why.

**Milestones 1 and 2 have passed; 2b, 2c, 3, 4a, 4b and 4c are built and not
yet accepted; 5 is built, reviewed twice and rebuilt after the first review, and
everything through it is merged. 5b, accounts, and 5c, agent tokens, are built
and not merged, and both are **deployed and enforced** on this machine since
2026-08-26.**
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
It streams a running session's output to a browser tab and notices when one
ends. You can reply from that tab: the box is multi-line and spell-checked, it
holds one draft that survives a reload and follows you between sessions, and it
sends what you wrote as a single message rather than a prompt per line —
measured by hand against one CLI, and the thing to re-check first if another
ever behaves oddly. It shows
that session's sub-agents as their own rows too — what each was asked to do, how
long it has run, the tool it is in, and its output once it finishes — and those
rows appear only for sessions Mustur started, because the hook that reports them
rides in on the command line Mustur builds.
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
revoked immediately rather than at the next restart. Without it, enforcement and
the mandate could not both be on — measured, not reasoned.

**A session on this machine now needs that token to make the mandated call.**
`mustur account tokens` says which exist; a session refused with 403 on `/mcp`
is missing one rather than looking at a stopped server.

Nothing below 5c is built; do not describe any of it in the present tense.

**Six pages carry script**: the session view, the composer, and the four
authentication surfaces — sign in, accept an invitation, account, and people.
Every other surface is server-rendered with nothing to fetch, and that is still
the rule. Each addition was a decision the owner took, never a precedent set by
building it: the composer on
[MUS-Q-0034](records/questions.md#mus-q-0034), the account page on
[MUS-Q-0047](records/questions.md#mus-q-0047). A seventh is a new decision
again.

**What the rule counts is itself open** on
[MUS-Q-0053](records/questions.md#mus-q-0053), because six is the count of
surfaces shipping a `<script>` tag and only two of them stop working without it.

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

  You may **withdraw** your own question. You may not **answer** it.

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
