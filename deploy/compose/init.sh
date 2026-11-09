#!/bin/sh
# One-shot before the stack starts: the secrets and the fleet repository a
# fresh stack needs. Runs on every `up` and leaves whatever exists alone, so
# a restart keeps the keys that sealed what is already in the database.
set -eu
umask 077
mkdir -p /secrets /data /shared

hex() { od -An -tx1 -N32 /dev/urandom | tr -d ' \n'; }
b64() { head -c32 /dev/urandom | base64; }
# Hex where the value lands in a URL or a header (base64 has / and +);
# base64 where Sextant expects base64 of 32 bytes.
for name in pg gate dex-client checkin; do
  [ -s "/secrets/$name" ] || hex > "/secrets/$name"
done
for name in session secret; do
  [ -s "/secrets/$name" ] || b64 > "/secrets/$name"
done

# The fleet repository: the example overlay, its own repository, with the
# Sextant flake taken from a published release instead of the checkout
# around it (a path input cannot leave the repository it sits in, #74).
# A bare repository on the shared volume plays the git server: the console
# pushes its change branches there and the gate-runner fetches them.
if [ ! -d /shared/fleet.git ]; then
  rm -rf /tmp/seed
  cp -r /overlay-src /tmp/seed
  cd /tmp/seed
  rm -f flake.lock regen-catalog.sh
  sed -i "s|sextant.url = \"path:../..\";|sextant.url = \"${SEXTANT_FLAKE}\";|" flake.nix
  grep -qF "$SEXTANT_FLAKE" flake.nix || { echo "init: could not point flake.nix at $SEXTANT_FLAKE" >&2; exit 1; }
  git init -q -b main
  git add -A
  git -c user.name=sextant -c user.email=sextant@localhost commit -qm "Start the fleet repository"
  git clone -q --bare /tmp/seed /shared/fleet.git
  rm -rf /tmp/seed
fi
[ -d /data/overlay/.git ] || git clone -q file:///shared/fleet.git /data/overlay

chown -R 65532:65532 /secrets /data /shared
# The other services wait for this file rather than for the init container:
# podman-compose turns depends_on into "must be running", which a one-shot
# container that already finished never is.
touch /secrets/.ready
echo "init: ready"
