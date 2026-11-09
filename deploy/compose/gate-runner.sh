#!/bin/sh
set -eu
# Waits for init (see init.sh).
until [ -f /secrets/.ready ]; do sleep 1; done
GATE_TOKEN="$(cat /secrets/gate)"
export GATE_TOKEN
exec /usr/local/bin/gate-runner "$@"
