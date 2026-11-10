# What an ordinary user may change on their own machine.
#
# The problem this solves, found on hardware 2026-07-30: a directory user
# (bbuijs, via SSSD) could not pick a WiFi network. Not a bug in the account -
# nixpkgs grants NetworkManager rights to members of the local `networkmanager`
# group, local accounts get their groups from the Nix config, and a user who
# comes from LDAP is in no local group at all. So every directory user on every
# workstation had no WiFi, no printer, no timezone. The laptop was unusable the
# moment it left the desk it was imaged on.
#
# Group membership is the wrong lever for this. It requires the directory to
# carry POSIX groups that happen to match Linux service names, it is invisible
# from the console, and it is all-or-nothing per service. What an organisation
# actually wants to say is "users may choose a WiFi network, and may not create
# accounts" - one decision per capability, written down, with a reason.
#
# polkit already models exactly that: every privileged desktop operation has an
# action id, and a rule can answer per action. So each right below is a named
# option that maps to the action ids it needs, and each is simply ON or OFF.
#
# IT USED TO TAKE FOUR VALUES - off, self, session, or group:<name> - and that
# was one lever too many. Naming a directory group INSIDE a value put scoping
# in a second place: Sextant already resolves every setting along
# organisation -> group -> device, and a group written into the value is
# invisible to that model. Two scoping mechanisms, one of them unable to see
# the other (decision 2026-08-06).
#
# What remains of that vocabulary is not an operator's choice at all. Whether a
# right asks for the user's OWN password is a property of the action: whoever
# knows what a polkit id does knows whether it deserves a prompt, and an
# operator asked to choose between "self" and "session" per right is being
# asked to know polkit. So each right declares `selfAuth` and the console shows
# a switch.
#
# Off means only an administrator, with a password - and away from the office
# that is a dead end. The prompt asks for the LOCAL ADMIN password, so the
# honest reading of "off" is "ask a colleague with the admin password to walk
# over". For somebody working from home that is not a slower path but no path,
# and the practical result is that the admin password gets shared until it is
# not a secret any more. That is what these rights exist to prevent.
#
# On is narrower than it sounds. Every rule requires subject.local AND
# subject.active: a real seat, in the foreground. Never SSH, never a background
# session, never a remote desktop. Somebody physically sitting at an unlocked,
# logged-in laptop choosing a WiFi network has not gained anything - they are
# already holding the machine. Refusing them is not security, it is theatre
# that ends with a support ticket.
#
# Rights are granted, never denied. A rule that matches returns YES; a rule
# that does not match returns NOT_HANDLED, so the administrator path survives
# untouched. Returning NO would short-circuit polkit for EVERYONE including an
# admin who legitimately needs the action, which is how a lockdown turns into
# an outage.
{ config, lib, pkgs, ... }:
let
  cfg = config.dawo.userRights;

  # The action ids behind each right, read from the installed polkit policies
  # rather than remembered. Grouped by what a person would ask for, because
  # "may I use WiFi" is one question and five action ids.
  rights = {
    wifi = {
      label = "Choose a network";
      description = ''
        Turn WiFi on or off, scan, and connect to a network, saving it for
        this user. Does not include company-wide connection profiles - those
        are `networkProfiles`. This is what a laptop needs to work anywhere
        other than the desk it was imaged on.
      '';
      actions = [
        "org.freedesktop.NetworkManager.enable-disable-network"
        "org.freedesktop.NetworkManager.enable-disable-wifi"
        "org.freedesktop.NetworkManager.enable-disable-wwan"
        "org.freedesktop.NetworkManager.wifi.scan"
        "org.freedesktop.NetworkManager.network-control"
        "org.freedesktop.NetworkManager.settings.modify.own"
      ];
    };

    networkProfiles = {
      label = "Change shared network profiles";
      description = ''
        Edit connections that apply to every user of the machine, including
        VPN profiles the organisation ships. Separate from `wifi` on purpose:
        joining a cafe network is a different act from rewriting the profile
        that carries company traffic. Hostname and global DNS are deliberately
        NOT included - those belong to the fleet, not to whoever is sitting at
        the machine.
      '';
      # Machine-wide, so it asks for the user's own password (A15.3).
      selfAuth = true;
      actions = [ "org.freedesktop.NetworkManager.settings.modify.system" ];
    };

    timezone = {
      label = "Change the time zone";
      description = ''
        Set the time zone when travelling. Setting the CLOCK itself is not
        included and should not be: time comes from the fleet's NTP servers,
        and a machine whose user can move the clock can move it past a
        certificate expiry or muddle every log timestamp on it.
      '';
      actions = [ "org.freedesktop.timedate1.set-timezone" ];
    };

    printers = {
      label = "Add and manage printers";
      description = ''
        Add, remove and configure printers. Needs `dawo.printing` to be on -
        without CUPS there is nothing to authorise. Job control for other
        people's jobs is not included; this is about a user connecting the
        printer in front of them.
      '';
      actions = [
        "org.opensuse.cupspkhelper.mechanism.printeraddremove"
        "org.opensuse.cupspkhelper.mechanism.printer-local-edit"
        "org.opensuse.cupspkhelper.mechanism.printer-enable"
        "org.opensuse.cupspkhelper.mechanism.printer-set-default"
        "org.opensuse.cupspkhelper.mechanism.devices-get"
      ];
    };

    removableMedia = {
      label = "Mount USB storage";
      description = ''
        Mount removable drives and memory cards. Mounting devices the system
        itself owns (an internal disk, anything in fstab) stays with the
        administrator, so this cannot be used to reach a partition the user
        was not meant to see. Note that USB device control, if enabled, still
        decides whether the drive is allowed to appear at all - that is a
        separate and earlier question.
      '';
      actions = [ "org.freedesktop.udisks2.filesystem-mount" ];
    };

    docks = {
      label = "Authorise a dock";
      description = ''
        Approve a Thunderbolt dock or device when it is plugged in. Without
        this, a docked laptop silently does nothing until an administrator
        visits it, which is the single most common first-day complaint about
        a managed workstation.
      '';
      actions = [ "org.freedesktop.bolt.enroll" ];
    };

    firmware = {
      label = "Install firmware updates";
      description = ''
        Let a user apply firmware updates offered by fwupd. DOWNGRADES are
        deliberately excluded: rolling firmware back to an older version is a
        way to reinstate a fixed vulnerability, so it stays an administrator's
        decision even where updating does not.
      '';
      # Flashing hardware for the whole machine: deliberate, own password.
      selfAuth = true;
      actions = [
        "org.freedesktop.fwupd.update-internal"
        "org.freedesktop.fwupd.update-hotplug"
        "org.freedesktop.fwupd.device-unlock"
      ];
    };
  };

  # Rights that are NOT offered here, and are not an oversight:
  #
  #   org.freedesktop.accounts.user-administration - creating, deleting or
  #     re-grouping accounts. On a fleet machine the account set comes from the
  #     directory; a local user who can add accounts can add one that outlives
  #     their own and answers to nobody.
  #   org.freedesktop.accounts.set-login-option - autologin and the greeter's
  #     user list. Autologin turns full-disk encryption into a locked door with
  #     the key taped to it.
  #   org.freedesktop.NetworkManager.settings.modify.hostname - the fleet
  #     identifies devices by name.
  #   *.downgrade-* on fwupd - see `firmware` above.
  #
  # They are absent rather than explicitly denied. An explicit NO would
  # short-circuit polkit for administrators too.

  granted = lib.filterAttrs (name: _: cfg.options.${name} or false) rights;

  ruleFor =
    name: right:
    let
      # A real seat, in the foreground: never SSH, never a background or
      # remote session. That clause is what makes granting these safe, so it
      # is part of EVERY rule rather than something a setting can opt out of.
      guard = "subject.local && subject.active";
      # Whether a right asks for the user's OWN password is a property of the
      # RIGHT, not an operator choice. Whoever knows what a polkit action does
      # knows whether it deserves a prompt; an operator asked to pick between
      # "self" and "session" per right is being asked to know polkit.
      #
      # AUTH_SELF_KEEP asks for the user's own password and remembers it
      # briefly, so a run of related steps is not a run of dialogs. YES grants
      # outright. Machine-wide changes take the first, changes that affect the
      # user's own session take the second.
      verdict = if (right.selfAuth or false) then "AUTH_SELF_KEEP" else "YES";
      ids = lib.concatMapStringsSep "\n    " (a: ''"${a}",'') right.actions;
    in
    ''
      // ${right.label} - dawo.userRights.options.${name} = true
      polkit.addRule(function(action, subject) {
        var ids = [
          ${ids}
        ];
        if (ids.indexOf(action.id) < 0) { return polkit.Result.NOT_HANDLED; }
        if (${guard}) { return polkit.Result.${verdict}; }
        // Not this user: fall through, so the administrator path still works.
        // Returning NO here would refuse the action for EVERYONE, admins
        // included, which turns a lockdown into an outage.
        return polkit.Result.NOT_HANDLED;
      });
    '';
in
{
  options.dawo.userRights = {
    enable = lib.mkEnableOption "let ordinary users change desktop settings themselves" // {
      label = "User rights";
    };

    options = lib.mapAttrs (
      _: right:
      lib.mkOption {
        type = lib.types.bool;
        default = false;
        inherit (right) description;
        example = true;
      }
      // { inherit (right) label; }
    ) rights;
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        # Granting printer rights without CUPS authorises a user to talk to a
        # service that is not running. The setting would read as done and
        # change nothing, which is worse than refusing it: a silent no-op is
        # indistinguishable from a working feature until somebody needs it.
        assertion = !cfg.options.printers || (config.dawo.printing.enable or false);
        message = ''
          dawo.userRights.options.printers is on but dawo.printing.enable is off.
          There is no print service for the right to apply to. Turn printing on,
          or turn this right back off.
        '';
      }
    ];

    # cups-pk-helper carries the printer action ids. Without it on the system
    # path those ids exist in our rule and nowhere else, so polkit never
    # consults the rule and the right silently does nothing.
    environment.systemPackages = lib.mkIf cfg.options.printers [ pkgs.cups-pk-helper ];

    security.polkit.enable = true;
    security.polkit.extraConfig = lib.concatStringsSep "\n" (
      lib.mapAttrsToList ruleFor granted
    );
  };
}
