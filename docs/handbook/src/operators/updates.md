# Ship an update

**Updates** (in the sidebar under Shipping) is the one board for the whole
journey from a proposed configuration change to it running on the fleet:
propose -> review -> ready -> rolling out in waves. Changes and Rollout are
drill-downs reached from cards on this board, not separate destinations you
navigate to directly.

## Step 1: review proposed changes

Every configuration edit - a setting, a policy, an integration - is staged as
a **change**: a named, audited unit of work on its own branch. A change moves
through:

- **Draft** - staged edits, not yet validated.
- **Building** - the Nix gate is evaluating the change (see
  [Safe writes and the Nix gate](../concepts/safe-writes.md)).
- **Ready** - the gate passed; it can be reviewed and merged.
- **Failed** - the gate rejected it; the distilled error is shown inline
  (see Troubleshooting below).

Open a change directly from the Updates board (**Change ID** + **title**,
then *Open change*), or let one open automatically: if your organisation
requires change requests (**Access -> Approval flows**), saving settings from
the Configuration editor stages the same edits as a fresh change instead of
committing straight to git, and lands you back here.

For a **Ready** change: *View diff* shows exactly what will change on disk
per host; *Approve* merges it (blocked on yourself if four-eyes review is
required); *Reject* abandons it. A **Draft** can be *Submitted* (sends it to
the gate) or abandoned.

## Step 2: roll out

Once there is a current revision to ship, this section is either an active
rollout in progress or the button to start one against the latest merged
revision.

Starting a rollout first shows what it will do: the revision, the wave plan,
and every change in the range that somebody marked as high-risk when they
saved it, with who did. Those did not roll out by themselves; confirming is
the moment they do.

Starting a rollout kicks off the wave plan from Step 3 (below) against the
current revision. Each wave shows a live status label as it progresses:

| Label | Meaning |
|---|---|
| Queued | Not yet reached |
| **Building** | Build-before-promote: the wave's release is being realised into the signed binary cache before its branch flips (see [Scaling to 10,000+ devices](../architecture/scale.md)) |
| Deploying | The ring branch has flipped; devices are pulling and converging |
| Soaking | Converged healthy; waiting out the wave's soak window |
| Awaiting approval | Soaked, but the wave requires a manual sign-off before the next one starts |
| Complete | Fully promoted, next wave underway or finished |

An owner can **approve** an awaiting wave to let the rollout proceed, or
**cancel** the whole run.

## Step 3: the rollout procedure

Two panels, both owner reach:

**Wave plan.** Each wave (a "ring") pins one device group plus its promotion
gates: a name, the group, a **soak** window (minutes healthy on target before
the next wave may start), a **minimum healthy %** (defaults to 100 - every
device healthy), an optional **max at once** (a count-capped canary that
widens cohort by cohort instead of releasing the whole group at once), and
whether the wave **requires approval** (a human checkpoint - the enterprise
"test sign-off" step). Size a wave small first (a canary), then wider; tune
the progression by ordering waves and sizing their groups.

**Governance.** Three checkboxes, organisation-wide (mirrored on
**Access -> Approval flows**):

- **Require change request** - configuration edits must go through a
  reviewed change (this board's Step 1), never a direct commit.
- **Require four-eyes** - a change may not be merged by its own author.
- **Require test wave** - a rollout must have a gated (manual-approval) wave
  before it starts; an owner can explicitly skip this per rollout, and the
  skip is logged.

[How a rollout ships](#how-a-rollout-ships) and
[Approval flows](#approval-flows) below explain what is behind both panels.

## How a rollout ships

An update does not reach the whole fleet at once. It promotes through ordered
**waves** (also called rings). Each wave is a group of devices; the next wave
only starts once the current one is converged healthy through its soak
window. A wave can require a manual approval gate - a human checkpoint that
the update was tested.

Promotion is on measured evidence, not on a timer. That is the difference
from the deferral-days model most fleet tools use, where wave two starts a
fixed number of days after wave one whatever wave one did.

### What a wave has to prove

Three things, and it needs all of them:

| | |
|---|---|
| **Enough devices reachable** | Absence starts after an hour of silence, so a wave run at night can have almost all of its laptops shut. A percentage of the two that happen to be awake is not evidence. Half the wave by default; set **min devices** per wave to change it. |
| **Enough of those healthy on the target** | The health floor, 95% by default. The rest become stragglers: the wave moves on, they stay visible, and they catch up on their next check-in because the ring branch already carries the release. |
| **A soak** | Time on the target after converging, so a release that breaks slowly has a chance to show it. |

A wave that cannot reach its floor because devices reached the target and
turned out demonstrably unwell does not wait: that is a bad release, and the
run halts.

At scale, a wave's release is also **built before it is promoted**: the
delivery pipeline realises the wave's closures on build workers and pushes
them to a signed binary cache before its branch flips, so devices substitute
(download) a pre-built release instead of each compiling it independently.
While this is happening the wave shows as **Building** on the
Updates board; once the release lands in the
cache the branch flips and the wave moves to **Deploying**. See
[Scaling to 10,000+ devices](../architecture/scale.md) for the reasoning and
the numbers behind it.

The **Updates** board (Step 3, "the rollout procedure") shows the plan as a
ladder: each wave with its device count (size it small first - a canary -
then wider), soak, health floor, evidence floor and gate. Size a wave by its group and order;
refine each wave with the gates - including an optional **max at once** cap,
so a wave widens cohort by cohort rather than releasing its whole group in
one shot.

An organisation can require a gated test wave before any rollout starts; an
owner may skip it for a specific rollout, and that is logged.

### How "behind" is judged

A device is *behind* when the revision it reports differs from its group's
target pin. For that comparison to work, the deployed revision the agent
reports must be the git revision the config was built from - the same kind of
value the pin holds - not a store label.

This requires **one line in the overlay flake**: set
`system.configurationRevision = self.rev` (or `self.shortRev`) on each host.
The Sextant agent module then publishes it to
`/etc/sextant/configuration-revision`, and the agent reports it on every
check-in. Without that line the field is empty and the agent falls back to
the store label, which can never equal a git-hash pin - so every pinned
device reads as falsely *behind*. A flake with uncommitted changes has no
`self.rev`; commit before building, or the revision is unavailable.

### A device's own safety net

Under the waves, every device checks itself. A few minutes after it switches
to a new release, it confirms three things: the switch completed, it has a
network route, and the Sextant agent runs. If any of those fails, the device
goes back to the release it ran before and keeps reporting that one. The wave
then sees a device that did not converge and stops, which is the point.

That only works while the release before is still on the device, and that is
decided by store cleanup. DAWO-Core removes old releases daily, keeping
everything from the last seven days plus the release that was running seven
days ago. So the release before a switch always survives until the check has
spoken. Cleanup that keeps nothing older, `nix-collect-garbage -d` or a
zero-day window, leaves a bad switch with nowhere to go. The device module
warns when an overlay sets it.

How much a device keeps belongs to the device's configuration in the overlay
(`nix.gc` in NixOS), not to a console setting. A longer window costs disk and
buys nothing for this safety net. Going back further than the release before
is not done on the device: revert the change in the fleet repository and roll
that out like any other.

## Approval flows

How changes and updates are reviewed before they take effect is configurable
per organisation, under **Access -> Approval flows** (owner). The same three
toggles also live on the Updates board's
governance panel (Step 3) - they are the same setting, shown in both places:

- **Four-eyes** - a change may not be merged by its own author.
- **Require change-request** - configuration edits must go through a
  reviewed change, never a direct commit. When this is on, saving from the
  Configuration editor does not fail - it transparently stages the same
  edits as a fresh change and sends you to the Updates board to see it
  through review.
- **Require test wave** - a rollout must have a gated test wave first; an
  owner can skip it per rollout, and the skip is logged.

Every configuration change is a git commit that passes the Nix gate first, so
an edit that would not build never reaches the fleet.

## Troubleshooting

**A change is stuck in Failed.**
The card shows the distilled error - the actionable line pulled out of the
gate's evaluation trace (e.g. *"device X: unknown hardware profile 'Y'"*).
Fix the underlying edit and resubmit; if the message is not enough to act on,
the merge/submit response (and the gate-runner's own logs) carry the full
multi-line trace behind a "technical detail" fold.

**A wave sits on "Building" for a long time.**
Build-before-promote realises the wave's whole release into the signed
binary cache before its branch flips - this is centralised, one-time compute
per distinct configuration shape, not per device, but it is real wall-clock
on the build workers. Check the gate-runner/build-worker health before
assuming something is stuck; a missing build worker delays a rollout, it
does not corrupt one.

**A wave never leaves "Deploying" / "Soaking".**
Devices pull on their own schedule (comin), so convergence is not
instantaneous - check the Devices page or [Compliance](./compliance.md) for
the specific machines that have not landed on the target revision yet;
"behind" incidents there point at exactly which ones and since when.

**"Require test wave" blocks starting a rollout.**
Either add a wave with **require approval** checked to the plan, or (if you
are an owner and this run genuinely does not need one) use the explicit skip
offered on the start form - it is logged either way.

**A wave is stuck on Building.**
Build-before-promote is centralised, one-time compute per distinct
configuration shape - not per device - but it still takes real time on the
build workers. Check the gate-runner/build-worker's health before assuming
the rollout is stuck; a missing or overloaded build worker delays a
promotion, it does not corrupt it.

**A wave never reads Complete even though every device shows online and on
the target revision.**
It may still be inside its soak window, or waiting on a manual approval -
check the wave's status label on the Updates board
rather than only the device list.

**Saving settings redirects to the Updates board instead of just saving.**
Expected when **Require change-request** is on - the edits were not lost,
they were staged as a new change. Continue the review from there (diff,
submit, approve).

**A change cannot be merged even though it is Ready.**
If **Four-eyes** is on, the merge button is unavailable to the change's own
author - have a different reviewer approve it.
