# comin-config-guard.nix - make a changed comin config actually take effect.
#
# WHY THIS EXISTS. comin's own unit carries `X-RestartIfChanged=false`, which
# tells switch-to-configuration never to restart it when its configuration
# changes. That is right in isolation: comin is the process performing the
# switch, so restarting it mid-activation would kill the deployment it is
# running and report a failure for a generation that actually applied.
#
# The consequence, found on test15 (2026-07-30): comin reads its configuration
# ONCE, at startup, and keeps it. So the branch a device follows is frozen at
# whatever it was when comin last started - and on a device imaged before the
# install path was fixed, that is `main`, not `rings/<group>`.
#
# The device therefore took every commit that landed on main: no ring, no wave,
# no soak gate, while the console showed it as a managed member of its ring. It
# ran ahead of its own ring, and comin then refused to come back down
# (hasNotBeenHardReset rejects a head that is not a descendant of what it
# runs), so it was wedged where no rollout could reach it. Everything about the
# generated configuration was correct the whole time; it simply was not in
# effect. That is the same failure shape as the agenix host key and the NSS
# cache earlier in this fleet's history, and the reason it hurts is always the
# same: nothing said so.
#
# A ring reassignment is a rare edit, which is exactly why this must not depend
# on someone remembering: the day it matters is the day nobody is looking.
{ config, lib, pkgs, ... }:
let
  # The config path the CURRENT system tells comin to use. Read out of the
  # generated unit rather than reconstructed, so it cannot drift from what
  # comin is actually started with.
  cominUnit = "/run/current-system/etc/systemd/system/comin.service";
in
{
  config = lib.mkIf config.services.comin.enable {
    systemd.services.dawo-comin-config-guard = {
      description = "Restart comin when its configuration no longer matches the system";
      after = [ "comin.service" ];
      wants = [ "comin.service" ];
      path = with pkgs; [ systemd gnugrep gnused coreutils procps ];
      serviceConfig = {
        Type = "oneshot";
        # Reads /proc/<pid>/cmdline of another process and restarts a unit.
        User = "root";
      };
      script = ''
        set -eu
        want="$(grep -oE -- '--config [^ ]+' ${cominUnit} | head -n1 | cut -d' ' -f2 || true)"
        if [ -z "$want" ]; then
          echo "guard: the current system's comin unit names no --config; nothing to compare"
          exit 0
        fi
        pid="$(systemctl show comin -p MainPID --value 2>/dev/null || true)"
        if [ -z "$pid" ] || [ "$pid" = "0" ] || [ ! -r "/proc/$pid/cmdline" ]; then
          echo "guard: comin is not running; leaving it to its own unit"
          exit 0
        fi
        # cmdline is NUL-separated; turn it into lines so the path compares
        # exactly rather than by substring (one config path must not match
        # another that merely contains it).
        have="$(tr '\0' '\n' < "/proc/$pid/cmdline" | grep -Fx -- "$want" || true)"
        if [ -n "$have" ]; then
          exit 0
        fi
        echo "guard: comin is running with a stale configuration; restarting so it follows the branch this generation assigns"
        # A restart at worst aborts an in-flight deployment, which comin
        # retries on its next poll. Following the wrong branch does not
        # self-heal, so the trade is not close.
        systemctl restart comin
      '';
    };

    # Run shortly after boot and then on a slow cadence. Not a restartTrigger on
    # the config: switch-to-configuration honours those through the same
    # mechanism X-RestartIfChanged=false suppresses, so a trigger would be
    # ignored - which is the whole reason this guard is a separate unit.
    systemd.timers.dawo-comin-config-guard = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "3min";
        OnUnitActiveSec = "1h";
        AccuracySec = "1min";
        Persistent = true;
      };
    };
  };
}
