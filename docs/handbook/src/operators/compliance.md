# Track compliance

**Compliance** answers one question: which devices are not to spec right now,
and why. It is the full drill-down behind the compliance donut on
**Overview** - the donut caps its attention queue at 8 items; this page lists
every open incident.

## Reading the page

The three summary chips (**All**, **Critical**, **Warning**) double as a
filter - click one to narrow the device table below it. Every active
(non-retired) device appears, worst status first:

- **Critical** - an error the device reported, or a wipe that failed or was
  refused.
- **Warning** - offline, never checked in, or behind its target revision.
- **To spec** - no open incident.

Each row lists the device's issues with a short **title**, a **detail** (the
specifics - e.g. which revision it is running versus its target, or when it
was last seen), and a suggested **action**.

## What raises an incident

| Kind | Severity | Raised when |
|---|---|---|
| Never seen | Warning | Enrolled but no check-in has ever arrived |
| Offline | Warning | Stopped checking in within the online window |
| Behind | Warning | Online, but running a different revision than its group's target |
| Errored | Critical | The device reported a build/apply error on check-in |
| Wipe refused | Warning | The device declined a wipe intent (unarmed, or an interlock - typically "not locked first" - blocked it) |
| Wipe failed | Critical | A crypto-wipe was attempted but did not confirm completion |

A single device can carry several incidents at once (e.g. offline *and*
behind). Suggested actions point at the fix: verify imaging and connectivity
for never-seen, check power/network for offline, check the rollout and
device logs for behind, open the device to inspect the failure for errored,
and re-arm or clear the interlock for a refused wipe (see
[Update, retire and wipe](./lifecycle.md)).

## Policy exposure

Below the device table, a per-policy table shows where each policy is
assigned (its scope targets) and, of the devices under those targets, how
many currently carry an open incident - a revision-level proxy for "this
policy may not actually be applied everywhere it is assigned yet". A policy
with no assignments shows as unassigned; it has no effect until targeted.

## The report per device

An auditor asks which control failed, on which device, measured when. Every
device page has **Full report (JSON)** next to its baseline, and the API
answers the same document at `GET /api/v1/compliance/{tag}` for anyone who
may view the device. It is computed when asked, from the state the console
already holds, so there is no stored copy that could disagree with the page.

The report names its shape in `schema` (`sextant.compliance-report/1`),
says when the device last reported in `measuredAt` (every observed value is
from then), and lists every requirement as an item with a `result` of `pass`,
`fail` or `unknown`:

| Kind | One item per | Fails when |
|---|---|---|
| `baseline` | criterion of the device list's verdict | the verdict fails it; `baseline` at the top is that verdict |
| `condition` | condition of a policy that reaches the device | the measurement does not meet it |
| `settings` | policy with settings that reaches the device | another scope or policy sets one of its keys differently here |

`unknown` means the console holds no measurement that decides it: the device
never reported, or it does not report that metric, or it runs an older
revision than the one the settings were read from. It is never a pass.
Condition and settings items carry the policy's `controls`, so an auditor can
go from "BIO 12.3.1" to the devices where it does or does not hold. Each item
keeps its `id` from one report to the next, so two reports compare item by
item. A retired device, or one not installed yet, has `baseline: not-judged`
and no items.

## Evidence for a period

**Audit -> Evidence export** downloads one JSON document for a period, and
`GET /api/v1/evidence` answers the same: the approval controls in force, every
commit to the fleet repository (who changed what), every change request
touched, every rollout wave promoted, and every reveal of a device secret.

Reveals are listed separately because reading a LUKS recovery key or an
administrator password changes nothing in the repository, so no commit shows
it. Each one is kept with who read which secret of which device, and when; a
later reveal or a new secret does not replace it. A database from before this
record carries over the last reveal of each secret it still held, and no
earlier ones, because those were never kept.

### Into a SIEM

`GET /api/v1/evidence/ocsf` answers the same period's commits and reveals as
[OCSF](https://schema.ocsf.io) 1.9.0 events, one JSON object per line, oldest
first, with the same `from` and `to`. Both are API Activity (class 6003): a
commit is an Update by its author, a reveal is a Read of medium severity by
whoever revealed. A SIEM collects them by calling the endpoint on a schedule
with a token that has the viewer role, for the period since its last call.

## Troubleshooting

**A device shows as behind right after a rollout started.**
Expected - "behind" just means the device has not yet pulled and converged
to its target revision. Give it a check-in cycle or two before treating it
as stuck; if it stays behind well past the wave's soak window, check
[Ship an update](./updates.md) for whether the wave itself is stalled.

**A device never clears "never seen".**
It has an enrolled record but has not reported at all. Confirm it was
actually imaged (see [Image a device from the console](./image-devices.md))
and that its agent credential and network reach the console - the same
credential/newline pitfall documented in
[Set up an imaging station](./station-nuc.md) applies to a device's own
agent credential too.

**A wipe keeps showing "refused".**
The root executor requires the device to be locked first (or the intent to
be forced) before it will act on a wipe. Re-arm the wipe intent, or lock the
device first, then retry - see
[Update, retire and wipe](./lifecycle.md).

**A policy's settings stay "unknown" in the report.**
The device does not run the revision the console reads the fleet from. On a
ring pinned to an older revision that is expected: the report does not read
old revisions, so it cannot say whether the settings were already in it.
Once the device runs the latest revision, the item passes.
