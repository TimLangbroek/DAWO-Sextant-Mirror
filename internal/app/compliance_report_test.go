package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
)

// reportFleet is baselineFleet plus a policy with a condition and controls,
// so every kind of item appears for lt-1.
func reportFleet(profileStamp string) *fleet.Fleet {
	f := baselineFleet(profileStamp)
	f.Policies["disk"] = fleet.Policy{Name: "Disk space", Controls: []string{"BIO 12.1.3"},
		Settings:   map[string]any{"nix.gc.automatic": true},
		Conditions: []fleet.Condition{{Metric: "disk.free_gb", Op: ">=", Value: 20, Detail: "Keep room for updates."}}}
	f.Assignments = append(f.Assignments, fleet.Assignment{Policy: "disk", Target: "org"})
	return f
}

func reportStatus() StatusView {
	st := goodStatus()
	st.Usage = observed.Usage{DiskTotalGB: 100, DiskUsedGB: 50}
	return st
}

func items(r ComplianceReport) map[string]ComplianceItem {
	m := map[string]ComplianceItem{}
	for _, it := range r.Items {
		m[it.ID] = it
	}
	return m
}

func current(t *testing.T, ps *fleet.Profiles) string {
	t.Helper()
	p, ok := ps.Get("laptop")
	if !ok {
		t.Fatal("profile fixture missing")
	}
	return p.Provenance()
}

// An auditor asks which control failed, on which device, measured when (#13).
// A healthy device answers every requirement with a pass and the value it was
// judged on.
func TestComplianceReportItemByItem(t *testing.T) {
	ps := baselineProfiles(t)
	now := time.Now()
	st := reportStatus()
	r, ok := NewComplianceReport(reportFleet(current(t, ps)), ps, "lt-1", st, true, "rev-good", now)
	if !ok {
		t.Fatal("a known device has no report")
	}
	if r.Schema != "sextant.compliance-report/1" || r.Device != "lt-1" || r.Baseline != ReportCompliant {
		t.Fatalf("header: %+v", r)
	}
	if r.MeasuredAt == nil || !r.MeasuredAt.Equal(st.LastSeen) || r.Revision != "rev-good" {
		t.Errorf("the report does not say when and on what it measured: %v %q", r.MeasuredAt, r.Revision)
	}
	got := items(r)
	for _, id := range []string{"baseline:recency", "baseline:drift", "baseline:posture", "baseline:revision",
		"condition:disk:disk.free_gb>=20", "settings:disk", "settings:laptop"} {
		if got[id].Result != ItemPass {
			t.Errorf("%s: %+v, want a pass", id, got[id])
		}
	}
	if len(got) != 7 {
		t.Errorf("want 7 items, got %d: %+v", len(got), r.Items)
	}
	c := got["condition:disk:disk.free_gb>=20"]
	if c.Measured != "disk.free_gb is 50" || c.Required != "disk.free_gb >= 20" ||
		!slices.Equal(c.Controls, []string{"BIO 12.1.3"}) || c.Name != "Disk space" || c.Detail != "Keep room for updates." {
		t.Errorf("a condition does not carry its control, requirement and measurement: %+v", c)
	}
	if got["settings:disk"].Required != "nix.gc.automatic" || got["settings:disk"].Measured != "revision rev-good" {
		t.Errorf("settings item: %+v", got["settings:disk"])
	}
}

// Every failure has its own item, and the baseline items fail exactly where
// the verdict the device list shows fails: the report and the list must not
// disagree.
func TestComplianceReportNamesEachFailure(t *testing.T) {
	ps := baselineProfiles(t)
	now := time.Now()
	f := reportFleet("laptop@stale")
	dev := f.Devices["lt-1"]
	dev.Settings["nix.gc.automatic"] = false
	f.Devices["lt-1"] = dev
	st := reportStatus()
	st.LastSeen = now.Add(-observed.InactiveWindow - time.Hour)
	st.SB = observed.SBOff
	st.Revision = "rev-old"
	st.Usage.DiskUsedGB = 95

	r, _ := NewComplianceReport(f, ps, "lt-1", st, true, "rev-good", now)
	if r.Baseline != ReportAttention {
		t.Fatalf("baseline %q", r.Baseline)
	}
	got := items(r)
	var baselineFails []string
	for _, it := range r.Items {
		if it.Kind == ItemBaseline && it.Result == ItemFail {
			baselineFails = append(baselineFails, it.ID)
		}
	}
	want := []string{"baseline:recency", "baseline:drift", "baseline:posture", "baseline:revision"}
	if !slices.Equal(baselineFails, want) {
		t.Errorf("baseline failures %v, want %v", baselineFails, want)
	}
	v := NewBaselineJudge(f, ps, now).Verdict("lt-1", st, true)
	if len(v.Failures) != len(baselineFails) {
		t.Errorf("the report fails %v where the verdict fails %v", baselineFails, v.Failures)
	}
	if m := got["baseline:drift"].Measured; m != "behind their profile: laptop" {
		t.Errorf("drift does not name the policy: %q", m)
	}
	if m := got["baseline:posture"].Measured; m != "Secure Boot off, TPM2 enrolled" {
		t.Errorf("posture measured %q", m)
	}
	if c := got["condition:disk:disk.free_gb>=20"]; c.Result != ItemFail || c.Measured != "disk.free_gb is 5" {
		t.Errorf("a failing condition: %+v", c)
	}
	s := got["settings:disk"]
	if s.Result != ItemFail || !strings.Contains(s.Measured, "nix.gc.automatic is false, set by device; the policy sets true") {
		t.Errorf("an overridden setting is not a failure that says by whom: %+v", s)
	}
	if l := got["settings:laptop"]; l.Result != ItemUnknown || l.Measured != "revision rev-old" {
		t.Errorf("a device on an older revision passes or fails its settings: %+v", l)
	}
}

// A device that never reported has nothing measured: what needs a report is
// unknown, never a pass, and the verdict's own failures stay failures.
func TestComplianceReportNeverReported(t *testing.T) {
	ps := baselineProfiles(t)
	r, _ := NewComplianceReport(reportFleet(current(t, ps)), ps, "lt-1", StatusView{}, false, "rev-good", time.Now())
	if r.MeasuredAt != nil || r.Revision != "" {
		t.Errorf("a device that never reported has a measurement time: %+v", r)
	}
	got := items(r)
	for id, want := range map[string]string{
		"baseline:recency": ItemFail, "baseline:posture": ItemFail, "baseline:revision": ItemUnknown,
		"condition:disk:disk.free_gb>=20": ItemUnknown, "settings:disk": ItemUnknown,
	} {
		if got[id].Result != want {
			t.Errorf("%s: %q, want %q", id, got[id].Result, want)
		}
	}
	if m := got["baseline:recency"].Measured; m != "never reported" {
		t.Errorf("recency measured %q", m)
	}
}

// A parked device and one still being installed have no verdict, so no
// items, and an unknown tag has no report at all.
func TestComplianceReportNotJudged(t *testing.T) {
	ps := baselineProfiles(t)
	r, ok := NewComplianceReport(reportFleet(current(t, ps)), ps, "retired", StatusView{}, false, "", time.Now())
	if !ok || r.Baseline != ReportNotJudged || len(r.Items) != 0 {
		t.Fatalf("retired device: %+v", r)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"items":[]`) {
		t.Errorf("items is not an empty list for a program to read: %s", b)
	}
	f := reportFleet(current(t, ps))
	f.Devices["new"] = fleet.Device{Hardware: "hw", State: fleet.DeviceProvisional}
	if r, _ := NewComplianceReport(f, ps, "new", StatusView{}, false, "", time.Now()); r.Baseline != ReportNotJudged || len(r.Items) != 0 {
		t.Errorf("a device not yet installed is judged: %+v", r)
	}
	if _, ok := NewComplianceReport(reportFleet(""), ps, "nope", StatusView{}, false, "", time.Now()); ok {
		t.Error("an unknown device has a report")
	}
}

// A device that follows the latest revision cannot be behind, so it has no
// revision item; posture appears only where a policy asks for it.
func TestComplianceReportOnlyWhatApplies(t *testing.T) {
	ps := baselineProfiles(t)
	st := goodStatus()
	st.Revision = "rev-head"
	r, _ := NewComplianceReport(reportFleet(current(t, ps)), ps, "srv-1", st, true, "rev-head", time.Now())
	got := items(r)
	if _, has := got["baseline:revision"]; has {
		t.Error("a device following the latest revision has a revision item")
	}
	if _, has := got["baseline:posture"]; has {
		t.Error("a device no policy asks posture of has a posture item")
	}
	if got["settings:laptop"].Result != ItemPass {
		t.Errorf("a device on the head revision: %+v", got["settings:laptop"])
	}
}
