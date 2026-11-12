#!/usr/bin/env bash
# Make (or check) the generated half of the starter fleet repository
# (design 0014): internal/starter/overlay/flake.lock and catalog.json.
#
# They are made together because the catalog describes exactly the core the
# lock pins. A lock bumped on its own leaves the settings page describing a
# core the devices no longer run.
#
# Usage:
#   scripts/regen-starter.sh            update the lock to the newest DAWO-Core
#                                       and Sextant main, then export the catalog
#   scripts/regen-starter.sh --check    keep the committed lock; fail if the
#                                       catalog differs from a fresh export, or
#                                       if anything a device builds changed in
#                                       this repository since the Sextant
#                                       revision the lock pins
#   scripts/regen-starter.sh --local    export the catalog against THIS working
#                                       tree instead of the pinned Sextant, for
#                                       trying a change before it is on
#                                       Codeberg. Writes catalog.json only, never
#                                       the lock.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
starter="$root/internal/starter/overlay"
mode="${1:-update}"
case "$mode" in update | --check | --local) ;; *)
  echo "usage: $0 [--check | --local]" >&2
  exit 2
  ;;
esac

# The flake is evaluated from a copy: nix only sees files git tracks, and the
# copy keeps a half-made lock out of the working tree if anything fails.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cp "$starter"/. "$work"/ -r
# The catalog export does not read catalog.json; nixosConfigurations does,
# and a first run has none yet.
[ -f "$work/catalog.json" ] || echo '[]' >"$work/catalog.json"
git -C "$work" init -q
git -C "$work" add -A

override=()
case "$mode" in
update) nix flake update --flake "$work" ;;
--local) override=(--override-input sextant "path:$root") ;;
--check)
  [ -f "$starter/flake.lock" ] || {
    echo "error: internal/starter/overlay/flake.lock is missing - run scripts/regen-starter.sh" >&2
    exit 1
  }
  ;;
esac

# Sorted keys and four-space indent, like the example overlay's catalog.
nix eval --json "${override[@]}" "$work#catalog" | jq -S --indent 4 . >"$work/catalog.out"

if [ "$mode" = --check ]; then
  # git diff, not diff(1): the CI runner carries git but no diffutils.
  if ! git --no-pager diff --no-index -- "$starter/catalog.json" "$work/catalog.out"; then
    echo "error: the starter's catalog.json is stale - run scripts/regen-starter.sh" >&2
    exit 1
  fi
  # The lock can only pin a Sextant revision older than the commit holding
  # it. What a device builds must not have moved since, or a new fleet runs
  # older device code than this commit claims to ship.
  rev="$(jq -r '.nodes.sextant.locked.rev' "$starter/flake.lock")"
  if ! git -C "$root" cat-file -e "$rev^{commit}" 2>/dev/null; then
    echo "error: the starter pins Sextant $rev, which this clone does not have" >&2
    exit 1
  fi
  if ! git -C "$root" diff --quiet "$rev" HEAD -- nix deploy/nixos agent; then
    echo "error: nix/, deploy/nixos/ or agent/ changed since the Sextant revision the starter pins ($rev):" >&2
    git -C "$root" --no-pager diff --stat "$rev" HEAD -- nix deploy/nixos agent >&2
    echo "Put that change on Codeberg main, then run scripts/regen-starter.sh." >&2
    exit 1
  fi
  echo "starter: catalog matches the lock, and the pinned Sextant is current"
  exit 0
fi

cp "$work/catalog.out" "$starter/catalog.json"
echo "wrote internal/starter/overlay/catalog.json ($(jq length "$starter/catalog.json") settings)"
if [ "$mode" = update ]; then
  cp "$work/flake.lock" "$starter/flake.lock"
  echo "wrote internal/starter/overlay/flake.lock"
fi
