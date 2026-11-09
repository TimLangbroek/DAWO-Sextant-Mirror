package starter

import (
	"io/fs"
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
