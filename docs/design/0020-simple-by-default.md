# 0020 - Simple by default

Status: accepted, 2026-11-11 (WP6). The rule it serves is in the 1.0 plan:
nothing new may make C1-C4 longer, and new functions go behind "Advanced"
or become default behaviour without a button.

## Problem

The sidebar shows eleven pages to everybody: Overview, Devices, Groups,
Enrollment, Settings, Policies, Hardware, Compliance, Elevation,
Integrations and Updates. Someone who set up a console to add five laptops
needs four of them. The rest are real and used, by fewer people and less
often, and each one in the bar is a question ("do I need this?") asked of
everyone, every day.

## Decision

A per-person switch, **Show all pages**, at the bottom of the sidebar.
Off by default.

- **Off (simple):** Overview, Devices, Groups, Settings, Integrations and
  Updates, plus Enrollment when the organisation has an imaging station.
  These are the pages the four gate flows go through.
- **On:** every page, as today.
- The page being viewed is always in the bar, so following a link to
  Compliance from the overview does not leave the reader on a page the bar
  pretends is not there.

It is navigation, not permission. Every page stays at its address and
behind the same role checks; a person who may see Policies can bookmark it
in simple mode. Hiding a page is never how access is limited.

The choice is stored with the person's other preferences (time zone,
language) and follows them across devices. A console without a database
has no preferences, and then the bar shows every page, since there is no
way to turn the switch on.

## Not doing

- Hiding fields or sections inside pages. Pages that need it already fold
  their advanced parts away (the wave ladder under Updates); a second
  mechanism for that would compete with the first.
- An organisation-wide default. One person's simple is another's missing
  page; it is a reading preference, like the language.

## Done when

A person sees six or seven pages in the bar until they turn the switch on,
then all of them, on every device they sign in from; a hidden page is still
reachable at its address with the same rights; the page being viewed is
always in the bar.
