package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/discovery"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/joincode"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
)

func withDiscovery(svc *web.Services) {
	svc.Discovery = app.NewDiscoveryService(&memDisc{sets: map[string][]discovery.Discovered{}}, clockNow{}, "")
}

// Add a station (design 0018): an owner makes a code on the stations page,
// and the machine that joins with it becomes a station.
func TestAddAStationShowsAStationCode(t *testing.T) {
	ts, joins, codes, cfg := joinConsoleFull(t, true, seedFleet, web.DevSessions{}, true, nil, withDiscovery)

	page := getBody(t, ts.URL+"/station")
	if !strings.Contains(page, `action="/stations/code"`) {
		t.Fatalf("the stations page does not offer Add a station:\n%s", page)
	}

	status, page, _ := postPage(t, ts.URL+"/stations/code", url.Values{})
	if status != http.StatusOK {
		t.Fatalf("make a station code = %d\n%s", status, page)
	}
	m := shownCodeRE.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no code on the page:\n%s", page)
	}
	for _, c := range codes.m {
		if c.Kind != joincode.KindStation || c.Uses != 1 {
			t.Fatalf("stored as kind %q with %d uses", c.Kind, c.Uses)
		}
	}

	got, err := joins.Join(context.Background(), app.JoinRequest{Code: m[1], Serial: "NUC12345", CPU: "GenuineIntel"})
	if err != nil {
		t.Fatalf("the shown code does not join: %v", err)
	}
	if _, ok := cfg.Fleet().Stations[got.Tag]; !ok || cfg.Fleet().Devices[got.Tag].Class != "station" {
		t.Fatalf("%s did not become a station", got.Tag)
	}
}

func TestAnOpenStationCodeIsListedAsOne(t *testing.T) {
	ts, joins, _, _ := joinConsoleFull(t, true, seedFleet, web.DevSessions{}, true, nil, withDiscovery)
	if _, _, err := joins.Create(context.Background(), app.NewCode{Station: true, By: "owner"}); err != nil {
		t.Fatal(err)
	}
	if list := getBody(t, ts.URL+"/devices/add"); !strings.Contains(list, `<a href="/station">imaging station</a>`) {
		t.Fatalf("the open station code is not listed as one:\n%s", list)
	}
}

const stationEditorSeed = `{
  "version": 3,
  "groups": {"alpha": {}},
  "access": [{"group": "org-eds", "role": "editor", "scope": "org"}]
}`

// An organisation editor may add laptops but not stations, and may not
// withdraw an owner's station code.
func TestOnlyAnOwnerAddsAStation(t *testing.T) {
	editor := identity.User{Subject: "e", Name: "Org Editor", Groups: []string{"org-eds"}}
	ts, joins, codes, _ := joinConsoleFull(t, true, stationEditorSeed, scopedSessions{editor}, true, nil, withDiscovery)

	if page := getBody(t, ts.URL+"/station"); strings.Contains(page, `action="/stations/code"`) {
		t.Error("an editor is offered Add a station")
	}
	if status, _, _ := postPage(t, ts.URL+"/stations/code", url.Values{"csrf": {"csrf"}}); status == http.StatusOK {
		t.Error("an editor made a station code")
	}
	if len(codes.m) != 0 {
		t.Fatalf("codes made: %+v", codes.m)
	}

	if _, _, err := joins.Create(context.Background(), app.NewCode{Station: true, By: "owner"}); err != nil {
		t.Fatal(err)
	}
	var id string
	for k := range codes.m {
		id = k
	}
	postPage(t, ts.URL+"/devices/add/"+id+"/withdraw", url.Values{"csrf": {"csrf"}})
	if codes.m[id].Uses != 1 {
		t.Error("an editor withdrew an owner's station code")
	}
}
