# 0014 - A starter fleet that works

Status: accepted, 2026-11-09 (private-repository pull decided by Bram, 2026-11-10)

## Problem

Since the chart requires a fleet repository, an install can start from an
empty repository on any forge: `sextant prepare-repo` fills it from the
starter embedded in the binary (`internal/starter/overlay`). That starter is
one file, `fleet.json`, and it does not make a working fleet:

- **No flake.** The validation gate evaluates the overlay's
  `nixosConfigurations`; there are none, so every change either fails the
  gate or, with `--gate none`, reaches devices unproven.
- **No catalog.** The settings page and the integrations cards edit only the
  keys in `catalog.json`. With none, a save is a silent no-op (measured in the
  NixOS VM test on 2026-11-09: the POST answers 303 and nothing changes).
- **Wrong vocabulary.** `"desktop": "plasma"` comes from the example
  overlay's demo core. DAWO-Core has `dawo.desktop.plasma.enable` and asserts
  exactly one desktop, so the first real evaluation would fail.
- **Nothing for devices to follow.** The repository address and branch a
  device pulls from are written into each overlay's Nix code by hand
  (`sextant.console.overlayRepoUrl` in the BB Open overlay).

A newcomer who follows the install page ends with a console that looks
ready and can change nothing. That is the C1 stream of v1 ("server in 15
minutes") failing at its last step.

## Decision

The starter is a complete overlay, the same files for every organisation,
generated and checked in Sextant's CI and embedded in the binary:

| File | Made by | Holds |
|---|---|---|
| `flake.nix` | hand-written, in `internal/starter/overlay` | DAWO-Core and Sextant as inputs, `mkFleet`, the catalog export |
| `flake.lock` | `scripts/regen-starter.sh` | the DAWO-Core and Sextant revisions the catalog was made from |
| `catalog.json` | `scripts/regen-starter.sh` | the settings vocabulary of exactly that lock |
| `hardware-profiles.json` | hand-written | `generic-intel`, `generic-amd`, `vm` |
| `fleet.json` | hand-written | one group, a desktop, a poll interval, and the `console` block below |

**Nothing per organisation in Nix.** Everything that differs between
organisations is data in `fleet.json`, so the flake, lock and catalog are
identical everywhere and can be made once, in CI. The new top-level block:

```json
"console": { "url": "https://console.example.org", "repo": "https://…/fleet.git", "branch": "main" }
```

`prepare-repo` writes `repo` and `branch` when it fills a repository; the
console writes `url` from its public address. The generator hands the block
to the device module with `mkDefault`, and ring branches derive from
`branch` instead of a hard-coded `main`.

**One device module.** The plumbing every overlay repeats - comin wired to
the fleet repository, the agent, the integrations the console's cards
configure, boot health, the update window, elevation requests, the comin
config guard - becomes `sextant.nixosModules.device`. The BB Open overlay
has them as ten files with its own console address written in; they move
here without it. Organisation-specific modules (SSH keys, branding,
stations) stay in the organisation's overlay.

**The lock is committed, never made at evaluation time.** The gate-runner
evaluates in pure mode or in a throwaway worktree; neither can lock safely,
and devices locking on their own would each pin a different core. The lock
can only pin a Sextant revision older than the commit that holds it, so CI
also checks that nothing a device builds (`nix/`, `deploy/nixos/`, `agent/`)
changed since the pinned revision.

**The catalog has labels for every option the core declares.** DAWO-Core
main has about twenty documented options newer than the BB Open pin, all
unlabelled, and the export refuses unlabelled options by design (ADR 0019).
Their labels go in `nix/catalog-presentation.json`; options a person should
never set (deploy-key paths) go in `managedPrefixes`.

## How a device reads a private fleet repository

A device pulls the fleet repository with comin. For a public repository that
needs nothing. For a private one it needs a credential on the device, and
there is no zero-touch way to put one there: BB Open uses an agenix-encrypted
forge token, which needs a key on the device first, and a forge's deploy key
is per forge and per repository.

Options:

1. **Public fleet repositories only, for v1.** Nothing to build. The
   repository holds settings and device names, not secrets (those are
   agenix-encrypted). Some organisations will refuse.
2. **The console serves the fleet repository to enrolled devices**, read
   only, over git's smart HTTP, behind the device credential the agent
   already has. Works with every forge and with private repositories, and
   adds no token to provision. Devices still pull (ADR 0023); the console
   becomes the address they pull from.
3. **A pull token per device from the forge.** Forge-specific, one API per
   forge, and a second credential to rotate.

**Decided: 2.** It is the only one that keeps "an empty repository and its
address" as the whole setup for a private fleet. The `console` block's
`url` is then also the address devices pull from, and a public repository
works the same way, so there is one path rather than two.

## Not doing

- A choice of core in the starter. DAWO-Core is the core; an organisation
  with its own replaces the input, which is one line.
- Regenerating the catalog in the console. The console has no nix. The
  gate-runner's core bump, which today commits `flake.lock` alone and so
  leaves the catalog describing the old core, regenerates `catalog.json`
  in the same commit.

## Done when

A NixOS VM test starts a console against an empty repository and, without
touching Nix, saves a setting that passes the gate and lands on a device
VM that pulls the repository.
