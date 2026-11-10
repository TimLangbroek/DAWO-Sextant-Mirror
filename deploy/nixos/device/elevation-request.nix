# "Ask an administrator" that actually reaches one.
#
# polkit answers a refused action with a dialog asking for an ADMINISTRATOR's
# password. On a fleet machine that means the local admin account, so away from
# the office it is not a slower path - it is no path, and the predictable
# result is that the admin password gets shared until it stops being a secret.
#
# This adds the missing route: the request appears in the console, an operator
# approves it, and the dialog completes without anybody typing an admin
# password.
#
# WHERE IT HOOKS IN, AND WHY THERE. polkit will not let an agent vouch for an
# identity - the response travels through the setuid polkit-agent-helper-1,
# which runs PAM. The PAM stack for the `polkit-1` service is therefore the
# only honest place to intercept, and it is what this module extends.
#
# WHAT MAKES IT SAFE. It ADDS one rule and replaces nothing, and the rule is
# `sufficient`: it can succeed an authentication, never refuse one. Denied,
# expired, console unreachable, helper crashed - every failure falls through to
# the password prompt exactly as before. The worst this module can do when it
# breaks is make the dialog behave the way it did before it existed.
#
# Ordering matters and is measured, not guessed: pam_unix sits at order 11700
# in the rendered polkit-1 stack, so 11500 puts this ahead of it.
{ config, lib, pkgs, ... }:
let
  cfg = config.dawo.elevationRequests;

  helper = pkgs.writeShellApplication {
    name = "sextant-elevation-request";
    runtimeInputs = with pkgs; [ curl jq systemd coreutils gawk ];
    text = ''
      # Invoked by pam_exec inside the polkit-1 stack. Exit 0 authenticates.
      #
      # PAM_USER here is the identity being authenticated - an ADMINISTRATOR
      # account, not the person asking. PAM is told neither who triggered the
      # dialog nor which polkit action it was for. So the request reports the
      # user on the active local seat, and the console treats that as context
      # while judging on the device identity, which IS authenticated.
      set -euo pipefail

      url="${cfg.consoleUrl}"
      tag="${cfg.tag}"
      cred_file="${cfg.credentialFile}"
      [ -r "$cred_file" ] || exit 1
      cred="$(tr -d '\n' < "$cred_file")"
      [ -n "$cred" ] || exit 1

      # Who is at the machine. Falls back to the PAM user rather than failing:
      # a request naming the wrong person is still answerable, a request that
      # never appears is not.
      seat_user="$(loginctl list-sessions --no-legend 2>/dev/null \
        | awk '/seat0/ {print $3; exit}')" || seat_user=""
      : "''${seat_user:=''${PAM_USER:-unknown}}"

      # No reason field. It used to carry a FIXED note configured per scope,
      # because polkit's dialog has a password field and nothing else, so the
      # user cannot type one. But a constant string is not a reason - the
      # console already knows the device, its group and the user, which is
      # everything that note repeated (removed 2026-08-06). Letting the
      # REQUESTER type one is a real feature and needs a device-side UI; this
      # PAM hook is headless and cannot ask.
      body="$(jq -nc --arg u "$seat_user" --arg a "''${PAM_SERVICE:-polkit-1}" \
        '{user:$u, action:$a}')"

      id="$(curl -fsS --max-time 10 \
        -H "Authorization: Bearer $cred" -H 'Content-Type: application/json' \
        --data "$body" "$url/api/device/$tag/elevation" \
        | jq -r '.id // empty')" || exit 1
      [ -n "$id" ] || exit 1

      # Poll until the console answers or the window closes. The deadline is
      # ours as well as the server's: if the console became unreachable
      # mid-wait we must give up rather than hold the dialog open forever. A
      # frozen dialog is worse than a refusal - the user cannot even fall back
      # to typing a password.
      deadline=$(( SECONDS + ${toString cfg.waitSeconds} ))
      while [ "$SECONDS" -lt "$deadline" ]; do
        answer="$(curl -fsS --max-time 5 -H "Authorization: Bearer $cred" \
          "$url/api/device/$tag/elevation/$id" || true)"
        # `granted` is the server's own verdict, not something derived here.
        # An approval that arrived after the window closed reads as state
        # "approved" and granted false, and a client working that out for
        # itself would eventually get it wrong in the direction of granting.
        if [ "$(printf '%s' "$answer" | jq -r '.granted // false')" = "true" ]; then
          exit 0
        fi
        case "$(printf '%s' "$answer" | jq -r '.state // ""')" in
          denied|expired) exit 1 ;;
        esac
        sleep 2
      done
      exit 1
    '';
  };
in
{
  options.dawo.elevationRequests = {
    enable = lib.mkEnableOption "let a user ask an operator for permission instead of needing the admin password" // {
      label = "Elevation requests";
    };

    consoleUrl = lib.mkOption {
      type = lib.types.str;
      default = config.sextant.console.url or "";
      description = "Console base URL the request is sent to.";
    } // { label = "Console URL"; };

    # The device's OWN identity, taken from the fleet - never an operator
    # setting. Two reasons it had to go (ADR 0019, 2026-08-06). It is not
    # policy: an operator typing another device's tag would have this machine
    # claim to be that one. And its default is host-dependent, so the catalog
    # export - which evaluates on a host that does not exist - published
    # "catalog-export" to operators as this option's default value.
    tag = lib.mkOption {
      type = lib.types.str;
      default = config.sextant.deviceTag or "";
      internal = true;
    };

    # The per-device credential the agent already holds, reused rather than
    # issued separately: a second credential for the same device would be a
    # second thing to rotate, revoke and forget about.
    #
    # Internal, and it DEFAULTS TO THE CACHE-AUTH OPTION rather than repeating
    # the literal path. Both used to declare the same constant as an operator
    # setting - two knobs for one file that must agree, and nothing to say so
    # when they stopped agreeing.
    credentialFile = lib.mkOption {
      type = lib.types.str;
      default = config.dawo.cacheAuth.credentialFile;
      internal = true;
    };

    waitSeconds = lib.mkOption {
      type = lib.types.ints.positive;
      default = 300;
      description = ''
        How long the dialog waits for an answer. Matches the console's own
        window; a device waiting longer would be sitting on a request the
        server has already declared dead.
      '';
    } // { label = "Wait (seconds)"; };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.consoleUrl != "" && cfg.tag != "";
        message = ''
          dawo.elevationRequests needs a console URL and a device tag. Without
          them the helper reaches nobody and every elevation falls through to
          the admin password, with nothing on screen saying this was ever on.
        '';
      }
    ];

    environment.systemPackages = [ helper ];

    security.pam.services."polkit-1".rules.auth.sextantElevation = {
      order = 11500; # ahead of pam_unix (11700) and pam_sss (12100)
      control = "sufficient";
      modulePath = "${pkgs.pam}/lib/security/pam_exec.so";
      args = [ "quiet" "${helper}/bin/sextant-elevation-request" ];
    };
  };
}
