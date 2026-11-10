package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
)

func TestDeviceSecretsPutGetListMarkRevealed(t *testing.T) {
	s := openStore(t).DeviceSecrets()
	ctx := context.Background()

	if _, _, ok, err := s.Get(ctx, "t1", "lt-1", secret.LUKS); err != nil || ok {
		t.Fatalf("empty get = %v %v", ok, err)
	}

	// Put -> Get round trip: ciphertext and metadata come back intact, and a
	// freshly stored secret reads as never revealed.
	cipher := []byte("sealed-luks-recovery-passphrase")
	if err := s.Put(ctx, "t1", "lt-1", secret.LUKS, cipher, "alice@example.com", t0); err != nil {
		t.Fatal(err)
	}
	got, meta, ok, err := s.Get(ctx, "t1", "lt-1", secret.LUKS)
	if err != nil || !ok {
		t.Fatalf("get = %v %v", ok, err)
	}
	if string(got) != string(cipher) {
		t.Fatalf("ciphertext round trip = %q, want %q", got, cipher)
	}
	if meta.CreatedBy != "alice@example.com" || meta.Kind != secret.LUKS || meta.Tag != "lt-1" {
		t.Fatalf("meta = %+v", meta)
	}
	if meta.EverRevealed() {
		t.Fatalf("freshly stored secret must read as never revealed: %+v", meta)
	}

	// List returns metadata only, never ciphertext, and covers every kind
	// stored for the device.
	if err := s.Put(ctx, "t1", "lt-1", secret.Admin, []byte("sealed-admin-pw"), "alice@example.com", t0); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, "t1", "lt-1")
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if list[0].Kind != secret.Admin || list[1].Kind != secret.LUKS {
		t.Fatalf("list not ordered by kind: %+v", list)
	}

	// MarkRevealed stamps who read it and when.
	revealedAt := t0.Add(time.Hour)
	if err := s.MarkRevealed(ctx, "t1", "lt-1", secret.LUKS, "bob@example.com", revealedAt); err != nil {
		t.Fatal(err)
	}
	_, meta, _, err = s.Get(ctx, "t1", "lt-1", secret.LUKS)
	if err != nil {
		t.Fatal(err)
	}
	if meta.RevealedBy != "bob@example.com" || !meta.EverRevealed() {
		t.Fatalf("reveal not recorded: %+v", meta)
	}
	if meta.Revealed != revealedAt.UTC().Format(time.RFC3339) {
		t.Fatalf("revealed timestamp = %q, want %q", meta.Revealed, revealedAt.UTC().Format(time.RFC3339))
	}

	// Put after a reveal (rotation) clears the revealed marker again, so a
	// freshly rotated secret does not read as already seen.
	if err := s.Put(ctx, "t1", "lt-1", secret.LUKS, []byte("new-sealed-value"), "carol@example.com", t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rotatedCipher, meta, _, err := s.Get(ctx, "t1", "lt-1", secret.LUKS)
	if err != nil {
		t.Fatal(err)
	}
	if string(rotatedCipher) != "new-sealed-value" {
		t.Fatalf("rotated ciphertext = %q", rotatedCipher)
	}
	if meta.EverRevealed() || meta.RevealedBy != "" || meta.CreatedBy != "carol@example.com" {
		t.Fatalf("rotation did not reset reveal state: %+v", meta)
	}
}

func TestDeviceSecretsTenantIsolation(t *testing.T) {
	s := openStore(t).DeviceSecrets()
	ctx := context.Background()

	if err := s.Put(ctx, "org-a", "lt-1", secret.LUKS, []byte("a-secret"), "alice", t0); err != nil {
		t.Fatal(err)
	}

	// A tag collision under a different tenant must not see tenant A's
	// secret via Get or List: kind alone does not identify the row.
	if _, _, ok, err := s.Get(ctx, "org-b", "lt-1", secret.LUKS); err != nil || ok {
		t.Fatalf("cross-tenant get = %v %v, want not found", ok, err)
	}
	list, err := s.List(ctx, "org-b", "lt-1")
	if err != nil || len(list) != 0 {
		t.Fatalf("cross-tenant list = %+v, %v, want empty", list, err)
	}

	// The owning tenant still sees it.
	if _, _, ok, err := s.Get(ctx, "org-a", "lt-1", secret.LUKS); err != nil || !ok {
		t.Fatalf("owning tenant get = %v %v", ok, err)
	}
}

// Sealed hands the start-up key check every ciphertext of one tenant, as
// stored, and nothing of another tenant's.
func TestDeviceSecretsSealedListsOneTenantsCiphertext(t *testing.T) {
	s := openStore(t).DeviceSecrets()
	ctx := context.Background()
	for _, r := range []struct{ tenant, tag string }{{"t1", "lt-2"}, {"t1", "lt-1"}, {"t2", "lt-9"}} {
		if err := s.Put(ctx, r.tenant, r.tag, secret.LUKS, []byte("sealed-"+r.tag), "a@example.com", t0); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Sealed(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Tag != "lt-1" || got[1].Tag != "lt-2" ||
		string(got[0].Ciphertext) != "sealed-lt-1" || got[0].Kind != secret.LUKS {
		t.Fatalf("sealed = %+v", got)
	}
}

// The last-reader stamp is overwritten by the next reveal and cleared by the
// next Put. The reveal log is what keeps them all: a second reveal and a
// rotation must leave the first reveal on record, a secret that does not
// exist logs nothing, and one tenant never sees another's reveals.
func TestDeviceSecretRevealsKeepEveryReveal(t *testing.T) {
	s := openStore(t).DeviceSecrets()
	ctx := context.Background()
	if err := s.Put(ctx, "t1", "lt-1", secret.Admin, []byte("sealed-1"), "agent", t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "t2", "lt-1", secret.Admin, []byte("sealed-x"), "agent", t0); err != nil {
		t.Fatal(err)
	}
	reveal := func(tenant, tag, by string, at time.Time) {
		t.Helper()
		if err := s.MarkRevealed(ctx, tenant, tag, secret.Admin, by, at); err != nil {
			t.Fatal(err)
		}
	}
	reveal("t1", "lt-1", "alice@example.com", t0.Add(time.Hour))
	reveal("t1", "lt-1", "bob@example.com", t0.Add(2*time.Hour))
	// A rotation: the stamp goes, the record must not.
	if err := s.Put(ctx, "t1", "lt-1", secret.Admin, []byte("sealed-2"), "agent", t0.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	reveal("t1", "lt-1", "carol@example.com", t0.Add(4*time.Hour))
	reveal("t1", "nope", "mallory@example.com", t0.Add(4*time.Hour))
	reveal("t2", "lt-1", "dave@example.com", t0.Add(4*time.Hour))

	got, err := s.Reveals(ctx, "t1", t0, t0.Add(5*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	by := make([]string, 0, len(got))
	for _, r := range got {
		by = append(by, r.By)
		if r.Tag != "lt-1" || r.Kind != secret.Admin {
			t.Errorf("reveal = %+v", r)
		}
	}
	if strings.Join(by, ",") != "alice@example.com,bob@example.com,carol@example.com" {
		t.Fatalf("reveals = %v, want alice, bob and carol in that order", by)
	}
	if !got[0].At.Equal(t0.Add(time.Hour)) {
		t.Errorf("first reveal at %v, want %v", got[0].At, t0.Add(time.Hour))
	}

	// The period is [from, to): bob's reveal at exactly +2h is in a window
	// starting there and out of one ending there.
	from, err := s.Reveals(ctx, "t1", t0.Add(2*time.Hour), t0.Add(3*time.Hour))
	if err != nil || len(from) != 1 || from[0].By != "bob@example.com" {
		t.Errorf("window from +2h = %+v, %v", from, err)
	}
	until, err := s.Reveals(ctx, "t1", t0, t0.Add(2*time.Hour))
	if err != nil || len(until) != 1 || until[0].By != "alice@example.com" {
		t.Errorf("window until +2h = %+v, %v", until, err)
	}
}

// A database from before the reveal log holds one reveal per secret, in the
// last-reader stamp. The migration that adds the log carries those over, so
// upgrading does not start the record at zero; a secret never revealed adds
// nothing.
func TestTheRevealLogMigrationKeepsTheLastReveals(t *testing.T) {
	st := openStore(t)
	s := st.DeviceSecrets()
	ctx := context.Background()
	for _, tag := range []string{"lt-1", "lt-2"} {
		if err := s.Put(ctx, "t1", tag, secret.LUKS, []byte("sealed"), "station", t0); err != nil {
			t.Fatal(err)
		}
	}
	// Back to the schema before the log, with lt-1 revealed the old way.
	for _, q := range []string{
		`DROP TABLE device_secret_reveals`,
		`DELETE FROM schema_migrations WHERE name = 'migrations/0021_device_secret_reveals.sql'`,
		`UPDATE device_secrets SET revealed = '2026-11-01T10:00:00Z', revealed_by = 'alice@example.com' WHERE tag = 'lt-1'`,
	} {
		if _, err := st.pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := Migrate(ctx, st.pool); err != nil {
		t.Fatal(err)
	}
	got, err := s.Reveals(ctx, "t1", t0.Add(-24*365*time.Hour), t0.Add(24*365*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tag != "lt-1" || got[0].By != "alice@example.com" ||
		!got[0].At.Equal(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("carried over = %+v, want lt-1's one reveal by alice", got)
	}
}
