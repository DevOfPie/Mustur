# Mustur

Read [Plan.md](Plan.md) for what is true, [workflow.md](workflow.md) for how the
work is done, and [decisions.md](decisions.md) for why.

**Milestones 1 and 2 have passed; 2b, 2c, 3, 4a and 4b are built and not yet
accepted.**
The `mustur` binary can hold this repository's records and routing, serve them
over MCP, audit them, and take a jot through an intake box — on a fresh clone it
holds nothing until `make seed`, serves nothing until `make serve`, and audits
nothing without a StrucGu checkout. On this machine the intake box is published
at `mustur.devofpie.com` behind Cloudflare Access; that is a deployment, not
something a clone inherits. What is unconditionally here is
[records/](records/README.md). It also
holds open questions and refuses to let work be reported complete around one.
It also starts agent sessions inside tmux, reports which are running, stops one,
and types an answered decision back into the session that raised it — where the
question named one with `--in`, which is the only way delivery has a target.
It streams a running session's output to a browser tab you can reply from, and
notices when one ends. **It does not restart anything** — an agent CLI that
crashed wants a person, not a loop. Nothing below milestone 4b is built; do not
describe any of it in the present tense.

The session view is the **only** page in this repository that carries script.
Every other surface is server-rendered with nothing to fetch, and that is the
rule; a second surface wanting a client layer is a new decision, not a
precedent.

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
