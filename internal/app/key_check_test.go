package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/secretbox"
)

func keyOf(b byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)) }

func sealerWith(t *testing.T, keys string) secretbox.Sealer {
	t.Helper()
	s, err := secretbox.New(keys)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A backup holds the sealed recovery keys and never the key that opens them
// (#7). A console restored under another key, or none, looks complete until
// somebody needs a recovery key. The check at start says so instead, names
// the key that sealed them, and goes quiet once that key is back.
func TestTheKeyCheckFindsSecretsTheKeyCannotOpen(t *testing.T) {
	ctx := context.Background()
	store := newMemSecretStore()
	old := NewDeviceSecretsService(store, sealerWith(t, keyOf(1)), newFakeClock(testT0), "")
	for _, tag := range []string{"lt-1", "lt-2"} {
		if err := old.Store(ctx, tag, secret.LUKS, "recovery-"+tag, "test"); err != nil {
			t.Fatal(err)
		}
	}

	if kc, err := old.CheckKey(ctx); err != nil || kc.Total != 2 || kc.Unopened != 0 {
		t.Fatalf("the key that sealed them: %+v %v", kc, err)
	}

	restored := NewDeviceSecretsService(store, sealerWith(t, keyOf(2)), newFakeClock(testT0), "")
	var log bytes.Buffer
	restored.RunKeyCheck(ctx, slog.New(slog.NewTextHandler(&log, nil)))
	f := restored.KeyFault()
	if f == nil || f.Total != 2 || f.Unopened != 2 {
		t.Fatalf("a console under another key reports %+v", f)
	}
	sealedWith := sealerWith(t, keyOf(1)).PrimaryKeyID()
	if !strings.Contains(f.Reason, sealedWith) || !strings.Contains(log.String(), "unopened=2") {
		t.Errorf("the fault does not name the key that sealed them (%s): %q\n%s", sealedWith, f.Reason, log.String())
	}
	if strings.Contains(f.Reason+log.String(), "recovery-lt-") {
		t.Error("the check revealed a secret")
	}
	if _, meta, _, _ := store.Get(ctx, "lt-1", "lt-1", secret.LUKS); meta.EverRevealed() {
		t.Error("the check stamped a reveal")
	}

	// The new key first, the old one after it: everything opens again.
	rotated := NewDeviceSecretsService(store, sealerWith(t, keyOf(2)+","+keyOf(1)), newFakeClock(testT0), "")
	rotated.RunKeyCheck(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if f := rotated.KeyFault(); f != nil {
		t.Errorf("a console holding the old key still reports %+v", f)
	}

	none := NewDeviceSecretsService(store, secretbox.Sealer{}, newFakeClock(testT0), "")
	none.RunKeyCheck(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if f := none.KeyFault(); f == nil || f.Unopened != 2 || !strings.Contains(f.Reason, "SEXTANT_SECRET_KEY") {
		t.Errorf("a console with no key at all reports %+v", f)
	}

	empty := NewDeviceSecretsService(newMemSecretStore(), secretbox.Sealer{}, newFakeClock(testT0), "")
	empty.RunKeyCheck(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if f := empty.KeyFault(); f != nil {
		t.Errorf("a console with nothing stored reports %+v", f)
	}
}
