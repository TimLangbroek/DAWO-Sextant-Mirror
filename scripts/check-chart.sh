#!/usr/bin/env bash
# Chart guards: the chart lints and renders, and every refusal it promises
# actually fires. Run by `just ci` and by the forge CI, so a guard is proven
# on a laptop before it is proven on a runner.
#
# The chart's DEFAULTS deliberately refuse to render (P4 fail-safe: write +
# gateMode none needs an explicit ack), so lint/render pin a valid combo, and
# the guards themselves are asserted to fire.
cd "$(dirname "$0")/.."
set -eu
helm lint deploy/helm --set gateMode=remote --set gateRunner.enabled=true \
  --set gateRunner.cache.keySecret=k --set gateRunner.cache.host=cache.example
helm template deploy/helm --set gateMode=remote --set gateRunner.enabled=true \
  --set gateRunner.cache.keySecret=k --set gateRunner.cache.host=cache.example \
  --set releaseCache=true >/dev/null
helm template deploy/helm --set gateMode=eval >/dev/null
helm template deploy/helm --set allowUnvalidated=true >/dev/null
if helm template deploy/helm >/dev/null 2>&1; then
  echo "P4 guard did not fire: default write+gateMode=none rendered without ack"; exit 1
fi
if helm template deploy/helm --set gateMode=eval --set replicaCount=2 \
  --set persistence.enabled=true >/dev/null 2>&1; then
  echo "HA guard did not fire: PVC + replicaCount>1 rendered"; exit 1
fi
# Backup guards. A half-configured backup is worse than none: it
# fills a bucket, moves the metrics, and cannot restore. Both
# halves are asserted to refuse.
if helm template deploy/helm --set gateMode=eval \
  --set cnpg.backup.enabled=true >/dev/null 2>&1; then
  echo "backup guard did not fire: enabled without destinationPath rendered"; exit 1
fi
if helm template deploy/helm --set gateMode=eval \
  --set cnpg.backup.enabled=true --set cnpg.backup.destinationPath=s3://b/x/ >/dev/null 2>&1; then
  echo "backup guard did not fire: enabled without secretName rendered"; exit 1
fi
# And a complete backup config must render BOTH objects. WAL
# archiving with no base backup is the failure this pairing
# exists to prevent (forgejo, 2026-06-01).
back=$(helm template deploy/helm --set gateMode=eval \
  --set cnpg.backup.enabled=true \
  --set cnpg.backup.destinationPath=s3://b/x/ \
  --set cnpg.backup.secretName=s)
echo "$back" | grep -q "barmanObjectStore" || { echo "backup enabled but no barmanObjectStore"; exit 1; }
echo "$back" | grep -q "kind: ScheduledBackup" || { echo "barmanObjectStore without a ScheduledBackup: WAL with no base is not recoverable"; exit 1; }
# The WAL fuse. Archiving switches a segment on a timer whether or
# not anything happened, so a volume too small to absorb a failed
# archive stops the database within hours - production, 2026-08-13.
# Asserted per unit, because a guard that only understood "Gi"
# would wave 500Mi through, which is the case it exists to catch.
for size in 2Gi 500Mi 1G; do
  if helm template deploy/helm --set gateMode=eval --set cnpg.storage=$size \
    --set cnpg.backup.enabled=true --set cnpg.backup.destinationPath=s3://b/x/ \
    --set cnpg.backup.secretName=s >/dev/null 2>&1; then
    echo "WAL fuse guard did not fire: $size with archiving rendered"; exit 1
  fi
done
# A deliberate small cell still renders, and a small volume WITHOUT
# archiving was never the problem.
helm template deploy/helm --set gateMode=eval --set cnpg.storage=2Gi \
  --set cnpg.acknowledgeSmallVolume=true --set cnpg.backup.enabled=true \
  --set cnpg.backup.destinationPath=s3://b/x/ --set cnpg.backup.secretName=s >/dev/null
helm template deploy/helm --set gateMode=eval --set cnpg.storage=2Gi >/dev/null
# Passwordless demo: the guards refuse the three combinations that
# would quietly make an owner-session console reachable. A guard
# that is never asserted is a comment.
helm template deploy/helm --set gateMode=eval --set devAuth.enabled=true \
  --set ingress.enabled=false >/dev/null
for bad in "ingress.enabled=true" "oidc.issuer=https://x" "simulator.enabled=true"; do
  if helm template deploy/helm --set gateMode=eval --set devAuth.enabled=true \
    --set ingress.enabled=false --set "$bad" >/dev/null 2>&1; then
    echo "devAuth guard did not fire for $bad"; exit 1
  fi
done

# The simulator clones the overlay itself, so a private remote
# needs the same netrc the console and the gate runner already
# seed. It did not get one: the demo cell spent nineteen days in
# CrashLoopBackOff on "could not read Username for ..." while the
# secret sat unused in the namespace (2026-08-20 .. 2026-09-08).
# Asserted in both directions, because a mount that appears
# unconditionally would carry a credential into cells that have
# none to give.
sim=$(helm template deploy/helm -s templates/simulator.yaml \
  --set gateMode=eval --set simulator.enabled=true \
  --set gitRemote.url=https://example.test/o.git \
  --set gitRemote.netrcSecret=demo-netrc)
echo "$sim" | grep -q "secretName: demo-netrc" || { echo "the simulator clones a private overlay without a netrc: that is a CrashLoopBackOff, not a warning"; exit 1; }
echo "$sim" | grep -q "mountPath: /netrc-secret" || { echo "netrc volume declared but never mounted into the simulator"; exit 1; }
nonetrc=$(helm template deploy/helm -s templates/simulator.yaml \
  --set gateMode=eval --set simulator.enabled=true \
  --set gitRemote.url=https://example.test/o.git)
echo "$nonetrc" | grep -q netrc && { echo "the simulator mounts a netrc for a remote that asked for none"; exit 1; }

# The chart has to install on a cluster that is not Cilium, which
# is most of them. A CiliumNetworkPolicy is an unknown kind there
# and the whole release fails, so the engine is asserted in all
# three positions rather than trusted.
k8s=$(helm template deploy/helm --set gateMode=eval --set networkPolicy.engine=kubernetes)
echo "$k8s" | grep -q "kind: CiliumNetworkPolicy" && { echo "engine=kubernetes still rendered a CiliumNetworkPolicy"; exit 1; }
echo "$k8s" | grep -q "kind: NetworkPolicy" || { echo "engine=kubernetes rendered no NetworkPolicy at all"; exit 1; }
cil=$(helm template deploy/helm --set gateMode=eval --set networkPolicy.engine=cilium)
echo "$cil" | grep -q "kind: CiliumNetworkPolicy" || { echo "engine=cilium rendered no CiliumNetworkPolicy"; exit 1; }
non=$(helm template deploy/helm --set gateMode=eval --set networkPolicy.engine=none)
echo "$non" | grep -qE "kind: (Cilium)?NetworkPolicy" && { echo "engine=none rendered a policy anyway"; exit 1; }

# Every setting the server reads from the environment is looked up
# with a SEXTANT_ prefix (config.EnvPrefix). A chart env var without
# it is not a typo the server complains about - it is silently
# ignored, the default applies, and the deployment looks correct
# while doing the opposite of what the values file says. That is
# exactly how SEXTANT_METRICS_ADDR shipped as METRICS_ADDR and left
# /metrics on the public listener.
#
# HOME is the one legitimate exception: it is read by git, not by us.
stray=$(helm template deploy/helm --set gateMode=eval \
  | grep -oE "name: [A-Z][A-Z0-9_]+" | awk "{print \$2}" | sort -u \
  | grep -vE "^(SEXTANT_|HOME$)" || true)
if [ -n "$stray" ]; then
  echo "environment variables the server will never read (missing the SEXTANT_ prefix):" >&2
  echo "$stray" >&2
  exit 1
fi
