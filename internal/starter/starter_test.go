package starter

import (
	"io/fs"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
)

// The starter is what a new console opens first; a document the console
// itself refuses would make setup end in an error page.
func TestStarterFleetDecodes(t *testing.T) {
	raw, err := fs.ReadFile(Overlay(), "fleet.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := fleet.Decode(raw)
	if err != nil {
		t.Fatalf("starter fleet.json does not decode: %v", err)
	}
	if len(f.Groups) == 0 {
		t.Fatal("starter has no group to put a first laptop in")
	}
}

// For fills in the console block and leaves the rest of the starter as it
// is: a starter that lost a file on the way would seed a fleet that cannot
// build.
func TestForFillsTheConsoleBlock(t *testing.T) {
	files, err := For("https://console.example.org/", "stable")
	if err != nil {
		t.Fatal(err)
	}
	want, err := fs.Glob(Overlay(), "*")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range want {
		if _, err := fs.Stat(files, name); err != nil {
			t.Errorf("starter file %s missing: %v", name, err)
		}
	}
	raw, err := fs.ReadFile(files, "fleet.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := fleet.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Console == nil || f.Console.URL != "https://console.example.org" || f.Console.Branch != "stable" {
		t.Fatalf("console block %+v", f.Console)
	}
	if len(f.Groups) == 0 {
		t.Fatal("the rest of the starter was lost")
	}

	// No address known: none written, rather than an empty one that reads
	// as set.
	files, err = For("", "main")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = fs.ReadFile(files, "fleet.json")
	if strings.Contains(string(raw), `"url"`) {
		t.Fatalf("an empty address was written:\n%s", raw)
	}
}

// A laptop that joins with a code (design 0015) falls back to a generic
// profile when nothing names its model. Without these in the starter, every
// join to a console that setup made refused with "no hardware profile fits",
// and the only way out was editing the repository by hand: the step C2 says
// a newcomer never takes.
func TestStarterCarriesTheJoinFallbackProfiles(t *testing.T) {
	raw, err := fs.ReadFile(Overlay(), fleet.HardwareProfilesFile)
	if err != nil {
		t.Fatal(err)
	}
	hw, err := fleet.ParseHardwareProfiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"generic-intel", "generic-amd", "vm"} {
		if !hw.Has(name) {
			t.Errorf("starter has no %q profile", name)
		}
	}
}
