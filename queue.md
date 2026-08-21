# Queue

Append a line. Don't name it, don't structure it, don't think about where it
goes. Triage sorts it out later.

This is also where an idea goes when it arrives **while you are working
something else** — append it here and carry on.

Anyone may append, including agents. Attribute yours.

Format: `- YYYY-MM-DD (who) — the thing, as loosely as you like`

**Nothing here is scheduled work.** A line is a note, not a commitment.

Append at the bottom, always.

---

<!-- new lines below -->
- 2026-08-19 (whippy) — queue.md itself will earn findings-queue audit findings (no evidence/review columns); reshaping it into the specified table vs keeping the jot log is a real choice someone should make before milestone 2b
- 2026-08-19 (whippy) — PR #1's body promises three open design questions in docs/ui-surfaces.md; the file marks two (composer, records) — find or retire the third
- 2026-08-19 (whippy) — Anthropic's paused metering change (support article 15036540) would move claude -p / Agent SDK usage off plan limits onto separate credits if it un-pauses; which invocation path the adapter uses decides Mustur's exposure — candidate Plan.md limitation row
- 2026-08-19 (whippy) — records/ now carries the same decisions and findings as decisions.md and queue.md, one addressable and one prose; milestone 2b should decide whether the repository's own StrucGu roles move to the export or stay on the files a person edits
- 2026-08-19 (whippy) — a seeded record's one-line summary can drift from the prose it links to and nothing detects it; a check comparing the two is cheap and nobody has asked for it
- 2026-08-19 (whippy) — `make check` now needs Go on the CI runner and ci/proposed/ci.yml is what pins it; until the owner applies that proposal the runner's preinstalled toolchain is whatever the image happens to carry
- 2026-08-20 (whippy) — workflow.md's "superseded decisions stay, with a pointer" and "never edit an entry" cannot both hold if the pointer goes in the superseded entry; milestone 2 read it as the later entry carrying the pointer, and the contract should say which
- 2026-08-20 (whippy) — nothing checks that the mandate clause in CLAUDE.md still says what milestone 1 scored; a string comparison against the fixture would be a few lines and no milestone has asked for one
- 2026-08-20 (whippy) — no automated check detects records/ drifting from the store, because the committed gate runs `mustur verify` without a store and CI has none to compare against; needs a design, not a line
- 2026-08-20 (whippy) — ci/proposed/README.md says a new check needs the owner: No, and reaches the next push; milestone 2's checks needed a workflow change the token cannot push, so that claim is falsified and the file is where the fix belongs
- 2026-08-20 (whippy) — should Mustur hold a triage rule as a record at all? Four of StrucGu's five roles are record kinds and triage-rule describes a document; nobody has asked for the fifth
- 2026-08-20 (whippy) — the store has no way to add a record after the seed, so every decision since milestone 2 has gone in by editing the seed and re-exporting from a scratch database; the live store cannot take them at all, and the seed refusing a non-empty store is what makes that true
- 2026-08-21 (whippy) — mustur.service passes --export at the live checkout's records/, so a boot-started daemon rewrites tracked files on whatever branch happens to be checked out, and make check's verify-records gate reads a directory that daemon can change mid-run; the export target is also hardcoded, so the unit breaks if the checkout moves. Needs a design (export somewhere the daemon owns? commit on filing? neither?), not a line
- 2026-08-21 (whippy) — commit 0a0a0ac carries three topics (shallow-clone skip, conformance counting, the systemd unit) against workflow.md's one-topic gate; splitting it means rewriting a pushed commit, which needs a force-push nobody here should be doing, so it stands as a defect on the record rather than a fix
- 2026-08-21 (whippy) — MUS-W-0005 and MUS-W-0010 were amended in the store, but internal/seed/data/work-units.json still ships the text they superseded, so a fresh clone seeds records that disagree with records/; the seed is history by design and the amendment log lives only in the store, and which of those a clone should get has never been decided
- 2026-08-21 (whippy) — nothing watches the origin behind Access: the hostname answers 302 whether or not mustur.service is running, so every check in docs/ingress.md passes against a dead box. Restart=always covers a crash, not a unit that failed to start
- 2026-08-21 (whippy) — the seed still ships milestone 3 and 4's pre-split done-when in internal/seed/data/milestones.json, as it ships the superseded MUS-W-0005 and MUS-W-0010 text; a fresh clone therefore seeds milestones that disagree with records/, and the seed-is-history decision has never been reconciled with the store being amended
- 2026-08-21 (whippy) — the decision queue was built from docs/ui-surfaces.md's brief rather than from its artboard in the published visual plan, which is the same route intake took and the thing the plan was published to stop; whether it gets redesigned from the plan or the plan gets amended to match is nobody's call yet
- 2026-08-21 (whippy) — queue.md's line calling the 0a0a0ac commit split impossible without a force-push overstates it: that commit is on an unmerged branch whose own workflow already presumes rebasing, so the option was available and was not taken
- 2026-08-21 (whippy) — nothing on the mustur_route path tells a session that a question is open; the gate catches it at make check, which is the end of the work rather than the start, and the mandated call is the surface that could say it first
- 2026-08-21 (whippy) — the queue-redesign line above is answered: MUS-Q-0010 settled it and the queue was rebuilt from the artboard, so that line is closed rather than open
- 2026-08-21 (whippy) — the `::` option separator only tolerates itself in the third part, so an option whose label or one-line contains it silently loses text; nothing rejects or warns, and MUS-W-0015 records it as a risk rather than a guard
- 2026-08-21 (whippy) — intake has no tab bar, so the only route from it to the queue is a banner that disappears when nothing is open; the queue can be reached from intake exactly when it has nothing to say
