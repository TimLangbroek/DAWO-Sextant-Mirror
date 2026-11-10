# 0017 - A local administrator password the console holds

Status: PROPOSED (2026-11-10). Closes #21 when built.

## Problem

Sextant escrows a device's LUKS recovery key (design 0009): stored sealed,
revealed through the console, every reveal audited. It does none of that for
the local administrator password, the credential an operator reaches for when
a device will not let them in.

Today `dawo.localAdmin.passwordSecret` names a secret holding a yescrypt
hash. The device consumes the hash and nobody can read the password back. The
plaintext lives wherever the operator put it, or nowhere. Found on
2026-08-10: the break-glass account for a ring existed, its SSH key worked,
and its password was in no vault, no document and no secret. The fallback was
real only because a shared bootstrap account with a documented password still
existed, which is the thing the local admin was meant to replace.

One hash per scope also means one password for every device in that scope:
whoever learns it once holds the whole group.

## Design

The device makes its own password and hands it to the console, the way it
already hands over its LUKS recovery key. Nothing new crosses the wire that
design 0009 does not already carry, and the console never has to push a
secret to a device.

**Opt in per scope.** `dawo.localAdmin.managed = true` next to `enable` and
`username`. With it, `passwordSecret` is not needed and is ignored. Without
it nothing changes: the hash path stays for whoever uses it.

**Device side (`sextant-actd`).** When the account is managed and the device
holds no password yet, the root executor:

1. draws a password (four groups of five from an unambiguous alphabet, about
   100 bits),
2. sets it with `chpasswd` at once, so no reboot or rebuild is needed,
3. keeps the yescrypt hash in `/var/lib/sextant-actd/admin.hash` (root,
   0600), which `users.users.<name>.hashedPasswordFile` points at, so the
   next activation keeps the same password instead of locking the account,
4. leaves the plaintext once in `/var/lib/sextant-agent/admin.key` for the
   agent.

The agent carries it on the next check-in in a new one-shot field,
`adminPassword`, and deletes its copy only on the `X-Admin-Password-Stored`
response header. Same rule as the recovery key: a console without a store
accepts the beat, does not send the header, and the device keeps the password
and tries again.

**Rotation is an intent**, `rotate-admin`: an audited commit like lock or
wipe. The executor draws a new password and the cycle above repeats. The old
password stops working the moment `chpasswd` runs, so the console holds the
only valid copy once the beat lands.

**Console side.** The check-in handler seals the password as kind `admin`
(the kind exists, unused) and confirms with the header. The device page
shows the password next to the recovery key: reveal is the existing path,
owner only, shown once, every reveal in the audit trail. "Rotate" sets the
intent. The page says when the device last escrowed one, and warns when the
account is managed and no password has arrived.

## What this deliberately does not do

- **No rotation after every reveal by default.** Windows LAPS offers it; here
  it would mean a password that changes while the operator is still typing it
  on a device that is offline. The page offers "rotate" right after a reveal
  instead. Open question 1.
- **No password from the console.** A password the console makes would have to
  reach the device, which is either a secret in the fleet repository or a
  channel this product does not have (design 0004: intents are data, not
  commands). The device making its own is the same direction design 0009
  already proved.
- **No change to the bootstrap account.** Turning it off is the overlay's
  step once a managed password has been revealed and tried, which is the
  point of #21.

## Open questions for Bram

1. Rotate automatically some hours after a reveal, or only by hand?
2. Is `managed` the right default for a NEW localAdmin block? The proposal
   keeps it off so nothing changes under an existing fleet.
3. Station class: same mechanism, or does a station keep a hash from the
   overlay because its operator is not a console owner?

## Test plan

- Device module check: managed without `passwordSecret` evaluates, points
  `hashedPasswordFile` at the executor's file, and the old path still
  asserts `passwordSecret` when not managed.
- actd: a VM test that mints, logs in with the escrowed password, rotates,
  and finds the old one refused.
- Agent: the field goes out once, the copy stays until the header, and is
  gone after it.
- Console: sealed as `admin`, header only when stored, reveal audited,
  rotate commits the intent.
