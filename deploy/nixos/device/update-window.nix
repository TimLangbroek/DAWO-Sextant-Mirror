# Maintenance window for fleet updates (design delivery-process §7.5).
# One setting per scope: dawo.updates.maintenanceWindow = "HH:MM-HH:MM",
# the hours during which comin may fetch and switch. Empty (the default)
# means always. Overnight windows ("18:00-07:00") work.
#
# Implementation is deliberately blunt and self-healing: a timer wakes
# every 10 minutes and reconciles - inside the window it resumes comin,
# outside it suspends (comin's own graceful suspend: a running deploy
# finishes, no new one starts). No calendars to keep in sync, no state.
{ config, lib, pkgs, ... }:
let
  cfg = config.dawo.updates;

  reconcile = pkgs.writeShellApplication {
    name = "dawo-update-window";
    runtimeInputs = [ pkgs.coreutils config.services.comin.package ];
    text = ''
      window="${cfg.maintenanceWindow}"
      [ -n "$window" ] || exit 0

      start="''${window%-*}"; end="''${window#*-}"
      now="$(date +%H:%M)"

      inside=0
      if [ "$start" \< "$end" ]; then
        # Same-day window, e.g. 12:00-17:00.
        if [ ! "$now" \< "$start" ] && [ "$now" \< "$end" ]; then inside=1; fi
      else
        # Overnight window, e.g. 18:00-07:00.
        if [ ! "$now" \< "$start" ] || [ "$now" \< "$end" ]; then inside=1; fi
      fi

      if [ "$inside" -eq 1 ]; then
        comin resume >/dev/null 2>&1 || true
      else
        comin suspend >/dev/null 2>&1 || true
      fi
    '';
  };
in
{
  options.dawo.updates = {
    maintenanceWindow = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "18:00-07:00";
      description = ''
        Hours during which this device may fetch and apply updates
        (HH:MM-HH:MM, overnight ranges allowed). Empty means always.
      '';
    };
  };

  config = lib.mkIf (cfg.maintenanceWindow != "") {
    systemd.services.dawo-update-window = {
      description = "DAWO update maintenance-window reconciler (comin suspend/resume)";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = lib.getExe reconcile;
      };
    };
    systemd.timers.dawo-update-window = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "2min";
        OnUnitActiveSec = "10min";
      };
    };
  };
}
