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
