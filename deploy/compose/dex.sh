#!/bin/sh
set -eu
# Waits for init (see init.sh).
until [ -f /secrets/.ready ]; do sleep 1; done
DEX_CLIENT_SECRET="$(cat /secrets/dex-client)"
export DEX_CLIENT_SECRET
exec dex serve /etc/dex/sextant.yaml
