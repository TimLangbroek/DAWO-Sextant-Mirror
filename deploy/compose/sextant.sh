#!/bin/sh
# Hands the generated secrets to Sextant through its environment, which is
# the only place it reads them from. Never as flags: those show in ps.
set -eu
# Waits for init (see init.sh).
until [ -f /secrets/.ready ]; do sleep 1; done
s() { cat "/secrets/$1"; }
# Assigned first and exported after: `export X="$(...)"` returns the status
# of export, so a secret that cannot be read would start the console with an
# empty one instead of stopping here.
SEXTANT_PG_DSN="postgres://sextant:$(s pg)@127.0.0.1:55432/sextant?sslmode=disable"
SEXTANT_SESSION_KEY="$(s session)"
SEXTANT_SECRET_KEY="$(s secret)"
SEXTANT_CHECKIN_TOKEN="$(s checkin)"
SEXTANT_GATE_TOKEN="$(s gate)"
SEXTANT_OIDC_CLIENT_SECRET="$(s dex-client)"
export SEXTANT_PG_DSN SEXTANT_SESSION_KEY SEXTANT_SECRET_KEY SEXTANT_CHECKIN_TOKEN \
  SEXTANT_GATE_TOKEN SEXTANT_OIDC_CLIENT_SECRET
exec /sbin/tini -- sextant "$@"
