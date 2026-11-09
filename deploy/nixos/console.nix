# The Sextant console as a systemd service.
#
# The container path without the container: one host carries the binary, you
# bring the database and the TLS. See
# docs/handbook/src/operators/deployment-paths.md for how the three compare.
#
# WHAT THIS MODULE LEARNED THE HARD WAY. It used to pass only --addr, so the
# service could not be given a repository or the write path at all - and
# --write is a flag with no environment equivalent, so an EnvironmentFile
# could not supply it either. The result was a console that started, looked
# healthy and could change nothing. Every setting that shapes the deployment
# is an option here now, and `extraArgs` is the escape hatch for the rest.
{ self }:
{ config, lib, pkgs, ... }:
let
  cfg = config.services.sextant;
  # The repository is the config plane: the service commits to it, so it has
  # to be writable by the unit. With ProtectSystem=strict the only writable
  # place is the state directory, which is why a repo outside it has to be
  # named explicitly in ReadWritePaths.
  stateDir = "/var/lib/sextant";
  # A path boundary, not a string prefix. hasPrefix alone reads
  # /var/lib/sextant-repos/main as living inside /var/lib/sextant, leaves it
  # out of ReadWritePaths, and hands back exactly the failure this module was
  # rewritten to prevent: a console that starts, looks healthy and cannot
  # write.
  inStateDir = p: p == stateDir || lib.hasPrefix (stateDir + "/") p;
  remote = cfg.gitRemote.url != null;
  # HOME for git while it talks to the forge: the netrc is linked in here,
  # on a tmpfs that goes away with the unit.
  runDir = "/run/sextant";
  needsRW = cfg.repo != null && !(inStateDir (toString cfg.repo));
  # DynamicUser is the right default for a read-only console: no account to
  # manage, no uid to leak. It is the wrong default the moment the console has
  # to WRITE a repository the operator prepared, because the uid does not
  # exist until the unit starts - so nothing can be chowned to it in advance,
  # and StateDirectory lives under /var/lib/private where root's own git
  # refuses to work ("dubious ownership"). Measured in the VM test on
  # 2026-08-23, which is what this option exists for.
  dynamic = cfg.user == null;
in
{
  options.services.sextant = {
    enable = lib.mkEnableOption "Sextant fleet control-plane";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.system}.sextant;
      description = "Sextant package to run.";
    };

    addr = lib.mkOption {
      type = lib.types.str;
      default = "127.0.0.1:8080";
      description = "HTTP listen address (put a TLS proxy in front).";
    };

    user = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "sextant";
      description = ''
        Run as this user instead of a DynamicUser, and create it. Set it when
        the console writes a repository you prepared yourself: a DynamicUser's
        uid does not exist until the unit starts, so there is nothing to give
        the repository to beforehand.

        Left unset the service runs as a DynamicUser, which is the better
        default for a read-only console.
      '';
    };

    repo = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = if remote then "${stateDir}/overlay" else null;
      defaultText = lib.literalExpression
        ''if gitRemote.url != null then "${stateDir}/overlay" else null'';
      example = "/var/lib/sextant/overlay";
      description = ''
        The overlay working tree: the fleet's configuration, which this
        service reads and commits to. Without it the console runs with no
        config plane - it starts and can change nothing.

        With gitRemote.url set this is filled from the remote before the
        console starts. A path outside ${stateDir} is added to
        ReadWritePaths automatically; it then needs a fixed `user`, so the
        directory can be made for that user before the unit starts.
      '';
    };

    gitRemote = {
      url = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        example = "https://github.com/acme/fleet.git";
        description = ''
          The fleet repository, on any https forge (GitHub, Codeberg,
          GitLab, Forgejo). Before the console starts, `sextant
          prepare-repo` clones it into `repo`, or fills it with a starter
          fleet and pushes when it is still empty, and the console pushes
          every change back to it. An empty repository and this one line
          are the whole setup.
        '';
      };

      branch = lib.mkOption {
        type = lib.types.str;
        default = "main";
        description = "The branch devices follow.";
      };

      netrcFile = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        example = "/run/secrets/sextant-netrc";
        description = ''
          A file with a netrc line for the forge
          (`machine github.com login <account> password <token>`), for a
          credential that may push. The unit reads it with LoadCredential,
          so it can stay root-only. Give it as a string: a Nix path literal
          would copy the token into the world-readable store.
        '';
      };
    };

    write = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = ''
        Enable the write path: mutations and commits. Off by default because
        a read-only console is the safe thing to start with, and because a
        write path with no validation gate is refused rather than assumed.
      '';
    };

    gate = lib.mkOption {
      type = lib.types.enum [ "eval" "remote" "none" ];
      default = "eval";
      description = ''
        The validation gate. `eval` runs nix in-process on this host;
        `remote` delegates to a gate-runner; `none` skips validation and,
        together with write, has to be acknowledged with allowUnvalidated.
      '';
    };

    gateUrl = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Base URL of the gate-runner, when gate = \"remote\".";
    };

    allowUnvalidated = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = ''
        Acknowledge running the write path with gate = "none". The server
        refuses that combination otherwise, deliberately: it commits config
        to devices without proving it builds.
      '';
    };

    publicUrl = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "https://console.example.org";
      description = ''
        The address people reach the console on, through your TLS proxy.
        Sign-in's redirect URL (<publicUrl>/callback, to register at the
        identity provider) and the links in notification mail follow from
        it.
      '';
    };

    secureCookies = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = ''
        Mark session cookies Secure. Required behind TLS on a non-loopback
        address - the server refuses to ship session cookies without it.
      '';
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = ''
        EnvironmentFile with the secrets (SEXTANT_PG_DSN, SEXTANT_SECRET_KEY,
        SEXTANT_SESSION_KEY, SEXTANT_OIDC_CLIENT_SECRET, ...), e.g. an
        agenix-rendered file. Secrets never go on the command line.
      '';
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Extra arguments, for anything this module does not name.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.gate != "remote" || cfg.gateUrl != null;
        message = "services.sextant.gate = \"remote\" needs services.sextant.gateUrl.";
      }
      {
        # The same refusal the server makes, made at build time instead: a
        # host that would not start is better caught by nixos-rebuild than by
        # a crash loop with the reason three levels deep in a log.
        assertion = !(cfg.write && cfg.gate == "none") || cfg.allowUnvalidated;
        message = ''
          services.sextant.write with gate = "none" commits unvalidated config
          to devices. Use gate = "remote" (recommended) or "eval", or set
          services.sextant.allowUnvalidated = true for a deliberate dev cell.
        '';
      }
      {
        assertion = !cfg.write || cfg.repo != null;
        message = "services.sextant.write needs services.sextant.repo: there is nothing to commit to.";
      }
      {
        assertion = !remote || cfg.repo != null;
        message = "services.sextant.gitRemote.url needs services.sextant.repo to clone into; leave repo unset for the default.";
      }
      {
        # A DynamicUser's uid does not exist before the unit starts, so a
        # directory outside the state directory cannot be made for it, and
        # ReadWritePaths on a path that is not there stops the unit.
        assertion = !remote || !dynamic || inStateDir (toString cfg.repo);
        message = "services.sextant.repo outside ${stateDir} needs services.sextant.user, so the directory can be made for that user.";
      }
      {
        assertion = cfg.gitRemote.netrcFile == null || lib.hasPrefix "/" cfg.gitRemote.netrcFile;
        message = "services.sextant.gitRemote.netrcFile must be an absolute path.";
      }
    ];

    users.users = lib.mkIf (!dynamic) {
      ${cfg.user} = {
        isSystemUser = true;
        group = cfg.user;
        description = "Sextant fleet control-plane";
      };
    };
    users.groups = lib.mkIf (!dynamic) { ${cfg.user} = { }; };

    # The repository directory has to be the service's before it starts.
    # Empty is fine: prepare-repo clones into it.
    systemd.tmpfiles.rules = lib.optional (!dynamic && cfg.repo != null)
      "d ${toString cfg.repo} 0750 ${cfg.user} ${cfg.user} -";

    systemd.services.sextant = {
      description = "Sextant fleet control-plane";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      # A forge that is not reachable yet at boot fails prepare-repo; the
      # unit keeps retrying with a growing delay instead of giving up after
      # five quick failures.
      startLimitIntervalSec = 0;
      # git for the config plane; nix only when this host runs the gate
      # itself. A remote gate keeps nix off the console host on purpose.
      path = [ pkgs.git ] ++ lib.optional (cfg.gate == "eval") pkgs.nix;
      environment = lib.mkIf remote { HOME = runDir; };
      serviceConfig = {
        ExecStartPre = lib.optionals remote (
          # %d is the unit's credentials directory: the token is linked,
          # not copied, and disappears with the unit.
          lib.optional (cfg.gitRemote.netrcFile != null)
            "${pkgs.coreutils}/bin/ln -sfn %d/netrc ${runDir}/.netrc"
          ++ [
            (lib.escapeShellArgs [
              (lib.getExe cfg.package)
              "prepare-repo"
              "--repo" (toString cfg.repo)
              "--url" cfg.gitRemote.url
              "--branch" cfg.gitRemote.branch
            ])
          ]);
        LoadCredential = lib.optional (remote && cfg.gitRemote.netrcFile != null)
          "netrc:${cfg.gitRemote.netrcFile}";
        RuntimeDirectory = lib.mkIf remote "sextant";
        RuntimeDirectoryMode = lib.mkIf remote "0700";
        ExecStart = lib.escapeShellArgs ([
          (lib.getExe cfg.package)
          "--addr" cfg.addr
          "--gate" cfg.gate
        ]
        ++ lib.optionals (cfg.repo != null) [ "--repo" (toString cfg.repo) ]
        ++ lib.optionals remote [ "--git-remote" "origin" ]
        ++ lib.optionals (cfg.gateUrl != null) [ "--gate-url" cfg.gateUrl ]
        ++ lib.optionals (cfg.publicUrl != null) [ "--public-url" cfg.publicUrl ]
        ++ lib.optional cfg.write "--write"
        ++ lib.optional cfg.allowUnvalidated "--allow-unvalidated"
        ++ lib.optional cfg.secureCookies "--secure-cookies"
        ++ cfg.extraArgs);
        EnvironmentFile = lib.mkIf (cfg.environmentFile != null) cfg.environmentFile;
        DynamicUser = dynamic;
        User = lib.mkIf (!dynamic) cfg.user;
        Group = lib.mkIf (!dynamic) cfg.user;
        StateDirectory = "sextant";
        WorkingDirectory = stateDir;
        ReadWritePaths = lib.optional needsRW (toString cfg.repo);
        # Graceful shutdown: SIGTERM, then give in-flight writes time.
        KillSignal = "SIGTERM";
        TimeoutStopSec = 30;
        Restart = "on-failure";
        RestartSec = 2;
        RestartSteps = 6;
        RestartMaxDelaySec = 120;
        # Hardening.
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];
        RestrictNamespaces = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        SystemCallArchitectures = "native";
        CapabilityBoundingSet = "";
      };
    };
  };
}
