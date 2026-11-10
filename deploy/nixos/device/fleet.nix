# fleet.nix - what makes a DAWO-Core machine a device of a Sextant fleet:
# comin follows the fleet repository on the branch the generator chose, and
# the agent reports to the console as the device's tag.
#
# The generator sets sextant.deviceTag and sextant.cominBranch per device
# (nix/generator.nix); this module turns them into the core's own knobs.
# Every value here is mkDefault, so an overlay can still pin a host by hand.
{ config, lib, ... }:
let
  cfg = config.sextant;
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

    # A private repository: comin's own remotes[].auth. The core's module has
    # no auth option, so the remote is declared here in full; the fields
    # mirror DAWO-Core's modules/services/auto-update.nix.
    services.comin.remotes = lib.mkIf (cfg.console.overlayTokenSecret != "") (lib.mkForce [
      {
        name = "dawo-image";
        url = config.dawo.autoUpdate.options.repoUrl;
        branches.main.name = config.dawo.autoUpdate.options.branch;
        poller.period = config.dawo.autoUpdate.options.pollSeconds;
        auth = {
          access_token_path = "/run/agenix/${cfg.console.overlayTokenSecret}";
          username = cfg.console.overlayTokenUser;
        };
      }
    ]);
  };
}
