# What the NixOS module would actually run, checked without building a system.
#
# The VM test next to this one boots a real machine and is the better proof,
# but it pulls a whole NixOS closure - on a machine without a warm cache that
# is hours of compiling, so nobody runs it and it proves nothing on the days
# it matters. This one evaluates the module and reads the unit back. It costs
# a second.
#
# It exists because the module used to pass only --addr: the service could not
# be given a repository or the write path at all, and nothing noticed.
{ self, pkgs, nixpkgs }:
let
  eval = cfg: (nixpkgs.lib.nixosSystem {
    system = pkgs.stdenv.hostPlatform.system;
    modules = [
      self.nixosModules.default
      ({ ... }: {
        boot.loader.grub.enable = false;
        fileSystems."/" = { device = "/dev/null"; fsType = "ext4"; };
        system.stateVersion = "24.05";
        services.sextant = cfg;
      })
    ];
  }).config;
  mk = cfg: (eval cfg).systemd.services.sextant.serviceConfig;
  # The module's own refusals, as the messages nixos-rebuild would print.
  refusals = cfg: builtins.concatStringsSep "\n"
    (map (a: a.message) (builtins.filter (a: !a.assertion) (eval cfg).assertions));

  full = mk {
    enable = true;
    repo = "/srv/overlay";
    write = true;
    gate = "remote";
    gateUrl = "https://gate.example.org";
    publicUrl = "https://console.example.org";
  };
  inState = mk {
    enable = true;
    repo = "/var/lib/sextant/overlay";
    gate = "eval";
  };
  # A sibling that only SHARES the state directory's name. It is outside, so
  # it needs the write path; a plain string prefix says it is inside and the
  # unit never gets one.
  beside = mk {
    enable = true;
    repo = "/var/lib/sextant-repos/main";
    write = true;
    gate = "eval";
  };
  # The fleet repository on a forge: prepare-repo fills the default repo
  # before the console starts, and the console pushes back to it.
  remoteSvc = (eval {
    enable = true;
    gitRemote.url = "https://forge.example.org/acme/fleet.git";
    gitRemote.netrcFile = "/run/secrets/sextant-netrc";
    gate = "eval";
  }).systemd.services.sextant;
  remote = remoteSvc.serviceConfig;
  # The database: local by default, over the socket, as the unit's user.
  dbDefault = eval { enable = true; user = "fleet"; };
  dbOff = (eval { enable = true; database.createLocally = false; }).systemd.services.sextant;
in
pkgs.runCommand "sextant-console-module-check"
{
  dbDsn = dbDefault.systemd.services.sextant.environment.SEXTANT_PG_DSN;
  dbAfter = builtins.concatStringsSep "," dbDefault.systemd.services.sextant.after;
  dbRequires = builtins.concatStringsSep "," dbDefault.systemd.services.sextant.requires;
  dbPg = if dbDefault.services.postgresql.enable then "on" else "off";
  dbOwner = builtins.toJSON dbDefault.services.postgresql.ensureUsers;
  dbOffDsn = dbOff.environment.SEXTANT_PG_DSN or "";
  dbOffAfter = builtins.concatStringsSep "," dbOff.after;
  remotePre = builtins.concatStringsSep "|" remote.ExecStartPre;
  remoteExec = remote.ExecStart;
  remoteCred = builtins.concatStringsSep "," remote.LoadCredential;
  remoteHome = remoteSvc.environment.HOME;
  inStatePre = builtins.concatStringsSep "|" (inState.ExecStartPre or [ ]);
  # A DynamicUser has no uid before the unit starts, so nothing outside the
  # state directory can be made for it.
  refusedOutside = refusals {
    enable = true;
    gitRemote.url = "https://forge.example.org/acme/fleet.git";
    repo = "/srv/overlay";
  };
  refusedRelative = refusals {
    enable = true;
    gitRemote.url = "https://forge.example.org/acme/fleet.git";
    gitRemote.netrcFile = "secrets/netrc";
  };
  acceptedRemote = refusals {
    enable = true;
    gitRemote.url = "https://forge.example.org/acme/fleet.git";
    user = "sextant";
    repo = "/srv/overlay";
  };
  execStart = full.ExecStart;
  rwPaths = builtins.concatStringsSep "," full.ReadWritePaths;
  rwInState = builtins.concatStringsSep "," inState.ReadWritePaths;
  rwBeside = builtins.concatStringsSep "," beside.ReadWritePaths;
  inStateExec = inState.ExecStart;
} ''
  fail() { echo "FAIL: $1" >&2; exit 1; }

  # Every option that shapes the deployment has to reach the command line.
  for flag in "--addr 127.0.0.1:8080" "--gate remote" "--repo /srv/overlay" \
              "--gate-url https://gate.example.org" "--write" "--secure-cookies" \
              "--public-url https://console.example.org"; do
    case "$execStart" in
      *"$flag"*) ;;
      *) fail "ExecStart is missing $flag: $execStart" ;;
    esac
  done

  # ProtectSystem=strict makes only the state directory writable, so a repo
  # outside it has to be named - and one inside it must NOT be, or the unit
  # carries a path it already owns.
  [ "$rwPaths" = "/srv/overlay" ] || fail "ReadWritePaths for an outside repo = '$rwPaths'"
  [ -z "$rwInState" ] || fail "a repo inside the state dir was added to ReadWritePaths: '$rwInState'"
  [ "$rwBeside" = "/var/lib/sextant-repos/main" ] || fail "a repo beside the state dir was treated as inside it: '$rwBeside'"

  # A read-only console must not be handed the write path by accident.
  case "$inStateExec" in
    *--write*) fail "write appeared without being asked for: $inStateExec" ;;
  esac

  # A remote is cloned (or filled) before the console starts, with the
  # credential linked from the unit's credentials, never copied or inlined.
  case "$remotePre" in
    *"prepare-repo --repo /var/lib/sextant/overlay --url https://forge.example.org/acme/fleet.git --branch main"*) ;;
    *) fail "ExecStartPre does not prepare the default repo: $remotePre" ;;
  esac
  case "$remotePre" in
    *"ln -sfn %d/netrc /run/sextant/.netrc|"*) ;;
    *) fail "the netrc is not linked before prepare-repo: $remotePre" ;;
  esac
  [ "$remoteCred" = "netrc:/run/secrets/sextant-netrc" ] || fail "LoadCredential = '$remoteCred'"
  [ "$remoteHome" = "/run/sextant" ] || fail "HOME = '$remoteHome'"
  for flag in "--repo /var/lib/sextant/overlay" "--git-remote origin"; do
    case "$remoteExec" in
      *"$flag"*) ;;
      *) fail "ExecStart with a remote is missing $flag: $remoteExec" ;;
    esac
  done
  [ -z "$inStatePre" ] || fail "prepare-repo ran without a remote: $inStatePre"
  case "$inStateExec" in
    *--git-remote*) fail "a console without a remote was told to push: $inStateExec" ;;
    *--public-url*) fail "a public url appeared without being set: $inStateExec" ;;
  esac

  case "$refusedOutside" in
    *"needs services.sextant.user"*) ;;
    *) fail "a DynamicUser repo outside the state dir was accepted: '$refusedOutside'" ;;
  esac
  case "$refusedRelative" in
    *"netrcFile must be an absolute path"*) ;;
    *) fail "a relative netrcFile was accepted: '$refusedRelative'" ;;
  esac
  [ -z "$acceptedRemote" ] || fail "a fixed user with an outside repo was refused: '$acceptedRemote'"

  # The observed plane needs a database; by default it is on this host,
  # reached over the socket as the unit's own user, with no password.
  [ "$dbPg" = on ] || fail "createLocally did not enable postgresql"
  [ "$dbDsn" = "postgres://fleet@/fleet?host=/run/postgresql" ] || fail "DSN = '$dbDsn'"
  case "$dbOwner" in
    *'"name":"fleet"'*) ;;
    *) fail "no database role for the unit's user: $dbOwner" ;;
  esac
  case ",$dbAfter," in *,postgresql.target,*) ;; *) fail "not ordered after postgresql.target: $dbAfter" ;; esac
  case ",$dbRequires," in *,postgresql.target,*) ;; *) fail "does not require postgresql.target: $dbRequires" ;; esac
  [ -z "$dbOffDsn" ] || fail "createLocally = false still set a DSN: $dbOffDsn"
  case ",$dbOffAfter," in *,postgresql.target,*) fail "createLocally = false still waits for postgres" ;; esac

  touch $out
''
