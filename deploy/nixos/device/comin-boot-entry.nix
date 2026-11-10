# comin-boot-entry.nix - give every comin switch a boot entry under Lanzaboote.
#
# THE GAP. comin records what it deploys in its OWN profile,
# /nix/var/nix/profiles/system-profiles/comin, and leaves the system profile
# alone. systemd-boot's installer reads system-profiles/* as well, so there
# this works. Lanzaboote's installer is handed /nix/var/nix/profiles/system-*-link
# and nothing else: a generation that comin switched to never becomes a boot
# entry.
#
# Measured 2026-11-06 on dawo-inspoelstraat: running generation from that
# afternoon (nixpkgs 20260809, Linux 6.18.43), boot default "Generation 39,
# 2026-07-28" (nixpkgs 20260611, Linux 6.18.35). Every reboot since July had
# brought back the July system, and comin then switched forward again - when
# it could. On that box the July system had no token for this private
# overlay, so after a reboot it would have stayed in July: no build runner,
# no gate tools, all CI down. The kernel never moved at all; a switch does
# not replace the running kernel, only a boot does.
#
# It also aims boot-health.nix at the wrong target: its rollback is
# `nix-env --profile .../system --rollback`, the SYSTEM profile, which on a
# comin device is months behind. A bad release would roll back to July, not
# to the release before it.
#
# THE FIX. After each deployment comin runs this hook. When comin's profile is
# the running system (a successful `switch`), the system profile is set to the
# same generation and the bootloader is installed, exactly what
# `nixos-rebuild switch` does. Nothing happens when:
#   - the deployment failed;
#   - comin's profile is not the running system: a `test` deployment (the
#     testing branch, which must NOT survive a reboot) leaves comin's profile
#     where it was, and a `boot` deployment is not running yet;
#   - the boot entry is already there.
# It runs after the switch has finished, so it never nests inside
# switch-to-configuration and its lock.
#
# FIRST ROLLOUT. comin reads its configuration once, at start; the comin that
# deploys THIS change runs without the hook, and comin-config-guard.nix
# restarts it afterwards. So the generation that brings the hook gets no boot
# entry from it; from the next deployment on, every switch does. The same
# script is on PATH as `comin-boot-entry` for a manual catch-up (as root).
{ config, lib, pkgs, ... }:
let
  hook = pkgs.writeShellApplication {
    name = "comin-boot-entry";
    runtimeInputs = [ pkgs.coreutils pkgs.util-linux config.nix.package ];
    text = ''
      # Never let the journal decide: a logger that fails must not abort the
      # hook halfway (errexit is on).
      log() { logger -t comin-boot-entry -- "$*" || true; echo "comin-boot-entry: $*"; }

      status="''${COMIN_STATUS:-manual}"
      sha="''${COMIN_GIT_SHA:-unknown}"
      system="$(readlink -e /nix/var/nix/profiles/system)"

      if [ "$status" = failed ]; then
        log "deployment $sha failed; boot entry stays at $system"
        exit 0
      fi

      comin="$(readlink -e /nix/var/nix/profiles/system-profiles/comin || true)"
      current="$(readlink -e /run/current-system)"
      if [ -z "$comin" ] || [ "$comin" != "$current" ]; then
        log "comin's profile is not the running system (test or boot deployment); boot entry stays at $system"
        exit 0
      fi
      if [ "$comin" = "$system" ]; then
        log "boot entry already at $system"
        exit 0
      fi

      nix-env --profile /nix/var/nix/profiles/system --set "$comin"
      "$comin/bin/switch-to-configuration" boot
      log "boot entry moved from $system to $comin (deployment $sha, status $status)"
    '';
  };
in
{
  config = lib.mkIf (config.services.comin.enable && (config.boot.lanzaboote.enable or false)) {
    services.comin.postDeploymentCommand = lib.getExe hook;
    environment.systemPackages = [ hook ];
  };
}
