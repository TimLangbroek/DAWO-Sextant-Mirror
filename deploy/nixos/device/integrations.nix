# integrations.nix - device-side integrations the Sextant console publishes.
#
# Each option under dawo.{netbird,identity,wazuh} is exported into catalog.json
# (nix eval .#catalog) so the console Integrations page can render a card and
# let an operator configure it per scope (org/group/device). Secret-typed
# options carry `// { secret = true; }`: the console stores only a secret-ref
# NAME, never the material. The name resolves to /run/agenix/<name> at runtime,
# where the fleet's agenix provisioning has decrypted it. Nothing secret ever
# enters git or the Nix store.
#
# netbird: self-hosted WireGuard mesh (services.netbird, in nixpkgs).
# identity: directory-backed device login via SSSD (services.sssd, in nixpkgs).
# wazuh:    endpoint security agent (packaged from upstream, wazuh-agent.nix).
{ config, lib, pkgs, ... }:
let
  nb = config.dawo.netbird;
  id = config.dawo.identity;
  wz = config.dawo.wazuh;
  ob = config.dawo.openbao;
  # Names claimed by both backends. agenix owns every secrets/<n>.age as
  # age.secrets.<n> (secrets.nix); the vault owns dawo.openbao.secrets.
  # The intersection must be empty - see the assertion below.
  # agenix comes with DAWO-Core; without it there is nothing to collide with.
  bothBackends = lib.intersectLists ob.secrets (builtins.attrNames (config.age.secrets or { }));
  la = config.dawo.localAdmin;
  # Names the fleet may not claim for a local admin. Handing `root` a
  # console-managed password, or colliding with a service account, is not a
  # configuration choice - it is a privilege-escalation bug with a settings UI.
  # The gate rejects it at eval, so the console refuses the write.
  reservedLogins = [ "root" "nobody" "wazuh" "sshd" "systemd-network" "messagebus" ];
  # Console stores a secret-ref NAME; agenix decrypts it here at runtime. Only
  # the name lives in git. Empty name = no secret wired yet.
  secretPath = name: if name == "" then null else "/run/agenix/${name}";
  wazuhAgent = pkgs.callPackage ./wazuh-agent.nix { };
in
{
  ###########################################################################
  # NetBird mesh
  ###########################################################################
  options.dawo.netbird = {
    enable = lib.mkEnableOption "join the self-hosted NetBird mesh";
    managementUrl = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "https://netbird.example.org";
      description = "Self-hosted NetBird management server URL (sovereign; no SaaS).";
    };
    setupKey = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Secret-ref name of the NetBird setup key (agenix). Reusable key enrolls the fleet.";
    } // { secret = true; };
  };

  ###########################################################################
  # Directory login (SSSD)
  ###########################################################################
  options.dawo.identity = {
    enable = lib.mkEnableOption "directory-backed device login (SSSD)";
    provider = lib.mkOption {
      type = lib.types.enum [ "ldap" "ad" "ipa" ];
      default = "ldap";
      description = "Directory backend SSSD authenticates against. Vendor-neutral.";
    };
    domain = lib.mkOption {
      type = lib.types.str;
      default = "dawo";
      description = "SSSD domain name.";
    };
    ldapUri = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "ldaps://id.example.org:636";
      description = "LDAP URI (identity/auth provider endpoint).";
    };
    searchBase = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "dc=example,dc=org";
      description = "LDAP search base.";
    };
    bindDN = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Service bind DN for SSSD (leave empty for anonymous bind).";
    };
    bindSecret = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Secret-ref name of a file holding LDAP_BIND_PW=<password> (agenix, SSSD environmentFile).";
    } // { secret = true; };
    tlsCaCert = lib.mkOption {
      type = lib.types.lines;
      default = "";
      description = "PEM CA certificate chain for an ldaps:// directory signed by a private CA (public material, no secret). Empty = the system trust store (fine for Let's Encrypt-style certs).";
    } // { label = "Directory CA-certificaat (PEM)"; };
    allowPlaintextLdap = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Allow device login over an unencrypted ldap:// directory. A simple bind carries the user's PASSWORD, so this turns on SSSD's own ldap_auth_disable_tls_never_use_in_production. Only for a directory that offers no TLS; the supported transport is ldaps:// (ADR 0021). Leave off unless you have accepted that risk in writing.";
    } // { label = "Onversleutelde directory-login toestaan"; };
    cacheCredentials = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Keep hashed credentials on the device so staff can sign in while offline (laptop on the road). Off forces a live directory bind for every login.";
    } // { label = "Offline login"; };
    offlineLoginDays = lib.mkOption {
      type = lib.types.ints.positive;
      default = 30;
      description = "How many days a cached login stays valid without reaching the directory. After this, the device requires a live sign-in again.";
    } // { label = "Offline login validity (days)"; };
  };

  ###########################################################################
  # Wazuh endpoint security agent
  ###########################################################################
  options.dawo.wazuh = {
    enable = lib.mkEnableOption "report to a Wazuh manager (endpoint security agent)";
    manager = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "wazuh.example.org";
      description = "Wazuh manager address the agent reports to.";
    };
    agentGroup = lib.mkOption {
      type = lib.types.str;
      default = "default";
      description = "Wazuh agent group (server-side policy grouping).";
    };
    enrollmentSecret = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Secret-ref name of the Wazuh enrollment password (agenix; agent-auth registers with it).";
    } // { secret = true; };
  };


  ###########################################################################
  # OpenBao secret delivery
  ###########################################################################
  options.dawo.openbao = {
    enable = lib.mkEnableOption "fetch device secrets from OpenBao (central vault)" // { label = "OpenBao secrets"; };
    address = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "http://openbao.openbao.svc:8200";
      description = "OpenBao API address as the DEVICE reaches it (mesh address once the routing peer is up).";
    } // { label = "Vault address"; };
    mount = lib.mkOption {
      type = lib.types.str;
      default = "fleet";
      description = "KV v2 mount that holds the fleet's secrets.";
    } // { label = "Secrets mount"; };
    secrets = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      example = [ "netbird-setup-key" ];
      description = "Secret names to fetch. Each lands at /run/secrets/<name> and is linked from /run/agenix/<name> when agenix does not already provide it, so settings that reference a secret name keep working unchanged.";
    } // { label = "Secrets to fetch"; };
    tokenSecret = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Secret-ref name of the vault token file (agenix bootstrap for the experiment; device identity auth replaces it later).";
    } // { secret = true; label = "Vault token"; };
  };

  ###########################################################################
  # Local administrator account
  ###########################################################################
  # A device needs SOMEONE who can log in when the directory, the mesh or the
  # network is down. That account used to be a hardcoded `dawo` with a
  # hardcoded password in the core - which is a shared credential on every
  # device in the fleet, i.e. exactly the thing an audit fails you for. Here it
  # becomes an ordinary fleet setting: off by default, its name chosen per
  # scope, and its password supplied as a secret-ref like every other secret.
  options.dawo.localAdmin = {
    enable = lib.mkEnableOption "a local administrator account for when the directory is unreachable" // {
      label = "Local admin account";
      # Marked high risk in the catalog itself, not left to the console's
      # integration-prefix rule: creating or locking an administrator on every
      # device should hold auto-promotion for approval on ANY console version,
      # including one that predates this card.
      riskClass = "high";
    };
    username = lib.mkOption {
      type = lib.types.str;
      default = "";
      example = "beheerder";
      description = "Login name for the local administrator. Pick a name per organisation rather than reusing one across fleets: a shared name is a shared credential.";
    } // { label = "Username"; };
    passwordSecret = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Secret-ref name of a file holding the password HASH (the output of `mkpasswd -m yescrypt`), not the password itself. Turning this integration off locks the account on every device, but does not delete it or its home directory.";
    } // { secret = true; label = "Password (hash)"; };
  };

  ###########################################################################
  # Implementations
  ###########################################################################
  config = lib.mkMerge [

    # Name validity is asserted whenever a name is SET, not only while the
    # account is enabled: the disable path below acts on that same name, and a
    # name that is unsafe to create is unsafe to run `passwd -l` against.
    (lib.mkIf (la.username != "") {
      assertions = [
        { assertion = builtins.match "[a-z_][a-z0-9_-]*" la.username != null;
          message = "dawo.localAdmin.username must be a lowercase POSIX login name ([a-z_][a-z0-9_-]*), got ${la.username}."; }
        { assertion = !(lib.elem la.username reservedLogins);
          message = "dawo.localAdmin.username may not be a system account (${lib.concatStringsSep ", " reservedLogins})."; }
      ];
    })

    (lib.mkIf la.enable {
      assertions = [
        { assertion = la.username != "";
          message = "dawo.localAdmin: username is required when the account is enabled."; }
        { assertion = la.passwordSecret != "";
          message = "dawo.localAdmin: passwordSecret (secret-ref) is required - an admin account nobody can authenticate to is not an account, and one with an empty password is worse."; }
      ];
      users.users.${la.username} = {
        isNormalUser = true;
        description = "Local administrator (fleet-managed)";
        extraGroups = [ "wheel" ];
        # hashedPasswordFile, never hashedPassword: a hash written into the
        # module would land in the world-readable nix store. The file arrives
        # through agenix, decryptable only by this device's host key.
        hashedPasswordFile = secretPath la.passwordSecret;
      };
      # No NOPASSWD rule. This account HAS a password, so sudo can ask for it -
      # which is the whole difference between this and the shared bootstrap
      # account it replaces.
    })

    # Turning the card off must actually take the account away from whoever
    # holds its password. NixOS cannot delete a user declaratively (mutable
    # users linger by design), so lock it instead: the account survives for
    # forensics and its home is untouched, but it can no longer authenticate.
    (lib.mkIf (!la.enable && la.username != "") {
      system.activationScripts.localAdminLock = {
        deps = [ "users" ];
        text = ''
          if ${pkgs.getent}/bin/getent passwd ${la.username} > /dev/null 2>&1; then
            ${pkgs.shadow}/bin/passwd -l ${la.username} > /dev/null || true
          fi
        '';
      };
    })

    (lib.mkIf nb.enable {
      assertions = [
        { assertion = nb.managementUrl != "";
          message = "dawo.netbird: managementUrl must be set when enabled."; }
        { assertion = nb.setupKey != "";
          message = "dawo.netbird: setupKey (secret-ref) is required to enroll."; }
      ];
      # Self-hosted management server. netbird stores ManagementURL as a Go
      # url.URL; seeding config.json with a bare string is fatal, so we set
      # NB_MANAGEMENT_URL in the wrapper env (used by both the daemon and the
      # login flow) and let `netbird up` persist it correctly. Headless
      # enrollment reads the agenix-decrypted setup key by path only.
      services.netbird.clients.dawo = {
        port = 51820;
        environment.NB_MANAGEMENT_URL = nb.managementUrl;
        login = {
          enable = true;
          setupKeyFile = secretPath nb.setupKey;
        };
      };
    })

    (lib.mkIf id.enable {
      assertions = [
        { assertion = id.ldapUri != "" && id.searchBase != "";
          message = "dawo.identity: ldapUri and searchBase are required."; }
        { assertion = id.bindDN == "" || id.bindSecret != "";
          message = "dawo.identity: bindDN set but bindSecret (secret-ref) is missing."; }
        # ADR 0021. A simple bind carries the end user's password, and the
        # WireGuard mesh only covers the device-to-cluster leg - from the
        # routing peer to the directory pod it crosses the cluster network in
        # the clear. Plain ldap:// stays reachable, because a directory with
        # no TLS is a real situation, but reaching it is an act rather than a
        # default. Same shape as the chart's gateMode: none guard.
        { assertion = id.ldapUri == ""
            || lib.hasPrefix "ldaps://" id.ldapUri
            || id.allowPlaintextLdap;
          message = ''
            dawo.identity: ldapUri is ${id.ldapUri}, which is unencrypted.
            Device login sends the user's password over it in the clear.
            Use ldaps:// (with identity.tlsCaCert for a private CA), or set
            identity.allowPlaintextLdap = true to accept that risk on record.
          ''; }
      ];
      warnings = lib.optional (id.allowPlaintextLdap && !(lib.hasPrefix "ldaps://" id.ldapUri))
        "dawo.identity: device login runs over unencrypted ${id.ldapUri}; staff passwords cross the network in the clear (ADR 0021).";
      security.pam.services.login.makeHomeDir = true;
      security.pam.services.systemd-user.makeHomeDir = true;
      # The NSS caching daemon (nsncd) dlopen's libnss_sss once and caches its
      # answers, but nixpkgs only restart-triggers sssd itself on a settings
      # change - so after a settings change the daemon keeps serving what it
      # learned under the OLD configuration. Found live 2026-07-30: SSSD
      # resolved the user perfectly (`sssctl user-checks bbuijs` returned
      # uid/gid/home) while `getent passwd bbuijs` stayed empty for hours, and
      # a plain `systemctl restart nscd` fixed it instantly. Trigger on a hash
      # so the settings themselves never land in the unit file.
      systemd.services.nscd.restartTriggers = [
        (builtins.hashString "sha256" (builtins.toJSON config.services.sssd.settings))
      ];
      # Private-CA ldaps: the cert is public material, so it may live in the
      # store; reqcert=demand still enforces verification against it.
      environment.etc."dawo/ldap-ca.pem" = lib.mkIf (id.tlsCaCert != "") {
        text = id.tlsCaCert;
      };
      services.sssd = {
        enable = true;
        environmentFile = lib.mkIf (id.bindSecret != "") (secretPath id.bindSecret);
        settings = {
          sssd = {
            config_file_version = 2;
            services = "nss, pam";
            domains = id.domain;
          };
          # offline_credentials_expiration is a PAM option, not a domain
          # option: in the domain section sssd's validator refuses it ("not
          # allowed in section"). Days since the last ONLINE login.
          pam = lib.optionalAttrs id.cacheCredentials {
            offline_credentials_expiration = id.offlineLoginDays;
          };
          "domain/${id.domain}" = {
            id_provider = id.provider;
            auth_provider = id.provider;
            cache_credentials = id.cacheCredentials;
            ldap_uri = id.ldapUri;
            ldap_search_base = id.searchBase;
            # When SSSD cannot reach the directory it marks the domain OFFLINE
            # and waits before trying again - by default 60s, DOUBLING up to an
            # hour. That default assumes a server on a fixed network. A laptop
            # is the opposite: it suspends, changes wifi, comes up before the
            # VPN does. With the stock back-off, a user whose network returned
            # can be locked out of their own machine for up to an hour, with
            # everything working and nothing explaining the wait.
            # Found while chasing a test that stayed offline past three minutes
            # with a directory that was reachable the whole time.
            offline_timeout = 15;
            offline_timeout_max = 120;
          }
          # Certificate policy follows the transport. An ldaps:// directory is
          # verified strictly. A plain ldap:// one is only reachable over the
          # WireGuard mesh (the routing peer publishes the cluster subnet and
          # nothing is exposed publicly - route decision 2026-07-27), so the
          # channel is already encrypted and authenticated at the network
          # layer; demanding a certificate there just makes SSSD refuse to
          # come online, and its password path additionally needs the
          # explicit opt-out. Found live 2026-07-30: SSSD sat "offline" with
          # a perfectly reachable directory.
          // (if lib.hasPrefix "ldaps://" id.ldapUri then {
            ldap_tls_reqcert = "demand";
          } else {
            ldap_tls_reqcert = "never";
            ldap_auth_disable_tls_never_use_in_production = true;
            # SSSD defaults ldap_id_use_start_tls to TRUE, so on a plain
            # ldap:// URI it opens the connection and immediately asks for
            # StartTLS (extended operation 1.3.6.1.4.1.1466.20037). A directory
            # that does not offer it answers "unsupported operation" and SSSD
            # counts that as a failed connection: the server is marked
            # unavailable, the domain goes offline, and identity is simply
            # dead - with a reachable directory and a correct bind.
            #
            # We were relying on our own directory happening to support
            # StartTLS. Say it instead: this branch is the deliberate
            # plain-transport case, where the channel is already encrypted and
            # peer-authenticated by the WireGuard mesh (route decision
            # 2026-07-27, ADR 0015). Asking for a second, redundant layer that
            # the directory may not offer buys nothing and costs everything.
            # An ldaps:// directory takes the branch above and is verified
            # strictly.
            ldap_id_use_start_tls = false;
          })
          // lib.optionalAttrs (id.tlsCaCert != "") {
            ldap_tls_cacert = "/etc/dawo/ldap-ca.pem";
          } // lib.optionalAttrs (id.bindDN != "") {
            ldap_default_bind_dn = id.bindDN;
            ldap_default_authtok_type = "password";
            ldap_default_authtok = "$LDAP_BIND_PW";
          };
        };
      };
    })

    (lib.mkIf wz.enable {
      assertions = [
        { assertion = wz.manager != "";
          message = "dawo.wazuh: manager must be set when enabled."; }
      ];
      environment.systemPackages = [ wazuhAgent ];
      # The agent hard-codes /var/ossec. We stage the read-only store copy into
      # the state dir on start (keeping etc/ and the queue writable), template
      # ossec.conf from the options, enroll once via agent-auth (using the
      # agenix enrollment password), then run the agent supervisor.
      # The agent binaries are compiled with a fixed runtime user/group and
      # refuse to start when they cannot resolve them ("Invalid user '' or
      # group 'wazuh'", found on the first live enrol) - NixOS does not
      # create them for a package that only ships binaries.
      users.groups.wazuh = { };
      users.users.wazuh = {
        isSystemUser = true;
        group = "wazuh";
        description = "Wazuh endpoint security agent";
        home = "/var/lib/ossec";
      };
      # The state tree's top directory, owned by the user the binaries drop to.
      # It must exist before the unit's first Exec command, because
      # WorkingDirectory applies to ExecStartPre as well - preStart creating it
      # would be too late on a freshly imaged device. tmpfiles also re-asserts
      # owner and mode every boot, which is exactly the invariant that broke.
      systemd.tmpfiles.rules = [ "d /var/lib/ossec 0750 wazuh wazuh -" ];

      systemd.services.wazuh-agent = {
        description = "Wazuh endpoint security agent";
        wantedBy = [ "multi-user.target" ];
        after = [ "network-online.target" ];
        wants = [ "network-online.target" ];
        # PATH: wazuh-control is a shell script that calls awk/sed/ps; without
        # them on PATH it dies mid-start with "awk: command not found" (found
        # on the first live enrol, 2026-07-29). WorkingDirectory: the agent
        # resolves its queue relative to the CWD, so starting anywhere else
        # makes it try to mkdir inside the read-only store.
        path = with pkgs; [ gawk gnused procps coreutils ];
        serviceConfig = {
          Type = "forking";
          # NOT StateDirectory. systemd re-applies the state directory's
          # ownership from the unit's own identity before EVERY Exec command,
          # and this unit runs as root while the wazuh binaries drop privileges
          # to the `wazuh` user. So systemd put /var/lib/ossec back to
          # root:root 0750 after the preStart's chown, and the wazuh user could
          # not TRAVERSE its own home. One directory, three symptoms that each
          # looked like a different bug (2026-07-30):
          #   - "Error reading XML file 'etc/ossec.conf': (line 0)" - never an
          #     XML problem at all; the file was simply unreadable. Hours went
          #     into validating the XML, substituting the package's own
          #     191-line template, and ruling out BOM/CR/NUL.
          #   - "(1402): Authentication key file 'etc/client.keys' not found"
          #     while the file sat there with 80 bytes in it.
          #   - "(1210): Queue 'queue/sockets/queue' not accessible:
          #     'Permission denied'".
          # tmpfiles owns the directory instead, so nothing chowns it back.
          WorkingDirectory = "/var/lib/ossec";
        };
        preStart = ''
          set -eu
          root=/var/lib/ossec
          install -d -m 0750 -o wazuh -g wazuh "$root"
          # Seed the writable tree from the store package on first run / upgrade.
          # client.keys is the only exclusion: it is the enrollment identity and
          # must survive an upgrade. The queue tree is NOT excluded, though it
          # looks like pure runtime state. The package ships content inside it -
          # queue/fim/db, queue/syscollector/db, queue/logcollector and
          # queue/syscollector/norm_config.json - and excluding the whole
          # subtree threw all of that away. syscheckd then died with
          # "CRITICAL: Error, id: sqlite: Unspecified type during
          # initialization of SQLite" because it had nowhere to create its FIM
          # database, which failed the unit AFTER agentd had already connected
          # to the manager: the console showed an enrolled agent with no data,
          # and the manager answered "there is no database for agent 029".
          # rsync runs without --delete, so syncing the tree creates the
          # skeleton and leaves existing databases untouched.
          #
          # --checksum, because rsync's quick check cannot see an upgrade from
          # the store. Every store file has mtime 1, and autoPatchelf writes
          # the package's own store path into each binary's runpath - a path
          # of fixed length. So a rebuilt agent's binaries have the same size
          # and the same mtime as the copies already here, rsync skips them,
          # and the old copies keep pointing at a store path that garbage
          # collection has since removed. Measured on the station 2026-11-05:
          # "wazuh-execd: error while loading shared libraries:
          # libwazuhext.so", while the store binary resolved it fine.
          # Comparing content costs a read of the package on each start; the
          # runtime databases are not in the package, so it never touches them.
          ${pkgs.rsync}/bin/rsync -a --checksum --chmod=u+w \
            --exclude=/etc/client.keys \
            ${wazuhAgent}/ossec/ "$root/"
          # /var/ossec is the compiled-in home; point it at the state dir.
          ln -sfn "$root" /var/ossec
          # Runtime dirs the package ships empty (so a tarball keeps none of
          # them) plus the ones it does not ship at all. Belt and braces next to
          # the rsync above: a state tree that lost a directory self-heals on
          # the next start instead of failing one daemon deep.
          mkdir -p "$root/queue/alerts" "$root/queue/diff" "$root/queue/fim" \
            "$root/queue/fim/db" "$root/queue/rids" "$root/queue/sockets" \
            "$root/queue/logcollector" "$root/queue/syscollector" \
            "$root/queue/syscollector/db" "$root/logs" \
            "$root/var/run" "$root/var/db" "$root/etc/shared"
          # ossec.conf: START from the package template and substitute the
          # placeholders, rather than writing a minimal file of our own. The
          # agent needs sections a hand-rolled config leaves out
          # (client_buffer, logging, the wodles) and rejects the whole file
          # when they are missing - "Error reading XML file 'etc/ossec.conf'
          # (line 0)", diagnosed on hardware 2026-07-29.
          ${pkgs.gnused}/bin/sed \
            -e "s|<address>MANAGER_IP</address>|<address>${wz.manager}</address>|g" \
            -e "s|MANAGER_IP|${wz.manager}|g" \
            ${wazuhAgent}/ossec/etc/ossec.conf > "$root/etc/ossec.conf"
          # Enrollment block: the template ships it disabled/absent, and the
          # group is console-managed.
          if ! grep -q "<enrollment>" "$root/etc/ossec.conf"; then
            ${pkgs.gnused}/bin/sed -i \
              "s|</server>|</server>\n    <enrollment><enabled>yes</enabled><groups>${wz.agentGroup}</groups></enrollment>|" \
              "$root/etc/ossec.conf"
          fi
          # Ownership last, so every file written above - the conf included -
          # is readable by the runtime user the binaries insist on. The
          # top-level directory matters most: at 0750 root:root the wazuh user
          # cannot even traverse it, and every read below fails with a message
          # that names something other than permissions.
          chown -R wazuh:wazuh "$root"
          chmod 0750 "$root"
        '';
        script = ''
          set -eu
          # The binaries in the STATE tree, never the store copies: each one
          # resolves its home relative to its own location, so a store binary
          # reads the store's placeholder ossec.conf ("Invalid server address
          # found: 'MANAGER_IP'") and cannot write its PID file into the
          # read-only store. Diagnosed on real hardware 2026-07-29 - the same
          # binaries started by hand from /var/ossec work perfectly.
          export PATH=/var/ossec/bin:$PATH
          # Enroll once (idempotent: skip if we already hold a key).
          if [ ! -s /var/ossec/etc/client.keys ]; then
            ${lib.optionalString (wz.enrollmentSecret != "") ''
              pw="$(cat ${secretPath wz.enrollmentSecret})"
              agent-auth -m ${wz.manager} -P "$pw" -G ${wz.agentGroup} -A ${config.networking.hostName} || true
            ''}
            ${lib.optionalString (wz.enrollmentSecret == "") ''
              agent-auth -m ${wz.manager} -G ${wz.agentGroup} -A ${config.networking.hostName} || true
            ''}
          fi
          exec wazuh-control start
        '';
      };
    })

    (lib.mkIf ob.enable {
      assertions = [
        { assertion = ob.address != "";
          message = "dawo.openbao: address must be set when enabled."; }
        { assertion = ob.tokenSecret != "";
          message = "dawo.openbao: tokenSecret (secret-ref) is required."; }
        # One secret, one backend (ADR 0018). A name served by both agenix
        # and the vault used to resolve by silent precedence - the symlink
        # was only created when agenix had not already claimed the path, so
        # the .age file won and a rotation in the vault never reached the
        # device while every layer reported success.
        #
        # Refused at eval instead, which means the console's gate refuses the
        # write: a collision is a configuration error, not something to
        # resolve at runtime with a rule nobody wrote down.
        { assertion = bothBackends == [ ];
          message = "dawo.openbao: ${lib.concatStringsSep ", " bothBackends} "
            + "exist(s) as both an agenix secret and a vault secret. A secret "
            + "has exactly one backend (ADR 0018): remove the .age file to let "
            + "the vault serve it, or drop the name from dawo.openbao.secrets "
            + "to keep it autonomous."; }
      ];
      # Fetch-on-boot plus a refresh timer: rotation in the vault reaches the
      # device without any configuration change (design 0007). Files live in
      # a tmpfs dir; the agenix-path symlink keeps the secret-ref contract
      # (integrations resolve /run/agenix/<name>) backend-agnostic.
      systemd.services.openbao-secrets = {
        description = "Fetch device secrets from OpenBao";
        wantedBy = [ "multi-user.target" ];
        after = [ "network-online.target" ];
        wants = [ "network-online.target" ];
        serviceConfig = {
          Type = "oneshot";
          RuntimeDirectory = "secrets";
          RuntimeDirectoryMode = "0700";
        };
        script = ''
          set -u
          token="$(cat ${secretPath ob.tokenSecret})"
          failed=""
          mkdir -p /run/agenix
          ${lib.concatMapStringsSep "\n" (name: ''
            # Each secret is attempted independently. Under `set -e` the first
            # unreachable secret aborted the loop, so every later one was
            # skipped and the unit's failure named only the first - the rest
            # looked untried rather than missing.
            if ${pkgs.curl}/bin/curl -sSf -H "X-Vault-Token: $token" \
                 "${ob.address}/v1/${ob.mount}/data/${name}" \
                 | ${pkgs.jq}/bin/jq -er '.data.data.value' \
                 > "/run/secrets/${name}.tmp"; then
              mv "/run/secrets/${name}.tmp" "/run/secrets/${name}"
              chmod 0400 "/run/secrets/${name}"
              # Unconditional: the assertion above guarantees this name is not
              # also an agenix secret, so there is nothing to defer to and no
              # precedence to apply.
              ln -sfn "/run/secrets/${name}" "/run/agenix/${name}"
            else
              rm -f "/run/secrets/${name}.tmp"
              failed="$failed ${name}"
            fi
          '') ob.secrets}
          if [ -n "$failed" ]; then
            # Loud, and naming every secret rather than the first. A
            # vault-managed secret that did not arrive is a device running
            # without it, which the fleet must be able to see (ADR 0018).
            echo "openbao: could not fetch:$failed" >&2
            exit 1
          fi
        '';
      };
      systemd.timers.openbao-secrets = {
        wantedBy = [ "timers.target" ];
        timerConfig = { OnBootSec = "2min"; OnUnitActiveSec = "1h"; };
      };
    })

  ];
}
