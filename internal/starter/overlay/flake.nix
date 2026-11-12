{
  # The fleet repository a new Sextant console starts from (design 0014).
  # Every organisation gets these same files; what differs between them is
  # data in fleet.json, which the console edits. Nothing here needs changing
  # to run a fleet. An organisation with its own core, its own hardware
  # modules or its own stations adds them here, the way the BB Open overlay
  # does.
  #
  # flake.lock and catalog.json are generated together by
  # scripts/regen-starter.sh in the Sextant repository: the catalog describes
  # exactly the core the lock pins. Bump them together or not at all.
  description = "Sextant fleet repository";

  inputs = {
    # The core: hardening, desktops, disk layout, hardware scaffolding, and
    # the dawo.* options the console's settings page edits.
    dawo.url = "git+https://codeberg.org/DAWO/DAWO-Core.git";
    # Sextant: the generator that turns fleet.json into one NixOS host per
    # device, and the device module (agent, comin, integrations).
    sextant.url = "git+https://codeberg.org/DAWO/DAWO-Sextant.git";
    # The core's blocks reach these through the `inputs` special argument;
    # follow its pins so there is one nixpkgs and one of everything else.
    nixpkgs.follows = "dawo/nixpkgs";
    nixpkgs-unstable.follows = "dawo/nixpkgs-unstable";
    nixos-hardware.follows = "dawo/nixos-hardware";
    disko.follows = "dawo/disko";
    home-manager.follows = "dawo/home-manager";
    comin.follows = "dawo/comin";
    nix-flatpak.follows = "dawo/nix-flatpak";
    nix-maid.follows = "dawo/nix-maid";
    lanzaboote.follows = "dawo/lanzaboote";
    agenix.follows = "dawo/agenix";
    deploy-rs.follows = "dawo/deploy-rs";
    rust-overlay.follows = "dawo/rust-overlay";
  };

  outputs = inputs@{ self, nixpkgs, dawo, sextant, home-manager, ... }:
    let
      system = "x86_64-linux";
      blocks = dawo.modules.nixos;
      fleet = builtins.fromJSON (builtins.readFile ./fleet.json);

      # What every device gets, whatever its class.
      baseline = [
        sextant.nixosModules.device
        inputs.agenix.nixosModules.default
      ];

      # A laptop or desktop: the core's own client recipe.
      workplace = [
        blocks.boot-loader
        blocks.boot-plymouth-bzk
        blocks.disko-single-nvme-luks
        blocks.profiles-dawo-generic
        blocks.maid-dawo-generic
      ];

      classModules = {
        laptop = workplace;
        desktop = workplace;
      };

      # An unknown class fails the evaluation, and so the console's gate,
      # rather than falling back to the wrong image.
      coreModulesForClass = context: class:
        assert nixpkgs.lib.assertMsg (classModules ? ${class})
          "${context}: class '${class}' has no image in this fleet repository";
        baseline ++ classModules.${class};
      coreModulesFor = tag:
        coreModulesForClass "device ${tag}" (fleet.devices.${tag}.class or "laptop");
      coreModules = baseline ++ workplace;

      # Keyed by the hardware field the console writes when a device joins.
      # The names match hardware-profiles.json beside this file; a join that
      # finds no profile for the model falls back to one of these by CPU.
      hardwareProfiles = {
        generic-intel = blocks.hardware-generic-intel;
        generic-amd = blocks.hardware-generic-amd;
        vm = { lib, modulesPath, ... }: {
          imports = [
            blocks.hardware-dawo-base
            (modulesPath + "/profiles/qemu-guest.nix")
          ];
          # The core's layout names an NVMe disk; a virtio disk is vda.
          disko.devices.disk.main.device = lib.mkForce "/dev/vda";
        };
      };

      specialArgsFor = tag: {
        inherit inputs;
        hostConfig = { name = tag; };
      };
      extraModules = [
        ({ ... }: { system.configurationRevision = self.rev or self.dirtyRev or null; })
        home-manager.nixosModules.home-manager
        ({ config, ... }: {
          home-manager.extraSpecialArgs = {
            inherit inputs;
            hostConfig = { name = config.sextant.deviceTag; };
          };
        })
      ];

      catalogKeys = map (e: e.name)
        (builtins.fromJSON (builtins.readFile ./catalog.json));
    in
    {
      nixosConfigurations = sextant.lib.generator.mkFleet {
        inherit nixpkgs system fleet coreModules coreModulesFor hardwareProfiles
          catalogKeys specialArgsFor extraModules;
      };

      # The settings vocabulary the console renders (Sextant ADR 0005), one
      # synthetic host per class so every entry says which classes have it.
      catalog =
        let
          exportHostFor = class:
            (sextant.lib.generator.mkFleet {
              inherit nixpkgs system coreModules hardwareProfiles
                specialArgsFor extraModules;
              coreModulesFor = _: coreModulesForClass "catalog export" class;
              fleet = fleet // {
                devices.catalog-export = {
                  inherit class;
                  hardware = "generic-intel";
                  groups = [ ];
                };
              };
            }).catalog-export;
        in
        sextant.lib.exportCatalogFromClassOptions
          (nixpkgs.lib.genAttrs (builtins.attrNames classModules)
            (class: (exportHostFor class).options));
    };
}
