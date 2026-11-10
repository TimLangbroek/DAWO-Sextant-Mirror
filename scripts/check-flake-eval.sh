#!/usr/bin/env bash
# Every flake check must at least EVALUATE. A host build does not import the
# checks, so CI stayed green while the NixOS tests could have stopped working
# weeks ago; the overlay's acceptance test had been unevaluable since a commit
# that changed three option interfaces at once. Evaluation is where interface
# drift shows, and it is cheap.
#
# The list is asserted non-empty first: a flake that exposes no checks must
# fail loudly rather than report a pass on nothing. Counted by nix itself,
# so the script needs neither jq nor python (the dev shell has no python:
# "exec: python3: niet gevonden", 2026-11-04).
#
# Run by CI ("Every check evaluates") and by `just flake-eval`.
set -euo pipefail

echo "checks: $(nix eval --json '.#checks.x86_64-linux' --apply builtins.attrNames)"
n=$(nix eval '.#checks.x86_64-linux' --apply 'c: builtins.length (builtins.attrNames c)')
if [ "$n" -lt 1 ]; then
  echo "no checks at all; refusing to report a pass on nothing" >&2
  exit 1
fi
nix flake check --no-build
