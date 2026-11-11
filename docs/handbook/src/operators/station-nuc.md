# The imaging station

An imaging station (the "inspoelstraat") is a small always-on box that boots
target devices over PXE and reports what it finds to the console. From there
you dispatch imaging jobs at the target devices - the station itself is set up
once (and occasionally re-provisioned), then left running.

This chapter is the full procedure, kale NUC to working station. Steps marked
**Manual step** are not driven by the console today - they are candidates for
future turnkey automation, but for now someone does them by hand at a
keyboard or a workstation shell.

## Checklist

Work through these in order. The station is done when the last box is ticked.

- [ ] **Manual step** - write a NixOS minimal USB installer
- [ ] **Manual step** - boot the NUC from the USB, bring up SSH
- [ ] **Manual step** - run `nixos-anywhere` from your workstation to install
      the `-install` variant (systemd-boot + LUKS passphrase, Secure Boot off)
- [ ] Register the station in the console and mint its report credential
      (owner, console step)
- [ ] **Manual step** - put the credential on the station and point the agent
      at the console
- [ ] Verify the station shows as registered and reachable in the console
- [ ] **Manual step** - PXE-boot a test target device on the station's network
      and confirm it appears as discovered
- [ ] Wire the station to your fleet overlay so it can dispatch real imaging
      jobs (see [Image a device from the console](#image-devices-from-the-console))
- [ ] *(Later, once the fleet needs it)* **Manual step** - re-provision the
      station on the `-sb` variant (Secure Boot on) and enrol TPM2, then move
      it to the steady-state `dawo-inspoelstraat` configuration

## 1. Write a NixOS minimal USB installer — manual step

```
lsblk                              # find the whole stick, e.g. /dev/sdb (NOT sdb1)
sudo dd if=<nixos-minimal>.iso of=/dev/sdX bs=4M status=progress oflag=sync
```

Double-check the device node against `lsblk` before running `dd` - the whole
disk, not a partition, and never the workstation's own disk.

## 2. Boot the NUC and bring up SSH — manual step

Plug in the USB and wired ethernet. Power on; the NUC's boot menu is **F10**
if it does not auto-boot the stick (some models use **F2** or **Esc** -
check the splash screen). At the installer prompt:

```
sudo systemctl start sshd
sudo mkdir -p /root/.ssh
sudo tee /root/.ssh/authorized_keys < keys/id_ed25519.pub   # operator pubkey
ip -brief a                                                  # note the NUC's IP
```

## 3. Install with nixos-anywhere — manual step

Pick a LUKS passphrase and store it in your password manager first, then
drive the install from your workstation (not the NUC itself):

```
cd inspoelstraat-appliance
printf %s '<LUKS-passphrase>' > /tmp/luks.key
NIX_SSHOPTS="-o IdentitiesOnly=yes -o StrictHostKeyChecking=no" \
  nix run github:nix-community/nixos-anywhere -- \
    --flake .#dawo-inspoelstraat-install \
    --target-host root@<installer-ip> \
    -i ../keys/id_ed25519 \
    --generate-hardware-config nixos-generate-config ./hardware-configuration.nix \
    --phases disko,install \
    --disk-encryption-keys /tmp/luks.key /tmp/luks.key
rm -f /tmp/luks.key
```

Notes:

- `--generate-hardware-config` re-probes the NUC and writes its real kernel
  modules before building - do not reuse a hardware config from a different
  box.
- Disko targets `/dev/nvme0n1` (the NUC standard). Adjust the disko module if
  your hardware differs.
- `printf %s` (not `echo`) writes the passphrase with **no trailing
  newline**. This matters again in step 4 - see the troubleshooting note
  below.
- This installs the `-install` variant: Secure Boot off, no TPM2 key
  enrolled, systemd-boot + a LUKS passphrase at boot. That is deliberate -
  Secure Boot and TPM2 are enrolled later, once the box is confirmed working.

The NUC reboots into its fresh, minimal NixOS install once this completes.

## 4. Register the station in the console

In the console, under **Organisation -> Imaging stations** (owner reach):

1. **Register an imaging station**: give it a tag (e.g. `dawo-inspoelstraat`,
   matching `[a-z0-9][a-z0-9-]*`), an optional description and site.
2. **Mint credential**: the console generates a bearer token and shows the
   **report endpoint** (`https://<console-host>/api/station/<tag>/report`)
   and the token together. This is a one-shot reveal - the token is never
   shown again, so copy both before leaving the page.

## 5. Put the credential on the station — manual step

Configure the station's report-agent (in the `inspoelstraat-appliance` flake)
with the endpoint and bearer token from step 4, so it can `POST` its
discovery reports to the console. Exactly how the agent picks up the token is
a station-flake concern (a runtime credential file is the usual pattern, same
shape as the per-device agent credential - see
[Install and configure Sextant](./deploy.md)); the important operational rule
is the same one that trips up the per-device credential too:

> **Write the token with no trailing newline.** `printf %s '<token>' > path`
> or a paste that strips the newline both work; `echo '<token>' > path` (or a
> plain heredoc) leaves a `\n` at the end of the file. A bearer token with a
> stray newline never matches what the console minted, and the station's
> reports fail authentication with no more detail than "unauthorized" - see
> Troubleshooting below.

## 6. Verify the station is live

Back in **Organisation -> Imaging stations**, open the station. If the agent
is configured correctly it appears registered and (once it has reported at
least once) shows a device count from its PXE network. If nothing shows up
yet, PXE-boot a target device on the station's network next - see step 7.

## 7. Confirm discovery — manual step (boot a target device)

PXE-boot any spare target device on the station's network. It should appear
under the station's discovered devices within a minute or two. This proves
the whole chain: agent -> report endpoint -> console -> discovered plane.

## 8. Wire the station to your fleet overlay

The station is now registered and reporting, but imaging real devices from it
needs the station's runner wired to your fleet overlay (so it can build and
install the right configuration for each target). See
[Image a device from the console](#image-devices-from-the-console) for the imaging flow
itself, driven from **Enrollment** in the console.

## Later: Secure Boot and TPM2

The steps above deliberately leave Secure Boot off and TPM2 unenrolled - get
a working station first. Once it is confirmed:

1. **Manual step** - re-provision with the `-sb` flake variant (Secure Boot
   enabled, still a LUKS passphrase at boot).
2. **Manual step** - enrol TPM2 on the device so the passphrase is no longer
   needed at boot (it remains as break-glass recovery - see
   [Manage secrets](./lifecycle.md#manage-secrets)).
3. Move the station onto the steady-state `dawo-inspoelstraat` configuration.

This same Install -> Secure Boot -> TPM2 -> Done progression is what the
provisioning wizard walks an operator through per target device - see
[Image a device from the console](#image-devices-from-the-console).

## Image devices from the console

Once the station is registered and reporting, imaging target devices is
driven entirely from the console's **Enrollment** page - no shell access to
the station is needed for a routine batch.

### Step 1: choose the station

Open **Enrollment**, pick the station whose PXE network the target devices
are on, and continue. If no station is listed, none is registered yet; the
steps above set one up.

### Step 2: batch-dispatch the discovered devices

**Pre-flight (one firmware visit per device, before PXE):** enable the
Security Chip (TPM2) and clear it once; enable Secure Boot AND reset it to
setup mode (clears the factory keys). With that state set, everything after
Dispatch runs hands-off - signed install, key enrolment, TPM2 sealing,
verification - with no BIOS visit afterwards.

PXE-boot the target devices; they appear as discovered rows (MAC, vendor,
model, disk size) under the chosen station. Imaging is **batch-only** by
design - one audited pass images a whole rack rather than one device at a
time:

1. Set the shared **hardware profile** (suggested from make and model where
   the profile catalog matches), **class** and **group** for this rack of
   otherwise-identical machines.
2. Tick the devices to include, and give each one a CMDB name (the tag it
   will enrol under).
3. **Dispatch imaging**. This creates each device's record, captures its
   hardware specs, and queues an image job per device.

### Step 3: watch the jobs run

The imaging-jobs table shows each device's status live: pending -> imaging
(with a progress bar and current step) -> installed, or failed. Under the
hood, the station runner claims each job, resolves the target's IP from its
DHCP lease, runs `nixos-anywhere` against that device's generated
configuration, and bakes in the device's one-time agent credential.

The install stages everything the security ceremony will need, so nothing
has to be generated or typed on the device later:

- **Per-device Secure Boot signing keys**, generated on the station (`sbctl`)
  and shipped with the install. When the device's resolved config enables
  `secureboot.enable`, the boot chain is signed *during the install itself* -
  no separate "audit mode" deploy round-trip.
- **A disk-encryption recovery phrase** - six plain words (diceware), typed
  exactly as revealed, instead of a 32-character random string. It is sealed
  into the per-device secret store; revealing it is owner-only and audited.
- **A one-shot TPM2 enrol key**, staged root-only inside the encrypted
  volume; the on-device executor uses it once to seal the disk to the TPM2
  and then shreds it.

After the install the device reboots into its new system by itself.

Open the **provisioning wizard** (linked from the jobs table) for the guided,
per-device view of the same batch. It walks a four-phase stepper - **Install
-> Secure Boot -> TPM2 -> Done** - and adapts to what each device's hardware
profile actually needs:

- Which phases apply is decided by the device's **resolved config**
  (`secureboot.enable`, `diskUnlock.tpm2.enable`) and its **hardware** (no
  EFI -> no Secure Boot phase; no TPM2 chip -> no TPM2 phase). A device that
  needs neither goes straight from installed to done on its first check-in.
- Exactly **one manual step** remains: when a device reaches the Secure Boot
  phase, the wizard shows a brand-specific firmware action (the entry key
  and the exact BIOS steps - e.g. Lenovo/ThinkPad enters on **F1**, Intel
  NUC on **F2**, HP on **F10**) and an in-console **reboot** control. Enable
  Secure Boot and reset to setup mode; everything after that is automatic -
  the device enrols its (pre-staged) keys, reboots enforcing, seals the disk
  to the TPM2 and reboots once more.
- Every phase turns green only on what the device itself reports (firmware
  state, executor acknowledgements) - the final **done** card is a
  verification: Secure Boot observed enforcing, TPM2 sealing confirmed by
  the executor that performed it.
- Each row carries a plain-language "Now:" line telling a non-expert exactly
  what is happening or what to do; the page refreshes itself.
- The wizard also tells you when it is safe to unplug a device (once its
  phase reads **done** - before that, keep it cabled so it can keep checking
  in and converging).
- If a device produced a one-time LUKS recovery key during provisioning, the
  wizard shows it once (a break-glass secret store keeps it recoverable
  later for an organisation owner - see [Manage secrets](./lifecycle.md#manage-secrets)).

### Step 4: converge

Each imaged device boots, checks in, and converges its configuration. It then
shows on the device page with its facts and posture (Secure Boot / TPM2
state), and on [Compliance](./compliance.md) if anything about it needs
attention.

## Troubleshooting

**The station never shows a device count / discovered devices.**
Check, in order: the station's network actually serves PXE to the target
device; the agent process is running on the station and can reach the
console's report endpoint (`curl -i <report-url>` from the station should at
least get a 401, not a connection failure); and the credential file has no
trailing newline (see step 5).

**Minting a new credential "loses" the station.**
It does not - re-minting only invalidates the *previous* token. Update the
credential file on the station with the new one (again, no trailing
newline) and it resumes reporting.

**`nixos-anywhere` fails partway through disko/install.**
Re-run it - `nixos-anywhere --phases disko,install` is safe to repeat against
a booted installer. If it fails consistently on the same step, boot the
installer again fresh (a half-partitioned disk can confuse a second disko
pass) before retrying.

**SSH from the workstation hangs or is refused.**
Confirm `ip -brief a` on the installer shows an address reachable from your
workstation (not just link-local), and that `sudo systemctl start sshd` was
run *after* the installer environment finished booting, not during POST.

**A discovered device never gets an IP / the job stalls at "imaging".**
The runner resolves the target's IP from its DHCP lease; if the station's PXE
network hands out leases slowly or the device NICs are ambiguous (multiple
discovered rows with a similar MAC prefix), remove the stale discovered row
and re-PXE-boot the device to get a fresh lease and report.

**A job fails.**
The jobs table and the wizard both surface the failure message inline. Most
failures are hardware-profile mismatches (wrong disk device assumed by disko)
or a target that lost network mid-install - cancel the job, fix the profile
or cabling, and dispatch it again; a failed job does not block the rest of
the batch.

**The Secure Boot step never completes.**
Check the manual firmware action shown in the wizard was actually completed
in the BIOS (Enabled + Reset to Setup Mode -> Save & Exit) before rebooting; a
reboot without that toggle just returns the device to the same phase. The key
enrolment itself is automatic once the firmware is in setup mode.

**The device converges but its posture still shows Secure Boot/TPM2 as not
enforcing.**
Give it one more check-in cycle - posture is self-reported by the device on
check-in, so it lags the wizard's own phase tracking by up to one interval.
