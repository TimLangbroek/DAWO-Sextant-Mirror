# Documentation

A hundred and four files, and until now the only index was the handbook's
own `SUMMARY.md`, which covers twenty-five of them. This file is the map.

Two kinds of document live here and telling them apart is the whole point.
A **living** document describes how things are and is expected to be wrong
if it stops being maintained. A **record** describes what was true on a
date, and is not wrong when it ages: it is evidence. Records carry their
date, either in the filename or in the table below.

## Where to start

| If you want to | Read |
|---|---|
| run or operate Sextant | [the handbook](handbook/src/SUMMARY.md), which is what gets published |
| understand a decision | [`adr/`](adr), twenty-nine of them, numbered in order |
| understand the shape of the code | [`architecture.md`](architecture.md), then [`architecture/scale.md`](architecture/scale.md) |
| know what is left before 1.0 | [`1.0-fit-gap.md`](1.0-fit-gap.md) and [`project/issues.json`](project/issues.json) |
| cut a release | [`releasing.md`](releasing.md) |
| fix something in production | [`runbooks/`](runbooks) |

## Subtrees

| Path | Kind | What it is |
|---|---|---|
| `adr/` | living | Architecture decisions, numbered. A decision is recorded once and superseded by a later ADR, never edited into a different decision. |
| `handbook/` | living | The published manual (`docs.sextantfleet.com`). `src/SUMMARY.md` is its table of contents. |
| `runbooks/` | living | What to do when something is broken. Written to be followed under pressure. |
| `compliance/` | living, all drafts | DPIA, processing register, ISO 27002 mapping, accessibility. None is approved or signed; each says so at the top. |
| `design/` | mixed | Numbered design notes (0001 and up) plus loose briefs from the reskin. The numbered ones are living; the SHOUTING ones are records of a moment. |
| `audit/` | records | Dated snapshots: `security-2026-08.md` and its siblings. The right convention, and the one the rest of `docs/` does not follow yet. |
| `architecture/` | living | Detail that outgrew `architecture.md`. Currently `scale.md`, which four Go files cite by path. |
| `release-notes/` | records | One file per release. |
| `project/` | living | `issues.json`: the backlog as data, applied to Codeberg by `scripts/codeberg-project-setup.sh`. |
| `man/`, `img/` | generated | `sxctl.1` is written by `sxctl man`; do not edit it here. |

## The loose files at this level

| File | Kind | As of |
|---|---|---|
| [`architecture.md`](architecture.md) | living | |
| [`capabilities.md`](capabilities.md) | living | |
| [`house-rules.md`](house-rules.md) | living | |
| [`releasing.md`](releasing.md) | living | |
| [`roadmap.md`](roadmap.md) | living | |
| [`threat-model.md`](threat-model.md) | living | |
| [`integration-test-runbook.md`](integration-test-runbook.md) | living | belongs under `runbooks/` |
| [`station-migration-runbook.md`](station-migration-runbook.md) | living | belongs under `runbooks/` |
| [`e2e-acceptance-plan.md`](e2e-acceptance-plan.md) | living | the plan itself; the results in it are records |
| [`1.0-fit-gap.md`](1.0-fit-gap.md) | record | 2026-08-18 |
| [`e2e-3-findings.md`](e2e-3-findings.md) | record | 2026-08-06 |
| [`e2e-2-findings.md`](e2e-2-findings.md) | record | 2026-07-30 |
| [`competitive-intake.md`](competitive-intake.md) | record | 2026-08-10 |

The four records are kept because a finding that was true once is worth
reading later. Their dates come from the last commit that touched them, not
from a line inside them, which is the reason the audit convention of putting
the date in the filename is better than this table.

## Where the same subject is described twice

Known, and listed rather than left for a reader to trip over.

- **Architecture**: `architecture.md` here, `handbook/src/architecture/overview.md`
  published. The handbook is for operators; this one is for contributors.
- **Scale**: `architecture/scale.md` (382 lines, cited from the code) against
  `handbook/src/architecture/scale.md` (a summary that links back to it).
  The long one is authoritative.
- **Roadmap**: `roadmap.md` here is the real one; the handbook page is a stub.
- **How we work**: `house-rules.md` and `CONTRIBUTING.md` split this
  deliberately, and `handbook/src/project/how-we-work.md` is the published
  version. `CONTRIBUTING.md` is the entry point.
