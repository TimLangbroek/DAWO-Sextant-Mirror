package app

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// rotationRig is a console with one or two devices, a secret store and a
// clock the test moves.
type rotationRig struct {
	cfg     *ConfigService
	dir     string
	clock   *fakeClock
	secrets *DeviceSecretsService
	rot     *AdminRotation
}

func newRotationRig(t *testing.T, devices string) *rotationRig {
	t.Helper()
	cfg, dir := newCatalogService(t, `{"version": 3, "devices": {`+devices+`}}`)
	clock := newFakeClock(testT0)
	secrets := NewDeviceSecretsService(newMemSecretStore(), testSealer(t), clock, "")
	return &rotationRig{cfg: cfg, dir: dir, clock: clock, secrets: secrets,
		rot: NewAdminRotation(secrets, cfg, quietLog())}
}

func (r *rotationRig) escrow(t *testing.T, tag string) {
	t.Helper()
	if err := r.secrets.Store(context.Background(), tag, secret.Admin, "abcde-fghij-klmno-pqrst", "device:"+tag); err != nil {
		t.Fatal(err)
	}
}

func (r *rotationRig) reveal(t *testing.T, tag string, kind secret.Kind) {
	t.Helper()
	if _, ok, err := r.secrets.Reveal(context.Background(), tag, kind, "owner@example.com"); err != nil || !ok {
		t.Fatalf("reveal %s/%s: ok=%v err=%v", tag, kind, ok, err)
	}
}

func (r *rotationRig) tick(t *testing.T) []string {
	t.Helper()
	got, err := r.rot.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (r *rotationRig) intent(tag string) string { return r.cfg.Fleet().Devices[tag].Intent }

// A revealed administrator password is rotated eight hours after the first
// reveal, not before, and a second reveal in between does not push it back.
func TestARevealedAdminPasswordIsRotatedEightHoursLater(t *testing.T) {
	r := newRotationRig(t, `"lt-1": {"hardware": "hw"}`)
	r.escrow(t, "lt-1")
	r.clock.Advance(time.Minute)
	r.reveal(t, "lt-1", secret.Admin)
	r.clock.Advance(4 * time.Hour)
	r.reveal(t, "lt-1", secret.Admin)

	r.clock.Advance(4*time.Hour - time.Second)
	if got := r.tick(t); len(got) != 0 || r.intent("lt-1") != "" {
		t.Fatalf("armed %v before eight hours, intent %q", got, r.intent("lt-1"))
	}
	r.clock.Advance(time.Second)
	if got := r.tick(t); !slices.Equal(got, []string{"lt-1"}) {
		t.Fatalf("armed %v at eight hours, want lt-1", got)
	}
	if got := r.intent("lt-1"); got != fleet.IntentRotateAdmin {
		t.Fatalf("intent %q, want %q", got, fleet.IntentRotateAdmin)
	}
	out, _ := exec.Command("git", "-C", r.dir, "log", "-1", "--format=%s|%an").Output()
	if got, want := strings.TrimSpace(string(out)),
		"intent: rotate-admin lt-1 (administrator password revealed 2026-07-09T12:01:00Z)|sextant-rotation"; got != want {
		t.Errorf("commit %q, want %q", got, want)
	}
}

// Once the device holds a new password, reveals of the old one count for
// nothing; the new one is rotated eight hours after it is read in turn.
func TestANewPasswordStartsItsOwnClock(t *testing.T) {
	r := newRotationRig(t, `"lt-1": {"hardware": "hw"}`)
	r.escrow(t, "lt-1")
	r.reveal(t, "lt-1", secret.Admin)
	r.clock.Advance(time.Hour)
	r.escrow(t, "lt-1") // rotated by hand

	r.clock.Advance(9 * time.Hour)
	if got := r.tick(t); len(got) != 0 {
		t.Fatalf("armed %v for a password that was never read", got)
	}
	r.reveal(t, "lt-1", secret.Admin)
	r.clock.Advance(AdminRotateAfter)
	if got := r.tick(t); !slices.Equal(got, []string{"lt-1"}) {
		t.Fatalf("armed %v, want lt-1", got)
	}
}

// A pending lock or wipe is never replaced, a rotation the device failed is
// not armed again every tick, and a recovery-key reveal rotates nothing.
func TestRotationLeavesOtherActionsAndFailuresAlone(t *testing.T) {
	r := newRotationRig(t, `"lt-1": {"hardware": "hw", "intent": "lock"},
  "lt-2": {"hardware": "hw"}, "lt-3": {"hardware": "hw"}`)
	for _, tag := range []string{"lt-1", "lt-2", "lt-3"} {
		r.escrow(t, tag)
	}
	if err := r.secrets.Store(context.Background(), "lt-3", secret.LUKS, "luks-phrase", "device:lt-3"); err != nil {
		t.Fatal(err)
	}
	r.reveal(t, "lt-1", secret.Admin)
	r.reveal(t, "lt-2", secret.Admin)
	r.reveal(t, "lt-3", secret.LUKS)
	r.clock.Advance(AdminRotateAfter)
	// lt-3's administrator password is read only now; its recovery key was
	// read eight hours ago, and that is not when this password was seen.
	r.reveal(t, "lt-3", secret.Admin)

	if got := r.tick(t); !slices.Equal(got, []string{"lt-2"}) {
		t.Fatalf("armed %v, want only lt-2", got)
	}
	if got := r.intent("lt-1"); got != fleet.IntentLock {
		t.Errorf("lt-1 intent %q, the lock must stand", got)
	}
	if got := r.intent("lt-3"); got != "" {
		t.Errorf("lt-3 intent %q, armed from a recovery-key reveal", got)
	}

	// The device answers admin-rotate-failed: the intent clears and the
	// password it holds is still the one that was read.
	if err := r.cfg.ApplyStructural(context.Background(), fleet.ClearDeviceIntent("lt-2"),
		"intent: rotate-admin completed on lt-2", ports.Author{Name: "sextant-agent", Email: "agent@sextant"}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Hour)
	if got := r.tick(t); len(got) != 0 {
		t.Fatalf("armed %v again for the same reveal", got)
	}
}

// Without a secret store there is nothing to sweep.
func TestNoRotationWithoutASecretStore(t *testing.T) {
	cfg, _ := newCatalogService(t, `{"version": 3, "devices": {}}`)
	if r := NewAdminRotation(nil, cfg, quietLog()); r != nil {
		t.Error("a sweep without a secret service")
	}
	if r := NewAdminRotation(NewDeviceSecretsService(nil, testSealer(t), newFakeClock(testT0), ""), cfg, quietLog()); r != nil {
		t.Error("a sweep without a store")
	}
}

// A lock armed between the sweep's read and its write stands: the write
// itself refuses a device with something pending.
func TestArmRotationRefusesAPendingAction(t *testing.T) {
	f := &fleet.Fleet{Devices: map[string]fleet.Device{
		"lt-1": {Intent: fleet.IntentLock},
		"lt-2": {},
	}}
	if err := armRotation("lt-1")(f); err == nil || f.Devices["lt-1"].Intent != fleet.IntentLock {
		t.Errorf("err %v, intent %q: the lock must stand", err, f.Devices["lt-1"].Intent)
	}
	if err := armRotation("lt-2")(f); err != nil || f.Devices["lt-2"].Intent != fleet.IntentRotateAdmin {
		t.Errorf("err %v, intent %q", err, f.Devices["lt-2"].Intent)
	}
}
