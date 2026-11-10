package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
)

// A device's compliance report is machine-readable, carries its schema id,
// judges the device against the revision the console reads (#13), and is
// served only for a device the caller may view: an invisible one answers
// like a missing one.
func TestComplianceReportEndpoint(t *testing.T) {
	svc, dir := seededService(t, visSeed)
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(string(out))

	fo := newFakeObserved()
	inv := app.NewInventoryService(fo, fo, fixedClock{time.Now()}, "")
	for _, tag := range []string{"a-1", "b-1"} {
		if err := inv.CheckIn(context.Background(),
			observed.CheckIn{Tag: tag, Revision: head, Phase: observed.Running}, nil); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	New(Services{Config: svc, Inventory: inv},
		Authz{Sessions: visSessions{identity.User{Subject: "u", Groups: []string{"org-team"}}}}, "", false,
		slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	code, body := get(t, srv, "/api/v1/compliance/b-1")
	if code != 200 {
		t.Fatalf("compliance = %d %s", code, body)
	}
	var rep struct {
		Schema     string     `json:"schema"`
		Device     string     `json:"device"`
		MeasuredAt *time.Time `json:"measuredAt"`
		Baseline   string     `json:"baseline"`
		Items      []struct {
			ID, Kind, Result, Required string
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Schema != app.ComplianceReportSchema || rep.Device != "b-1" || rep.MeasuredAt == nil || rep.Baseline != "compliant" {
		t.Errorf("report header: %s", body)
	}
	results := map[string]string{}
	for _, it := range rep.Items {
		results[it.ID] = it.Result
	}
	if results["baseline:recency"] != "pass" || results["settings:pol-beta"] != "pass" {
		t.Errorf("a device on the console's revision is not judged against it: %v", results)
	}

	if code, _ := get(t, srv, "/api/v1/compliance/ghost"); code != 404 {
		t.Errorf("unknown device = %d, want 404", code)
	}
	scoped := http.NewServeMux()
	New(Services{Config: svc, Inventory: inv},
		Authz{Sessions: visSessions{identity.User{Subject: "u", Groups: []string{"alpha-team"}}}}, "", false,
		slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(scoped)
	srv2 := httptest.NewServer(scoped)
	t.Cleanup(srv2.Close)
	if code, _ := get(t, srv2, "/api/v1/compliance/b-1"); code != 404 {
		t.Errorf("a device outside the caller's scope = %d, want 404", code)
	}
	if code, _ := get(t, srv2, "/api/v1/compliance/a-1"); code != 200 {
		t.Errorf("a device in the caller's scope = %d, want 200", code)
	}
}
