# Mustur

One authenticated web surface over agent work that runs on machines the owner
already owns. Mustur does not run the agents and does not replace where they run.

It is the place where the facts that span repositories, machines and people are
kept — where work goes, which machine holds it, what has been decided, who may
see it — and it delivers them into a session by being **called**, from a thin
repo-local file that mandates the call.

**Milestones 1 and 2 have passed; 2b and 2c are built, reviewed and awaiting
acceptance, and nothing below them is built.** What exists is one binary that
can hold this project's records and routing, serve them to a session through a
single mandated tool call, audit its own records against the conventions this
repository declares, and take a jot into its own findings queue through one box.
That box is published at `mustur.devofpie.com`, behind Cloudflare Access and
answered by a service that starts at boot; [docs/ingress.md](docs/ingress.md) is
what it took. The one sentence still unproven is 2c's own — a jot filed *from a
phone* — because only the owner can get through Access.

| | |
| --- | --- |
| What is true | [Plan.md](Plan.md) |
| Why it is that way | [decisions.md](decisions.md) |
| How the work is done | [workflow.md](workflow.md) |
| Surfaces awaiting design | [docs/ui-surfaces.md](docs/ui-surfaces.md) |
| The records themselves | [records/](records/README.md) |
| Where it came from | [IdeaWarehouse `agent-workflow-web-platform`](https://github.com/DevOfPie/IdeaWarehouse/blob/main/ideas/agent-workflow-web-platform.md) |

## The one thing to read first

Every module here is reached by an agent calling a central service, and the
measurement that constrains the whole design says agents do **not** call a store
they must choose to call — 0 memory operations in 114 turns against a pre-seeded
store. The answer is to mandate the call from a repo-local file that loads
unconditionally, and **that answer has never been measured**.

Milestone 1 was the test of it. Its decision rule was fixed and committed
before the runs, and a failure would have killed the idea rather than shrunk
it. It passed, 20 of 20 against a rule of 18 of 20 —
[the record](docs/investigations/0001-mandated-tool-call.md).

What ships is a descendant of what was measured, not the same thing. The clause
in [CLAUDE.md](CLAUDE.md) is the fixture's wording plus a paragraph for when the
tool is absent, and the fixture ran over stdio — where the client launches the
server, so the tool is always there — while this repository registers HTTP on
loopback, where it is absent unless somebody has run `make serve`. All three
differences are named in
[decisions.md](decisions.md#what-the-mandate-keeps-from-the-fixture-and-what-it-does-not).
Re-scoring the clause on the transport that ships has not been done.

## Running it

```sh
make build            # the binary
make seed             # once, on an empty store: what already existed
make export           # render the store into records/, and commit the diff
make serve            # the one tool call, on loopback
make audit            # this tree against the StrucGu modules it adopts
make check            # every gate this tree enforces mechanically
```

`make audit` needs a [StrucGu](https://github.com/DevOfPie/StrucGu) checkout
beside this one, or `MUSTUR_STRUCGU` pointing at one; nothing here vendors a
copy of somebody else's specification, and CI checks one out rather than
carrying one. Its exit status is zero whenever the audit ran — findings are
output, and gating on them is `--gate`, which you ask for after your first clean
run rather than before.

Records are written with `mustur add`, which allocates the next identifier and
appends to the store. **The store is the source and it is not in this
repository** — a binary file in git is a record nobody can review — so
[records/](records/README.md) is the reviewable half, regenerated with
`make export` and committed alongside whatever changed it.

That means no unattended run can regenerate the export to check it. What
catches drift is `mustur verify --db <store>`, and the export's diff in the pull
request, which is what a reader who does not run the binary reads anyway.
