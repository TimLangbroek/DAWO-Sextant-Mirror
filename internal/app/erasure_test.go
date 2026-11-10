package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// erasure_test.go. What is asserted here is mostly the HONESTY of the
// report, not the delete: a GDPR art. 17 answer that overstates what it
// removed is how somebody ends up telling a data subject their data is gone
// when it is not.

type memErasure struct {
	counts  ports.PersonalDataCounts
	erased  bool
	subject string
	user    string
}

func (m *memErasure) CountPersonalData(_ context.Context, _, subject, user string) (ports.PersonalDataCounts, error) {
	m.subject, m.user = subject, user
	return m.counts, nil
}

func (m *memErasure) ErasePersonalData(_ context.Context, _, subject, user string) (ports.PersonalDataCounts, error) {
	m.erased = true
	m.subject, m.user = subject, user
	return m.counts, nil
}

// TestPreviewRemovesNothing is the first half of the two-act design: an
// operator sees what would go before anything goes.
func TestPreviewRemovesNothing(t *testing.T) {
	store := &memErasure{counts: ports.PersonalDataCounts{SeenUser: 1, Notifications: 4}}
	s := NewErasureService(store, nil, "t1", nil)

	rep, err := s.Preview(context.Background(), "sub-1", "ada")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if store.erased {
		t.Error("a preview deleted data")
	}
	if !rep.DryRun {
		t.Error("the report does not say it was a preview")
	}
	if rep.Removed.Total() != 5 {
		t.Errorf("preview counted %d, want 5", rep.Removed.Total())
	}
}

// TestBothIdentifiersTravel: a person is an OIDC subject in the console and
// an OS username on their device, and the two do not have to match. Passing
// only one and reporting success is the failure this guards.
func TestBothIdentifiersTravel(t *testing.T) {
	store := &memErasure{}
	s := NewErasureService(store, nil, "t1", nil)
	if _, err := s.Erase(context.Background(), "sub-1", "ada", ports.Author{Name: "op"}); err != nil {
		t.Fatal(err)
	}
	if store.subject != "sub-1" || store.user != "ada" {
		t.Errorf("store got subject=%q user=%q; both must travel", store.subject, store.user)
	}
}

func TestErasureNeedsAtLeastOneIdentifier(t *testing.T) {
	s := NewErasureService(&memErasure{}, nil, "t1", nil)
	if _, err := s.Preview(context.Background(), "  ", " "); err == nil {
		t.Error("an erasure with no identifier at all was accepted")
	}
	// Blank identifiers are the dangerous case: an empty subject matches
	// every row whose subject column is empty, and group notifications have
	// exactly that shape.
	store := &memErasure{}
	s = NewErasureService(store, nil, "t1", nil)
	if _, err := s.Erase(context.Background(), "", "", ports.Author{}); err == nil {
		t.Error("a blank erasure ran")
	}
	if store.erased {
		t.Error("a blank erasure reached the store")
	}
}

// TestTheReportAlwaysNamesWhatSurvives is the point of the whole feature.
func TestTheReportAlwaysNamesWhatSurvives(t *testing.T) {
	s := NewErasureService(&memErasure{}, nil, "t1", nil)
	rep, err := s.Erase(context.Background(), "sub-1", "ada", ports.Author{Name: "op"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Remaining) == 0 {
		t.Fatal("the report claims nothing remains; the git history always does")
	}
	joined := strings.ToLower(strings.Join(rep.Remaining, " "))
	for _, want := range []string{"git history", "diagnostics", "device-secret reveals"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the report does not mention %q:\n%s", want, joined)
		}
	}
}

// TestAMissingIdentifierIsCalledOut: an operator who supplies only one name
// must be told which half was not searched, rather than reading a clean
// report and believing it was complete.
func TestAMissingIdentifierIsCalledOut(t *testing.T) {
	s := NewErasureService(&memErasure{}, nil, "t1", nil)

	rep, err := s.Preview(context.Background(), "sub-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Remaining, " "), "device username") {
		t.Error("no device username was given and the report does not say so")
	}

	if rep, err = s.Preview(context.Background(), "", "ada"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Remaining, " "), "console subject") {
		t.Error("no console subject was given and the report does not say so")
	}
}

// TestDecisionsMadeForOthersAreLeftAndExplained: erasing a request this
// person DECIDED would destroy a different data subject's record of who
// approved their access.
func TestDecisionsMadeForOthersAreLeftAndExplained(t *testing.T) {
	store := &memErasure{counts: ports.PersonalDataCounts{Elevation: 2, ElevationDecided: 3}}
	s := NewErasureService(store, nil, "t1", nil)
	rep, err := s.Erase(context.Background(), "sub-1", "ada", ports.Author{Name: "op"})
	if err != nil {
		t.Fatal(err)
	}
	// Decisions are not counted as removed.
	if rep.Removed.Total() != 2 {
		t.Errorf("removed total = %d, want 2 (the raised requests only)", rep.Removed.Total())
	}
	joined := strings.Join(rep.Remaining, " ")
	if !strings.Contains(joined, "DECIDED") {
		t.Errorf("the surviving decisions are not explained:\n%s", joined)
	}
}

// erasureSeed: three devices, one assigned to the person being erased under
// their console subject, one under their device username, one belonging to
// somebody else whose name merely starts the same way.
const erasureSeed = `{
  "version": 3,
  "org": {"settings": {}},
  "groups": {"pilot": {}},
  "devices": {
    "lt-sub":   {"groups": ["pilot"], "hardware": "hw", "assignedUser": "Sub-1"},
    "lt-user":  {"groups": ["pilot"], "hardware": "hw", "assignedUser": "ada"},
    "lt-other": {"groups": ["pilot"], "hardware": "hw", "assignedUser": "adalovelace"},
    "lt-free":  {"groups": ["pilot"], "hardware": "hw"}
  }
}`

func erasureConfig(t *testing.T) *ConfigService {
	t.Helper()
	dir := t.TempDir()
	sh(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), []byte(erasureSeed), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, dir, "add", "fleet.json")
	sh(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "seed")
	repo, err := git.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewConfigService(repo, ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// TestErasureClearsTheRightDevicesAndOnlyThose covers the half of run() that
// touches the fleet document, which had no test.
//
// Both directions are damage. Missing a device leaves a person's name in a
// file that is published to every machine, after telling them their data was
// erased. Clearing one too many rewrites somebody else's assignment on the
// word of a partial name match.
func TestErasureClearsTheRightDevicesAndOnlyThose(t *testing.T) {
	ctx := context.Background()
	cfg := erasureConfig(t)
	store := &memErasure{}
	s := NewErasureService(store, cfg, "t1", nil)

	rep, err := s.Erase(ctx, "sub-1", "ada", ports.Author{Name: "dpo"})
	if err != nil {
		t.Fatalf("erase: %v", err)
	}

	cleared := map[string]bool{}
	for _, tag := range rep.DevicesCleared {
		cleared[tag] = true
	}
	// Matched on the subject, matched on the username, and case does not
	// decide: whoever typed "Sub-1" into the field is not the reason a
	// request fails.
	for _, want := range []string{"lt-sub", "lt-user"} {
		if !cleared[want] {
			t.Errorf("%s was not cleared; the name stays in a file every machine reads", want)
		}
	}
	// "adalovelace" starts with "ada" and is a different person.
	if cleared["lt-other"] {
		t.Error("lt-other was cleared on a partial name match; that is somebody else's assignment")
	}
	if cleared["lt-free"] {
		t.Error("a device with no assigned user was reported as cleared")
	}

	// And the document actually changed, not just the report.
	f := cfg.Fleet()
	for _, tag := range []string{"lt-sub", "lt-user"} {
		if got := f.Devices[tag].AssignedUser; got != "" {
			t.Errorf("%s still carries %q after the erasure reported it cleared", tag, got)
		}
	}
	if got := f.Devices["lt-other"].AssignedUser; got != "adalovelace" {
		t.Errorf("lt-other lost its assignment: %q", got)
	}
}

// TestAPreviewLeavesTheFleetDocumentAlone: the two-act design has to hold on
// this side too. A preview that rewrites the document is a deletion nobody
// confirmed.
func TestAPreviewLeavesTheFleetDocumentAlone(t *testing.T) {
	ctx := context.Background()
	cfg := erasureConfig(t)
	s := NewErasureService(&memErasure{}, cfg, "t1", nil)

	rep, err := s.Preview(ctx, "sub-1", "ada")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(rep.DevicesCleared) != 2 {
		t.Errorf("preview named %v, want the two it would clear", rep.DevicesCleared)
	}
	f := cfg.Fleet()
	if f.Devices["lt-sub"].AssignedUser == "" || f.Devices["lt-user"].AssignedUser == "" {
		t.Error("the preview cleared an assignment; nobody confirmed that")
	}
}
