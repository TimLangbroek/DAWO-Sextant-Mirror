package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
)

// compliance_report.go: one device's compliance, item by item, in a form a
// program can read (#13). The baseline verdict (design 0008) says whether a
// device needs attention; an auditor asks which control failed, on which
// device, measured when. The report answers that from the same state the
// console already holds, computed when asked, like the verdict itself.

// ComplianceReportSchema names the report's shape. A consumer checks it
// before reading on; removing or renaming a field, or changing what a value
// means, takes a new number. Adding a field does not.
const ComplianceReportSchema = "sextant.compliance-report/1"

// The values of ComplianceReport.Baseline.
const (
	ReportCompliant = "compliant"
	ReportAttention = "attention"
	// ReportNotJudged is a retired or provisional device: parked, or not yet
	// installed, so there is nothing to judge (see BaselineJudge.Verdict).
	ReportNotJudged = "not-judged"
)

// The values of ComplianceItem.Result.
const (
	ItemPass = "pass"
	ItemFail = "fail"
	// ItemUnknown means the console holds no measurement that decides it,
	// which is never the same as a pass.
	ItemUnknown = "unknown"
)

// The values of ComplianceItem.Kind.
const (
	ItemBaseline  = "baseline"  // a design 0008 criterion; these make up the verdict
	ItemCondition = "condition" // a policy's requirement on observed state
	ItemSettings  = "settings"  // a policy's settings, as configured for this device
)

// ComplianceReport is one device's compliance at GeneratedAt.
type ComplianceReport struct {
	Schema      string    `json:"schema"`
	Device      string    `json:"device"`
	GeneratedAt time.Time `json:"generatedAt"`
	// MeasuredAt is the device's last check-in: every observed value in the
	// items is from then. Absent when the device never reported.
	MeasuredAt *time.Time `json:"measuredAt,omitempty"`
	// Revision is the fleet repository commit the device last said it runs.
	Revision string `json:"revision,omitempty"`
	// Baseline is the verdict the device list shows. Only the baseline items
	// make it up; condition and settings items carry their own result.
	Baseline string           `json:"baseline"`
	Items    []ComplianceItem `json:"items"`
}

// ComplianceItem is one judged requirement.
type ComplianceItem struct {
	// ID stays the same for the same requirement from one report to the
	// next, so two reports can be compared item by item.
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Policy string `json:"policy,omitempty"`
	Name   string `json:"name"`
	// Controls are the policy's framework references ("BIO 12.3.1"): the
	// auditor's way from a control to what enforces it.
	Controls []string `json:"controls,omitempty"`
	Result   string   `json:"result"`
	Required string   `json:"required"`
	Measured string   `json:"measured,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// NewComplianceReport judges one device. st and has are what the inventory
// holds for it, head the revision the fleet document was read at. It answers
// false for a tag the fleet does not know.
func NewComplianceReport(f *fleet.Fleet, profiles *fleet.Profiles, tag string,
	st StatusView, has bool, head string, now time.Time) (ComplianceReport, bool) {
	dev, ok := f.Devices[tag]
	if !ok {
		return ComplianceReport{}, false
	}
	r := ComplianceReport{Schema: ComplianceReportSchema, Device: tag,
		GeneratedAt: now.UTC(), Items: []ComplianceItem{}}
	if has {
		at := st.LastSeen.UTC()
		r.MeasuredAt, r.Revision = &at, st.Revision
	}
	if dev.Retired() || dev.Provisional() {
		r.Baseline = ReportNotJudged
		return r, true
	}
	judge := NewBaselineJudge(f, profiles, now)
	verdict := judge.Verdict(tag, st, has)
	r.Baseline = ReportAttention
	if verdict.Compliant {
		r.Baseline = ReportCompliant
	}
	r.Items = append(r.Items, baselineItems(f, judge, verdict, dev, tag, st, has)...)
	r.Items = append(r.Items, conditionItems(f, tag, st, has)...)
	r.Items = append(r.Items, settingsItems(f, tag, st, has, head)...)
	return r, true
}

// baselineItems spells out the verdict criterion by criterion. Whether one
// fails is the verdict's own answer, never a second judgement made here, so
// the report cannot disagree with the device list.
func baselineItems(f *fleet.Fleet, judge *BaselineJudge, v Baseline, dev fleet.Device,
	tag string, st StatusView, has bool) []ComplianceItem {
	result := func(label string) string {
		if slices.Contains(v.Failures, label) {
			return ItemFail
		}
		return ItemPass
	}
	never := "never reported"
	out := make([]ComplianceItem, 0, 4)

	seen := never
	if has {
		seen = "last check-in " + st.LastSeen.UTC().Format(time.RFC3339)
	}
	out = append(out, ComplianceItem{ID: "baseline:recency", Kind: ItemBaseline, Name: "Recent check-in",
		Result:   result(BaselineRecency),
		Required: fmt.Sprintf("a check-in within the last %d days", int(observed.InactiveWindow.Hours()/24)),
		Measured: seen})

	drift := ComplianceItem{ID: "baseline:drift", Kind: ItemBaseline, Name: "Policies match their profiles",
		Result:   result(BaselineDrift),
		Required: "every policy made from a profile is at that profile's current version"}
	if stale := judge.DriftedPolicies(tag); len(stale) > 0 {
		drift.Measured = "behind their profile: " + strings.Join(stale, ", ")
	}
	out = append(out, drift)

	resolved := f.ResolveValues(tag)
	want := func(key string) bool { b, _ := resolved[key].(bool); return b }
	if want(KeySecureBoot) || want(KeyTPM2) {
		var req, got []string
		if want(KeySecureBoot) {
			req = append(req, "Secure Boot enforcing")
			got = append(got, "Secure Boot "+orNotReported(string(st.SB)))
		}
		if want(KeyTPM2) {
			req = append(req, "disk unlock enrolled in the TPM2")
			got = append(got, "TPM2 "+orNotReported(string(st.TPM2)))
		}
		measured := never
		if has {
			measured = strings.Join(got, ", ")
		}
		out = append(out, ComplianceItem{ID: "baseline:posture", Kind: ItemBaseline, Name: "Security posture",
			Result: result(BaselinePosture), Required: strings.Join(req, ", "), Measured: measured})
	}

	// A device that follows the latest revision cannot be behind; the
	// criterion only exists for a ring pinned to one.
	if target := TargetRevision(f, dev); target != "" {
		it := ComplianceItem{ID: "baseline:revision", Kind: ItemBaseline, Name: "Runs its ring's revision",
			Result: result(BaselineRevision), Required: "revision " + target, Measured: never}
		if has {
			it.Measured = "revision " + orNotReported(st.Revision)
		} else {
			it.Result = ItemUnknown
		}
		out = append(out, it)
	}
	return out
}

// conditionItems judges every condition of the policies that reach the
// device against its last report, the same evaluation the incidents use.
func conditionItems(f *fleet.Fleet, tag string, st StatusView, has bool) []ComplianceItem {
	var metrics map[string]float64
	if has {
		metrics = st.Usage.Metrics()
	}
	pcs := f.ConditionsFor(tag)
	out := make([]ComplianceItem, 0, len(pcs))
	for _, pc := range pcs {
		c := pc.Condition
		it := ComplianceItem{
			ID:   fmt.Sprintf("condition:%s:%s%s%s", pc.Policy, c.Metric, c.Op, trimNum(c.Value)),
			Kind: ItemCondition, Policy: pc.Policy, Name: policyName(pc.Policy, pc.Name),
			Controls: f.Policies[pc.Policy].Controls,
			Required: fmt.Sprintf("%s %s %s", c.Metric, c.Op, trimNum(c.Value)),
			Detail:   strings.TrimSpace(c.Detail),
		}
		holds, known := c.Holds(metrics)
		switch {
		case !known:
			it.Result, it.Measured = ItemUnknown, c.Metric+" not reported"
		case holds:
			it.Result = ItemPass
		default:
			it.Result = ItemFail
		}
		if known {
			it.Measured = fmt.Sprintf("%s is %s", c.Metric, trimNum(metrics[c.Metric]))
		}
		out = append(out, it)
	}
	return out
}

// settingsItems reports, per policy that reaches the device, whether its
// settings are what the device is configured with, and whether the device
// runs that configuration. A setting another scope or policy overrides is a
// failure: the policy's control does not hold on this device as written.
// Running an older revision is unknown rather than a failure, because the
// settings may well be in it; this report does not read old revisions.
func settingsItems(f *fleet.Fleet, tag string, st StatusView, has bool, head string) []ComplianceItem {
	resolved := f.Resolve(tag)
	var ids []string
	for _, a := range f.Assignments {
		p, ok := f.Policies[a.Policy]
		if !ok || len(p.Settings) == 0 || slices.Contains(ids, a.Policy) {
			continue
		}
		if slices.Contains(f.AssignmentDevices(a), tag) {
			ids = append(ids, a.Policy)
		}
	}
	sort.Strings(ids)
	out := make([]ComplianceItem, 0, len(ids))
	for _, id := range ids {
		p := f.Policies[id]
		keys := make([]string, 0, len(p.Settings))
		for k := range p.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		it := ComplianceItem{ID: "settings:" + id, Kind: ItemSettings, Policy: id,
			Name: policyName(id, p.Name), Controls: p.Controls,
			Required: strings.Join(keys, ", ")}
		var overridden []string
		for _, k := range keys {
			if got := resolved[k]; !reflect.DeepEqual(got.Value, p.Settings[k]) {
				overridden = append(overridden, fmt.Sprintf("%s is %s, set by %s; the policy sets %s",
					k, jsonText(got.Value), got.Source, jsonText(p.Settings[k])))
			}
		}
		switch {
		case len(overridden) > 0:
			it.Result, it.Measured = ItemFail, strings.Join(overridden, "; ")
		case !has:
			it.Result, it.Measured = ItemUnknown, "never reported"
		case head == "" || st.Revision != head:
			it.Result = ItemUnknown
			it.Measured = "revision " + orNotReported(st.Revision)
			it.Detail = "the device does not run the revision these settings were read from"
		default:
			it.Result, it.Measured = ItemPass, "revision "+st.Revision
		}
		out = append(out, it)
	}
	return out
}

func policyName(id, name string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}

func orNotReported(s string) string {
	if s == "" {
		return "not reported"
	}
	return s
}

// jsonText prints a setting value the way fleet.json writes it.
func jsonText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
