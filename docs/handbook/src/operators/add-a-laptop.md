# Add a laptop

A laptop joins the fleet with a code. You make the code in the console, and
somebody types it on the laptop. Nobody edits the fleet repository, writes a
hardware profile or copies a credential by hand.

The design is in
[design 0015](https://codeberg.org/DAWO/DAWO-Sextant/src/branch/main/docs/design/0015-join-with-a-code.md).

## Make a code

Click **Add a laptop** at the top of the console, choose the group the laptop
belongs in, and click **Make a code**.

```
Code for pilot
K7QM-4XWD
Valid until Wed 11 Nov 09:14 · 1 laptop · shown only now
```

- The code works for **24 hours**, for **one laptop**.
- For a delivery of several laptops, open **More than one laptop** and enter
  how many (at most 100). The code still closes after 24 hours.
- The laptop gets the group's settings from its first start.
- You need editor rights on the group. An organisation editor can also make a
  code without a group.

The code is shown once. The console keeps only a keyed hash of it, so it
cannot show it again. If you lose it, withdraw it and make a new one.

## On the laptop

The laptop needs the Sextant agent. On NixOS:

```nix
imports = [ sextant.nixosModules.agent ];
services.sextant-agent.enable = true;
```

Leave `tag` and `url` out: the code provides both. Then, as root:

```
sudo sextant-agent join https://console.example.org K7QM-4XWD
```

Case, dashes and spaces do not matter, and the letters O, I and L are read
as 0, 1 and 1, so a code read out over the phone still works.

The agent tells the console the serial number, vendor and model from the
firmware, the processor vendor, and whether the machine is virtual. It gets
back the laptop's name and its credential, writes both to
`/var/lib/sextant-agent`, and the agent service starts by itself. The laptop
is under **Devices** right away, and becomes active when it first checks in,
usually within a minute.

> The installation USB that brings the agent with it is the next step of
> design 0015. Until it exists, the agent comes from the NixOS configuration
> above. Such a laptop reports to the console and shows its state there. It
> keeps running its own configuration until it is installed from your fleet
> repository; settings made in the console reach it from then on.

## What the console decides

The console looks for the serial number in the fleet first:

| The serial number is | Result |
|---|---|
| New | A new device. Its name is `laptop-` plus the last six letters and digits of the serial number, or `--name` if you gave one. Without a usable serial number the six characters are random. |
| On a device that never checked in | The same device. A laptop that lost its connection halfway can simply run `join` again. |
| On a device that is in use | Refused: "this laptop is already enrolled as ...". Use **Re-enrol** on that device instead. |
| On a retired device | A new device. A returned laptop that is handed out again starts clean. |

The hardware profile follows from the vendor and model when the fleet has one
for that model. Otherwise the console picks the generic Intel or AMD profile
from the processor, or the VM profile for a virtual machine.

The new device goes through the same gate as every other change to the fleet.
If the gate refuses it, the laptop gets the gate's message and the code keeps
its use.

## When it does not work

| The laptop says | What it means |
|---|---|
| "this code does not work: it is mistyped, used up or expired" | Make a new code. The console does not say which of the three, so a guessed code tells nobody anything. |
| "this laptop is already enrolled as ..." | The laptop is a working device already. Use **Re-enrol** on its page. |
| "the name ... is taken" | The `--name` you gave is in use. Choose another or leave it out. |
| "... trying again in 10s" | The console could not be reached or did not answer in time. The agent tries three more times, then stops with exit code 5. |

Exit codes, for scripts: `0` joined, `2` wrong arguments, `4` refused by the
console, `5` console not reachable, `6` the answer could not be written.

## Re-enrol a laptop

When a laptop has lost its credential - a new disk, a reinstall - open its
page under **Devices** and click **Re-enrol**. You get a code for that one
laptop, for one use. The laptop keeps its name and history and gets a new
credential. The old credential stops working.

## Withdraw a code

**Add a laptop** lists the open codes you can see, with how many laptops each
one has left. **Withdraw** closes a code at once. Withdraw a code that was
shared in the wrong place: until it closes, anybody who has it can add a
laptop to that group.

## How the code is protected

- A code is 8 characters from a 32-letter alphabet: about a trillion
  possibilities.
- The join address accepts two attempts per second from one address, with a
  burst of ten, and every refused code is logged with the address it came
  from.
- The console stores a keyed hash (HMAC-SHA256) of each code, never the code.
- If anything fails after a code is accepted - the gate refuses, the
  laptop gives up waiting - the code gets its use back.
