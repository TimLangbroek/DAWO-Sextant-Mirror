# 0016 - An installation USB per organisation

Status: accepted, 2026-11-10 (Bram took all three recommendations at the
end). Builds on 0014 (the starter fleet) and 0015 (join with a code).

## Problem

Design 0015 gives a laptop its identity with one code, but the laptop
still needs the Sextant agent first, and a disk laid out the way its fleet
configuration says. Today that means NixOS installed by hand, which is the
step v1 has to remove: the audience runs Windows or Ubuntu and reinstalls.

The station path (design 0011) installs laptops without anybody touching
them, but it needs a station, and the station's own configuration is not in
the open repository (#181). An organisation with five laptops will not
build a PXE network for them.

Bram decided on 2026-11-09: an ISO, and Sextant builds it per organisation.
Organisations have their own fleet repositories with their own drivers,
firmware and settings, so one generic image would install a system that is
not theirs.

## Decision

### What is on the stick

A NixOS live system built from the organisation's own fleet repository,
with:

- the Sextant agent, git, disko and sbctl;
- the console's address, read from `fleet.json` (`console.url`, design
  0014), so the person at the laptop types the code and nothing else;
- the fleet repository's kernel, firmware and network settings, so the
  live system sees the same hardware the installed one will.

**No code and no credential.** The code is typed at the laptop, and a lost
stick gives nobody access (0015, "Not doing").

### What the stick does

On tty1, without a login, one program in plain words:

1. **Network.** Wired works by itself. Without it, a list of wireless
   networks and a password field.
2. **Code.** `sextant-agent join <console> <code> --state-dir /tmp/joined`.
   A refusal is shown as the console words it, and the program asks again.
   The disk has not been touched yet, so a wrong code costs nothing.
3. **Confirm.** "This laptop becomes laptop-3abcde in group pilot. Everything
   on Samsung PM9A1 (512 GB) is erased." The disk is the largest internal
   one that is not the stick itself. One key to go on.
4. **Install.** Clone `<console>/fleet.git` with the credential (design
   0014), run the device's disko layout, then `nixos-install --flake
   <clone>#<tag>`. Substitutes come from the release cache with the same
   credential: the gate-runner already accepts device credentials there.
5. **Hand over.** Copy the credential and `joined.json` into
   `/mnt/var/lib/sextant-agent`, and reboot.

From the first boot the agent checks in with the credential the installer
copied, and the laptop is active in the console.

### Disk layout (Bram, 2026-11-10)

Nothing in a fleet said how to lay out a disk: there is no `disko.devices`
in the starter fleet or in a generated device configuration, so "the
device's disko layout" this note relied on did not exist. Decided: easy by
default, and room for whoever wants more.

- **Default from Sextant.** The device module brings one layout through
  disko: GPT, a 1 GB EFI system partition, and the rest one LUKS2 volume
  holding the root file system. A fleet that says nothing gets it, and the
  person at the laptop is never asked.
- **The fleet's own layout wins.** A fleet that declares `disko.devices`
  for a device, a group or the whole organisation replaces the default
  (`lib.mkDefault` on Sextant's side). Swap, btrfs subvolumes, a second disk:
  the power user writes disko and the installer runs it unchanged.
- **Encryption is not optional.** The gate refuses a layout without a LUKS
  volume on the root, because escrow (design 0009) and **Secure this
  laptop** both assume one.
- The installer evaluates the device's configuration and runs whatever
  `disko.devices` comes out, default or not. Step 3's refusal ("this fleet
  does not say how to lay out a disk") becomes unreachable and goes.

This adds disko as an input of the Sextant flake, pinned like nixpkgs.

### Disk and encryption

The disk is encrypted from the first boot, and the first boot asks
nobody for anything.

- The installer creates the LUKS volume with a phrase of five groups of
  five characters from the administrator password's alphabet (no I, O, 0
  or 1; 125 bits), and seals a TPM2 keyslot **without PCR binding**. The
  laptop unlocks at first boot by itself; the disk is still useless outside
  this machine. (The note said six words, "as the station does"; the
  station's script is not in this repository, and a laptop without a TPM2
  types this phrase at every start, so it follows the one alphabet Sextant
  already has.)
- The same phrase is staged as `luks-enroll.key` for **Secure this
  laptop**, which shreds it once it has sealed to PCR 7.
- The phrase is escrowed **before the restart** (Bram, 2026-11-11). The
  installer puts it where the agent picks up a recovery key and runs one
  check-in of the agent from the live system with the new credential
  (`sextant-agent --once`). The console seals it and confirms, and the
  agent deletes its copy: the escrow path of design 0009, unchanged, only
  earlier. Without this a laptop that asks for its phrase at first start
  (no TPM2, or the binding failed) would ask for one nobody knows, since
  the installed agent only runs behind that prompt. When the console does
  not confirm, the installer shows the phrase to write down and waits for
  a key. The installed system also gets it as `recovery.key`; a second
  escrow of the same phrase changes nothing.
- A laptop without a TPM2 has no unattended unlock. See question 2.

### Securing it afterwards

The laptop is managed from the first check-in, and its page says it is not
secured yet, with one button: **Secure this laptop**. That is the existing
provision ceremony (designs 0001, 0004, 0009):

- the installer generates the Secure Boot keys on the laptop
  (`/var/lib/sbctl`), so they never leave it, and lanzaboote signs the boot
  chain at install time;
- it stages the one-shot enrol key at `/var/lib/sextant-actd/luks-enroll.key`,
  inside the encrypted volume;
- the one manual step stays the firmware's: switch Secure Boot on and reset
  to setup mode. sextant-actd enrols the keys, reboots, and seals LUKS to
  PCR 7.

One change to sextant-actd: when it seals to PCR 7 it removes the unbound
TPM2 keyslot from the install (`--wipe-slot=tpm2` in the same call).
Otherwise the slot that ignores Secure Boot would keep unlocking the disk.

### Who builds it

The console has no Nix; the gate-runner does, and already builds closures
for a revision as a job (`POST /build`, `cmd/gate-runner/build.go`). The
ISO is one more kind of job:

- `POST /artifact {rev, name: "installer"}` builds
  `packages.x86_64-linux.installer` from the fleet repository at that
  revision. Poll-style and idempotent per (rev, name), like `/build`.
- The result is kept next to the cache, the last two revisions only.
- The fleet flake exposes it in one line:
  `installer = sextant.lib.mkInstaller { inherit self; };`. The starter
  fleet (0014) has that line; for a fleet without it, the console shows the
  line to add.

### Where it is downloaded

**Add a laptop** gets a second button: **Download the installation USB**.
The first click asks the gate-runner for the current main revision and shows
"building, about ten minutes"; when done, the console streams the file to
editors. The gate-runner stays internal, as it is now.

The page shows the revision and the date the stick was built. A stick from
an older revision still works: the installer installs what the fleet
repository says at install time, not what the stick was built from. A new
stick is only needed for new hardware support in the live system.

## What changes

- `sextant.lib.mkInstaller` in the Sextant flake, and the `installer`
  output in the starter fleet.
- `POST /artifact` on the gate-runner, and a download route on the console.
- A default disko layout in the device module, which a fleet's own
  `disko.devices` replaces, and a gate check that the root is on LUKS
  (above, "Disk layout").
- sextant-actd removes the unbound TPM2 keyslot when it seals to PCR 7.
- The add-laptop page gets its USB line back (removed in aed02d5 until
  there is a stick).

## Not doing

- A code or credential in the image (above).
- One generic ISO from the Sextant project. Decided per organisation.
- Dual boot or keeping the old disk. The disk is the fleet's.
- Building ISOs for aarch64. The fleet is x86_64 today.
- The station (WP4). It reuses this stick with a "this becomes a station"
  choice, in its own design.

## Decided by Bram (2026-11-10): all three recommendations

1. **Unbound TPM2 at first boot.** Between install and **Secure this
   laptop**, the disk unlocks on this machine without Secure Boot. The
   alternative is a phrase typed at every boot until then, which puts a
   person back in front of the laptop. Recommended: unbound TPM2, with the
   device page showing "not secured" in warning colour until the ceremony
   is done.
2. **Laptops without a TPM2.** (a) refuse at step 3, (b) install and ask
   for the phrase at every boot (the owner reveals it in the console).
   Recommended: (b), it is how the station path behaves today.
3. **Download size and place.** Not measured yet; the NixOS minimal ISO is
   about 1 GB, and firmware and the agent add to it. Two revisions next to
   the cache on the gate-runner's volume. Recommended: yes, and measure the
   size on the first build before deciding anything else.

## Done when

A NixOS VM test: a console with a fleet repository builds the installer
through the gate-runner; a second VM with an empty disk and a TPM2 (swtpm)
boots it, types a code, installs, reboots without anyone typing anything,
and checks in as an active device whose recovery phrase the console holds.
