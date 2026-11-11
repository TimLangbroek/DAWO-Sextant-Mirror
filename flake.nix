{
  description = "Sextant - declarative fleet control-plane for NixOS";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];

      # Go 1.27.4, ahead of nixpkgs, because GO-2026-6859 (an HTTP/1 server
      # crash through a MaxBytesReader header race in net/http) is fixed there
      # and nowhere earlier, and this console is an HTTP/1 server that uses
      # MaxBytesReader on its request bodies. nixpkgs unstable ships 1.27.3 as
      # go. Excepting it in .govulncheck-exceptions would be accepting a
      # reachable crash, not a fix.
      #
      # Used only where this flake builds Go: the sextant package and the
      # devshell's go. Not an overlay on nixpkgs: an overlay changed every
      # Go-built package in the VM tests' systems, nothing came from the
      # binary cache, and the flake check ran past its timeout (CI run 830).
      #
      # REMOVE THIS the moment nixpkgs ships 1.27.4 or later. A local version
      # override is a small fork of the toolchain: it stops receiving whatever
      # nixpkgs does to its go derivation. `nix flake update nixpkgs` then
      # `nix develop --command go version` is the whole check. The hash is the
      # sha256 go.dev publishes for the source tarball.
      goVersion = "1.27.4";
      goFor = pkgs: pkgs.go.overrideAttrs (_old: {
        version = goVersion;
        src = pkgs.fetchurl {
          url = "https://go.dev/dl/go${goVersion}.src.tar.gz";
          hash = "sha256-c8+4VxrsDAEXdofbhxY9Rqog6XeWcncokaoXZU2aYJk=";
        };
      });

      forAll = f: nixpkgs.lib.genAttrs systems
        (system: f nixpkgs.legacyPackages.${system});

      # One version, read from the chart, because the chart's is the one a
      # release already guards against the tag (release.yml, "Chart version
      # guard"). Before this, the flake said 0.1.0 while the chart said 0.92.0,
      # so `nix build .#sextant` produced sextant-0.1.0 and a binary that
      # reported "dev" - on the path this product tells operators to use.
      version =
        let
          chart = builtins.match ".*\nversion: ([0-9]+\.[0-9]+\.[0-9]+)\n.*"
            (builtins.readFile ./deploy/helm/Chart.yaml);
          cargo = builtins.match ".*\nversion = \"([0-9]+\.[0-9]+\.[0-9]+)\"\n.*"
            (builtins.readFile ./agent/Cargo.toml);
        in
        if chart == null then throw "no version: line in deploy/helm/Chart.yaml"
        else if cargo == null then throw "no version = line in agent/Cargo.toml"
        # The agent carries its own version into its binary through
        # CARGO_PKG_VERSION, so nix cannot paper over a difference: the drift
        # has to be refused rather than hidden behind a matching store path.
        else if builtins.head cargo != builtins.head chart
        then throw ("agent/Cargo.toml is at ${builtins.head cargo} and "
          + "deploy/helm/Chart.yaml is at ${builtins.head chart}; one release, one number")
        else builtins.head chart;
    in
    {
      # The resolver/generator contract (ADR 0005): overlays import these
      # from this flake so console and generator share one source.
      lib = {
        resolve = import ./nix/resolve.nix { };
        generator = import ./nix/generator.nix { lib = nixpkgs.lib; };
        exportCatalog =
          (import ./nix/export-catalog.nix { lib = nixpkgs.lib; }).exportCatalog;
        exportCatalogFromOptions =
          (import ./nix/export-catalog.nix { lib = nixpkgs.lib; }).exportCatalogFromOptions;
        # Per-class export: an overlay passes one evaluated host per device
        # class and every entry is tagged with the classes whose image
        # defines it. This is what makes CatalogEntry.AppliesTo and the
        # generator's class filtering able to keep their promise.
        exportCatalogFromClassOptions =
          (import ./nix/export-catalog.nix { lib = nixpkgs.lib; }).exportCatalogFromClassOptions;
        tests = import ./nix/tests.nix { lib = nixpkgs.lib; };
      };

      packages = forAll (pkgs: rec {
        sextant = (pkgs.buildGoModule.override { go = goFor pkgs; }) {
          pname = "sextant";
          inherit version;
          # Only the Go source tree feeds the build - filtered rather than
          # `src = self` (the whole flake tree). Unfiltered, the source
          # derivation's hash depended on every unrelated file in the repo
          # (docs, chart, nix fixtures), so touching any of them busted the
          # Go build cache with no Go source change, and any accidentally
          # committed non-Go file would ride along into the store path.
          src = pkgs.lib.fileset.toSource {
            root = ./.;
            fileset = pkgs.lib.fileset.unions [
              ./go.mod
              ./go.sum
              ./vendor
              ./cmd
              ./internal
              # The committed manual page: the package does not install this
              # copy (sxctl writes its own), but the drift test in cmd/sxctl
              # reads it, and the check phase runs inside this source.
              ./docs/man
            ];
          };
          # Dependencies are vendored; no network during build.
          vendorHash = null;
          subPackages = [ "cmd/sextant" "cmd/sxctl" ];
          env.CGO_ENABLED = "0";
          # The Dockerfile has stamped main.version since the first release and
          # this build did not, so the same commit produced a console that knew
          # its release in a container and called itself "dev" on NixOS.
          ldflags = [ "-s" "-w" "-X main.version=${version}" ];
          # sxctl writes its own manual, so the package installs the page the
          # binary itself produces rather than a copy of a file in the tree.
          # A packaged sxctl and its `man sxctl` cannot disagree.
          postInstall = ''
            mkdir -p $out/share/man/man1
            $out/bin/sxctl man > $out/share/man/man1/sxctl.1
          '';
          meta = {
            description = "Declarative fleet control-plane for NixOS";
            homepage = "https://code.overheid.nl/MinBZK/DAWO-Sextant";
            mainProgram = "sextant";
          };
        };
        default = sextant;

        # The device agent (ADR 0010): Rust, static-friendly, observes and
        # reports only - comin does the converging.
        sextant-agent = pkgs.rustPlatform.buildRustPackage {
          pname = "sextant-agent";
          inherit version;
          src = ./agent;
          cargoLock.lockFile = ./agent/Cargo.lock;
          meta = {
            description = "Sextant device agent: check-in and hardware facts";
            homepage = "https://code.overheid.nl/MinBZK/DAWO-Sextant";
            mainProgram = "sextant-agent";
          };
        };

        # An SBOM and a CVE report for a fleet's closures (audit finding S3).
        # It lives here rather than in an overlay because every fleet needs
        # it and none of them should carry its own copy: an overlay runs
        # `nix run sextant#fleet-sbom -- . out` against its own flake.
        #
        # sbomnix and vulnxscan are deliberately NOT in runtimeInputs, and it
        # is not because they are awkward to package (they are: sbomnix does
        # not build against this nixpkgs pin, whose pandas is newer than one
        # of its dependencies accepts). Pinning a vulnerability scanner is
        # wrong on its own terms. A scanner and its feed age badly, and one
        # frozen in this lock would quietly stop seeing everything published
        # after the day we last bumped it - the exact failure a CVE report is
        # supposed to prevent. The caller supplies them, so the answer comes
        # from today's tooling:
        #
        #   nix shell github:tiiuae/sbomnix -c \
        #     nix run .#fleet-sbom -- . out
        #
        # The script checks for them on PATH and says so plainly if absent.
        fleet-sbom = pkgs.writeShellApplication {
          name = "fleet-sbom";
          runtimeInputs = with pkgs; [ jq nix coreutils findutils ];
          text = builtins.readFile ./scripts/fleet-sbom.sh;
          meta = {
            description = "SBOM and vulnerability report for a fleet's NixOS closures";
            homepage = "https://code.overheid.nl/MinBZK/DAWO-Sextant";
            mainProgram = "fleet-sbom";
          };
        };
      });

      # A NixOS VM that boots the module, points the console at a repository
      # and a database, and uses it. Runs with `nix build .#checks.<system>
      # .console-on-nixos`; needs KVM.
      checks = forAll (pkgs: {
        # Cheap: evaluates the module and reads the unit back.
        console-module = import ./tests/console-module.nix { inherit self pkgs nixpkgs; };
        # Cheap: the device bundle on stubbed core options (design 0014).
        device-module = import ./tests/device-module.nix { inherit self pkgs nixpkgs; };
        # Deep: boots a machine and uses the console. Needs KVM, and a warm
        # cache - from cold it builds a whole NixOS closure.
        console-on-nixos = import ./tests/console-on-nixos.nix { inherit self pkgs; };
        # Deep: first start with nothing configured, through the setup
        # pages to a signed-in owner (design 0013).
        setup-on-nixos = import ./tests/setup-on-nixos.nix { inherit self pkgs; };
        # Deep: a managed local administrator's password is drawn, logs in,
        # is rotated and survives a reboot (design 0017).
        admin-password-on-nixos = import ./tests/admin-password-on-nixos.nix { inherit self pkgs; };
      });

      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            (goFor pkgs)
            golangci-lint
            just
            git
            postgresql # psql client for local dev against the compose database
            tailwindcss # `just css` rebuilds the console stylesheet
            jq # catalog drift guard canonicalises the export (regen-catalog.sh)
            kubernetes-helm # chart guards (scripts/check-chart.sh)
            govulncheck # vulnerability gate runs on THIS pinned toolchain (scripts/vulncheck.sh)
            # Rust toolchain for the device agent (agent/): the README and
            # `just ci` run cargo fmt/clippy/test, so the shell must carry them.
            cargo
            rustc
            clippy
            rustfmt
            gcc # `cc` for crate build scripts (proc-macro2, ring)
          ];
        };
      });

      # Device-side agent module (ADR 0010).
      # Each carries a key, so importing it twice - directly and through
      # nixosModules.device - is one import, not a duplicate declaration.
      nixosModules.agent = {
        key = "sextant/agent";
        imports = [ (import ./deploy/nixos/agent.nix { inherit self; }) ];
      };

      # Root action executor: consumes the agent's intent spool to lock or
      # crypto-wipe a device (design 0004). Wipe is gated and default-off.
      nixosModules.actd = {
        key = "sextant/actd";
        imports = [ ./deploy/nixos/actd.nix ];
      };

      # Everything a DAWO-Core machine needs to be a device of a Sextant
      # fleet, in one import (design 0014): fleet wiring, the integrations
      # the console configures, update window, boot health, elevation
      # requests, user rights, cache auth, agenix secrets.
      nixosModules.device = import ./deploy/nixos/device { inherit self; };

      # The console as a systemd service. One host, your own database and
      # proxy - the container path without the container (see
      # docs/handbook/src/operators/deployment-paths.md).
      nixosModules.default = import ./deploy/nixos/console.nix { inherit self; };

    };
}
