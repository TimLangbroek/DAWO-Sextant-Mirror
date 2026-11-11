# 0018 - An imaging station from the same USB

Status: accepted, 2026-11-11 (Bram took all three recommendations, see
the end). Builds on 0011 (the station surface), 0015
(join with a code) and 0016 (the installation USB). Closes #181.

## Problem

Gate C3 for 1.0 says: an imaging station is set up with the installation
USB and one code, and the laptops it installs come out with Secure Boot in
that one installation. Today none of that holds.

- The station's own configuration is not in the open repository. The
  handbook (`operators/station-nuc.md`) says `cd inspoelstraat-appliance`
  and `--flake .#dawo-inspoelstraat-install`; nobody outside has that
  flake (#181). The station that does run uses modules in a private
  overlay, so the appliance flake is stale as well.
- Setting up a station is nine manual steps: a NixOS minimal stick, SSH by
  hand, `nixos-anywhere` from a workstation, a credential pasted into a
  file with no trailing newline.
- The station installs a target over SSH with an operator key baked into
  a private netboot image (`dawo-installer-netboot`), and runs
  `nixos-anywhere` against it. That is a second installer next to the one
  0016 built, with its own disk layout, its own phrase and its own
  failure modes.
- All of the station's logic is bash in systemd timers: report the
  dnsmasq leases, claim jobs, run the install, post the status. It works
  against the API in `internal/http/api/station.go`, but nothing in this
  repository tests it.

## Decision

### One installer, two ways in

The program on the USB (`sextant-installer`, 0016) is also what a target
runs when it PXE-boots from a station. The station serves the netboot form
of the same live system: same kernel, firmware, disko, agent, phrase
rules, escrow and handover. The difference is where the code comes from.

- **From the USB**, a person types the code.
- **From a station**, there is no code: the installer asks the station
  for its job by MAC address (`GET http://<station>/job`, the station
  reads the MAC from its own DHCP lease). The job is what the console's
  claim already returns: tag, credential, hardware, revision. The
  installer shows "This laptop becomes laptop-3abcde. Everything on
  ... is erased" and installs **without** waiting for a key, because the
  operator already approved this machine in the console's enroll queue
  (0011). It reports its steps to the station, which relays them to the
  console as job status, so the console's job page keeps working as it
  does.

No SSH into the target, no operator key, no `nixos-anywhere`. A station
installs a laptop exactly the way the USB does, so there is one install
path to test and to fix.

### The station is a fleet device

A station is a device of class `station` in the fleet repository, with an
entry under `stations` (as today). Its NixOS configuration is generated
like every other device's and imports `sextant.nixosModules.station`,
which is new and lives in this repository:

- **DHCP, TFTP and HTTP on a dedicated interface** (dnsmasq, iPXE, nginx),
  serving the organisation's netboot installer. The interface and subnet
  are options; nothing is hardcoded.
- **A binary cache** (harmonia) so a room of laptops downloads the system
  once.
- **`sextant-station`**, a Go program in `cmd/sextant-station`, replacing
  the bash: it reports leases, claims jobs, serves `/job` to targets on
  the imaging network, and relays their status. Tested with `go test`
  against the real API handlers, like `cmd/fleetsim` does today.
- **The agent and actd**, as on every device, so the station updates
  through the rollout ladder like the rest of the fleet.

The station needs its own netboot artefacts, which are the organisation's
installer built from the fleet repository (0016's `mkInstaller`, netboot
output). The station builds them from the revision it was told to use, so
a target is never installed from a different fleet revision than the one
the console approved.

### Setting it up: Add a station, one code, the USB

1. In the console, **Organisation → Imaging stations → Add a station**
   gives a code, like Add a laptop does, but the code is marked as a
   station code. (Name and site are optional; the tag is drawn as for
   laptops.)
2. The person boots the USB on the box that will be the station and types
   the code. The installer reads from the console's answer that this
   becomes a station and says so: "This machine becomes imaging station
   station-7k2p9q. Everything on ... is erased."
3. Joining with a station code makes the device (class `station`) and the
   `stations` entry in one commit, and returns the device credential as
   for a laptop. The station API accepts that credential for the station
   with the same tag, so a station holds one credential, not two, and
   nobody copies a token into a file. (A separately minted station
   credential keeps working for stations set up the old way.)
4. After the reboot the station checks in as a device and starts
   reporting. The console's station list shows it within a minute.

The person never chooses "laptop or station" on the stick; the code
decides, so a wrong choice cannot happen.

### Secure Boot in one installation

A laptop that a station installs is part of a batch, and the point of the
station is that nobody touches each laptop afterwards. So:

- The installer creates per-device Secure Boot keys (`sbctl create-keys`
  into the installed system's `/var/lib/sbctl`), as the station's runner
  does today.
- At first boot actd's existing `provision` step finds the firmware in
  Setup Mode and the keys present, enrols them, reboots once, and seals
  the disk to PCR 7, removing the unbound slot (`--wipe-slot=tpm2`,
  already built for 0016).
- A laptop whose firmware is not in Setup Mode installs as from the USB:
  unbound TPM2, "not secured" on its device page, **Secure this laptop**
  afterwards.

Putting the firmware in Setup Mode stays the one thing a person does per
laptop before it goes on the imaging network (or the vendor ships it so).
The station's job page says per laptop whether it came out secured.

The USB path keeps 0016's behaviour: unbound until **Secure this laptop**.

## What changes

- New: `cmd/sextant-station`, `deploy/nixos/station/`,
  `nixosModules.station`, a netboot output next to `mkInstaller`.
- `sextant-installer` gets a station mode (`--station <url>`): job by MAC,
  no confirmation key, status to the station, Secure Boot keys created.
- Join codes get a kind (`laptop`, `station`). The station API accepts a
  station device's own credential for its own tag.
- Console: **Add a station** on the Imaging stations page.
- Handbook: `operators/station-nuc.md` becomes one page with the three
  steps above. The nine manual steps and every reference to the appliance
  flake go.
- The starter fleet's generator gives class `station` the station module.

## Not doing

- Proxy-DHCP on an existing office network. The station owns a dedicated
  interface and hands out its own addresses, as today; a network that
  already runs DHCP gets a separate switch or VLAN. Proxy-DHCP can follow
  without changing the rest.
- Moving BB Open's own station onto this module. That is an operator job
  on real hardware after 1.0's acceptance test passes, on Bram's go.
- Wireless PXE, aarch64, more than one station per site (0011).

## Decided by Bram (2026-11-11): all three recommendations

1. **Targets run the installer, no SSH.** The alternative was porting the
   SSH + `nixos-anywhere` runner, which works and is proven on hardware.
   What decided it: the runner sends the disk key to the console as a
   status message (`luks-recovery-key:`), the USB through the agent's
   escrow (0009); one path for the most sensitive data is one path to
   test and audit. The installer has to be proven on hardware for C2
   anyway, so that proof covers the station too. The step-by-step job
   status in the console stays: the installer reports the same steps.
2. **The code decides laptop or station**, not a menu on the stick. The
   plan said "a choice on the USB"; this is that choice made in the
   console where the operator already is.
3. **Station-installed laptops seal at first boot** when the firmware is in
   Setup Mode, without **Secure this laptop**. That is what makes C3's
   "Secure Boot in one installation" true.

What the job hands a target over the imaging network is the device's
one-time credential. That is no worse than today: the SSH runner does not
check the target's host key either. The station hands each job out once,
only to the MAC that holds the lease, and the console revokes and reissues
it on a re-image.

## Done when

A NixOS VM test: a console with a fleet repository; a VM boots the USB with
a station code and becomes a station; a second VM PXE-boots on the
station's network, is approved in the console, installs, comes up, checks
in with its credential and has escrowed its phrase. Firmware in Setup Mode
(OVMF with swtpm) ends with Secure Boot on and the disk sealed to PCR 7.
Part of the C1-C4 acceptance test.
