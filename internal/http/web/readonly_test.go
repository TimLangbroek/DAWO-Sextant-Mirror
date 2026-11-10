package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// readonly_test.go: a console started without --write. The NixOS module's
// default, so it is what a new install there looks like after setup.

func consoleWithWrite(t *testing.T, write bool) *httptest.Server {
	t.Helper()
	cfg := seedConsoleConfig(t, ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	srv, err := web.New(web.Services{Config: cfg}, web.DevSessions{}, write,
		nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// A read-only console says so before anyone fills in a form, and a form
// sent anyway is answered inside the console, in words, with where changes
// are switched on - not with "server is read-only" in plain text.
func TestAReadOnlyConsoleSaysSoInWords(t *testing.T) {
	const banner = "this console shows your devices and changes nothing"

	ts := consoleWithWrite(t, false)
	resp, err := client().Get(ts.URL + "/settings")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), banner) {
		t.Error("a read-only console does not say so on its pages")
	}

	resp, err = client().PostForm(ts.URL+"/settings", url.Values{"csrf": {"dev-csrf"}, "scope": {"org"}, "v:desktop": {"gnome"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(body)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a change on a read-only console answered %d", resp.StatusCode)
	}
	for _, want := range []string{"nothing was changed", "services.sextant.write = true", `id="content"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the refusal lacks %q:\n%.400s", want, page)
		}
	}

	ts = consoleWithWrite(t, true)
	resp, err = client().Get(ts.URL + "/settings")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), banner) {
		t.Error("a console that writes says it is read-only")
	}
}
