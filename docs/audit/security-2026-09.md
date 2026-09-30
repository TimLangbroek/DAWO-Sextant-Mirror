# Security and quality review, September 2026

Read against `main` at `43cf67d`, with the working tree of
`test/the-station-half-of-the-bridge-rule` checked out on 2026-09-30, which
is main plus tests. Static review: no running instance was attacked. What
was checked is stated so the next review does not re-derive it, and every
claim names where it was read.

Method: the paths in `docs/threat-model.md` were walked per boundary rather
than per file, and each of its claims was held against the code. The
quality half measured what a reader can measure: function length, comment
blocks, conversation artefacts, dead markers.

## Scope

- the Rust agent (`agent/src`), entire
- the check-in API and the intent nonce (`internal/http/api`)
- the console middleware, sessions, CSRF and cookies (`internal/http/web`,
  `internal/adapters/oidc`)
- the gate-runner (`cmd/gate-runner`)
- the deployment: the Helm templates, the Dockerfiles, and
  `deploy/nixos/actd.nix`

## What held

The August findings (H1-H3) and the fixes that followed them were not
re-litigated, only checked for their current shape. Verified:

- **Agent.** The intent set in `action.rs` is fixed and unknown intents are
  ignored; every external program runs as an argv slice with no shell
  (`collect.rs`, `posture.rs`); the console URL must be https or loopback
  (`config.rs`); the credential is read from a systemd credential file and
  the hand-written `Debug` redacts it; a wipe intent acts only inside a
  15-minute window (`main.rs` `wipe_is_fresh`); the spool is 0700 with a
  fixed file name per intent.
- **Wipe chain.** `actd.nix` compiles the wipe code only when
  `services.sextant-actd.armWipe` is set, so an unarmed host cannot erase;
  the lock interlock refuses an unlocked wipe unless overridden at build
  time; a partial erase acks `wipe-failed`, keeps the intent, and does not
  power off.
- **Check-in.** The body is capped at 320 KiB (`checkin.go`); the per-device
  credential is bound to the tag; the shared bridge token is refused for
  any device that has a credential of its own (`authorized`,
  `checkin.go:347`); a wipe ack is believed only with a valid HMAC-SHA256
  nonce inside the window (`intent_nonce.go`), compared constant-time with
  both timestamp directions checked; a recovery key is capped at 256 bytes,
  sealed, and cleared on the device only after `X-Recovery-Key-Stored`.
- **Console.** Every mutation carries a constant-time CSRF check and a
  1 MiB body cap (`middleware.go`); cookies are HttpOnly, Secure and
  SameSite=Strict; the error page's back-link accepts only a same-host
  referer path; the dev session stub refuses to start on a non-loopback
  address (`config.go:370`); authenticated pages carry `no-store`.
- **Gate-runner.** The token is env-only, and a non-loopback listener
  without `GATE_TOKEN` exits at startup (`cmd/gate-runner/main.go:106-113`).
- **Scans.** `govulncheck ./...` reports zero called vulnerabilities on the
  pinned toolchain (`scripts/vulncheck.sh`). By pattern and by reading: no
  SQL string concatenation, no `template.HTML`, no shell interpolation, no
  raw crypto misuse.
- **Deployment.** `runAsNonRoot: 65532`, capabilities drop ALL, seccomp
  RuntimeDefault, `readOnlyRootFilesystem` where the sidecar allows it;
  both Dockerfiles end in `USER 65532`.

## Findings

None medium or high. Four low, one of them already tracked.

### F1 - A failed chmod in the agent was invisible -> #175

`tighten()` in `agent/src/action.rs` drops the spool and lock-flag
directories to 0700 and discarded failure. Ownership remains the primary
control, so refusing to panic was right; what was missing is that the
journal never said the second layer was lost. Fixed in PR #177.

### F2 - The advisory scans are not in the local gate -> #134

The first statement of this finding said the agent has no advisory scan,
and that was wrong: CI runs `cargo audit` (`.forgejo/workflows/ci.yml`,
the `cargo audit (agent)` step) and `govulncheck`. What is true is that
neither is in the devshell or in `just ci`, so the pre-push gate can pass
where the runner fails. That is already issue #134, and a comment there
records the re-confirmation. Recorded here so the next review does not
restate the wider claim as if it were new.

### F3 - Four advisories in modules the code requires but does not call

`govulncheck` reports four vulnerabilities in required modules whose
affected functions this code does not call. Informational today. The number
moves with dependency bumps and belongs in the release notes rather than in
this register.

### F4 - The wipe unit's syscall sandbox stays loose

Unchanged, and already the residual risk R5 in `docs/threat-model.md`: the
hardening waits on hardware coverage. Listed here because a review that
omits a known risk looks complete when it is not.

## Quality

- **One god function.** `buildConfigPlane` in `cmd/sextant/capabilities.go`
  measures 382 lines, the only function over 200. Issue #139 was closed
  with the wiring accepted, so this is a re-reading rather than a new
  finding: it is where the console's growth will break first.
- **One split candidate.** `agent/src/collect.rs` carries usage, health,
  integrations and the comin error in one 925-line file. The functions are
  short and tested; the file is the problem, not the code.
- **Page handlers** at 150-170 lines (`overview.go`, `settings.go:245`,
  `devices_page.go:443`, `compliance.go:36`) each assemble one screen and
  read that way. Left alone.
- **`catalog.go` at 2076 lines** is translation data with a drift guard
  (`examples/overlay/regen-catalog.sh --check`), not logic.
- **Comment blocks**: 75 blocks of 12 lines or more, the longest 41
  (`internal/app/verdicts.go:3`). This is the house style saying why with
  its measurement; nothing read as filler.
- **No conversation artefacts.** A sweep for the phrases an assistant
  leaves behind (certainly, great question, user asked, as an AI, and
  company) found only the deliberate attribution line and two design-brief
  documents that instruct a designer by name. Zero TODO or FIXME markers.

## What a static review cannot answer

Nothing here was attacked while running. The rate limits, the session
expiry and the API-scope gaps #168 names are claims about behaviour, and
they need a live console with `fleetsim` driving it. That session is
separable, and worth scheduling before 1.0.
