# Mustur

Read [Plan.md](Plan.md) for what is true, [workflow.md](workflow.md) for how the
work is done, and [decisions.md](decisions.md) for why.

**Milestone 2 is built.** `mustur` holds this repository's records and routing
and serves them over MCP. Nothing below milestone 2 is built; do not describe
any of it in the present tense.

Three rules bind every session in this repository:

- **Milestone 1 has run and passed**, 20 of 20 against a rule of 18 of 20 fixed
  beforehand ([the record](docs/investigations/0001-mandated-tool-call.md)). It
  gated everything below it, and the mandate it measured is the one at the
  bottom of this file.
- **No file in any other project is touched.** Not read for restructuring, not
  edited, not migrated. Onboarding another project is a milestone with its own
  verdict.
- **Every decision or question for the owner goes in a prompt**, never in prose,
  a report or a pull request body. A pull request out of draft says work needs
  review; it never asks a decision.

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
