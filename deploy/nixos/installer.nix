# The live system on an organisation's installation USB (design 0016).
#
# It boots, starts cmd/sextant-installer on tty1 without a login, and that
# program does the rest: network, the code from the console, the disk, and
# the install from the fleet repository. Nothing on the stick grants access;
# the console address is the only thing built in.
#
# Built per fleet by sextant.lib.mkInstaller, which reads the address from
# the fleet's fleet.json and passes it as sextant.installer.console.
{ self }:
{ config, lib, pkgs, modulesPath, ... }:
let
  cfg = config.sextant.installer;
  system = pkgs.stdenv.hostPlatform.system;
  sextant = self.packages.${system};
  disko = self.inputs.disko.packages.${system};
in
{
  imports = [ (modulesPath + "/installer/cd-dvd/installation-cd-minimal.nix") ];

  options.sextant.installer.console = lib.mkOption {
    type = lib.types.str;
    example = "https://sextant.example.org";
    description = "The console this stick joins laptops to.";
  };

  config = {
    assertions = [{
      assertion = lib.hasPrefix "https://" cfg.console || lib.hasPrefix "http://" cfg.console;
      message = "sextant.installer.console must be the console's address, got '${cfg.console}'";
    }];

    isoImage.volumeID = lib.mkForce "SEXTANT";
    image.baseName = lib.mkForce "sextant-installer";
    # The person at the laptop reads one program; no boot menu to choose from.
    boot.loader.timeout = lib.mkForce 1;

    # Wireless through NetworkManager, which the installer drives with nmcli;
    # the minimal image's wpa_supplicant would fight it for the card.
    networking.networkmanager.enable = true;
    networking.wireless.enable = lib.mkForce false;
    hardware.enableRedistributableFirmware = true;

    nix.settings.experimental-features = [ "nix-command" "flakes" ];

    # The screen belongs to the installer: boot and service messages printed
    # over its questions made them unreadable (seen in the first boot of the
    # image, 2026-11-12). The journal still has them, on tty2 and after.
    boot.consoleLogLevel = 0;
    boot.kernelParams = [ "quiet" "systemd.show_status=false" "udev.log_level=3" ];

    # The minimal image starts sshd. A stick that installs laptops and holds
    # a device credential while it does has no business accepting logins
    # from the network.
    services.openssh.enable = lib.mkForce false;

    # The installer owns tty1. The other consoles keep their login prompt, for
    # whoever needs to look underneath.
    systemd.services."getty@tty1".enable = false;
    systemd.services."autovt@tty1".enable = false;
    systemd.services.sextant-installer = {
      description = "Sextant installation";
      wantedBy = [ "multi-user.target" ];
      # Wired needs nobody, so wait for it before asking for a wireless
      # network: started earlier, the installer found no connection yet and
      # asked for one on a laptop with a cable in it.
      wants = [ "network-online.target" ];
      after = [ "network-online.target" "systemd-user-sessions.service" ];
      path = [
        sextant.sextant-agent
        sextant.sextant-installer
        disko.disko-install
        pkgs.sbctl
        pkgs.git
        pkgs.cryptsetup
        pkgs.util-linux
        pkgs.coreutils
        pkgs.networkmanager
        config.systemd.package
        config.nix.package
        config.system.build.nixos-install
      ];
      serviceConfig = {
        ExecStart = "${sextant.sextant-installer}/bin/sextant-installer --console ${lib.escapeShellArg cfg.console}";
        StandardInput = "tty-force";
        StandardOutput = "tty";
        StandardError = "tty";
        TTYPath = "/dev/tty1";
        TTYReset = true;
        TTYVHangup = true;
        # Start on a clear screen, not under what the kernel printed.
        TTYVTDisallocate = true;
        Restart = "always";
      };
    };
  };
}
