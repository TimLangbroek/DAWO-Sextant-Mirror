package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
)

// integration_test_button.go: the Test button on an integration's card
// (design 0019). Two answers: whether the servers the settings name answer
// from the console, and what the devices that have the integration on say
// about it. It changes nothing and keeps nothing.

// integrationTest is the answer shown on the card.
type integrationTest struct {
	Key string
	// Console is one line per address tried.
	Console []app.ReachResult
	// Unset lists the address settings that have no value at this scope.
	Unset []string
	// HasAddresses is false for an integration with no server to try.
	HasAddresses bool
	// Devices counts the devices in scope that have it on.
	Up, Down, NoReading int
	// DownTag and DownDetail are one device's own words on a "down": the
	// most recently seen one.
	DownTag, DownDetail string
}

// Devices is the number of devices that have the integration on.
func (t *integrationTest) Devices() int { return t.Up + t.Down + t.NoReading }

func (s *Server) postIntegrationTest(w http.ResponseWriter, r *http.Request, v view) error {
	key := r.PathValue("key")
	i := slices.IndexFunc(knownIntegrations, func(ig integration) bool { return ig.Key == key })
	if i < 0 {
		return fmt.Errorf("unknown integration %q", key)
	}
	ig := knownIntegrations[i]
	scope := r.FormValue("scope")
	if scope == "" {
		scope = "org"
	}
	// The same people who may change it: a test opens connections to the
	// addresses in the configuration.
	if err := s.requireWeb(v, scope, identity.Editor); err != nil {
		return err
	}
	f := s.svc.Config.Fleet()
	values, err := scopeValues(f, scope)
	if err != nil {
		return err
	}

	t := &integrationTest{Key: key}
	for _, a := range app.IntegrationAddresses[key] {
		t.HasAddresses = true
		val, _ := values[a.Option].(string)
		if strings.TrimSpace(val) == "" {
			t.Unset = append(t.Unset, a.Option)
			continue
		}
		t.Console = append(t.Console, s.svc.Reacher.Reach(r.Context(), a.Kind, val)...)
	}
	s.countReadings(r, f, scope, ig, t)

	tried := make([]string, 0, len(t.Console))
	for _, c := range t.Console {
		tried = append(tried, fmt.Sprintf("%s ok=%v", c.Target, c.OK))
	}
	s.log.Info("integration tested", "integration", key, "scope", scope, "by", v.User.Subject,
		"tried", strings.Join(tried, ", "), "up", t.Up, "down", t.Down, "noReading", t.NoReading)
	s.renderIntegrations(w, r, v, scope, t)
	return nil
}

// scopeValues is what a scope sees: its own settings over the
// organisation's, the values a device in that scope starts from.
func scopeValues(f *fleet.Fleet, scope string) (map[string]any, error) {
	own, _, err := f.ScopeSettings(scope)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if scope != "org" && f.Org != nil {
		for k, val := range f.Org.Settings {
			out[k] = val
		}
	}
	for k, val := range own {
		out[k] = val
	}
	return out, nil
}

// countReadings counts the devices in scope that have the integration on,
// by what they last reported of it. Unmeasured is its own count, never up
// and never down (#49).
func (s *Server) countReadings(r *http.Request, f *fleet.Fleet, scope string, ig integration, t *integrationTest) {
	group, inGroup := strings.CutPrefix(scope, "group:")
	statuses := map[string]app.StatusView{}
	if s.svc.Inventory != nil {
		all, err := s.svc.Inventory.StatusAll(r.Context())
		if err != nil {
			s.log.Warn("integration test: device readings unavailable", "err", err)
		}
		for _, st := range all {
			statuses[st.Tag] = st
		}
	}
	var downSeen app.StatusView
	for _, tag := range f.DeviceTags() {
		d := f.Devices[tag]
		if d.State == fleet.DeviceRetired || (inGroup && !slices.Contains(d.Groups, group)) {
			continue
		}
		if !integrationEnabled(f.ResolveValues(tag), ig.Prefix) {
			continue
		}
		st := statuses[tag]
		obs := st.Integrations[ig.Key]
		switch obs.State {
		case "up":
			t.Up++
		case "down":
			t.Down++
			if t.DownTag == "" || st.LastSeen.After(downSeen.LastSeen) {
				t.DownTag, t.DownDetail, downSeen = tag, obs.Detail, st
			}
		default:
			t.NoReading++
		}
	}
}
