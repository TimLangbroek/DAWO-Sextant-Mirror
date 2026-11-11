# Integrations

Integrations are device-side capabilities the overlay publishes so you can turn
them on and configure them per scope from the console, without editing Nix. The
console shows a card per integration; a card is **available** once the overlay
publishes its options in the catalog, and **not published** otherwise.

Three ship with the BB Open overlay:

- **NetBird** - join a self-hosted WireGuard mesh, so a roaming device stays
  reachable and can pull and push from anywhere. You set the management URL and
  a setup key.
- **Directory login (LDAP)** - device login against your directory over SSSD
  (LDAP, AD or IPA). You set the provider, domain, server and a bind secret.
- **Wazuh** - an endpoint security agent that reports to a Wazuh manager. You
  set the manager address, an agent group and an enrollment secret.

## Secrets are references, never values

A field that holds a secret - a setup key, a bind password, an enrollment
password - is stored as a **reference**: the name of a secret you registered,
not the secret itself. The console renders it as a picker of registered names,
so a raw secret can never be typed into the console or committed to git. The
device resolves the name to the decrypted material at runtime (agenix); only the
name travels through the config repo.

Register the secrets first on the **Secrets** page (see
[Manage secrets](./lifecycle.md#manage-secrets)), then point the integration field at one by
name.

## Setting it up

1. Open **Integrations**. A card that reads *available* is ready to configure.
2. Enable it and fill the fields. Secret fields offer the registered
   references, with a shortcut to register a new one inline.
3. Save. This writes at the **organisation** scope - the Integrations page is
   an org-wide quick-config surface for the catalog keys the overlay
   publishes. To narrow an integration to one group or device instead (or to
   review it alongside every other setting), open the same keys from
   **Settings** (the Configuration editor) and pick a group or device with
   its scope selector - integration settings flow down the scope chain like
   any other setting.
4. Every save passes the Nix gate and commits to git like every edit (or
   stages as a change, if your organisation requires change requests - see
   [Ship an update](./updates.md)).

## Does it work? Press Test

Every card has a **Test** button for editors. It changes nothing and answers
in two parts on the card:

- **From the console**: whether each server the card names answers from where
  the console stands. NetBird's management URL, OpenBao's address and the
  Fleet server get one request; the LDAP server a connection, and for
  `ldaps://` a check of its certificate; the Wazuh manager a connection on
  1514 and 1515. A certificate the devices would refuse is named as such. The
  console's network is not the devices', so this is evidence, not proof.
- **From the devices**: of the devices in the scope that have the integration
  on, how many report it working, not working, or nothing yet, and the words
  of the last device that reported it not working.

The console never sends a secret: it holds only references. A wrong setup key
or bind password therefore shows in the devices' part, as the integration not
working with the unit's error, not in the console's part.

If a card reads *not published*, the overlay has not exported that
integration's options yet: add its module to the overlay and regenerate the
catalog (`nix eval .#catalog --json > catalog.json`).

Want one that does not ship with the overlay? An integration is a NixOS module
that publishes options - no console change needed. See
[Build your own integration](../extending/integration.md).

## Troubleshooting

**A card reads "not published" even though I added the overlay module.**
Regenerate the catalog (`nix eval .#catalog --json > catalog.json`) and
restart or reload the console's config snapshot - the catalog is generated,
not live-read from the overlay's Nix source.

**Saving an integration field fails the gate.**
Same as any setting - the distilled error names the actionable line (e.g. an
out-of-range value or an unknown option). See
[Safe writes and the Nix gate](../concepts/safe-writes.md).

**Test says the server answers, but the devices say it does not work.**
The console reaches the server and the devices do not, or the secret is
wrong. Read the device's words on the card: a refused key or password names
itself there; a timeout points at the network between the devices and the
server (a firewall, or a mesh route that is not up yet).

**A secret field's picker is empty.**
No secret reference has been registered yet - use the inline shortcut next
to the field, or register one first on the [Secrets](./lifecycle.md#manage-secrets) page.
