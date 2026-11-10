package web_test

import (
	"strings"
	"testing"
)

// A device still being installed has no baseline to fail, so its page shows
// no verdict. It showed "attention" over an empty list of failures: the
// verdict for a provisional device is empty, and the page rendered it anyway.
func TestAProvisionalDeviceShowsNoBaselineVerdict(t *testing.T) {
	ts := newConsoleWithFleet(t, `{
  "version": 3,
  "groups": {"pilot": {}},
  "devices": {
    "lt-new": {"groups": ["pilot"], "hardware": "hw", "state": "provisional"},
    "lt-1": {"groups": ["pilot"], "hardware": "hw"}
  }
}`)
	_, page := getPage(t, ts, "/devices/lt-new")
	if strings.Contains(page, "data-compliance-report") || strings.Contains(page, "Not meeting the baseline") {
		t.Error("a device that is not installed yet shows a baseline verdict")
	}
	// The installed one, which has never checked in, still does.
	if _, page := getPage(t, ts, "/devices/lt-1"); !strings.Contains(page, "Not meeting the baseline") {
		t.Error("the control case lost its verdict")
	}
}
