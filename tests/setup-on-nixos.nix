# First start on NixOS (design 0013): the module with nothing configured,
# a test identity provider next to it, and the setup pages followed from
# the one-time link to a signed-in owner.
#
# This is 0013's "done when" for the NixOS path. What it counts is what a
# person types: the issuer, the client id and the client secret. The
# console's own address is filled in from the browser, the keys are made
# for you, and the fleet repository is one button.
{ self, pkgs }:
let
  issuer = "http://127.0.0.1:5556/dex";
  console = "http://127.0.0.1:8080";
in
pkgs.testers.runNixOSTest {
  name = "sextant-setup-on-nixos";

  nodes.machine = { pkgs, ... }: {
    imports = [ self.nixosModules.default ];

    # Nothing about sign-in, keys or the fleet repository: that is what
    # setup is for. Plain http is allowed on this machine only, which is
    # where the whole test runs.
    services.sextant = {
      enable = true;
      secureCookies = false;
    };

    # Dex as the organisation's identity provider, with one person in it.
    # The hash is bcrypt("password"), the value from Dex's own example
    # configuration; the client secret is a test value.
    services.dex = {
      enable = true;
      settings = {
        inherit issuer;
        storage.type = "memory";
        web.http = "127.0.0.1:5556";
        enablePasswordDB = true;
        staticPasswords = [{
          email = "owner@example.org";
          hash = "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W";
          username = "owner";
          userID = "08a8684b-db88-4b73-90a9-3cd1661f5466";
        }];
        staticClients = [{
          id = "sextant";
          name = "Sextant";
          secret = "test-client-secret";
          redirectURIs = [ "${console}/callback" ];
        }];
        oauth2.skipApprovalScreen = true;
      };
    };

    environment.systemPackages = [ pkgs.curl ];
  };

  testScript = ''
    machine.wait_for_unit("dex.service")
    machine.wait_for_open_port(5556)
    machine.wait_for_unit("sextant.service")
    machine.wait_for_open_port(8080)

    # Nobody can sign in, so the console is in setup mode: everyone is told
    # to wait, and the log holds the one link that opens setup.
    machine.succeed("test \"$(curl -s -o /dev/null -w '%{http_code}' ${console}/)\" = 503")
    link = machine.wait_until_succeeds(
        "journalctl -u sextant.service -o cat "
        "| grep -o '${console}/setup?token=[A-Za-z0-9_-]*' | tail -n1 | grep ."
    ).strip()

    jar = "curl -fsS -b /tmp/setup -c /tmp/setup"
    machine.succeed(f"{jar} -o /dev/null -L '{link}'")
    # Without the link's cookie, setup stays shut.
    machine.fail("curl -fsS -o /dev/null ${console}/setup")

    # Keys: made for you. The secret key comes as a file, and setup goes on
    # only once you say you stored it.
    recovery = machine.succeed(f"{jar} ${console}/setup/recovery-key")
    assert "SEXTANT_SECRET_KEY=" in recovery, "no secret key in the recovery file"
    machine.fail(f"{jar} -o /dev/null ${console}/setup/keys --data stored=")
    machine.succeed(f"{jar} -o /dev/null ${console}/setup/keys --data stored=yes")

    # Fleet repository: "create one for me" is one button.
    machine.succeed(f"{jar} -o /dev/null ${console}/setup/repo --data choice=new")

    # Sign-in: the three values a person types. The address field arrives
    # filled in with the address the browser used.
    page = machine.succeed(f"{jar} ${console}/setup")
    assert 'name="public_url" type="url" value="${console}"' in page, "the console's address is not filled in"
    page = machine.succeed(
        f"{jar} ${console}/setup/signin --data action=check "
        "--data-urlencode public_url=${console} "
        "--data-urlencode issuer=${issuer} "
        "--data-urlencode client_id=sextant "
        "--data-urlencode client_secret=test-client-secret"
    )
    assert "/setup/signin/start" in page, "the provider was not accepted"

    def sign_in(cookies, start):
        # The provider's login form, then back to the console. Dex has one
        # connector, so the start lands on its form directly.
        form = machine.succeed(f"curl -fsS -b {cookies} -c {cookies} -L -o /dev/null -w '%{{url_effective}}' {start}")
        assert "/dex/auth/local" in form, f"not on the provider's login form: {form}"
        return machine.succeed(
            f"curl -fsS -b {cookies} -c {cookies} -L '{form}' "
            "--data-urlencode login=owner@example.org --data-urlencode password=password"
        )

    # The test sign-in: a real login at the provider, and the console says
    # who came back.
    sign_in("/tmp/setup", "${console}/setup/signin/start")
    page = machine.succeed(f"{jar} ${console}/setup")
    assert "owner@example.org" in page, "setup does not show who signed in"

    # Done: the console writes the answers and starts again in place, now
    # with sign-in.
    machine.succeed(f"{jar} -o /dev/null ${console}/setup/signin --data action=finish")
    machine.wait_until_succeeds("curl -fsS ${console}/login | grep -q /login/start", timeout=60)
    machine.succeed("systemctl is-active sextant.service")

    # And the person who did setup signs in and is the owner: no group was
    # mapped, the subject was recorded.
    home = sign_in("/tmp/owner", "${console}/login/start")
    assert "/login/start" not in home, "sent back to the sign-in page after signing in"
    # Service accounts is for owners only: a 403 here fails the test.
    machine.succeed("curl -fsS -b /tmp/owner -o /dev/null ${console}/service-accounts")

    # The link has done its work: setup is gone.
    machine.fail(f"curl -fsS -o /dev/null '{link}'")
  '';
}
