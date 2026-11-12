# sextant.nixosModules.device - everything a DAWO-Core machine needs to be a
# managed device of a Sextant fleet, in one import (design 0014):
#
#   fleet.nix              comin follows the fleet repository, the agent reports
#   integrations.nix       the integrations the console's cards configure
#                          (NetBird, directory login, Wazuh, OpenBao, local admin)
#   update-window.nix      a group's maintenance window
#   boot-health.nix        a switch confirms its own health or rolls back
#   comin-boot-entry.nix   every comin switch gets a boot entry under Lanzaboote
#   comin-config-guard.nix a changed comin config takes effect
#   elevation-request.nix  a user asks an operator instead of needing a password
#   user-rights.nix        what a non-admin user may do
#   cache-auth.nix         the device authenticates to the release cache
#   secrets.nix            the overlay's agenix secrets, by name
#   disk.nix               the default disk layout, and a root that must be encrypted
#
# It assumes DAWO-Core underneath (comin, agenix, Lanzaboote and the dawo.*
# options these build on). Anything an organisation adds - its SSH keys, its
# branding, its stations - stays in the organisation's overlay.
#
# Each file came from the BB Open overlay, where they ran on the fleet this
# is developed against; what was BB Open's (addresses, a cache) is now an
# option with no default.
{ self }:
{
  key = "sextant/device";
  imports = [
    self.nixosModules.agent
    self.nixosModules.actd
    ./fleet.nix
    ./integrations.nix
    ./update-window.nix
    ./boot-health.nix
    ./comin-boot-entry.nix
    ./comin-config-guard.nix
    ./elevation-request.nix
    ./user-rights.nix
    ./cache-auth.nix
    ./secrets.nix
    (import ./disk.nix { inherit self; })
  ];
}
