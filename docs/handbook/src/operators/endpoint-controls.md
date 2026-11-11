# Endpoint controls and elevation

Four settings groups decide what a person can do on their own machine without
calling anyone: USB device control, printing, user rights, and the local
administrator account. They are ordinary fleet settings, so they resolve
organisation → group → device and can be locked like any other.

## USB device control

`usbDevices.enable` turns on USBGuard: a device plugged into a running machine
is blocked unless a rule allows it.

`usbDevices.allowlist` takes extra USBGuard rules, one per line, for the things
that must keep working - a specific model of card reader, a signature pad.

**The enable carries a high risk class**, so the console asks for an explicit
extra confirmation before it changes. That is not ceremony: switching it on
across a fleet stops hardware people are holding in their hands, and switching
it off removes a control somebody signed for.

## Printing

`printing.enable` turns on CUPS. `printing.discover` finds printers announced
on the local network over mDNS/IPP, which is what most offices want and what
most home setups need.

`printing.drivers` chooses the driver set:

- **open** - the standard IPP/PostScript path. Covers most office printers made
  this decade, and keeps the closure small.
- **broad** - the wider vendor driver set, for hardware the open path does not
  reach.

## User rights

`userRights.enable` lets ordinary users change desktop settings that would
otherwise need an administrator. Each right is set individually under
`userRights.options.*`, and each takes one of four modes:

| Mode | Meaning |
|---|---|
| **off** | Nobody but an administrator. |
| **self** | The user may, after typing **their own** password. polkit remembers it briefly, so a run of related steps is not a run of dialogs. |
| **session** | Anyone in a real, foreground session on the machine. |
| **group:&lt;name&gt;** | Only members of that directory group, in a real session. |

**Every mode requires a local, active session** - never SSH, never a background
or remote one. That clause is what makes granting these safe, so it is part of
every rule rather than something a mode can opt out of.

The rights on offer are the ones that otherwise generate a support call from
somebody who cannot work: approving a dock when it is plugged in, applying a
firmware update fwupd offers, editing a network connection that applies to the
whole machine.

### What is deliberately not on the list

Four things are absent, and it is not an oversight:

- **Creating, deleting or re-grouping accounts.** On a fleet machine the account
  set comes from the directory. A local user who can add accounts can add one
  that outlives their own and answers to nobody.
- **Autologin and the greeter's user list.** Autologin turns full-disk
  encryption into a locked door with the key taped to it.
- **Changing the hostname.** The fleet identifies devices by name.
- **Firmware downgrades.** Rolling firmware back to an older version is a way to
  reintroduce a fixed vulnerability, so the upgrade path is offered and the
  downgrade path is not.

They are **absent rather than explicitly denied**. An explicit no would
short-circuit polkit for administrators too, which would take away the path that
is supposed to remain.

## Local administrator

A local administrator is **discouraged**, and off unless you turn it on. The
console, the recovery key and the remote actions are the way into a device
that will not let anybody in. Keep a local administrator only if you need a
way in that works with the console unreachable too.

`localAdmin.enable` creates an administrator who can sign in when the directory
or the network is unreachable. `localAdmin.username` is the login name - pick
one per organisation rather than inheriting a default - and
`localAdmin.passwordSecret` names a **secret reference** holding the password
hash, so no credential is shared between fleets and none of it passes through
the console.

This is also high risk class, in both directions. Off locks the account on every
device it applies to; on, it creates a way in that does not depend on your
directory being up.

### A password the console holds

With `localAdmin.managed` on, there is no password to keep anywhere else. Each
device draws its own (twenty characters in groups of five, about 100 bits) at
first boot and hands it to the console on its next check-in, the way it hands
over its LUKS recovery key. The device page then shows it to an owner, once
per reveal, and every reveal is in the
[evidence export](./compliance.md#evidence-for-a-period). `passwordSecret` is
ignored.

**Rotate** next to the password asks the device for a new one. The old one
stops working the moment the device sets the new one, and the console has the
new one on the beat after. The device keeps its copy until the console confirms
holding it, so a console that cannot store secrets yet (no
`SEXTANT_SECRET_KEY`) costs nothing but a delay.

A revealed password does not stay the password. Eight hours after the first
reveal the console asks the device for a new one by itself, so a password that
was read, typed and perhaps written down stops working without anybody
remembering to press **Rotate**. Revealing it again within those hours does not
move the rotation. If the device could not rotate, the console does not ask
again for the same reveal: press **Rotate** once the cause is fixed.

Managed passwords need the root executor on the device
(`services.sextant-actd.enable`, in your overlay); the build refuses
`managed` without it. Wipe stays refused unless you arm it separately.

Related: [Manage secrets](./lifecycle.md#manage-secrets) for registering the password reference,
and [Approve a request for privilege](#approve-a-request-for-privilege) for the path a user takes
when they hit something none of these rights cover.

## Approve a request for privilege

Somebody on a fleet laptop tries to do one privileged thing - install a printer
driver, change a network setting - and the system asks for an administrator.
This is where that request lands.

### The problem it solves

polkit's answer to "you may not do this" is a dialog asking for an
**administrator's password**. On a managed fleet machine that means the local
admin account. Away from the office, that is not a slower path - it is no path
at all.

The predictable outcome is that somebody shares the admin password to get the
user unstuck, and then it is shared again, until it has stopped being a secret.

So the request becomes something an administrator answers **centrally**, and it
is logged.

### What an approval is, and what it is not

Sextant **decides**. It never asserts.

The grant does not come from the console and cannot: polkit will not let an
agent vouch for an identity. The answer travels through polkit's own setuid
helper, which runs PAM, and PAM turns the answer into an authentication - or
does not. The console's role is to say yes or no; the device's own machinery
enforces it.

This matters for how you read the request.

### Approve on who and where, not on what

Each request carries four things:

| | |
|---|---|
| **Device** | Taken from the device's authenticated check-in, never from the request itself - otherwise any device could raise a request in another's name. |
| **User** | Who is asking. |
| **Action** | What the session says it is trying to do. |
| **Reason** | What the user typed. The only field a human wrote, and usually the most useful. |

**The action is context, not proof.** PAM is never told the polkit action id, so
the device's own session supplies that string - and a session is not a
trustworthy narrator about itself. Approve on the strength of *who* is asking
and *where*, both of which are established by the device's authenticated
check-in, and read the action as what the user *says* they are doing.

### Five minutes, and why it is short

A request waits **five minutes** for an answer.

That is deliberately short. Somebody is standing in front of a frozen dialog for
the whole window, so a generous timeout is not generosity. Five minutes is long
enough for an administrator who is watching, and short enough that a user who
gets no answer finds out while they still remember what they were trying to do.

### Four outcomes, and one of them is not a decision

- **Approved** - the console said yes; PAM completes the authentication.
- **Denied** - the console said no.
- **Pending** - waiting, within the window.
- **Expired** - nobody answered in time.

Expired is a distinct outcome rather than a flavour of denied, on purpose. "We
said no" and "nobody was there" are different conversations, and a list of
expired requests is a **staffing problem, not a policy one**. If they are
piling up, the answer is not to tighten the rules.

Expiry is derived from the clock rather than written down, so a request cannot
sit Pending for ever because whatever was supposed to expire it died.

### Where to find them

The **Requests** page in the console. Every decision records who made it and
when, and lands in the audit log like any other action.
