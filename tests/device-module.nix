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

  baseFleet = { version = 3; groups.pilot = { }; devices.lt-1 = { groups = [ "pilot" ]; hardware = "x"; }; };
  host = hostFleet baseFleet;
  hostFleet = fleet: extra: (lib.nixosSystem {
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
    ] ++ (self.lib.generator.mkModules { inherit fleet; tag = "lt-1"; }) ++ [ extra ];
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
  # Design 0014 end to end: only a console block in fleet.json, and the
  # device reports to the console, pulls the fleet repository from it with
  # its own credential, and follows the block's branch.
  viaConsole = hostFleet
    (baseFleet // { console = { url = "https://console.example.org/"; branch = "stable"; }; })
    { };
  vc = viaConsole.config;
  credPath = name: h: h.config.systemd.paths.${name}.pathConfig.PathChanged or "";
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
  vcFailed = builtins.concatStringsSep "|" (failed viaConsole);
  vcRepo = vc.dawo.autoUpdate.options.repoUrl;
  vcBranch = vc.dawo.autoUpdate.options.branch;
  vcAgentUrl = vc.services.sextant-agent.url;
  vcAuth = builtins.toJSON (map (r: r.auth or null) vc.services.comin.remotes);
  vcRestart = credPath "sextant-comin-credential" viaConsole;
  vcRestartCmd = vc.systemd.services.sextant-comin-credential.serviceConfig.ExecStart;
  wiredRestart = credPath "sextant-comin-credential" wired;
  privateRestart = credPath "sextant-comin-credential" privateRepo;
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

  [ -z "$vcFailed" ] || fail "a device with only a console block was refused: $vcFailed"
  [ "$vcRepo" = "https://console.example.org/fleet.git" ] || fail "comin pulls '$vcRepo', not the console"
  [ "$vcBranch" = stable ] || fail "comin follows '$vcBranch', the console block said stable"
  [ "$vcAgentUrl" = "https://console.example.org" ] || fail "agent reports to '$vcAgentUrl'"
  [ "$vcAuth" = '[{"access_token_path":"/var/lib/sextant-agent/credential","username":"lt-1"}]' ] \
    || fail "the console pull is not authenticated with the device's own credential: $vcAuth"
  [ "$vcRestart" = /var/lib/sextant-agent/credential ] || fail "a new credential does not restart comin: '$vcRestart'"
  case "$vcRestartCmd" in
    *"systemctl try-restart comin.service") ;;
    *) fail "the credential watch does not restart comin: $vcRestartCmd" ;;
  esac
  [ -z "$wiredRestart" ] || fail "a public repository got a credential watch"
  [ -z "$privateRestart" ] || fail "an agenix token got the device-credential watch"

  [ "$catalogSize" -gt 0 ] || fail "the bundle exports no settings"

  touch $out
''
