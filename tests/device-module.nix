# nixosModules.device, evaluated without building a system.
#
# The bundle assumes DAWO-Core underneath, which this flake does not depend
# on, so the few core options it builds on are stubbed here with the same
# names and types. What is checked is the bundle's own behaviour: what it
# wires from the generator's bridge, what it refuses, and that its catalog
# export passes both presentation rules - an unlabelled option would reach
# the console as a raw dotted path.
{ self, pkgs, nixpkgs }:
let
  lib = nixpkgs.lib;

  # The parts of DAWO-Core the bundle reads or sets.
  coreStub = { lib, pkgs, ... }: {
    options = {
      dawo.autoUpdate = {
        enable = lib.mkOption { type = lib.types.bool; default = false; };
        options.repoUrl = lib.mkOption { type = lib.types.str; default = "https://codeberg.org/DAWO/DAWO-Core.git"; };
        options.branch = lib.mkOption { type = lib.types.str; default = "main"; };
        options.pollSeconds = lib.mkOption { type = lib.types.int; default = 1800; };
      };
      dawo.printing.enable = lib.mkOption { type = lib.types.bool; default = false; };
      services.comin = {
        enable = lib.mkOption { type = lib.types.bool; default = true; };
        package = lib.mkOption { type = lib.types.package; default = pkgs.hello; };
        postDeploymentCommand = lib.mkOption { type = lib.types.nullOr lib.types.str; default = null; };
        remotes = lib.mkOption { type = lib.types.listOf lib.types.attrs; default = [ ]; };
      };
      boot.lanzaboote.enable = lib.mkOption { type = lib.types.bool; default = false; };
      age.secrets = lib.mkOption { type = lib.types.attrsOf lib.types.attrs; default = { }; };
    };
  };

  host = extra: (lib.nixosSystem {
    system = pkgs.stdenv.hostPlatform.system;
    modules = [
      coreStub
      self.nixosModules.device
      # Imported a second time, as an overlay that lists it on its own does:
      # the key makes that one import.
      self.nixosModules.agent
      ({ ... }: {
        boot.loader.grub.enable = false;
        fileSystems."/" = { device = "/dev/null"; fsType = "ext4"; };
        system.stateVersion = "24.05";
      })
    ] ++ (self.lib.generator.mkModules {
      fleet = { version = 3; groups.pilot = { }; devices.lt-1 = { groups = [ "pilot" ]; hardware = "x"; }; };
      tag = "lt-1";
    }) ++ [ extra ];
  });

  failed = cfg: map (a: a.message) (lib.filter (a: !a.assertion) cfg.config.assertions);

  wired = host {
    sextant.console.url = "https://console.example.org";
    sextant.console.overlayRepoUrl = "https://forge.example.org/acme/fleet.git";
  };
  bare = host { };
  privateRepo = host {
    sextant.console.url = "https://console.example.org";
    sextant.console.overlayRepoUrl = "https://forge.example.org/acme/fleet.git";
    sextant.console.overlayTokenSecret = "fleet-pull";
  };
  halfCache = host {
    sextant.console.url = "https://console.example.org";
    sextant.console.overlayRepoUrl = "https://forge.example.org/acme/fleet.git";
    sextant.releaseCache.url = "https://cache.example.org/cache";
  };
  c = wired.config;
in
pkgs.runCommand "sextant-device-module-check"
{
  wiredFailed = builtins.concatStringsSep "|" (failed wired);
  bareFailed = builtins.concatStringsSep "|" (failed bare);
  halfCacheFailed = builtins.concatStringsSep "|" (failed halfCache);
  repoUrl = c.dawo.autoUpdate.options.repoUrl;
  branch = c.dawo.autoUpdate.options.branch;
  autoUpdate = if c.dawo.autoUpdate.enable then "on" else "off";
  agentUrl = c.services.sextant-agent.url;
  agentTag = c.services.sextant-agent.tag;
  agentOn = if c.services.sextant-agent.enable then "on" else "off";
  publicRemotes = builtins.toJSON c.services.comin.remotes;
  privateAuth = builtins.toJSON (map (r: r.auth or null) privateRepo.config.services.comin.remotes);
  substituters = builtins.toJSON (c.nix.settings.extra-substituters or [ ]);
  # Both presentation rules, on the bundle's own option tree. A throw fails
  # the evaluation of this derivation, which is the failure we want.
  catalogSize = toString (builtins.length (self.lib.exportCatalogFromOptions wired.options));
} ''
  fail() { echo "FAIL: $1" >&2; exit 1; }

  [ -z "$wiredFailed" ] || fail "a wired device was refused: $wiredFailed"
  case "$bareFailed" in
    *"sextant.console.url is not set"*"overlayRepoUrl is not set"*) ;;
    *) fail "a device without a console or repository was accepted: '$bareFailed'" ;;
  esac
  case "$halfCacheFailed" in
    *"releaseCache.url and publicKey go together"*) ;;
    *) fail "a cache without its key was accepted: '$halfCacheFailed'" ;;
  esac

  [ "$repoUrl" = "https://forge.example.org/acme/fleet.git" ] || fail "comin follows '$repoUrl'"
  [ "$branch" = "main" ] || fail "comin branch '$branch', the generator said main"
  [ "$autoUpdate" = on ] || fail "auto-update is off"
  [ "$agentUrl" = "https://console.example.org" ] || fail "agent reports to '$agentUrl'"
  [ "$agentTag" = "lt-1" ] || fail "agent tag '$agentTag'"
  [ "$agentOn" = on ] || fail "agent is off"

  [ "$publicRemotes" = "[]" ] || fail "a public repository got a comin remote with auth: $publicRemotes"
  case "$privateAuth" in
    *'"access_token_path":"/run/agenix/fleet-pull"'*) ;;
    *) fail "a private repository's token is not wired: $privateAuth" ;;
  esac
  [ "$substituters" = "[]" ] || fail "a release cache appeared without being set: $substituters"

  [ "$catalogSize" -gt 0 ] || fail "the bundle exports no settings"

  touch $out
''
