#!/usr/bin/env bash
# The runner and the laptop check the same things (#134). The justfile used
# to say it mirrored the workflow "exactly" while CI had nineteen steps and
# `just ci` ran ten recipes; the pre-push hook advertised the narrower set as
# the one the runner runs.
#
# Every step in .forgejo/workflows/ci.yml is listed below with the recipe
# that does the same locally, and every recipe `ci` and `ci-full` depend on
# has a step. A step without a recipe, a recipe without a step, or a mapped
# recipe that neither list runs fails this check. Steps that only prepare the
# runner (the event line, the checkout) map to "-".
#
# Run by CI ("CI parity") and by `just ci-parity`, which `just ci` includes.
set -euo pipefail

workflow=.forgejo/workflows/ci.yml

recipe_for() {
  case "$1" in
    "Event" | "Checkout") echo "-" ;;
    "CI parity") echo "ci-parity" ;;
    "Commit messages") echo "commit-check" ;;
    "Format check") echo "fmt-check" ;;
    "Vet") echo "vet" ;;
    "Lint") echo "lint" ;;
    "Test (race + coverage)") echo "test coverage-floor" ;;
    "Build") echo "build" ;;
    "Nix package") echo "nix-build" ;;
    "Every check evaluates") echo "flake-eval" ;;
    "Run the checks") echo "flake-check" ;;
    "Container image (default target must be the server alone)") echo "image-check" ;;
    "Catalog drift guard") echo "catalog-check" ;;
    "Stylesheet drift guard") echo "css-check" ;;
    "Helm lint + render") echo "chart-check" ;;
    "Agent (fmt + clippy + test)") echo "agent-ci" ;;
    "Agent package") echo "agent-build" ;;
    "govulncheck (Go)") echo "vulncheck" ;;
    "cargo audit (agent)") echo "cargo-audit" ;;
    *) return 1 ;;
  esac
}

# The dependencies of a recipe, from its header line `name: dep dep ...`.
deps_of() {
  sed -n "s/^$1:\(.*\)$/\1/p" justfile | tr -s ' ' '\n' | sed '/^$/d'
}

local_set=$( (deps_of ci; deps_of ci-full) | grep -vx ci | sort -u)
if [ -z "$local_set" ]; then
  echo "ci parity: the justfile has no ci recipe to compare with" >&2
  exit 1
fi

fails=0
mapped=""
steps=$(sed -n 's/^      - name: //p' "$workflow")
if [ -z "$steps" ]; then
  echo "ci parity: no steps found in $workflow" >&2
  exit 1
fi
while IFS= read -r step; do
  if ! recipes=$(recipe_for "$step"); then
    echo "ci parity: CI step \"$step\" has no local recipe; add one and map it in $0" >&2
    fails=$((fails + 1))
    continue
  fi
  [ "$recipes" = "-" ] && continue
  for r in $recipes; do
    mapped="$mapped $r"
    if ! grep -qx "$r" <<<"$local_set"; then
      echo "ci parity: CI step \"$step\" maps to recipe $r, which neither \`just ci\` nor \`just ci-full\` runs" >&2
      fails=$((fails + 1))
    fi
  done
done <<<"$steps"

for r in $local_set; do
  if ! tr ' ' '\n' <<<"$mapped" | grep -qx "$r"; then
    echo "ci parity: recipe $r runs locally but no CI step runs it" >&2
    fails=$((fails + 1))
  fi
done

if [ "$fails" -gt 0 ]; then
  exit 1
fi
echo "ci parity: $(wc -l <<<"$steps") CI steps, $(wc -w <<<"$local_set") recipes, the same checks"
