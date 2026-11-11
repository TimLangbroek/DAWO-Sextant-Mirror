# 0019 - Test an integration

Status: accepted, 2026-11-11 (WP5, gate C4). Builds on the Integrations
page and the agent's integration probes (#49).

## Problem

Gate C4 for 1.0 says: an integration is turned on on one page, and each
integration answers a Test button. The page exists, one card per
integration, and the agent already reports per device whether the unit
that carries an integration is up (`INTEGRATION_UNITS` in
`agent/src/collect.rs`, shown on the device page). What is missing is the
question an operator asks right after Save: does it work? Today the answer
means opening devices one by one, and before any device has the
configuration there is no answer at all. A mistyped manager address is
found when the first laptop's agent fails, hours later, on another screen.

## Decision

A **Test** button on every card. It answers in two parts, both on the card,
in plain words, and changes nothing.

1. **From the console: is the server there?** For each address the card
   holds (NetBird's management URL, the LDAP URI, the Wazuh manager, the
   OpenBao address, the Fleet server), the console tries to reach it, with a
   five-second limit:
   - `https://` and `http://`: a GET, redirects not followed. Any HTTP
     answer counts as reachable and is shown with its status; a TLS error
     is named as one, because a certificate the devices will refuse is the
     most common mistake.
   - `ldap://` and `ldaps://`: a TCP connection to the port (389 or 636 by
     default), and for `ldaps` a TLS handshake.
   - A Wazuh manager is a host: TCP to 1514 (events) and 1515
     (enrolment), each reported.

   The console never sends a credential. Secrets are references to agenix
   or OpenBao (design 0007) and the console does not hold their values, so
   it cannot test a bind password or a setup key, and the card says so
   rather than suggesting it did.

2. **From the devices: does it work there?** Of the devices that have the
   integration on, how many report it up, down, or nothing yet, and the
   device's own words for the most recent "down". This is the reading the
   device page already shows, counted for the fleet. It is the part that
   tests the credentials: a wrong setup key shows as NetBird down on the
   devices, with the unit's error.

An integration without an address (the local admin account) has only the
second part.

The console's network is not the devices' network, so "reachable from the
console" is evidence, not proof, and the card says which part says what.
Both parts are needed: the first answers before any device has the
configuration, the second is the only one that sees what a device sees.

**Who may test.** An editor at the page's scope: the same people who can
change the integration. A Test makes the console open connections to
addresses in the configuration, so it is not offered to viewers. Every
test is logged with who ran it and what was tried.

## Not doing

- Asking devices to probe on demand. That is an intent round-trip with a
  minute of latency (design 0010 has the shape) for a reading the agent
  already sends every check-in.
- Testing credentials from the console. It would need the console to read
  secrets it deliberately does not hold.
- Keeping test results. A test answers now; the device readings are the
  history.

## Done when

Every card on the Integrations page has a Test button that an editor can
press and a viewer is not offered; a reachable and an unreachable address
are each told apart in a test, a TLS failure is named, and the device
counts match the readings. Part of the C1-C4 acceptance test.
