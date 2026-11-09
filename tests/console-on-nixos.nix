# Sextant on NixOS, in a VM that boots the module and uses the console.
#
# The path exists so somebody can run their own fleet from a single NixOS
# host. Nothing proved that it worked: the module used to pass only --addr,
# which started a console that could not be given a repository or the write
# path, and the flake had no test that would have noticed.
{ self, pkgs }:
pkgs.testers.runNixOSTest {
  name = "sextant-console-on-nixos";

  nodes.machine = { config, pkgs, ... }: {
    imports = [ self.nixosModules.default ];

    services.sextant = {
      enable = true;
      addr = "127.0.0.1:8080";
      # A fixed user, because the console WRITES this repository and the
      # operator has to be able to prepare it. With a DynamicUser the uid does
      # not exist until the unit starts, and StateDirectory then lives under
      # /var/lib/private where git refuses to work on a tree it does not own.
      # That is what the first version of this test hit.
      user = "sextant";
      repo = "/srv/sextant/overlay";
      # The fleet repository, as an operator gives it: its address.
      # file:// stands in for a forge; prepare-repo clones it before the
      # console starts and the console pushes back to it.
      gitRemote.url = "file:///srv/fleet.git";
      gitRemote.netrcFile = "/etc/sextant-netrc";
      write = true;
      # A VM has no gate-runner and cannot evaluate this overlay, so the test
      # says so out loud rather than pretending: this is the acknowledged
      # dev combination, and the module's assertion is what forces the
      # acknowledgement.
      gate = "none";
      allowUnvalidated = true;
      secureCookies = false;
      extraArgs = [ "--dev-auth" ];
      environmentFile = "/etc/sextant.env";
      # No database settings: the module runs PostgreSQL on this host and
      # connects over the socket, which /station below depends on.
    };

    environment.etc."sextant.env".text = ''
      SEXTANT_CHECKIN_TOKEN=vm-test
    '';

    # A stand-in credential: file:// needs none, but the unit must still
    # hand it to git through HOME.
    environment.etc."sextant-netrc" = {
      text = "machine forge.invalid login vm password vm-test\n";
      mode = "0400";
    };

    # The stand-in forge is a directory on this machine, so the unit needs to
    # write to it; a real forge is reached over https. "-": it does not
    # exist until the test script makes it.
    systemd.services.sextant.serviceConfig.ReadWritePaths = [ "-/srv/fleet.git" ];

    environment.systemPackages = [ pkgs.git pkgs.curl pkgs.jq ];
  };

  testScript = ''
    machine.wait_for_unit("postgresql.service")

    # The fleet repository on its forge. The example overlay, because the
    # write below needs a catalog to write against. The service has been
    # retrying since boot, because there was nothing at the address yet.
    machine.succeed(
        "git init -q --bare -b main /srv/fleet.git && "
        "cp -r ${self}/examples/overlay /tmp/fleet && chmod -R u+w /tmp/fleet && "
        "cd /tmp/fleet && git init -q -b main && "
        "git -c user.name=t -c user.email=t@t add -A && "
        "git -c user.name=t -c user.email=t@t commit -qm seed && "
        "git push -q /srv/fleet.git main && "
        "chown -R sextant:sextant /srv/fleet.git"
    )
    seeded = machine.succeed("git --git-dir=/srv/fleet.git rev-parse main").strip()

    machine.succeed("systemctl restart sextant.service")
    machine.wait_for_unit("sextant.service")
    machine.wait_for_open_port(8080)

    # prepare-repo cloned it into the directory the module made, before the
    # console started.
    head = machine.succeed("runuser -u sextant -- git -C /srv/sextant/overlay rev-parse HEAD").strip()
    assert head == seeded, f"overlay at {head}, remote at {seeded}"
    # The credential reached git's HOME as a link into the unit's
    # credentials, readable by the service and gone with the unit.
    machine.succeed("runuser -u sextant -- grep -q forge.invalid /run/sextant/.netrc")
    machine.succeed("test -L /run/sextant/.netrc")

    # It answers, and it answers with a real page rather than an error shell.
    machine.succeed("curl -fsS http://127.0.0.1:8080/devices | grep -q '</html>'")

    # The observed plane is wired: without a database this endpoint is a 503.
    machine.succeed("curl -fsS -o /dev/null http://127.0.0.1:8080/station")

    # And the write path actually writes. This is the half the old module
    # could not do at all. The org-level value, not just the word: the
    # example fleet already has gnome on a group, and a grep for it passed
    # whether or not anything was written.
    org_desktop = "jq -e '.org.settings.desktop == \"gnome\"'"
    machine.fail(f"{org_desktop} /srv/sextant/overlay/fleet.json")
    machine.succeed(
        "curl -fsS -c /tmp/c -o /dev/null http://127.0.0.1:8080/settings && "
        "curl -fsS -b /tmp/c -o /dev/null -X POST http://127.0.0.1:8080/settings "
        "--data-urlencode csrf=dev-csrf --data-urlencode scope=org "
        "--data-urlencode v:desktop=gnome"
    )
    machine.succeed(f"{org_desktop} /srv/sextant/overlay/fleet.json")
    # ...and pushes it to the fleet repository, which is what devices follow.
    machine.wait_until_succeeds(
        f"runuser -u sextant -- git --git-dir=/srv/fleet.git show main:fleet.json | {org_desktop}",
        timeout=60,
    )
  '';
}
