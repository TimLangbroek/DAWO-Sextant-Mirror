# Device lifecycle and secrets

What happens to a device between its first check-in and the day it leaves:
the state it is in before it has spoken, retiring it, locking and wiping it,
and the secrets the console keeps on its behalf. Updating it is its own page,
[Ship an update](./updates.md).

## Provisional: a device that exists but has not spoken

Enrolling a device creates its record immediately, in the **provisional** state.
The first successful check-in promotes it to active.

That state exists because an install can fail, and a record left behind by a
failed attempt used to break more than itself: a ring made up entirely of
devices that never arrived could not converge by definition, so the whole
rollout waited for machines that were never coming.

So a provisional device is counted differently. It is a real record - you can
see it, name it, and re-image the same chassis onto it rather than minting a
second one - but it does not hold a ring back, because it has never claimed to
be running anything.

**Abandoned enrolments are listed rather than deleted.** Somebody starts an
installation that never reports: unfamiliar hardware, a slow link, a station
operator called away, a laptop enrolled on Friday that does not boot until
Monday. The console surfaces those as a list for an operator to act on, because
the two mistakes are not symmetric - reaping too early deletes a record somebody
is still using, reaping too late leaves a stale row in a list.

## Retire

Retiring a device keeps its record for audit but stops image builds, check-ins
and rollout counting. Reactivation is an explicit, audited step.

## Lock and wipe

Lock and wipe are **intent-as-data**: the console records the intent as an
audited change (on a device's own page, under the red-bordered remote-actions
panel); the device pulls it on check-in and acts locally. There is no live
command channel.

- **Lock** locks all sessions and persists across reboot; clear the intent to
  release.
- **Wipe** cryptographically erases the device by destroying its LUKS key slots.
  It is irreversible and gated: the root executor refuses a wipe unless the
  device is locked first, unless the intent is explicitly sent with force
  (which the console's wipe action does, backed instead by a
  type-the-device-tag confirmation as the human safety net). Arm a device for
  wipe only when it is cleared to be wiped.

## Manage secrets

Sextant handles two different kinds of secret, and the **Secrets** page (plus
a device's own page) is where both are managed. They are not interchangeable:
one is a *reference* an operator points settings at; the other is *material
the platform generated on a device's behalf* and holds so it can be recovered
later.

### Secret references (for settings and integrations)

A setting field that needs a secret value - a NetBird setup key, an LDAP bind
password, a Wazuh enrollment secret, an SMTP password - never accepts the
value itself in the console. Instead you register a **name**, and settings
pick that name from a list:

1. Open **Secrets**.
2. **Register a secret**: give it a name (`[a-z0-9][a-z0-9-]*`, e.g.
   `netbird-setupkey`) and an optional description.
3. Point any secret-typed setting field at it, in
   [Configuration editor](../concepts/safe-writes.md) or
   [Integrations](./integrations.md) - the field renders as a picker of
   registered names, with a shortcut to register a new one inline if you
   started from the setting itself.

The console never sees or stores the plaintext: only the *name* travels
through the config repo and Sextant's own state. The device resolves the
name to the decrypted material at runtime via agenix. Removing a registered
name breaks the build for anything still pointing at it - the console warns
before you confirm.

### Per-device secrets (break-glass recovery)

Some material is generated *for* a specific device during provisioning and
has nowhere else to live: a LUKS disk-encryption recovery passphrase, or a
break-glass local-administrator password. TPM2 enrolment makes the LUKS
passphrase unnecessary at day-to-day boot, but it remains the recovery path
if TPM2 unsealing ever fails - so it has to survive somewhere, encrypted at
rest, reachable only to someone who genuinely needs it.

Sextant seals this material (AES-256-GCM by default; a drop-in external key
manager such as OpenBao/Vault is the production posture) the moment it is
produced - at provisioning, from the imaging wizard - and never stores it in
the clear.

**Revealing** it:

- Reveal is **organisation-owner** reach only - not editor, not viewer.
- From a device's page (or the provisioning wizard, while a job is still
  fresh), *Reveal* shows the plaintext exactly once, rendered directly on the
  response - never redirected, so it never lands in a URL, browser history,
  or an access-log line.
- Every reveal is recorded: who, and when. There is no silent read.
- Once revealed, treat it as no longer fresh - the console flags a
  previously-revealed secret as such, since anyone who saw it once could
  have copied it.

If the secret store is not configured (no encryption key set), Sextant does
not store per-device secrets at all rather than write them in the clear - the
one-time value is then shown only at the moment it is generated (during
imaging) and never again.

### Who can decrypt them, and where that identity lives

Write this down for your own organisation. That it was never written down is
how BB Open ended up, on 2026-07-31, with four fleet secrets and no living
person able to open them - see the note at the end of this section.

An overlay's `secrets/*.age` files are encrypted to a RECIPIENT SET: a list of
public keys, any one of which can decrypt. In a Sextant fleet that set has two
kinds of member.

**Device host keys.** Every imaged device gets an SSH host keypair, and its
public half is added as a recipient so the device can decrypt the secrets it
needs at activation. The imaging station reports the public key, the console
records it, and `scripts/rekey-secrets.sh` re-encrypts for all of them. This
part is automatic and it is not the part that goes wrong.

**An admin identity.** A key a PERSON holds, so somebody can rekey after a
device is replaced, add a secret, or read one in an emergency. `rekey-secrets.sh`
takes it with `-i` and always includes its public key, precisely so an operator
cannot lock themselves out.

The trap is that the second kind is easy not to have. Nothing fails without it:
devices decrypt fine, activation succeeds, the fleet converges. It only
surfaces the day you need to rekey and discover that the only identities that
can open your secrets are the machines themselves - and if you re-image those
machines, imaging mints a fresh host key and destroys the old one.

So:

- Decide which key is the admin identity, and say so somewhere durable. A
  personal SSH key (`~/.ssh/<you>.pub`) is a fine choice and means an everyday
  key works.
- Keep it out of the fleet it protects. A copy in a password manager and a
  break-glass copy somewhere the cluster's failure cannot reach.
- Name at least two people. One identity is a single point of failure wearing
  a different hat.
- Check it periodically by using it. An identity nobody has exercised is a
  belief, not a capability.

#### What happened here, and why the guidance above is phrased that way

BB Open had no admin identity. An older `secrets.nix` recorded that device host
private keys live in a password manager and are baked onto devices at install,
so the de-facto admin identity was A DEVICE HOST KEY. A stopgap from an earlier
test run quietly became policy: nobody decided it, it just stayed, and it was
recorded nowhere.

It surfaced on the eve of a re-image. The four secrets were recovered in time
through a device that still held the key, but only because somebody asked the
question first. Imaging that device an hour later would have made them
unrecoverable.

Two details worth carrying: re-encrypting must be BYTE-EXACT (three of those
four carry no trailing newline; use `printf '%s'`, never `echo`), and a
credential that has passed through a terminal during recovery should be rotated
at its source on a normal schedule afterwards.

## Troubleshooting

**A wipe shows "refused" on the device page or in
[Compliance](./compliance.md).**
The device declined the intent - typically because a local interlock blocked
it even with force set. Clear the intent, confirm the device is locked, then
re-send the wipe.

**A wipe shows "failed".**
The device attempted the crypto-wipe but never confirmed completion back to
the console. Treat the disk as *not yet confirmed destroyed* - verify by
other means before reusing or disposing of the hardware.

**Retiring a device does not remove it from the fleet count.**
That is by design - retiring keeps the audit record and simply stops new
image builds, check-ins and rollout counting. Use *Remove* instead if the
device should be unenrolled entirely; unlike a retire, removal cannot be
undone by reactivation.

**A setting's secret picker is empty.**
No secret has been registered yet under that name pattern - register one on
the Secrets page first, or use the inline shortcut next to the field.

**Removing a secret reference breaks a build.**
Expected - any setting still pointing at that name fails the Nix gate on the
next change. Re-point the setting at a different registered name (or clear
it) before removing the reference, not after.

**"Reveal" is not available on a device.**
Either the secret store is not configured for this deployment (no
`SEXTANT_SECRET_KEY` / no external sealer wired up), no such secret was ever
generated for this device, or you are not signed in with organisation-owner
reach - reveal is deliberately not available to editors or viewers.

**The revealed LUKS passphrase does not unlock the device.**
Confirm you copied it in full (it is shown once, select-all) and that you
are unlocking the *current* value - if the device was re-imaged since the
secret was last generated, an old reveal (or a note copied from an earlier
session) no longer matches.
