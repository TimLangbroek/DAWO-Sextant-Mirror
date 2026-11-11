# Your laptop and Sextant

This page is for the person who uses a laptop that Sextant manages, not for
the IT department that runs it. It says what Sextant does on your machine,
what it can see, and what to do when something gets in your way. Your
organisation decides the details; where it can choose, this page says so.

## What it does

Your laptop runs NixOS, and its whole configuration is written down in one
place your IT department keeps. Sextant is the tool they use to change that
configuration and to see whether each laptop has taken the change.

The laptop fetches its configuration itself, at regular intervals, and
applies it. Nobody sends commands to it, and there is no way for anyone to take over
your screen or run something on your machine from a distance. That is a
design choice, not a setting your organisation forgot to turn on.

## What it can see

The laptop tells the console:

- which configuration it runs, and whether applying the last one worked;
- a few facts about the machine: hostname, kernel, CPU and memory use;
- whether its security features are on: disk encryption, Secure Boot, the
  TPM chip;
- when it last checked in.

It does **not** send keystrokes, screen contents, browsing history, your
location, or which programs you use.

When a laptop misbehaves, IT can ask it for a **diagnostics bundle**: pieces
of the system log. A log can contain file names and the names of machines you
connected to, so a bundle is asked for one laptop at a time, kept sealed, and
deleted after 14 days. Your organisation's privacy statement says whether and
how you are told when that happens.

The laptop is registered under your name or account, so IT knows whose it is
when you call them or when it goes missing.

## Updates

Updates arrive the same way as every other change: the laptop fetches them
and applies them in the background. Most take effect at once; a new kernel
takes effect at the next restart.

Not every laptop gets an update on the same day. Updates go out in waves, and
a wave only goes ahead when the one before it is healthy. If yours is in an
early wave, you are helping to catch a problem before everyone else meets it.

A few minutes after an update, the laptop checks itself. If it finds the
update broke something it needs, it goes back to the version before, on its
own.

## Starting the laptop

The disk is encrypted. Normally the laptop's security chip (TPM) unlocks it
when it starts, and you notice nothing. Some laptops have no such chip; those
ask for a passphrase every time they start.

If the laptop asks for its **recovery phrase** when it usually does not, the
chip has noticed something changed in how the laptop starts, such as a
firmware update or a changed BIOS setting. Call IT. The phrase is six plain
words; IT can look it up, and every time somebody does, it is written down
who.

## When you need administrator rights

Some actions ask for an administrator, such as installing a printer driver
or changing a network setting. On a managed laptop you are not given the
administrator password. Instead, depending on how your organisation set it
up, the dialog sends your request to IT:

1. Type a short reason. It is the part IT reads first, so say what you are
   trying to do.
2. Wait at the dialog. Somebody approves or refuses it in the console.
3. After **five minutes** without an answer, the request expires. That does
   not mean no; it means nobody was watching. Call IT, or try again later.

USB devices and printing can be limited too. If a USB stick or a printer
does not work, that may be your organisation's setting rather than a fault;
ask IT before you go looking for a workaround.

## Lost or stolen

Tell IT straight away. They can lock the laptop, which locks every session
and stays locked after a restart, and they can wipe it, which destroys the
keys to the encrypted disk so that nothing on it can be read again.

Both take effect the next time the laptop is online and checks in, not the
moment IT presses the button. That is the price of a laptop that nobody can
send commands to, and it is also why reporting it quickly matters.

## Your data

What is kept about you, and for how long, is set out in your organisation's
privacy statement. Sextant keeps check-ins for a limited time and can erase
what it holds about a person when they leave. The record of who changed what
in the configuration is kept, because that record is what an audit relies
on.
