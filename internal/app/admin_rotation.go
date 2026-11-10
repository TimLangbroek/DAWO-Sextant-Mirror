package app

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// admin_rotation.go: a revealed local administrator password does not
// outlive its use (design 0017). Some hours after somebody first read the
// password a device holds, the console arms rotate-admin on that device; the
// device draws a new one and escrows it, and the one that was read stops
// working.

// AdminRotateAfter is how long a revealed administrator password stands
// before the console asks the device for a new one: time to type it on a
// device that is offline, not long enough to become the password.
const AdminRotateAfter = 8 * time.Hour

// AdminRotation arms rotate-admin on devices whose administrator password was
// revealed AdminRotateAfter ago. It holds no state that matters across a
// restart: everything it decides on is in the secret store and the fleet.
type AdminRotation struct {
	secrets *DeviceSecretsService
	config  *ConfigService
	log     *slog.Logger

	mu sync.Mutex
	// armed remembers, per device, the reveal a rotation was armed for, so a
	// rotation the device reports as failed is not armed again for the same
	// reveal on every tick. The owner sees the failure and presses Rotate.
	armed map[string]time.Time
}

// NewAdminRotation returns the rotation sweep, or nil when there is no
// secret store to read reveals from.
func NewAdminRotation(secrets *DeviceSecretsService, config *ConfigService, log *slog.Logger) *AdminRotation {
	if secrets == nil || secrets.store == nil || config == nil {
		return nil
	}
	return &AdminRotation{secrets: secrets, config: config, log: log, armed: map[string]time.Time{}}
}

// Run sweeps every interval until ctx ends.
func (r *AdminRotation) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.Tick(ctx); err != nil && ctx.Err() == nil {
				r.log.Error("administrator password rotation sweep failed", "err", err)
			}
		}
	}
}

// Tick arms rotate-admin on every device that is due and returns their tags.
// A device is due when the first reveal of the password it holds now is at
// least AdminRotateAfter old and it has no other action pending; a pending
// lock or wipe is never replaced.
func (r *AdminRotation) Tick(ctx context.Context) ([]string, error) {
	now := r.secrets.clock.Now()
	reveals, err := r.secrets.Reveals(ctx, time.Unix(0, 0), now)
	if err != nil {
		return nil, err
	}
	byTag := map[string][]time.Time{}
	for _, rv := range reveals {
		if rv.Kind == secret.Admin {
			byTag[rv.Tag] = append(byTag[rv.Tag], rv.At)
		}
	}
	tags := make([]string, 0, len(byTag))
	for tag := range byTag {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	done := make([]string, 0, len(tags))
	for _, tag := range tags {
		first, ok, err := r.firstRevealOfCurrent(ctx, tag, byTag[tag])
		if err != nil {
			return done, err
		}
		if !ok || now.Sub(first) < AdminRotateAfter || r.wasArmed(tag, first) {
			continue
		}
		dev, inFleet := r.config.Fleet().Devices[tag]
		if !inFleet || dev.Retired() || dev.Intent != fleet.IntentNone {
			continue
		}
		msg := fmt.Sprintf("intent: rotate-admin %s (administrator password revealed %s)",
			tag, first.UTC().Format(time.RFC3339))
		if err := r.config.ApplyStructural(ctx, armRotation(tag), msg,
			ports.Author{Name: "sextant-rotation", Email: "rotation@sextant"}); err != nil {
			return done, fmt.Errorf("arm rotate-admin on %s: %w", tag, err)
		}
		r.markArmed(tag, first)
		done = append(done, tag)
	}
	return done, nil
}

// firstRevealOfCurrent returns the first reveal of the administrator password
// the device holds now. A reveal of a password it has since replaced does not
// count, and neither does a device that has no password escrowed.
func (r *AdminRotation) firstRevealOfCurrent(ctx context.Context, tag string, at []time.Time) (time.Time, bool, error) {
	_, meta, ok, err := r.secrets.store.Get(ctx, r.secrets.tenant, tag, secret.Admin)
	if err != nil || !ok || !meta.EverRevealed() {
		return time.Time{}, false, err
	}
	created, err := time.Parse(time.RFC3339, meta.Created)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("administrator password of %s: created %q: %w", tag, meta.Created, err)
	}
	for _, t := range at {
		// Created is stored to the second; a reveal in that same second
		// still read this password.
		if !t.Truncate(time.Second).Before(created) {
			return t, true, nil
		}
	}
	return time.Time{}, false, nil
}

func (r *AdminRotation) wasArmed(tag string, reveal time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.armed[tag]
	return ok && t.Equal(reveal)
}

func (r *AdminRotation) markArmed(tag string, reveal time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.armed[tag] = reveal
}

// armRotation sets rotate-admin only on a device with nothing pending: the
// fleet may have moved between the sweep's read and this write, and a lock
// or wipe armed in between must stand.
func armRotation(tag string) fleet.Mutation {
	set := fleet.SetDeviceIntent(tag, fleet.IntentRotateAdmin, false)
	return func(f *fleet.Fleet) error {
		if d, ok := f.Devices[tag]; ok && d.Intent != fleet.IntentNone {
			return fmt.Errorf("device %q has %q pending", tag, d.Intent)
		}
		return set(f)
	}
}
