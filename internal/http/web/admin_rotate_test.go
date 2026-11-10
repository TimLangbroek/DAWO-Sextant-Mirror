package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
)

// The administrator password a device escrowed (design 0017) can be
// rotated from its row: the button arms the rotate-admin intent, which the
// device carries out and answers with a new password. A recovery key has no
// such button; it rotates with the provisioning ceremony.
func TestTheAdministratorPasswordRotatesFromItsRow(t *testing.T) {
	secrets := app.NewDeviceSecretsService(newMemDevSecrets(), webSealer(t), clockNow{}, "")
	ts := newSecretConsoleWith(t, secrets, seedFleet)
	if err := secrets.Store(context.Background(), "lt-1", secret.LUKS, "z7Xq-9pLm", "device:lt-1"); err != nil {
		t.Fatal(err)
	}
	if _, page := getPage(t, ts, "/devices/lt-1"); !strings.Contains(page, "/secret/luks/reveal") || strings.Contains(page, "data-rotate-admin") {
		t.Error("a recovery key offers to rotate, or is not listed")
	}
	if err := secrets.Store(context.Background(), "lt-1", secret.Admin, "qwert-asdfg", "device:lt-1"); err != nil {
		t.Fatal(err)
	}
	_, page := getPage(t, ts, "/devices/lt-1")
	if !strings.Contains(page, `name="intent" value="rotate-admin"`) || strings.Count(page, "data-rotate-admin") != 1 {
		t.Fatal("the administrator password row has no rotate button")
	}

	resp, err := client().PostForm(ts.URL+"/devices/lt-1/intent",
		url.Values{"csrf": {"dev-csrf"}, "intent": {"rotate-admin"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("arming the rotation = %d", resp.StatusCode)
	}
	if _, page := getPage(t, ts, "/devices/lt-1"); !strings.Contains(page, `<span class="tag bad">rotate-admin</span>`) {
		t.Error("the page does not show the rotation armed")
	}
}
