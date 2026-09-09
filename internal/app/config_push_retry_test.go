package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// config_push_retry_test.go: the shape of the HA write loop.
//
// applyWithGate is the only path a device-visible change takes, and on a repo
// with a remote it is a loop: sync, apply, push, and on a lost race do it
// again on the new base. The happy path and one real race are covered
// elsewhere. What was not covered is what the loop refuses to retry, and that
// is the half that matters: retrying the wrong error either hides a broken
// remote behind five attempts or, worse, re-runs a mutation whose gate said
// no.
//
// The fake wraps the REAL git repo and only lies about Push and Sync, so
// everything the transaction does to the working tree stays honest.

type flakyRepo struct {
	ports.ConfigRepo
	pushErrs []error // consumed in order; nil means success
	pushes   int
	syncs    int
	syncErr  error
}

func (r *flakyRepo) Push(ctx context.Context) error {
	r.pushes++
	if len(r.pushErrs) > 0 {
		err := r.pushErrs[0]
		r.pushErrs = r.pushErrs[1:]
		if err != nil {
			return err
		}
	}
	return r.ConfigRepo.Push(ctx)
}

func (r *flakyRepo) Sync(ctx context.Context) error {
	r.syncs++
	if r.syncErr != nil {
		return r.syncErr
	}
	return r.ConfigRepo.Sync(ctx)
}

// retryFixture is a repo with a real remote, wrapped so pushes can be made to
// fail on demand.
func retryFixture(t *testing.T) (*ConfigService, *flakyRepo) {
	t.Helper()
	dir := t.TempDir()
	sh(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), []byte(reapSeed), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, dir, "add", "fleet.json")
	sh(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "seed")
	bare := filepath.Join(t.TempDir(), "remote.git")
	sh(t, dir, "clone", "-q", "--bare", dir, bare)
	sh(t, dir, "remote", "add", "origin", bare)
	sh(t, dir, "push", "-q", "origin", "main")

	real, err := git.Open(dir, "origin")
	if err != nil {
		t.Fatal(err)
	}
	fake := &flakyRepo{ConfigRepo: real}
	svc, err := NewConfigService(fake, ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if !svc.repo.HasRemote() {
		t.Fatal("fixture: the service does not see a remote, so the loop under test never runs")
	}
	return svc, fake
}

// assign is a device-visible change, so it goes through the gated path.
// The device is fixed: which one it is has no bearing on the loop, and
// naming it once keeps every case reading the same.
const raceDevice = "stale-active"

func assignAda() fleet.Mutation {
	user := "ada"
	return fleet.UpdateDevice(raceDevice, fleet.DevicePatch{AssignedUser: &user})
}

func TestALostRaceIsRetried(t *testing.T) {
	svc, fake := retryFixture(t)
	fake.pushErrs = []error{fmt.Errorf("remote advanced: %w", ports.ErrConflict)}

	if err := svc.Apply(context.Background(), assignAda(),
		"devices: assign", ports.Author{Name: "t"}, raceDevice); err != nil {
		t.Fatalf("a single lost race was not retried: %v", err)
	}
	if fake.pushes != 2 {
		t.Errorf("pushes = %d, want 2: one lost race, one that landed", fake.pushes)
	}
	if fake.syncs != 2 {
		t.Errorf("syncs = %d, want one before each attempt; a retry on a stale base rewrites somebody else's commit", fake.syncs)
	}
}

func TestAPushErrorThatIsNotAConflictIsNotRetried(t *testing.T) {
	svc, fake := retryFixture(t)
	boom := errors.New("remote refused the connection")
	fake.pushErrs = []error{boom, boom, boom, boom, boom}

	err := svc.Apply(context.Background(), assignAda(),
		"devices: assign", ports.Author{Name: "t"}, raceDevice)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the push error itself", err)
	}
	if fake.pushes != 1 {
		t.Errorf("pushes = %d, want 1: a broken remote is not a race, and five attempts only hide it", fake.pushes)
	}
}

func TestAnUnreachableRemoteFailsBeforeAnythingIsWritten(t *testing.T) {
	svc, fake := retryFixture(t)
	fake.syncErr = errors.New("host is down")

	err := svc.Apply(context.Background(), assignAda(),
		"devices: assign", ports.Author{Name: "t"}, raceDevice)
	if !errors.Is(err, fake.syncErr) {
		t.Fatalf("error = %v, want the sync error", err)
	}
	if fake.pushes != 0 {
		t.Errorf("pushes = %d, want 0: nothing may be pushed when the base could not be refreshed", fake.pushes)
	}
}

func TestPersistentConflictsGiveUpAndSaySo(t *testing.T) {
	svc, fake := retryFixture(t)
	conflict := fmt.Errorf("remote advanced: %w", ports.ErrConflict)
	for range maxPushRetries {
		fake.pushErrs = append(fake.pushErrs, conflict)
	}

	err := svc.Apply(context.Background(), assignAda(),
		"devices: assign", ports.Author{Name: "t"}, raceDevice)
	if err == nil {
		t.Fatal("a write that never landed reported success")
	}
	if !errors.Is(err, ports.ErrConflict) {
		t.Errorf("error %v does not carry ErrConflict; a caller cannot tell a race from a failure", err)
	}
	if fake.pushes != maxPushRetries {
		t.Errorf("pushes = %d, want %d: the loop is bounded", fake.pushes, maxPushRetries)
	}
}
