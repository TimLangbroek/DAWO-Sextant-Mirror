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

## On the laptop: the installation USB

The usual way. The stick erases the laptop's disk and installs it from your
fleet repository, so it works the same for a laptop that came with Windows
as for one that ran something else before.

1. On **Add a laptop**, click **Download the installation USB**. The first
   time, the console has it built, which takes about ten minutes; the page
   then shows the stick with the revision and date it was built from.
2. Write it to a USB stick of at least 2 GB, with Fedora Media Writer or
   `dd`. One stick serves every laptop of the organisation, and it holds no
   code and no credential: a lost stick gives nobody access.
3. Start the laptop from the stick. It asks for a wireless network when
   there is no cable, then for the code.
4. It names the laptop and the disk it will erase, for example "This laptop
   becomes laptop-3abcde. Everything on Samsung PM9A1 (512 GB) is erased."
   Nothing has been touched until you confirm. A wrong code is asked again.
5. It installs, hands the disk phrase to the console, and restarts. From the
   first start the laptop checks in and is active under **Devices**.

The disk is encrypted from the first start. A laptop with a TPM2 chip
unlocks it by itself; one without asks for the phrase at every start, and an
owner can reveal that phrase on the laptop's page. Until you click **Secure
this laptop** on that page, the page shows the laptop as not secured: the
TPM2 unlock does not yet depend on Secure Boot.

A stick from an older revision still installs what the fleet repository says
at the moment of installing. A new stick is only needed when the live system
itself needs newer hardware support.

If the page says the stick could not be built, your fleet repository has no
installer output. A repository the console set up has one; an older one adds
the line the page shows to its `flake.nix`.

## On a laptop that already runs NixOS

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
as 0, 1 and 1, so a code read out over the phone still works. The stick reads
codes the same way.

The agent tells the console the serial number, vendor and model from the
firmware, the processor vendor, and whether the machine is virtual. It gets
back the laptop's name and its credential, writes both to
`/var/lib/sextant-agent`, and the agent service starts by itself. The laptop
is under **Devices** right away, and becomes active when it first checks in,
usually within a minute.

Such a laptop keeps running its own configuration until it is installed from
your fleet repository; settings made in the console reach it from then on.

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
