# secrets.nix - fleet agenix provisioning. Every <name>.age in the overlay's
# secrets directory becomes age.secrets.<name>, decrypted by the device's SSH host key to
# /run/agenix/<name> at activation - the path integrations.nix resolves a
# console secret-ref to. Adding a secret is: age-encrypt to the target
# devices' host keys (plus the admin key), drop the file in secrets/,
# register the NAME as a secret-ref in the console. The material never
# enters git plaintext, the store, or the console.
{ config, lib, ... }:
let
  dir = config.dawo.fleetSecrets.dir;
  names =
    if dir != null && builtins.pathExists dir
    then lib.filter (n: lib.hasSuffix ".age" n) (builtins.attrNames (builtins.readDir dir))
    else [ ];
in
{
  # The directory is an OPTION because it lives in the overlay, not here:
  # an overlay with secrets sets `dawo.fleetSecrets.dir = ./secrets;`, one
  # without leaves it null. A test points it at a fixture directory whose
  # throwaway key is deliberately not a recipient of any fleet's secrets.
  #
  # Deliberately UNDOCUMENTED (no `description`, internal = true): a
  # documented option under dawo.* is exported into catalog.json and becomes
  # an operator-editable console setting (export-catalog.nix), and "which
  # directory holds the fleet's secrets" must never be one. This comment is
  # its documentation - do not add a description.
  options.dawo.fleetSecrets.dir = lib.mkOption {
    type = lib.types.nullOr lib.types.path;
    default = null;
    internal = true;
  };

  config.age.secrets = lib.listToAttrs (map
    (n: {
      name = lib.removeSuffix ".age" n;
      value.file = dir + "/${n}";
    })
    names);
}
