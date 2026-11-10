# A local administrator whose password the device makes and the console
# holds (design 0017), on a booted machine: the executor mints one at boot,
# it logs in, a rotate intent replaces it, the old one is refused, and the
# new one survives a reboot - which is the claim that the hash file keeps an
# immutable-users activation from locking the account again.
#
# Without the agent: what reaches the console is the agent's half, tested in
# agent/src. This is the half that only a real machine can show.
{ self, pkgs }:
pkgs.testers.runNixOSTest {
  name = "admin-password-on-nixos";

  nodes.machine = { config, ... }: {
    imports = [ self.nixosModules.actd ];
    services.sextant-actd = {
      enable = true;
      managedAdmin = "beheer";
    };
    # What dawo.localAdmin.managed wires on a real device.
    users.mutableUsers = false;
    users.users.beheer = {
      isNormalUser = true;
      extraGroups = [ "wheel" ];
      hashedPasswordFile = config.services.sextant-actd.adminHashFile;
    };
    # The agent's state directory, which the agent itself would create.
    systemd.tmpfiles.rules = [ "d /var/lib/sextant-agent 0700 root root -" ];
  };

  testScript = ''
    import re

    shape = re.compile(r"[A-HJ-NP-Z2-9]{5}(-[A-HJ-NP-Z2-9]{5}){3}")

    # Each attempt on a fresh virtual console, so one attempt's screen
    # cannot satisfy the next one's wait.
    def login(tty, password, works):
        if tty != 1:
            machine.send_key(f"alt-f{tty}")
            machine.wait_until_succeeds(f"[ $(fgconsole) = {tty} ]")
        machine.wait_for_unit(f"getty@tty{tty}.service")
        machine.wait_until_succeeds(f"pgrep -f 'agetty.*tty{tty}'")
        machine.wait_until_tty_matches(str(tty), "login: ")
        machine.send_chars("beheer\n")
        machine.wait_until_tty_matches(str(tty), "Password: ")
        machine.send_chars(password + "\n")
        if works:
            machine.wait_until_succeeds(f"pgrep -u beheer -t tty{tty} bash")
        else:
            machine.wait_until_tty_matches(str(tty), "Login incorrect")

    machine.wait_for_unit("multi-user.target")

    with subtest("the device makes a password at boot and leaves it for the agent"):
        machine.wait_until_succeeds("test -s /var/lib/sextant-agent/admin.key")
        first = machine.succeed("cat /var/lib/sextant-agent/admin.key")
        assert shape.fullmatch(first), f"unexpected password shape: {len(first)} characters"
        assert machine.succeed("stat -c %a /var/lib/sextant-actd/admin.hash").strip() == "600"
        machine.succeed(
            "[ \"$(getent shadow beheer | cut -d: -f2)\" = \"$(cat /var/lib/sextant-actd/admin.hash)\" ]"
        )
        login(1, first, True)

    with subtest("a rotate intent replaces it and the old one stops working"):
        machine.succeed("mkdir -p /run/sextant-intent && touch /run/sextant-intent/rotate-admin.intent")
        machine.wait_until_succeeds("[ \"$(cat /var/lib/sextant-agent/action.ack)\" = admin-rotated ]")
        second = machine.succeed("cat /var/lib/sextant-agent/admin.key")
        assert shape.fullmatch(second) and second != first, "the rotation did not draw a new password"
        login(2, first, False)
        login(3, second, True)

    with subtest("the password survives a reboot"):
        machine.shutdown()
        machine.start()
        machine.wait_for_unit("multi-user.target")
        assert machine.succeed("cat /var/lib/sextant-agent/admin.key") == second, "a reboot drew a new password"
        login(1, second, True)
  '';
}
