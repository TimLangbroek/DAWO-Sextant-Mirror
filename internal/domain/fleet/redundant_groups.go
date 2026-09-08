package fleet

import "fmt"

// A device may be listed in a group AND in one of that group's own ancestors.
// That second entry adds nothing: GroupAncestry already walks the parents, so
// the ancestor is in the scope chain either way.
//
// It is not harmless, though. Two entries mean the tie-break rule applies, and
// that rule is array order rather than tree depth - a position nothing on
// screen shows. Today it never fires because the redundant ancestors carry no
// settings; the day one does, the answer depends on the order of a list nobody
// can see. Measured 2026-08-26: 27 of 148 devices on the demo fleet, every one
// of them a group plus its own parent, and zero devices anywhere in two
// genuinely independent groups.
//
// So this removal has no losing group to choose between, which is exactly why
// it can be done mechanically and proven rather than decided.

// RedundantGroups returns the entries in a device's group list that are
// already reachable as an ancestor of another entry, in the order they appear.
// An empty result means the list carries no duplication.
func (f *Fleet) RedundantGroups(tag string) []string {
	d, ok := f.Devices[tag]
	if !ok || len(d.Groups) < 2 {
		return nil
	}

	// Every ancestor of every entry, excluding the entry itself: being your
	// own ancestor is not redundancy.
	reachable := map[string]bool{}
	for _, g := range d.Groups {
		for _, anc := range f.GroupAncestry(g) {
			if anc != g {
				reachable[anc] = true
			}
		}
	}

	var out []string
	for _, g := range d.Groups {
		if reachable[g] {
			out = append(out, g)
		}
	}
	return out
}

// PruneRedundantGroups drops those entries from every device. The scope chain
// each device resolves through is unchanged by construction - an ancestor that
// is removed from the list is still walked from the entry that named it - and
// TestPruningRedundantGroupsChangesNothingThatResolves proves it rather than
// asserting it.
//
// Devices in two independent groups are left alone. Those are a real choice
// about which group wins, and a migration is not the place to make it.
func PruneRedundantGroups() func(*Fleet) error {
	return func(f *Fleet) error {
		for tag, d := range f.Devices {
			drop := f.RedundantGroups(tag)
			if len(drop) == 0 {
				continue
			}
			cut := map[string]bool{}
			for _, g := range drop {
				cut[g] = true
			}
			kept := make([]string, 0, len(d.Groups))
			for _, g := range d.Groups {
				if !cut[g] {
					kept = append(kept, g)
				}
			}
			d.Groups = kept
			f.Devices[tag] = d
		}
		return nil
	}
}

// rejectAncestorDuplication refuses a membership list that names a group and
// one of its own ancestors. The ancestor is in the scope chain regardless, so
// the entry buys nothing - and having two entries is what makes the invisible
// array ordering decide ties.
//
// Refusing at the write is the cheap half of #115: it does not touch the
// model or the nix twin, and it closes the door the demo fleet walked through
// 27 times. The message names both groups, because "you cannot do that" is
// useless to somebody who cannot see the hierarchy from where they stand.
// rejectNewAncestorDuplication refuses ONE addition: putting `added` next to
// a group it already contains, or that already contains it. Pairs that were
// there before are not this call's business.
//
// That distinction is the whole point. CreateGroupWithDevices appends to a
// membership somebody else wrote, and on the demo fleet 27 of 148 devices
// carry a group and its own parent from before this guard existed. Judging
// the combined list refused every one of them for a group they were not being
// added to, and named two groups the operator never touched.
func (f *Fleet) rejectNewAncestorDuplication(existing []string, added string) error {
	for _, g := range existing {
		if g == added {
			continue
		}
		for _, anc := range f.GroupAncestry(g) {
			if anc == added {
				// added contains g, so g is the one to keep: it brings added
				// along through the scope chain.
				return ancestorDuplication(g, added)
			}
		}
		for _, anc := range f.GroupAncestry(added) {
			if anc == g {
				return ancestorDuplication(added, g)
			}
		}
	}
	return nil
}

// ancestorDuplication is the refusal both guards speak. child sits inside
// ancestor; keeping the child is what preserves the chain.
func ancestorDuplication(child, ancestor string) error {
	return fmt.Errorf(
		"group %q is already inside %q, so listing both adds nothing "+
			"and makes the order of the list decide which one wins: "+
			"keep %q", child, ancestor, child)
}

func (f *Fleet) rejectAncestorDuplication(groups []string) error {
	for _, g := range groups {
		for _, anc := range f.GroupAncestry(g) {
			if anc == g {
				continue
			}
			for _, other := range groups {
				if other == anc {
					// g is the descendant: GroupAncestry walks upward, so
					// anc contains g and not the other way round.
					return ancestorDuplication(g, anc)
				}
			}
		}
	}
	return nil
}
