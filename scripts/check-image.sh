#!/usr/bin/env bash
# The container image's RUNTIME stage, built and never pushed: that the COPY
# paths resolve, the pinned base digest still exists, and the entrypoint is
# there. `go build` covers the compile stage; the Dockerfile is otherwise
# exercised only at release time, the worst moment to find out.
#
# The assertion is the point, more than the build. The Dockerfile has several
# targets and the release builds WITHOUT --target, so it relies on the server
# stage being last - a property nobody sees in a diff and one that silently
# inverts the day somebody appends a stage. A release would then push the
# tools image, with a fleet simulator inside the control plane, as
# production. So: build the default target and prove it holds the server and
# nothing else.
#
# Run by CI ("Container image") and by `just image-check`.
set -euo pipefail

check='
  set -euo pipefail
  podman build -q -t sextant-ci-check . >/dev/null
  got="$(podman run --rm --entrypoint sh sextant-ci-check -c "ls /usr/local/bin | sort | tr \"\\n\" \" \"")"
  echo "image carries: $got"
  [ "$got" = "sextant " ] || {
    echo "the default build target is not the server alone - a release would publish this as production" >&2
    exit 1
  }
  podman rmi -f sextant-ci-check >/dev/null 2>&1 || true
'

# A podman the host already has comes with its own rootless setup; the one
# from nixpkgs does not find it on a non-NixOS laptop ("configure storage:
# mkdir /run/containers: permission denied", Fedora 44, 2026-11-10). The
# runner has none of its own, so it takes the nixpkgs one, with the setuid
# newuidmap/newgidmap from the NixOS wrappers.
if command -v podman >/dev/null; then
  bash -c "$check"
else
  export PATH="/run/wrappers/bin:$PATH"
  nix shell nixpkgs#podman --command bash -c "$check"
fi
