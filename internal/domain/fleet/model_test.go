package fleet

import "testing"

// Auto-flow (ADR 0012) is on unless the org says otherwise, so an unset
// field must read as enabled - only an explicit false is manual dispatch.
func TestRolloutAutoFlowEnabled(t *testing.T) {
	on, off := true, false
	cases := []struct {
		name string
		pol  *RolloutPolicy
		want bool
	}{
		{"no plan at all", nil, false},
		{"unset means on", &RolloutPolicy{Rings: []RolloutRing{{Group: "a"}}}, true},
		{"explicit true", &RolloutPolicy{Rings: []RolloutRing{{Group: "a"}}, AutoFlow: &on}, true},
		{"explicit false", &RolloutPolicy{Rings: []RolloutRing{{Group: "a"}}, AutoFlow: &off}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pol.AutoFlowEnabled(); got != tc.want {
				t.Fatalf("AutoFlowEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRolloutHasTestGate(t *testing.T) {
	var nilPol *RolloutPolicy
	if nilPol.HasTestGate() {
		t.Fatal("nil policy should have no test gate")
	}
	noGate := &RolloutPolicy{Rings: []RolloutRing{{Group: "a"}, {Group: "b"}}}
	if noGate.HasTestGate() {
		t.Fatal("plan without an approval ring should have no test gate")
	}
	gated := &RolloutPolicy{Rings: []RolloutRing{{Group: "test", RequireApproval: true}, {Group: "b"}}}
	if !gated.HasTestGate() {
		t.Fatal("plan with an approval ring should have a test gate")
	}
}

// The console block is how a device finds its console (design 0014). The
// console rewrites fleet.json on every saved change, so a field it does not
// know would vanish on the first edit and leave devices without an address.
func TestConsoleBlockSurvivesARewrite(t *testing.T) {
	in := []byte(`{"version":3,"console":{"url":"https://console.example.org","branch":"stable"}}`)
	f, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	if f.Console == nil || f.Console.URL != "https://console.example.org" || f.Console.Branch != "stable" {
		t.Fatalf("console block decoded as %+v", f.Console)
	}
	out, err := f.Encode()
	if err != nil {
		t.Fatal(err)
	}
	again, err := Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	if again.Console == nil || *again.Console != *f.Console {
		t.Fatalf("console block lost in a rewrite:\n%s", out)
	}
}
