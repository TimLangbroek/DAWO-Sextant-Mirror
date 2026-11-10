# 0015 - A laptop joins with one code

Status: proposed, 2026-11-10. Open question for Bram: laptops that already
run NixOS (below).

## Problem

Adding a laptop without a station is not a path today; it is a
reconstruction from code. An operator:

1. enrols a record (console form, `sxctl devices enroll` or the API) and
   picks a hardware profile that must already exist in the fleet repository;
2. copies the credential from a page that shows it once, through a cookie
   that lives sixty seconds;
3. as root on the laptop, writes `SEXTANT_URL`, `SEXTANT_TAG` and a
   credential file by hand;
4. gets the laptop onto `nixosConfigurations.<tag>`, which needs Nix.

Thirty to sixty minutes for someone who knows Nix, impossible for someone
who does not. Nothing records the serial number, so a laptop enrolled twice
is two devices (#79). This is the C2 stream of v1: one code, no git, no Nix.

## Decision

### The code

"Add a laptop" in the console asks one question, the group, and answers
with a code:

```
K7QM-4XWD        valid until tomorrow 14:05      [QR]
```

- Eight characters of Crockford base32 (no I, L, O, U; typed `o` reads as
  0, `i` and `l` as 1, dashes and spaces are ignored), 40 bits.
- Valid 24 hours, one laptop by default. "More than one" (a batch) is a
  number on the same form, not a different kind of code.
- Kept as an HMAC-SHA256 of the code under a key derived from the session
  key, in a Postgres table `join_codes` (hash, group, uses left, expiry,
  who made it, optional device tag). A database read alone does not give
  the codes back, where a plain SHA-256 of 40 bits would. Expiry is
  computed from the clock, as elevation requests do; nothing has to run to
  expire a code.
- Open codes are listed on the same page and can be withdrawn.

**The code is the approval.** The first plan had a waiting room for codes
used more than once. It is dropped: the admin chose the group and the
number when making the code, and a waiting room is a second screen to
watch for an answer that was already given.

A code gives exactly one thing: a new device in the chosen group, with its
own credential. That is what a stolen device credential gives today, minus
an existing device's identity. It gives no console access.

**Guessing.** `POST /api/join` is limited like sign-in (2 per second per
address, burst 10). That is 172,800 tries per address per day against
2^40 codes: about 1 in 6 million that one address hits one open code in
its lifetime. Every refused attempt is logged with its address.

### The exchange

`POST /api/join`, outside `/api/v1` like `/api/checkin`:

```json
{ "code": "K7QM-4XWD", "name": "", "serial": "PF3ABCDE", "vendor": "LENOVO",
  "model": "21BR", "cpu": "GenuineIntel", "virtualisation": "none" }
```

The console:

1. **Spends one use of the code** in one statement (`uses_left > 0 and not
   expired`). Unknown, used and expired all get the same answer, so the
   endpoint is not an oracle for which codes exist.
2. **Picks the tag**: `name` when given, a free slug and allowed; otherwise
   `laptop-` plus the last six letters and digits of the serial; otherwise
   six random characters. A taken tag gets `-2`, `-3`.
3. **Matches the serial.** A provisional device with the same serial is
   reused, as the batch path already does. An active device with the same
   serial is refused: "this laptop is already enrolled as `lt-042`; use
   Re-enrol on its page". The serial is not a secret, so a code must never
   be able to take over a device that exists. Re-enrol on a device's page
   makes a code bound to that tag; using it rotates the credential.
4. **Picks the hardware profile** with the existing `Suggest(vendor,
   model)`. No match: `generic-intel` or `generic-amd` by the processor's
   vendor string, `vm` when `systemd-detect-virt` says virtual. A fleet with none of those
   refuses with the vendor and model to add a profile for.
5. **Commits through the gate** (`ConfigService.Apply`, `AddDevice` with
   `itam.serial` and `itam.model` filled), so the new host must evaluate
   before it exists.
6. **Issues the credential** (`DeviceCredentials.Issue`).

A failure at 4, 5 or 6 gives the use back and answers with the reason in
plain words. The answer on success:

```json
{ "tag": "laptop-abcde1", "credential": "sxt_…",
  "console": { "url": "https://console.example.org", "branch": "main" } }
```

The request waits for the gate. A cold remote gate takes about ninety
seconds, so the agent waits up to fifteen minutes and says what it is
waiting for.

### The laptop

`sextant-agent join <console-url> <code> [--name N]`, as root:

- reads the serial, vendor and model from `/sys/class/dmi/id`, the
  processor's vendor from `/proc/cpuinfo` and `systemd-detect-virt`;
- refuses a URL that is not https (or loopback), as the agent already does;
- writes the credential to `/var/lib/sextant-agent/credential`, mode 0600,
  atomically and without a trailing newline, and the tag, URL and branch to
  `/var/lib/sextant-agent/joined.json`;
- prints what comes next.

From there two cases.

**The installer (WP3).** `join` runs in the live system before the disk is
touched. The installer then clones `<url>/fleet.git` with the credential
and runs `nixos-install --flake <clone>#<tag>`. The disk layout is the
profile's disko, so the configuration describes the disk it was installed
on by construction. This is the path the v1 handbook shows.

**A laptop that already runs NixOS.** Its disk layout is not the
profile's: the fleet configuration would declare file systems that do not
exist, and the next boot fails (the previous generation stays in the boot
menu). Three ways out:

1. **Not in v1.** An existing NixOS laptop is reinstalled from the USB
   stick, like any other laptop. One path, one thing to test. The local
   disk is lost; the audience for v1 runs Windows or Ubuntu today and
   reinstalls anyway.
2. **The disk becomes data.** `join` also sends the current mounts (root,
   boot, swap, LUKS devices by UUID); the console stores them on the device
   in `fleet.json`, and the generator writes `fileSystems` from them instead
   of disko. One field, and a second file-system path to keep working.
3. **A local flake** on the laptop that imports the fleet configuration plus
   its own `hardware-configuration.nix`. Refused: the gate would never see
   what the laptop builds.

**Recommended: 1.** Option 2 can follow without changing anything above.

## What changes for the existing paths

- "Add a laptop" is the one entry on the Devices page. The current form
  (tag, group, profile, class) moves under Advanced; `sxctl devices enroll`
  and `POST /api/v1/devices` stay for scripts.
- The station path is unchanged: a station enrols in batches and gets its
  credentials per job (design 0011).
- The facter report arrives with the first check-in, as it does today.
  Making it the hardware configuration (`hardware.facter.reportPath`, in
  current nixpkgs) is a later step; until then a model without a
  profile gets the generic one.

## Not doing

- An approval queue (above).
- A code inside an image. The installer asks for it at first start; a lost
  USB stick gives no access.
- Joining without a database. Credentials already need one; the page says
  so instead of offering a button that cannot work.

## Done when

A NixOS VM test: a console makes a code, a second VM runs
`sextant-agent join`, the device is in `fleet.json` with its serial and in
the chosen group, and its credential checks in. Using the code a second
time is refused, and so is a join from a machine whose serial belongs to an
active device.
