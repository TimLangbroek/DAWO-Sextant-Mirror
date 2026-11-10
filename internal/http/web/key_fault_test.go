package web_test

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/secretbox"
)

// A console whose secret key does not open the recovery keys it holds says
// so on every page, with the key that sealed them (#7). Before, the first
// sign was a reveal that failed when somebody needed one.
func TestEveryPageSaysTheKeyDoesNotOpenTheSecrets(t *testing.T) {
	sealer := func(b byte) secretbox.Sealer {
		key := make([]byte, 32)
		for i := range key {
			key[i] = b
		}
		s, err := secretbox.New(base64.StdEncoding.EncodeToString(key))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	store := newMemDevSecrets()
	ctx := context.Background()
	if err := app.NewDeviceSecretsService(store, sealer(1), clockNow{}, "").
		Store(ctx, "lt-1", secret.LUKS, "z7Xq-9pLm-R2wK", "svc:station"); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	right := app.NewDeviceSecretsService(store, sealer(1), clockNow{}, "")
	right.RunKeyCheck(ctx, quiet)
	if _, page := getPage(t, newSecretConsoleWith(t, right), "/devices"); strings.Contains(page, "data-key-fault") {
		t.Error("a console whose key opens its secrets says it does not")
	}

	wrong := app.NewDeviceSecretsService(store, sealer(2), clockNow{}, "")
	wrong.RunKeyCheck(ctx, quiet)
	_, page := getPage(t, newSecretConsoleWith(t, wrong), "/devices")
	for _, want := range []string{"data-key-fault", "cannot open: 1 / 1.", sealer(1).PrimaryKeyID()} {
		if !strings.Contains(page, want) {
			t.Errorf("a console whose key does not open its secrets does not show %q", want)
		}
	}
	if strings.Contains(page, "z7Xq-9pLm-R2wK") {
		t.Error("the warning shows a secret")
	}
}
