# The disk layout of a device, and the rule that its root is encrypted
# (design 0016, "Disk layout").
#
# Easy by default: a fleet that says nothing about disks gets one layout,
# GPT with a 1 GB EFI system partition and the rest one LUKS2 volume holding
# an ext4 root. The installer (cmd/sextant-installer) points the disk called
# `main` at the disk it chose and puts the passphrase where `passwordFile`
# says; change either here and change it there.
#
# The fleet's own layout wins: everything below is lib.mkDefault, so a fleet
# that declares disko.devices for a device, a group or the organisation
# replaces it whole.
#
# Encryption is not optional. Escrow (design 0009) and "Secure this laptop"
# both assume the root is on LUKS, so an evaluation whose root is not on a
# mapped device fails, and with it the console's gate.
{ self }:
{ config, lib, ... }:
let
  cfg = config.sextant.disk;
  root = config.fileSystems."/" or null;
in
{
  imports = [ self.inputs.disko.nixosModules.disko ];

  options.sextant.disk.defaultLayout = lib.mkOption {
    type = lib.types.bool;
    default = true;
    description = ''
      Lay out the disk with Sextant's default when the fleet declares no
      disko.devices. Off for a fleet that mounts its file systems another
      way, such as a hardware configuration written by nixos-generate-config.
    '';
  };

  options.sextant.disk.requireEncryption = lib.mkOption {
    type = lib.types.bool;
    default = true;
    description = ''
      Refuse a configuration whose root file system is not on an encrypted
      volume. Only for test machines that never hold data.
    '';
  };

  config = {
    disko.devices = lib.mkIf cfg.defaultLayout (lib.mkDefault {
      disk.main = {
        type = "disk";
        # The installer overrides this with the disk it chose.
        device = "/dev/disk/by-id/sextant-install-target";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "1G";
              type = "EF00";
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
                mountOptions = [ "umask=0077" ];
              };
            };
            root = {
              size = "100%";
              content = {
                type = "luks";
                name = "cryptroot";
                passwordFile = "/tmp/sextant-disk.key";
                settings.allowDiscards = true;
                content = {
                  type = "filesystem";
                  format = "ext4";
                  mountpoint = "/";
                };
              };
            };
          };
        };
      };
    });

    assertions = lib.optional cfg.requireEncryption {
      assertion = root != null
        && lib.hasPrefix "/dev/mapper/" root.device
        && config.boot.initrd.luks.devices != { };
      message = ''
        The root file system is not on an encrypted volume${lib.optionalString (root != null) " (it is on ${root.device})"}.
        A Sextant device keeps its disk phrase in the console and seals it
        to the TPM2, which both need LUKS. Leave disko.devices to the
        default layout, or put the root of your own layout on a "luks"
        content (design 0016).
      '';
    };
  };
}
