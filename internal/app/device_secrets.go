package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// DeviceSecretsService is the application surface of the per-device secret
// store: provisioning seals a secret at rest, and an authorized operator
// reveals it. The domain names the kinds; a ports.Sealer encrypts (in-process
// AES-256-GCM by default, an external key manager - OpenBao / Vault - as a
// drop-in); the store persists ciphertext only. RBAC is enforced by the caller
// (reveal is owner reach) - this service records who acted so every reveal is
// auditable.
type DeviceSecretsService struct {
	store  ports.DeviceSecretStore
	sealer ports.Sealer
	clock  ports.Clock
	tenant string
	// fault holds what the start-up key check found, when it found secrets
	// the key does not open; nil otherwise.
	fault atomic.Pointer[KeyCheck]
}

// NewDeviceSecretsService wires the per-device secret store for one tenant.
func NewDeviceSecretsService(store ports.DeviceSecretStore, sealer ports.Sealer, clock ports.Clock, tenant string) *DeviceSecretsService {
	if tenant == "" {
		tenant = DefaultTenant
	}
	return &DeviceSecretsService{store: store, sealer: sealer, clock: clock, tenant: tenant}
}

// Enabled reports whether a secret store and an encryption key are both present.
// A disabled service stores nothing (rather than persisting plaintext), so the
// caller can fall back or warn.
func (s *DeviceSecretsService) Enabled() bool {
	return s != nil && s.store != nil && s.sealer.Enabled()
}

// Store seals plaintext and persists it for a device+kind. It refuses an
// unknown kind, an empty value, and a disabled encryption key - never writing a
// secret it cannot protect.
func (s *DeviceSecretsService) Store(ctx context.Context, tag string, kind secret.Kind, plaintext, createdBy string) error {
	if err := kind.Validate(); err != nil {
		return err
	}
	if plaintext == "" {
		return fmt.Errorf("refusing to store an empty %s secret for %s", kind, tag)
	}
	if !s.sealer.Enabled() {
		return ports.ErrSealerDisabled
	}
	sealed, err := s.sealer.Seal([]byte(plaintext))
	if err != nil {
		return fmt.Errorf("seal %s secret for %s: %w", kind, tag, err)
	}
	return s.store.Put(ctx, s.tenant, tag, kind, sealed, createdBy, s.clock.Now())
}

// Reveal unseals a device's secret and stamps who read it. RBAC (owner) is the
// caller's responsibility; revealedBy is recorded for the audit trail. Returns
// ok=false when no such secret exists.
func (s *DeviceSecretsService) Reveal(ctx context.Context, tag string, kind secret.Kind, revealedBy string) (string, bool, error) {
	if err := kind.Validate(); err != nil {
		return "", false, err
	}
	sealed, _, ok, err := s.store.Get(ctx, s.tenant, tag, kind)
	if err != nil || !ok {
		return "", ok, err
	}
	plaintext, err := s.sealer.Open(sealed)
	if err != nil {
		return "", true, fmt.Errorf("open %s secret for %s: %w", kind, tag, err)
	}
	if err := s.store.MarkRevealed(ctx, s.tenant, tag, kind, revealedBy, s.clock.Now()); err != nil {
		return "", true, fmt.Errorf("record reveal of %s secret for %s: %w", kind, tag, err)
	}
	return string(plaintext), true, nil
}

// Reveals returns every reveal of a device secret in [from, to), oldest
// first: the audit record, which keeps the reveals Meta's last-reader stamp
// overwrote.
func (s *DeviceSecretsService) Reveals(ctx context.Context, from, to time.Time) ([]secret.Reveal, error) {
	return s.store.Reveals(ctx, s.tenant, from, to)
}

// List returns the metadata of a device's secrets (which kinds exist, who
// created them, whether they have been revealed) - never any plaintext.
func (s *DeviceSecretsService) List(ctx context.Context, tag string) ([]secret.Meta, error) {
	return s.store.List(ctx, s.tenant, tag)
}

// KeyCheck is what the console found when it tried its secret key on every
// device secret it holds (#7). A backup holds the sealed secrets and never the
// key, so a restore under the wrong key, or with none, looks complete until
// somebody needs a recovery key and cannot open it. The check finds that at
// start instead.
type KeyCheck struct {
	Total    int // device secrets stored
	Unopened int // of those, how many the configured key does not open
	// Reason is the first refusal. The sealer names the key that sealed a
	// value it cannot open, so this says which key to put back. It names a
	// key id, which identifies a key and reveals nothing of it.
	Reason string
}

// CheckKey tries the configured key on every stored device secret, without
// revealing any or stamping a reveal.
func (s *DeviceSecretsService) CheckKey(ctx context.Context) (KeyCheck, error) {
	rows, err := s.store.Sealed(ctx, s.tenant)
	if err != nil {
		return KeyCheck{}, err
	}
	kc := KeyCheck{Total: len(rows)}
	for _, r := range rows {
		if !s.sealer.Enabled() {
			kc.Unopened, kc.Reason = len(rows), "no secret key is configured (SEXTANT_SECRET_KEY is empty)"
			break
		}
		if _, err := s.sealer.Open(r.Ciphertext); err != nil {
			kc.Unopened++
			if kc.Reason == "" {
				kc.Reason = err.Error()
			}
		}
	}
	return kc, nil
}

// RunKeyCheck runs CheckKey once and keeps the result for KeyFault. Once is
// enough: the key only changes with a restart, and everything sealed after
// start is sealed with the key in use.
func (s *DeviceSecretsService) RunKeyCheck(ctx context.Context, log *slog.Logger) {
	kc, err := s.CheckKey(ctx)
	switch {
	case err != nil:
		log.Error("secret key check: cannot read the stored device secrets", "err", err)
	case kc.Unopened > 0:
		s.fault.Store(&kc)
		log.Error("the secret key does not open every stored device secret; they cannot be revealed until it does",
			"unopened", kc.Unopened, "stored", kc.Total, "first", kc.Reason,
			"remedy", "set SEXTANT_SECRET_KEY to the key that sealed them, or add it after the current one")
	default:
		log.Info("secret key opens every stored device secret", "stored", kc.Total)
	}
}

// KeyFault reports the start-up key check when it found secrets the key does
// not open, and nil otherwise.
func (s *DeviceSecretsService) KeyFault() *KeyCheck {
	if s == nil {
		return nil
	}
	return s.fault.Load()
}
