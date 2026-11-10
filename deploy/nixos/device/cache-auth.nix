# Authenticate this device to the release binary cache.
#
# The cache is served on a public host. Every path in it is signed and devices
# pin the org public key, so INTEGRITY was never the gap - but a signature
# stops somebody serving you a tampered closure, it does nothing about them
# reading yours, and the two are easy to conflate. What reading it yields is
# one infrastructure hostname and a package manifest: which versions of what
# this fleet runs (Sextant threat model, control 8).
#
# nix authenticates to a substituter from a netrc file and speaks nothing else,
# so that is what this writes.
#
# WHAT IT SENDS. The per-device credential the agent already holds, written
# once at provisioning and never in git or the store. Deliberately not a new
# shared secret in agenix: that would need a working recipient set on every
# device before the server could require it, and a shared token cannot be
# revoked for one machine. This can - retiring a device takes its cache access
# with it, because the console stops recognising the credential.
#
# A device that cannot authenticate does not fail loudly - it falls back to
# building the closure itself, which is hours per machine and presents as a
# slow rollout rather than an access problem. That is the whole reason the
# ordering matters and the reason this file says so twice.
{ config, lib, pkgs, ... }:
let
  cfg = config.dawo.cacheAuth;
  provisioned = cfg.enable;

  # One script, two triggers. Activation covers boot and every rebuild; the
  # path unit below covers a credential that CHANGES while the system stays up.
  # Sharing the script is the point: when the two drifted apart the netrc kept
  # a rotated-away credential and nix silently stopped using the cache.
  writeNetrc = pkgs.writeShellScript "sextant-cache-netrc" ''
    set -eu
    umask 077
    install -d -m 0700 /run/sextant
    # No credential yet (a device mid-provisioning) means no netrc rather
    # than an empty one: nix reading a netrc with a blank password sends
    # blank Basic auth, which the server rejects, and a rejection reads as
    # a wrong credential rather than as an absent one.
    if [ -s ${cfg.credentialFile} ]; then
      printf 'machine %s login device password %s\n' \
        '${cfg.host}' "$(tr -d '\n' < ${cfg.credentialFile})" \
        > /run/sextant/netrc.tmp
      mv -f /run/sextant/netrc.tmp /run/sextant/netrc
    else
      rm -f /run/sextant/netrc
    fi
  '';
in
{
  options.dawo.cacheAuth = {
    enable = lib.mkEnableOption "authenticate to the release binary cache" // {
      label = "Cache authentication";
    };

    host = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "cache.example.org";
      description = ''
        Hostname of the release cache. Must match the substituter's host
        exactly: netrc is matched by machine name, and a mismatch is silent -
        nix simply sends no credential and the device starts compiling.
      '';
    } // { label = "Cache host"; };

    # The path the agent owns, not a policy choice. Deliberately UNDOCUMENTED
    # and internal: a documented option under dawo.* becomes an operator-
    # editable console setting (export-catalog.nix), and "which file holds the
    # device credential" must never be one - pointing it elsewhere silently
    # breaks cache authentication, and the failure is quiet by design (a device
    # that cannot authenticate falls back to building its own closure).
    #
    # It was worse than merely unnecessary: elevation-request.nix declared the
    # SAME path as its own setting, so the console offered two knobs for one
    # file that must agree, with nothing saying so when they stopped agreeing
    # (ADR 0019, 2026-08-06). Both now read this one option.
    #
    # The username is not configurable because the server ignores it - netrc
    # demands one, and honouring it would imply a second per-device secret.
    credentialFile = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/sextant-agent/credential";
      internal = true;
    };
  };

  config = lib.mkIf provisioned {
    assertions = [{
      assertion = cfg.host != "";
      message = "dawo.cacheAuth.host must name the release cache (the substituter's host).";
    }];

    # Built at activation rather than in the store: the token is the content,
    # and a netrc assembled at build time would put it in a world-readable
    # store path - which is the exact disclosure this module exists to close.
    system.activationScripts.sextantCacheNetrc.text = "${writeNetrc}";

    # A credential can be re-issued from the console at any time, and the
    # device picks the new one up without a rebuild: the agent re-reads its
    # file every run. The netrc did not, so after a rotation nix kept
    # presenting the dead credential - measured on e2e5, 2026-08-04. Nothing
    # failed loudly; the substituter rejected it and nix built locally
    # instead, hours per machine, looking like a slow rollout.
    #
    # PathChanged only, and NOT PathExists. Both together is a busy loop:
    # PathExists holds while the file is there, so the moment the oneshot
    # finishes systemd starts it again, and again, until the start limit trips
    # and BOTH units land in failed. Measured on e2e5, 2026-08-05 - the service
    # log reads "Finished ..." immediately followed by "Start request repeated
    # too quickly", which is a success and a failure one second apart.
    #
    # PathChanged is enough on its own: it fires on close-after-write, and a
    # file being created counts. The device-provisioned-before-the-credential
    # case is covered by the activation script, which runs the same script on
    # every activation and at boot.
    systemd.paths.sextant-cache-netrc = {
      wantedBy = [ "multi-user.target" ];
      pathConfig.PathChanged = cfg.credentialFile;
    };

    systemd.services.sextant-cache-netrc = {
      description = "Rewrite the binary-cache netrc from the device credential";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${writeNetrc}";
      };
    };

    # nix reads this as root, which is who substitutes.
    nix.settings.netrc-file = "/run/sextant/netrc";
  };
}
