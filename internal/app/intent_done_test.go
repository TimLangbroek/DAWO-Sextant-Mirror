package app

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
)

// A one-shot intent retires itself when the device reports it done, success
// or failure, and nothing else retires it: not an ack for another intent,
// not a beat without an ack, and never a lock, which stands until lifted.
func TestACompletedIntentRetiresItself(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		intent, ack string
		cleared     bool
	}{
		{fleet.IntentReboot, observed.AckRebooted, true},
		{fleet.IntentDiagnostics, observed.AckDiagnostics, true},
		{fleet.IntentDiagnostics, observed.AckDiagnosticsFailed, true},
		{fleet.IntentRotateAdmin, observed.AckAdminRotated, true},
		{fleet.IntentRotateAdmin, observed.AckAdminRotateFailed, true},
		{fleet.IntentRotateAdmin, observed.AckDiagnostics, false},
		{fleet.IntentRotateAdmin, "", false},
		{fleet.IntentLock, observed.AckLock, false},
	} {
		t.Run(c.intent+"/"+c.ack, func(t *testing.T) {
			svc, dir := newCatalogService(t, `{"version": 3,
  "devices": {"lt-1": {"hardware": "hw", "intent": "`+c.intent+`"}}}`)
			if err := svc.ClearCompletedIntent(ctx, observed.CheckIn{Tag: "lt-1", Ack: c.ack}); err != nil {
				t.Fatal(err)
			}
			got := svc.Fleet().Devices["lt-1"].Intent
			if c.cleared && got != "" {
				t.Errorf("intent %q stands after %q", got, c.ack)
			}
			if !c.cleared && got != c.intent {
				t.Errorf("intent %q became %q after %q", c.intent, got, c.ack)
			}
			if c.cleared {
				out, _ := exec.Command("git", "-C", dir, "log", "-1", "--format=%s %an").Output()
				if want := "intent: " + c.intent + " completed on lt-1 sextant-agent"; strings.TrimSpace(string(out)) != want {
					t.Errorf("commit %q, want %q", strings.TrimSpace(string(out)), want)
				}
			}
		})
	}
	svc, _ := newCatalogService(t, `{"version": 3, "devices": {}}`)
	if err := svc.ClearCompletedIntent(ctx, observed.CheckIn{Tag: "gone", Ack: observed.AckRebooted}); err != nil {
		t.Errorf("an ack from a device no longer in the fleet: %v", err)
	}
}
