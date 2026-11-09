# Sextant on one machine

A console you can sign in to, with the remote validation gate behind it,
from one command. For trying Sextant and for testing a change to it. Not for
running a fleet: the identity provider lets anyone who reaches it in as
owner.

```sh
cd deploy/compose
podman-compose up -d          # or: docker compose up -d
```

Open <http://127.0.0.1:8080>, choose **Sign in with SSO**, then **a test
user**. You are signed in as owner.

The first `up` builds the console and the gate-runner from this checkout,
which takes a few minutes. The first change you save takes longer than the
ones after it: the gate-runner fetches nixpkgs before it can evaluate
anything.

## What runs

| Service | Address | What it is |
|---|---|---|
| `sextant` | 127.0.0.1:8080 | The console, writing, with `--gate=remote` |
| `gate-runner` | 127.0.0.1:8090 | Evaluates every change before it can merge |
| `dex` | 127.0.0.1:5556 | A test identity provider with one person in it |
| `postgres` | 127.0.0.1:55432 | Device state, changes, sealed secrets |
| `init` | | Runs once per `up`: secrets and the fleet repository |

Everything uses the host network and listens on 127.0.0.1, so the browser
and the console reach the identity provider at the same address. That is the
whole reason for the host network: an issuer the console calls `dex` and the
browser calls `localhost` is two issuers to OIDC.

## The fleet repository

`init` makes one from `examples/overlay`, with the Sextant flake taken from
a published release instead of the checkout around it. It sits as a bare
repository on the `shared` volume, which plays the git server: the console
pushes its change branches there and the gate-runner fetches them from it.
Your edits in the console land as commits in that repository.

## Secrets

`init` generates them on the first `up` into the `secrets` volume. They are
handed to each service through its environment, never as flags, and never
leave the volume. A later `up` keeps them, so whatever they already sealed
stays readable.

## Starting over

```sh
podman-compose down -v
```

removes every volume: the database, the repository and the keys.

## From here to a real install

Replace `dex` with your own identity provider (issuer, client id and
secret, and the group that should be owner), put the console behind TLS
with `--secure-cookies`, and point the repository at your own git server.
The [deployment paths](../../docs/handbook/src/operators/deployment-paths.md) cover the NixOS
module and the Helm chart.
