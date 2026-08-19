# Proposed workflows

Changes to `.github/workflows/` are written here and applied by the owner. This
file says why, what belongs here rather than in the Makefile, and the commands
that apply a proposal. The mechanism is LinkCtrl's, adopted whole; its
`ci/proposed/README.md` carries the longer argument.

## Why a proposal and not a commit

The token the agent building this repository holds is a fine-grained PAT
without the `Workflows` permission, and that is deliberate: a workflow file is
code that runs with `GITHUB_TOKEN`, whose own `permissions:` block overrides
the repository's read-only default. GitHub refuses the **push** of any branch
touching `.github/workflows/`, so a workflow change cannot even arrive as a PR
from the agent — it arrives as a file here, at a path that is not
`.github/workflows/`.

## What lives where

| Change | Where | Needs the owner |
| --- | --- | --- |
| A new check, or a changed one | A make target reached by `make check` | No |
| What a check actually does | `scripts/*.sh` | No |
| Triggers, `permissions:`, `concurrency:`, action pins, `runs-on` | `.github/workflows/` | **Yes** |

Adding a check reaches the next push; changing what CI *is* takes a proposal.

## Applying a proposal

```sh
mkdir -p .github/workflows
git mv ci/proposed/ci.yml .github/workflows/ci.yml
git commit
```

`make workflow-proposals` reports what is pending, with a diff against the live
file when one exists. It is deliberately not a gate: a pending proposal is a
normal state, and failing CI on one would turn every proposal into a red build.
