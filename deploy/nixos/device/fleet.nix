# fleet.nix - what makes a DAWO-Core machine a device of a Sextant fleet:
# comin follows the fleet repository on the branch the generator chose, and
# the agent reports to the console as the device's tag. When the repository
# is the one the console serves, comin pulls it with the agent's credential.
#
# The generator sets sextant.deviceTag and sextant.cominBranch per device
# (nix/generator.nix); this module turns them into the core's own knobs.
# Every value here is mkDefault, so an overlay can still pin a host by hand.
{ config, lib, ... }:
let
  cfg = config.sextant;
  repoUrl = config.dawo.autoUpdate.options.repoUrl;
  credentialFile = config.services.sextant-agent.credentialFile;

  # How comin authenticates its poll. A token from agenix when the overlay
  # names one; the device's own tag and credential when the repository is
  # the console's (design 0014: the console serves it, behind the credential
  # the agent already holds); nothing for a public repository elsewhere.
  fromConsole = cfg.console.url != "" && lib.hasPrefix "${cfg.console.url}/" repoUrl;
  auth =
    if cfg.console.overlayTokenSecret != "" then {
      access_token_path = "/run/agenix/${cfg.console.overlayTokenSecret}";
      username = cfg.console.overlayTokenUser;
    }
    else if fromConsole then {
      access_token_path = credentialFile;
      username = cfg.deviceTag;
    }
    else null;

  # The first remote, then each mirror in order. comin deploys only a commit
  # on top of what the device already runs, and refuses one that rewrote the
  # branch, so a mirror that lags is ignored and one that was tampered with
  # is refused; while the first forge is down, the next one still delivers.
  remote = name: url: auth': {
    inherit name url;
    branches.main.name = config.dawo.autoUpdate.options.branch;
    poller.period = config.dawo.autoUpdate.options.pollSeconds;
  } // lib.optionalAttrs (auth' != null) { auth = auth'; };
  mirrorRemotes = lib.imap1
    (i: m: remote "mirror-${toString i}" m.url
      (if m.tokenSecret == "" then null else {
        access_token_path = "/run/agenix/${m.tokenSecret}";
        username = m.tokenUser;
      }))
    cfg.console.overlayMirrors;
in
{
  options.sextant = {
    console = {
      url = lib.mkOption {
        type = lib.types.str;
        default = "";
        example = "https://console.example.org";
        description = "The console's base URL: where the agent checks in.";
      };
      overlayRepoUrl = lib.mkOption {
        type = lib.types.str;
        default = "";
        example = "https://codeberg.org/acme/fleet.git";
        description = "The fleet repository devices follow with comin.";
      };
      # A private fleet repository: comin authenticates its poll with a
      # read-only token from an agenix secret. Empty means a public
      # repository, no auth.
      overlayTokenSecret = lib.mkOption {
        type = lib.types.str;
        default = "";
        description = "Secret-ref name (agenix) of a file holding only a read-only token for the fleet repository; empty for a public one.";
      };
      overlayTokenUser = lib.mkOption {
        type = lib.types.str;
        default = "comin";
        description = "Username sent with the token. Forgejo wants the account name; GitHub and GitLab accept anything non-empty.";
      };
      # Further copies of the fleet repository (#16). A device follows one
      # remote until one of these is named; losing that forge then left every
      # device unable to change until a forge existed again at the same URL.
      overlayMirrors = lib.mkOption {
        type = lib.types.listOf (lib.types.submodule {
          options = {
            url = lib.mkOption {
              type = lib.types.str;
              example = "https://git.example.net/acme/fleet.git";
              description = "A copy of the fleet repository, kept in step by the forge (a push mirror) or by whoever pushes.";
            };
            tokenSecret = lib.mkOption {
              type = lib.types.str;
              default = "";
              description = "Secret-ref name (agenix) of a read-only token for this copy; empty for a public one.";
            };
            tokenUser = lib.mkOption {
              type = lib.types.str;
              default = "comin";
              description = "Username sent with the token.";
            };
          };
        });
        default = [ ];
        description = "Copies of the fleet repository comin polls after the first, in order, so a device keeps updating while one forge is down. A copy that lags is ignored; one whose branch was rewritten is refused.";
      };
    };

    # A release cache (build-before-promote): the pipeline builds a ring's
    # release once and publishes it, signed, so devices substitute it rather
    # than each compiling it. The key pins integrity; an unreachable cache
    # only means a local build. Not under dawo.*, so not a console setting.
    releaseCache = {
      url = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        example = "https://cache.example.org/cache";
        description = "Substituter URL of the fleet's release cache.";
      };
      publicKey = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        description = "The cache's signing key, name:base64.";
      };
    };
  };

  config = {
    assertions = [
      {
        assertion = cfg.console.url != "";
        message = "sextant.console.url is not set: the agent has no console to report to.";
      }
      {
        assertion = cfg.console.overlayRepoUrl != "";
        message = "sextant.console.overlayRepoUrl is not set: comin has no fleet repository to follow.";
      }
      {
        assertion = (cfg.releaseCache.url == null) == (cfg.releaseCache.publicKey == null);
        message = "sextant.releaseCache.url and publicKey go together: a cache without its key is never trusted, a key without a cache is unused.";
      }
    ];

    # The update funnel (ADR 0011): comin follows the branch the generator
    # chose - rings/<group> for a ring device, main for the rest.
    dawo.autoUpdate.enable = lib.mkDefault true;
    dawo.autoUpdate.options.repoUrl = lib.mkDefault cfg.console.overlayRepoUrl;
    dawo.autoUpdate.options.branch = lib.mkDefault cfg.cominBranch;

    nix.settings = lib.mkIf (cfg.releaseCache.url != null) {
      extra-substituters = [ cfg.releaseCache.url ];
      extra-trusted-public-keys = [ cfg.releaseCache.publicKey ];
    };

    # Observe and report only. The per-device credential is written once at
    # provisioning and never enters the store.
    services.sextant-agent = {
      enable = lib.mkDefault true;
      url = lib.mkDefault cfg.console.url;
      tag = lib.mkDefault cfg.deviceTag;
    };

    # An authenticated pull: comin's own remotes[].auth. The core's module
    # has no auth option, so the remote is declared here in full; the fields
    # mirror DAWO-Core's modules/services/auto-update.nix.
    # Mirrors make the list longer than the core's one entry, so they too
    # need it declared here.
    services.comin.remotes = lib.mkIf (auth != null || mirrorRemotes != [ ]) (lib.mkForce
      ([ (remote "dawo-image" repoUrl auth) ] ++ mirrorRemotes));

    # comin reads its token once, when it starts. A credential the console
    # re-issues, or one written after comin started (a device that is still
    # being provisioned), only counts after a restart; without one comin
    # keeps offering the old credential and the device quietly stops
    # updating. PathChanged alone, as in cache-auth.nix: with PathExists the
    # unit would fire for as long as the file exists.
    systemd.paths.sextant-comin-credential = lib.mkIf fromConsole {
      wantedBy = [ "multi-user.target" ];
      pathConfig.PathChanged = credentialFile;
    };
    systemd.services.sextant-comin-credential = lib.mkIf fromConsole {
      description = "Restart comin so it pulls with the current device credential";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${config.systemd.package}/bin/systemctl try-restart comin.service";
      };
    };
  };
}
