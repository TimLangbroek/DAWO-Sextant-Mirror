# Install and configure Sextant

Git is not an integration in Sextant - it is the storage. A fleet's whole
configuration is **data in a git overlay repository** (`fleet.json`, policies,
overlays). Sextant reads and writes that repository directly: every change is
an audited commit pushed to the remote, and the remote is the source of truth.
So "connecting Sextant to git" means pointing the console at your overlay repo;
there is nothing else to wire.

One binary, four ways to run it. They differ in what carries the database,
the secrets and the TLS, not in what the console does. This page helps you
pick one and get it started, then covers what every path needs after that:
the fleet repository, sign-in, the validation gate, and the settings that
bite.

Every command here was run on 2026-08-22 against release 0.91.0, and the traps
are the ones that actually bit.

## Pick a way to run it

| | What it is | What it is for |
|---|---|---|
| `deploy/compose` | console, gate-runner, a test identity provider, a database | trying it, and testing a change, on your own machine |
| The NixOS module | the console under systemd, PostgreSQL on the same host | a NixOS server, which a NixOS fleet tends to have already |
| A container | one image, your database, your proxy | a single host that runs podman or docker |
| The Helm chart | console, gate-runner, CloudNativePG | a cluster, and the only path with HA and backups |

The NixOS module's first start, from nothing configured to a signed-in owner,
is a VM test (`checks.setup-on-nixos`), so it is walked on every change.

## What you need

- **An overlay repository** - a git repo that consumes a NixOS core flake and
  holds your `fleet.json`. One repo per organisation (tenant). It is the same
  repo the devices follow via comin.
- **Postgres** - the observed plane (check-ins, tokens, image jobs, prefs). A
  single instance next to the console is fine; it partitions per tenant.
- **An OIDC identity provider** - console login (one IdP per instance), mapped
  to roles by group. Optional LDAP for the group-picker source.
- **A validation gate** - `eval` runs nix in-process; `remote` delegates to a
  small nix-capable gate-runner (the console image itself ships no nix). Use
  `remote` in production; it is fail-closed, and it is the chart's default.

## Before you start

| | |
|---|---|
| Go | 1.25 or newer, to build from source |
| PostgreSQL | The chart brings its own via CloudNativePG, `deploy/compose` and the NixOS module start one. The container expects one |
| Kubernetes | any recent version; the chart uses no alpha APIs. Helm 3 |
| CloudNativePG | the operator must be installed before the chart, which creates a `postgresql.cnpg.io/v1` Cluster. Skip it with `cnpg.enabled: false` and bring your own database |
| Nix | only on the gate-runner. The console image ships none, deliberately |

To see it working before you deploy anything, `deploy/compose` starts a
console, a database, the validation gate and a test identity provider on
your own machine; see [`deploy/compose`](#1-deploycompose) below.

## First start: the setup page

A console that starts with no way to sign in - no identity provider and no
`--dev-auth` - opens its setup page instead of a login nobody can pass
(design 0013). It writes one line to its log, with a link that works once:

```
level=WARN msg="setup: open http://127.0.0.1:8080/setup?token=..."
```

The address is the console's public URL when one is set, otherwise the
address it listens on (`localhost` when that is all interfaces).

Whoever can read the log can set the console up; everybody else is told to
wait. On NixOS, `services.sextant.enable = true;` with nothing else is
enough to get there, and `journalctl -u sextant | grep setup:` shows the
link. In a container, give it a volume at `/data`, where setup writes its
answers.

The page asks, in this order:

1. **Keys.** Made for you. The secret key comes as a file to keep somewhere
   the server's own failure cannot reach: the database holds the disk
   recovery keys sealed with it, and a restored database cannot be read
   without it. Setup goes on once you say you stored it.
2. **Fleet repository.** "Create one for me" starts it in the data
   directory, or give the address and a token of one you have, with a Test
   button that checks the console can reach it with that token.
3. **Sign-in.** The issuer, the client id and the client secret of your
   identity provider. The page shows the redirect address to register
   there. **Test sign-in** runs a real login and shows what came back,
   including the groups it found.
4. **Done.** The console checks that it will start with these answers,
   restarts itself, and the person who did the test sign-in is the owner.
   No group has to be mapped first.

What a deployment sets itself - a NixOS option, a Helm value, an environment
variable - is not asked again, and the page says it was set by the
deployment.

The NixOS module starts the console read-only (`write = false`), and every
page says so. A change goes through a validation gate that builds the
configuration of the devices it touches from the fleet repository, so set
`services.sextant.write = true;` once that repository has a flake that
builds them. The repository setup creates holds only `fleet.json` so far.

## 1. `deploy/compose`

```sh
git clone https://codeberg.org/DAWO/DAWO-Sextant.git && cd DAWO-Sextant/deploy/compose
podman-compose up -d
```

Console on **http://127.0.0.1:8080**. Choose **Sign in with SSO**, then **a
test user**, and you are owner. Every change you save goes through the
remote validation gate, as it does in production.

It brings its own identity provider (Dex, with one test person and no
password), so it is for your own machine only. The
[compose README](https://codeberg.org/DAWO/DAWO-Sextant/src/branch/main/deploy/compose/README.md)
says what runs and how to start over.

## 2. The NixOS module

`nixosModules.default` in the flake runs the console under systemd with
`DynamicUser`, and starts PostgreSQL on the same host
(`database.createLocally`, on by default). One line is enough to begin:

```nix
services.sextant.enable = true;
```

That console has nothing to sign in with, so it opens the setup page above;
`journalctl -u sextant | grep setup:` gives the link. Whatever you set in
the module is not asked again. Set it all and setup never appears:

```nix
services.sextant = {
  enable = true;
  gitRemote.url = "https://codeberg.org/acme/fleet.git";
  # A netrc line for a credential that may push. A string, not a path
  # literal, so the token never lands in the Nix store.
  gitRemote.netrcFile = "/run/secrets/sextant-netrc";
  write = true;
  gate = "eval"; # this host has nix; the gate runs here
  publicUrl = "https://console.example.org"; # your TLS proxy in front
  oidc.issuer = "https://id.example.org"; # secret: SEXTANT_OIDC_CLIENT_SECRET
  oidc.clientId = "sextant";
  ownerGroups = [ "it-admins" ];
  environmentFile = "/run/secrets/sextant.env"; # the keys; PostgreSQL runs on this host
};
```

The fleet repository is one setting: give it an empty repository on any
https forge and it is filled with a starter fleet before the console first
starts.

## 3. A container, on any host

```sh
podman run -d --name sextant --network host \
  -e SEXTANT_PG_DSN="postgres://sextant@127.0.0.1:5432/sextant?sslmode=disable" \
  -e SEXTANT_CHECKIN_TOKEN="…" \
  -e SEXTANT_SECRET_KEY="$(head -c 32 /dev/urandom | base64)" \
  -v /srv/sextant/overlay:/data/overlay:z \
  git.edano.eu/bb-open/sextant:0.93.0 \
  --addr 127.0.0.1:8080 --repo /data/overlay --write --gate remote \
  --gate-url https://gate.example.org
```

**The image runs as uid 65532 and will not start on a volume it cannot write.**
The state directory defaults to `<repo>/.sextant-state`, so a bind mount owned
by your own user fails with:

```
sextant: state dir: mkdir /data/overlay/.sextant-state: permission denied
```

Chown the volume to that uid before the first run (`podman unshare chown -R
65532:65532 /srv/sextant/overlay` for a rootless podman), or point `--state-dir`
at a volume that is writable.

**`--dev-auth` only works on loopback**, which inside a container is the
container's own. `--network host` is why the example above works; a real
deployment uses an IdP and does not need it.

Verified on 0.91.0: five capabilities mounted, `/station` answered 200, and a
setting saved in the console landed in `fleet.json` inside the mounted overlay.

Put TLS in front of it and set `--public-url` to the https address. Behind
TLS on a non-loopback address the console refuses to ship session cookies
without the Secure flag, deliberately; an https public URL turns it on.

## 4. The Helm chart

```sh
kubectl create namespace sextant
kubectl -n sextant create secret generic sextant \
  --from-literal=SEXTANT_CHECKIN_TOKEN='…' \
  --from-literal=SEXTANT_SECRET_KEY="$(head -c 32 /dev/urandom | base64)" \
  --from-literal=SEXTANT_SESSION_KEY="$(head -c 32 /dev/urandom | base64)" \
  --from-literal=SEXTANT_OIDC_CLIENT_SECRET='…'
helm install sextant ./deploy/helm -n sextant -f my-values.yaml
```

`my-values.yaml` needs at least `gitRemote.url`: the chart refuses to render
without a fleet repository. An empty one is fine; the chart fills it before
the console starts.

This is the only path that brings its own database: the chart creates a
CloudNativePG cluster and wires the console to it, so `SEXTANT_PG_DSN` is not
in that secret. It needs the CloudNativePG operator installed first.

The walk-through, including the values that bite, is
[First deploy, end to end](#first-deploy-end-to-end) below.

## Connect the console to your overlay repo

Point the console at the repo and a push remote:

```
--repo /data/overlay        # the working tree the console edits
--git-remote origin         # the push remote (HA source of truth)
```

Under Helm (`deploy/helm`), the same as values. The chart requires
`gitRemote.url` and refuses to render without it:

```yaml
gitRemote:
  url: https://github.com/your-org/fleet.git   # any https forge
  branch: main
  netrcSecret: sextant-overlay-netrc   # a credential that may push
gateMode: remote                       # or eval / none
oidc:
  issuer: https://id.example.com
  clientId: "<client-id>"
publicUrl: https://console.example.com # empty: https://<ingress.host>
```

Register `<publicUrl>/callback` as the redirect URL at the identity provider.

The repository may be empty. Before the console starts, `sextant
prepare-repo` clones it and, when there is nothing in it yet, pushes a
starter fleet; a credential that can read but not push fails that step with
a message that says so. On every later start it only fetches.

The console clones the repo, keeps its snapshot in sync (the remote is
authoritative - commits made by engineers or CI show up without a restart),
and the devices follow the same repo: comin on each device tracks
`rings/<group>` (or `main`), so a rollout that advances a ring branch lands on
the devices in that ring.

## Settings that are environment-only

Secrets never go in the fleet document or in Helm values as plain text. These
are read from the environment, and the chart mounts them from one secret
(`secretName`):

| Variable | What it is |
|---|---|
| `SEXTANT_PG_DSN` | The observed plane's database. The chart sets it for you from the CloudNativePG cluster it creates (`cnpg.enabled: true`); supply it yourself only when you point at your own Postgres with `cnpg.enabled: false`. **Without it the console starts anyway** and mounts three capabilities instead of five: no device status, no compliance verdicts, and `/station` answers 503. That is a working config plane and half a product, so check it on a first deploy rather than wondering later. |
| `SEXTANT_CHECKIN_TOKEN` | The shared token devices present when they check in. The agent carries the same value. |
| `SEXTANT_SECRET_KEY` | Base64 of exactly 32 bytes. Seals typed secrets (SMTP passwords, LUKS recovery keys) at rest. Without it those features disable themselves and say so, rather than storing anything in the clear. |
| `SEXTANT_SESSION_KEY` | Base64 of exactly 32 bytes. Seals session cookies. **Required** as soon as an OIDC issuer is set: the console refuses to start without it rather than fall back to something weaker. Keep the same value across replicas and restarts, or everyone is logged out. |
| `SEXTANT_API_TOKEN` | Bearer token for the machine API; what that API offers is in the [API reference](../reference/api.md). |
| `SEXTANT_OIDC_CLIENT_SECRET` | The console's OIDC client secret. |
| `SEXTANT_LDAP_BIND_PASSWORD` | Bind password, when LDAP supplies the group picker. |

Every flag has an environment equivalent under the same prefix
(`SEXTANT_SECURE_COOKIES`, `SEXTANT_TRUST_PROXY`, `SEXTANT_SHUTDOWN_GRACE`,
and so on); `sextant --help` lists the flags.

## Keep the secret key apart

`SEXTANT_SECRET_KEY` seals the LUKS recovery keys the console holds. The
database backup carries the sealed keys and never the key that opens them,
so a backup without the key restores ciphertext nobody can read. Keep a copy
of the key somewhere the console's own failure cannot take with it: a
password manager or a safe, not a Secret in the same cluster. The setup page
offers the key it makes as a file for exactly that.

At start the console tries the key on every recovery key it holds. If it
cannot open some, every page says how many and names the key that sealed
them; the log says the same. Put that key back, after the current one:

```text
SEXTANT_SECRET_KEY=<current key>,<the key that sealed them>
```

The first key seals; every key listed opens. That is also how to rotate:
put the new key first and keep the old one after it.

## First deploy, end to end

1. Create a repository on your forge (GitHub, Codeberg, GitLab, Forgejo). Under
   Helm it may stay empty: the chart fills it with a starter fleet. Put a
   netrc with a credential that may push in a Secret for `gitRemote.netrcSecret`.
2. Deploy the console with [the Helm chart](#4-the-helm-chart), as above.

   The chart creates a CloudNativePG cluster and wires the console to it, so
   the DSN is not in that secret. Its backup is **off** by default, because a
   default pointing at an object store this chart cannot assume would fail
   every install - and this database holds the only copy of the LUKS recovery
   keys, since a device erases its own once the console acknowledges it. Turn
   `cnpg.backup` on before the fleet grows.

   Or the container, or the NixOS module, with the
   `gitRemote`, `gateMode`, `oidc` and Postgres settings above. Behind TLS,
   set `--public-url https://...` (or `--secure-cookies`): the console refuses
   session cookies without the Secure flag on a non-loopback address, and an
   https public URL turns that flag on.
3. Under Helm the gate-runner comes with `gateMode: remote`, the default, and
   shares a bearer token with the console that the chart generates and keeps
   (a `GATE_TOKEN` in your secret wins, if you set one). Elsewhere, run one
   beside the console. It keeps a warm clone of the overlay and evaluates
   each candidate before the console commits.
4. Log in via your IdP. Enroll a device, assign it to a group, edit settings -
   every change passes the gate and commits to the overlay. Stage a rollout to
   land updates in waves.

## Build-before-promote

At scale, ring promotion should not mean 10,000 devices each independently
compiling the same closures on weak edge hardware. With `gateMode: remote`
and a gate-runner `cache` configured (a signing key secret, and optionally a
dedicated cache host), a rollout's wave builds its release into that signed
binary cache *before* its ring branch flips - devices then substitute
(download) the pre-built closure instead of building it. Enable the console
side with `releaseCache: true`. See
[Scaling to 10,000+ devices](../architecture/scale.md) for the reasoning, and
[Ship an update](./updates.md) for what a wave's **Building** status means
day to day.

## Notification e-mail (SMTP)

In-app notifications work with no extra setup. To also deliver them by mail,
an owner configures SMTP per organisation under **E-mail (SMTP)** in the
console (host, port, from, security). The password is set one of two ways:

- **A secret reference** (recommended) - enter the *name* of a secret; the
  value lives in agenix or a cluster Secret mounted at `SECRET_DIR` (default
  `/run/secrets/<name>`). Sextant reads only the name, never storing the value.
- **A typed password** - available only when `SEXTANT_SECRET_KEY` is set (a
  base64 32-byte key). The password is then sealed (AES-256-GCM) and stored in
  Postgres. Without the key this option is disabled and the console says so.

Both can also be set at deploy time. `SEXTANT_SECRET_KEY` is an environment-only
secret; add it to the same secret the chart mounts (`secretName`).

Multi-tenant (model B): one overlay repo per organisation, isolated stores,
one console instance per repo. See `docs/adr/` for the decisions behind this.

## What differs between them

The console is the same binary in all four. What changes:

- **The database.** The compose stack starts one in a volume, the NixOS
  module runs one on the host (`database.createLocally`, on by default), the
  chart creates one; the container expects one.
- **Validation.** The compose stack runs the remote gate, as production does.
  The container and the chart should use
  `--gate remote` with a nix-capable gate-runner - the console image ships no
  nix on purpose.
- **Backups.** Only the chart has an opinion, and its backup is **off** by
  default while the database holds the only copy of the LUKS recovery keys.
  Whichever path you take, that is your decision to make and not one to
  inherit.

## Troubleshooting

**Check before you restart.** `sextant check-config` with the same flags and
environment as the console says what it would run with (address, sign-in,
fleet repository, gate, database) or lists every reason it would refuse,
each with what to do about it. It starts nothing and prints no secret.

**Helm refuses the values: "additional properties ... not allowed".**
The chart's schema names every value it reads and refuses the rest, so a
misspelled key stops the install instead of leaving a default in place. The
message names the key and where it sits. Correct it, or remove it if the
chart stopped reading it in an upgrade. A Flux HelmRelease shows the same
message in its status.

**The console refuses to start / refuses session cookies.**
Behind TLS on a non-loopback `--addr`, Sextant refuses to ship session
cookies without the Secure flag - this is deliberate fail-closed behaviour,
not a bug. Set `--public-url` to the https address people use, which turns
the flag on, or set `--secure-cookies` (`SEXTANT_SECURE_COOKIES=true`). A
console reached only at a plain-http loopback address
(`--public-url http://localhost:8080`, a container with its port published
on the host's loopback) needs neither.

**Sign-in says the identity provider has not answered.**
The console asked the provider's discovery address (`<issuer>/.well-known/openid-configuration`)
at start and got no answer. It keeps asking every 30 seconds and signs
people in as soon as the provider answers, without a restart; until then
`/readyz` reports not ready and the log has the provider's error. A
provider that starts after the console, as after a cluster restart, needs
nothing from you. If it lasts, check the issuer address from the console's
own host.

**Every write is refused with `gateMode: remote`.**
The gate is fail-closed: no reachable gate-runner means no writes, by
design. On a first install the runner waits for the repository the console
fills, so writes start working a minute or so after the console is up.
Moving an existing install to `remote`: set `gateRunner.enabled: true`
first, check the gate-runner's `/healthz`, then flip `gateMode`.

**A rollout wave never leaves "Building".**
With build-before-promote enabled (`releaseCache: true` +
`gateRunner.cache`), check the gate-runner's cache is healthy and its signing
key secret is present - a wave cannot promote until its release lands in the
signed cache.

**Devices check in but the console shows no status, and /station is 503.**
The observed plane has no database. The console mounts what it can and keeps
going rather than refusing to start, so this looks like a product missing
features rather than a missing setting. Check `SEXTANT_PG_DSN` reached the
pod, and that the CloudNativePG cluster is `Ready` if the chart created one.

**SMTP is configured but no mail arrives.**
Confirm the password resolved: a secret reference must exist under
**Secrets** with a value the runtime can actually read (agenix or the
mounted `SECRET_DIR`); a typed password needs `SEXTANT_SECRET_KEY` set on
the deployment, or the console disables that option outright.
