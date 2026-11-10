package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
)

// A device hands over the administrator password it drew for itself the
// way it hands over its recovery key (design 0017): sealed as the admin
// kind, confirmed with its own header, and never confirmed when it was not
// stored, because the device deletes its only copy on that header.
func TestCheckinEscrowsTheAdministratorPassword(t *testing.T) {
	beat := func(t *testing.T, svc *app.DeviceSecretsService, body string) *http.Response {
		t.Helper()
		fo := newFakeObserved()
		inv := app.NewInventoryService(fo, fo, fixedClock{time.Now()}, "")
		c := NewCheckin(inv, nil, "tok")
		if svc != nil {
			c = c.WithDeviceSecrets(svc)
		}
		mux := http.NewServeMux()
		c.Routes(mux)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		req, _ := http.NewRequest("POST", srv.URL+"/api/checkin", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	const pass = "qwert-asdfg-zxcvb-yuiop"
	body := `{"tag":"lt-1","revision":"r1","phase":"running","ack":"admin-rotated","adminPassword":"` + pass + `"}`

	store := newEscrowStore()
	svc := escrowService(t, store)
	resp := beat(t, svc, body)
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("X-Admin-Password-Stored") != "1" {
		t.Fatalf("status %d, header %q", resp.StatusCode, resp.Header.Get("X-Admin-Password-Stored"))
	}
	if resp.Header.Get("X-Recovery-Key-Stored") != "" {
		t.Error("a beat without a recovery key confirmed one")
	}
	if raw := string(store.ciph[ekey("lt-1", secret.Admin)]); raw == "" || strings.Contains(raw, pass) {
		t.Fatal("the password is not stored, or stored in the clear")
	}
	if got, ok, err := svc.Reveal(context.Background(), "lt-1", secret.Admin, "test"); err != nil || !ok || got != pass {
		t.Fatalf("reveal = %q %v %v", got, ok, err)
	}

	if resp := beat(t, nil, body); resp.Header.Get("X-Admin-Password-Stored") != "" {
		t.Error("a console without a store confirmed the password")
	}
	failing := newEscrowStore()
	failing.putErr = errors.New("disk full")
	if resp := beat(t, escrowService(t, failing), body); resp.Header.Get("X-Admin-Password-Stored") != "" || resp.StatusCode != http.StatusNoContent {
		t.Errorf("a failed store: status %d, header %q", resp.StatusCode, resp.Header.Get("X-Admin-Password-Stored"))
	}
	big := `{"tag":"lt-1","revision":"r1","phase":"running","adminPassword":"` + strings.Repeat("x", 300) + `"}`
	if resp := beat(t, escrowService(t, newEscrowStore()), big); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an oversized password = %d, want 400", resp.StatusCode)
	}
}
