# 0013 - Setup mode: a console that configures itself on first start

Status: accepted 2026-11-09 (Bram: "stupid simple bij v1"), building now.

## Problem

Getting from an installed binary to a first login takes 15 to 20 settings on
five surfaces: a forge repo laid out by hand, three secrets generated with
`head -c32 | base64`, an OIDC client, a values file or a flag list, and the
console's own pages. Only the Helm chart seeds a starting repo. The NixOS
module and the container start half-configured, and a missing piece shows up
as a refusal written for someone who already knows the answer
(`refusing --write with --gate none`, `oidc needs --oidc-client-id ...`).

A newcomer should reach a working login in fifteen minutes and type at most
five things.

## Decision

A console that starts without a way to sign in starts in **setup mode**, and
setup mode asks for what is missing in a browser.

### When setup mode starts

Neither OIDC nor dev auth is configured. That is the one state in which the
console is unusable today, so nothing that works now changes: every existing
deployment sets one of the two and starts exactly as before.

### Where the answers go

A data directory (`--data-dir`, `SEXTANT_DATA_DIR`; default
`/var/lib/sextant` on NixOS and in the container, `./sextant-data` when run by
hand) holds two files the console writes itself:

- `settings.json`: what setup asked for (repo, OIDC issuer and client id,
  public URL, owner). Mode 0600.
- `keys.json`: the generated secrets (session key, secret key, check-in
  token, OIDC client secret). Mode 0600, never shown again after setup except
  the secret key, which setup offers as a recovery file (issue #7).

Precedence becomes **flags > environment > data directory > defaults**. A
value set by the deployment always wins and is shown in setup as "set by the
deployment", read-only. So Helm values and NixOS options stay authoritative,
and the file only fills what nobody set.

Secrets keep their rule: never from flags. Environment or `keys.json` only.

### Who may run setup

Setup mode prints a one-time URL to the log at start:
`setup: open http://<addr>/setup?token=<random>`. The token is held in
memory, compared in constant time, and dies with the process. Without it,
every page answers "this console is being set up; the link is in its log". On
a loopback address the token is still required: a browser on the same
machine is not the only process that can reach loopback.

### The steps

1. **Keys.** Generated, written to `keys.json`. The page offers the secret
   key as a download with "I have stored it" before it lets you continue,
   because the database holds the only escrow of the disk keys and a restore
   without this key cannot read them.
2. **Fleet repository.** "Create one for me" seeds a minimal overlay in the
   data directory (the same seed on every install path, replacing the chart's
   shell init container), or "use an existing one": URL and token, with a
   Test button that clones.
3. **Sign-in.** Issuer, client id, client secret. The page shows the redirect
   URL to paste into the identity provider. "Test sign-in" runs a real login
   and shows the claims that came back, including which groups it found. The
   person who completes the test becomes the owner (recorded by subject), so
   no group mapping is needed to get in. A string-valued groups claim counts
   as one group (issue #180).
4. **Done.** The console writes the files and restarts itself (re-exec), and
   the page waits for `/readyz` and sends you to sign in.

Database and gate are not steps. The database comes with the install path
(the NixOS module and the compose file start one; the chart already has
`database.mode`), and the gate defaults to the gate-runner that each path
ships. Setup shows their state and says what to change if one is missing.

### Public URL

The console's own address is one setting (`publicUrl`), asked in step 3 and
defaulting to the address the browser used. The OIDC redirect URL and the
notification links derive from it. Today the chart derives the redirect URL
from `ingress.host` even when the ingress is off, which ties sign-in to a
value that looks unused.

## Not doing

- Editing the configuration from the console after setup. A setting set by
  the deployment is changed in the deployment; this is a first-start screen,
  not a second control plane next to Helm values and NixOS options.
- A built-in identity provider. Every organisation that runs Sextant has one,
  and a second user store is a second thing to secure.
- Setup over the API. It is a page a person uses once.

## Done when

On each install path (NixOS module, container, chart), starting with nothing
configured and following the page reaches a signed-in owner in at most five
typed values, proven by a NixOS VM test that drives the page with a test
identity provider.
