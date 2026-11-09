# Three ways to run Sextant

One binary, three deployments. They differ in what carries the database, the
secrets and the TLS, not in what the console does.

Every command here was run on 2026-08-22 against release 0.91.0, and the traps
are the ones that actually bit.

| | What it is | What it is for |
|---|---|---|
| `deploy/compose` | console, gate-runner, a test identity provider, a database | trying it, and testing a change, on your own machine |
| A container | one image, your database, your proxy | a single host, NixOS or anything else that runs podman |
| The Helm chart | console, gate-runner, CloudNativePG | a cluster, and the only path with HA and backups |

There is also a **NixOS module** (`nixosModules.default` in the flake) which
runs the same binary under systemd with `DynamicUser`. It is the container path
without the container; the same database and secret rules apply.

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

## 2. A container, on NixOS or anywhere

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

## 3. The Helm chart

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

The full walk-through, including the values that bite, is
[Install and configure Sextant](./deploy.md).

## What differs, and what does not

The console is the same binary in all three. What changes:

- **The database.** The compose stack starts one in a volume; the container
  and the NixOS module expect one; the chart creates one.
- **Validation.** The compose stack runs the remote gate, as production does.
  The container and the chart should use
  `--gate remote` with a nix-capable gate-runner - the console image ships no
  nix on purpose.
- **Backups.** Only the chart has an opinion, and its backup is **off** by
  default while the database holds the only copy of the LUKS recovery keys.
  Whichever path you take, that is your decision to make and not one to
  inherit.
