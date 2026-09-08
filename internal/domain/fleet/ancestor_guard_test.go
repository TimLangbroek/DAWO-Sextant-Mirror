package fleet

import (
	"strings"
	"testing"
)

// The console has two ways to change membership - editing a device and adding
// a device to a group - and both have to refuse the same thing. A guard on one
// path only is worse than none, because it teaches an operator the rule exists
// and then lets the other route through.

func guarded() *Fleet {
	return &Fleet{
		Version: 3,
		Org:     &Scope{Settings: map[string]any{}},
		Groups: map[string]Group{
			"gemeente-a": {},
			"stad":   {Parent: "gemeente-a"},
			"kantoor":    {},
		},
		Devices: map[string]Device{"lt-1": {Groups: []string{"stad"}}},
	}
}

func TestEditingADeviceCannotAddItsOwnAncestor(t *testing.T) {
	f := guarded()
	groups := []string{"stad", "gemeente-a"}

	err := UpdateDevice("lt-1", DevicePatch{Groups: &groups})(f)
	if err == nil {
		t.Fatal("adding a group's own parent was accepted; that is the duplication " +
			"that makes array order decide ties")
	}
	// The message has to name both, since an operator editing a device does not
	// see the hierarchy from there. Naming them is not enough: it has to say
	// which one contains which. stad is the child of gemeente-a, and the
	// sentence used to claim the opposite, which is worse than saying nothing
	// to a reader who cannot see the tree.
	if want := `"stad" is already inside "gemeente-a"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not read %s", err, want)
	}
	if got := f.Devices["lt-1"].Groups; len(got) != 1 {
		t.Errorf("the device was changed anyway: %v", got)
	}
}

func TestAddingADeviceToAGroupCannotDuplicateAnAncestor(t *testing.T) {
	f := guarded()

	if err := CreateGroupWithDevices("nieuw", Group{Parent: "stad"}, []string{"lt-1"})(f); err == nil {
		t.Fatal("the group route accepted a child of a group the device is already in")
	}
	if got := f.Devices["lt-1"].Groups; len(got) != 1 {
		t.Errorf("membership changed despite the refusal: %v", got)
	}
}

// The guard must not overreach. Two groups from unrelated branches remain a
// legitimate thing to write until the model itself changes (#115), and a
// device that is simply moved to another group has nothing to do with
// ancestry at all.
func TestTheGuardLeavesLegitimateMembershipAlone(t *testing.T) {
	f := guarded()

	both := []string{"stad", "kantoor"}
	if err := UpdateDevice("lt-1", DevicePatch{Groups: &both})(f); err != nil {
		t.Errorf("two independent groups were refused: %v", err)
	}

	one := []string{"kantoor"}
	if err := UpdateDevice("lt-1", DevicePatch{Groups: &one})(f); err != nil {
		t.Errorf("moving to a single group was refused: %v", err)
	}
	if got := f.Devices["lt-1"].Groups; len(got) != 1 || got[0] != "kantoor" {
		t.Errorf("groups = %v, want the move to have happened", got)
	}
}

// TestOldRedundancyDoesNotBlockAnUnrelatedBulkAdd is the shape the demo fleet
// is in: 27 of 148 devices list a group and its own parent, written before
// this guard existed. Adding one of them to a group that has nothing to do
// with either must work. Judging the whole combined membership refused it and
// named two groups the operator never touched, from a screen that offers no
// way to fix them.
func TestOldRedundancyDoesNotBlockAnUnrelatedBulkAdd(t *testing.T) {
	f := guarded()
	d := f.Devices["lt-1"]
	d.Groups = []string{"stad", "gemeente-a"} // grandfathered
	f.Devices["lt-1"] = d

	if err := CreateGroupWithDevices("backoffice", Group{}, []string{"lt-1"})(f); err != nil {
		t.Fatalf("adding a grandfathered device to an unrelated group was refused: %v", err)
	}
	got := f.Devices["lt-1"].Groups
	if len(got) != 3 || got[2] != "backoffice" {
		t.Errorf("groups = %v, want the old two plus backoffice", got)
	}
}

// And the guard still has to fire when the ADDITION is the duplication: a new
// group under stad, for a device already in stad.
func TestAddingAGroupUnderOneAlreadyThereIsStillRefused(t *testing.T) {
	f := guarded()
	err := CreateGroupWithDevices("stad-noord", Group{Parent: "stad"}, []string{"lt-1"})(f)
	if err == nil {
		t.Fatal("adding a group under one the device already has was accepted")
	}
	if !strings.Contains(err.Error(), "stad-noord") || !strings.Contains(err.Error(), "stad") {
		t.Errorf("error %q does not name both groups", err)
	}
	if got := f.Devices["lt-1"].Groups; len(got) != 1 {
		t.Errorf("the device was changed anyway: %v", got)
	}
}
