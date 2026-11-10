package web_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
)

// failingStatus is an observed store that cannot answer.
type failingStatus struct{ *manyStatus }

func (failingStatus) Get(context.Context, string, string) (observed.DeviceStatus, bool, error) {
	return observed.DeviceStatus{}, false, errors.New("database unreachable")
}

// The device page offers the device's compliance report as a file, the same
// document the API answers (#13): schema id, the device, when it was
// measured, and an attachment name an auditor can file.
func TestDevicePageHandsOutTheComplianceReport(t *testing.T) {
	ts := consoleWithGhostCheckIns(t)
	_, page := getPage(t, ts, "/devices/lt-1")
	if !strings.Contains(page, `href="/devices/lt-1/compliance.json" data-compliance-report`) {
		t.Error("the device page has no link to the compliance report")
	}

	resp, err := client().Get(ts.URL + "/devices/lt-1/compliance.json")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("report = %d\n%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	want := `attachment; filename="compliance-lt-1-` + time.Now().UTC().Format("2006-01-02") + `.json"`
	if cd := resp.Header.Get("Content-Disposition"); cd != want {
		t.Errorf("content-disposition = %q, want %q", cd, want)
	}
	var rep app.ComplianceReport
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Schema != app.ComplianceReportSchema || rep.Device != "lt-1" || rep.MeasuredAt == nil || rep.Revision != "abc123" {
		t.Errorf("report: %s", body)
	}

	// test9 checked in once but is no longer in the fleet.
	for _, path := range []string{"/devices/test9/compliance.json", "/devices/nope/compliance.json"} {
		r, err := client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, r.StatusCode)
		}
	}
}

// Without the device's status the report would say it never reported, a
// claim about the device that is really about the database. It refuses.
func TestTheComplianceReportRefusesWithoutStatus(t *testing.T) {
	ts := consoleWithStatus(t, failingStatus{&manyStatus{}})
	resp, err := client().Get(ts.URL + "/devices/lt-1/compliance.json")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || strings.Contains(string(body), "never reported") {
		t.Errorf("report without status = %d\n%s", resp.StatusCode, body)
	}
}

// The report is as confidential as the device page: a device outside the
// viewer's scope answers like a missing one.
func TestTheComplianceReportStaysInScope(t *testing.T) {
	ts := newScopedConsole(t, identity.User{Subject: "u", Groups: []string{"alpha-team"}})
	for path, want := range map[string]int{
		"/devices/a-1/compliance.json": http.StatusOK,
		"/devices/b-1/compliance.json": http.StatusNotFound,
	} {
		resp, err := client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s = %d, want %d", path, resp.StatusCode, want)
		}
	}
}
