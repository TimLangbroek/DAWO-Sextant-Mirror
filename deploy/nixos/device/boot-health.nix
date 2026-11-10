# Boot-health watchdog with automatic rollback (greenboot-style, design
# delivery-process §7.8). Always on, no options - this is the per-device
# safety net UNDER the rollout rings: a device that switches to a release
# and turns out unhealthy rolls itself back to the previous NixOS
# generation and keeps reporting the OLD revision, which the ring's health
# gate then sees as non-convergence and halts the wave.
#
# Mechanics: every activation (a comin switch, or booting into a new
# generation) arms a pending marker; a periodic check a few minutes later
# either clears it (healthy) or rolls back. The rollback does not fight
# comin: comin only acts on NEW commits, so a rolled-back device stays
# rolled back until a fixed release lands.
#
# Health means, deliberately minimal (dead simple, no config):
#   - default.target is active (the boot/switch completed),
#   - there is a default route (the device can reach anything at all),
#   - the sextant agent runs (the device can tell the console its story).
# A failed printer daemon must NOT roll a device back; unreachable-and-mute
# must.
{ config, lib, pkgs, ... }:
let
  healthCheck = pkgs.writeShellApplication {
    name = "dawo-boot-health";
    runtimeInputs = [ pkgs.systemd pkgs.iproute2 pkgs.coreutils pkgs.nix pkgs.gnugrep ];
    text = ''
      pending=/var/lib/dawo-health/pending
      [ -e "$pending" ] || exit 0

      fail=""
      systemctl is-active --quiet default.target || fail="default.target not active"
      if [ -z "$fail" ]; then
        ip route show default | grep -q . || fail="no default route"
      fi
      if [ -z "$fail" ]; then
        state="$(systemctl is-active sextant-agent.service 2>/dev/null || true)"
        case "$state" in
          active|activating) ;;
          *) fail="sextant-agent is $state" ;;
        esac
      fi

      if [ -z "$fail" ]; then
        rm -f "$pending"
        echo "dawo-boot-health: healthy - generation confirmed"
        exit 0
      fi

      echo "dawo-boot-health: UNHEALTHY ($fail) - rolling back to the previous generation" >&2
      # Disarm first: the rollback switch re-runs activation, which re-arms
      # the marker and re-checks the OLD generation on the next tick.
      rm -f "$pending"
      cur="$(readlink -f /run/current-system)"
      if nix-env --profile /nix/var/nix/profiles/system --rollback; then
        /nix/var/nix/profiles/system/bin/switch-to-configuration switch || true
        echo "dawo-boot-health: rolled back from $cur" >&2
      else
        # First generation ever: nothing to roll back to. Log and stand down;
        # the operator sees an unhealthy device, not a crash loop.
        echo "dawo-boot-health: no previous generation to roll back to" >&2
      fi
    '';
  };
in
{
  systemd.tmpfiles.rules = [ "d /var/lib/dawo-health 0700 root root -" ];

  # Arm on every activation: a switch into a new generation must prove
  # itself. (A plain reboot into the same, already-confirmed generation
  # finds no marker and checks nothing.)
  system.activationScripts.dawoBootHealthArm.text = ''
    mkdir -p /var/lib/dawo-health
    touch /var/lib/dawo-health/pending
  '';

  systemd.services.dawo-boot-health = {
    description = "DAWO boot-health check with automatic generation rollback";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = lib.getExe healthCheck;
    };
  };

  # First check ~3 minutes after boot/arming, then every 3 minutes while a
  # marker is pending (the service exits immediately otherwise).
  systemd.timers.dawo-boot-health = {
    wantedBy = [ "timers.target" ];
    timerConfig = {
      OnBootSec = "3min";
      OnUnitActiveSec = "3min";
    };
  };
}
