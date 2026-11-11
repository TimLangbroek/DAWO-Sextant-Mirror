package app

import (
	"context"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
)

type fixedFleet struct{ f *fleet.Fleet }

func (s fixedFleet) Fleet() *fleet.Fleet { return s.f }

func stationFleet() *fleet.Fleet {
	return &fleet.Fleet{
		Stations: map[string]fleet.Station{"station-a": {}, "laptop-x": {}, "station-old": {}},
		Devices: map[string]fleet.Device{
			"station-a":   {Class: "station"},
			"station-b":   {Class: "station"}, // a device, never registered as a station
			"laptop-x":    {Class: "laptop"},  // registered under stations by mistake
			"station-old": {Class: "station", State: fleet.DeviceRetired},
		},
	}
}

func TestAStationDeviceSpeaksForItsOwnStationOnly(t *testing.T) {
	ctx := context.Background()
	store := newMemTokenStore()
	clock := newFakeClock(testT0)
	devices := NewDeviceCredentials(store, clock)
	auth := NewStationAuth(NewStationCredentials(store, clock), devices, fixedFleet{stationFleet()})

	secret := map[string]string{}
	for _, tag := range []string{"station-a", "station-b", "laptop-x", "station-old"} {
		s, err := devices.Issue(ctx, tag)
		if err != nil {
			t.Fatal(err)
		}
		secret[tag] = s
	}

	if !auth.AuthenticateTag(ctx, secret["station-a"], "station-a") {
		t.Fatal("a station device was refused for its own station")
	}
	for _, c := range []struct{ secret, station, why string }{
		{secret["station-a"], "station-b", "another station"},
		{secret["station-b"], "station-b", "a station device that is not registered as a station"},
		{secret["laptop-x"], "laptop-x", "a laptop registered under stations"},
		{secret["station-old"], "station-old", "a retired station"},
		{"not-a-token", "station-a", "garbage"},
	} {
		if auth.AuthenticateTag(ctx, c.secret, c.station) {
			t.Errorf("accepted %s", c.why)
		}
	}

	for tag, want := range map[string]bool{"station-a": true, "laptop-x": false, "station-c": false} {
		has, err := auth.HasCredential(ctx, tag)
		if err != nil {
			t.Fatal(err)
		}
		if has != want {
			t.Errorf("HasCredential(%s) = %v, want %v", tag, has, want)
		}
	}
}

func TestAMintedStationCredentialStillWorks(t *testing.T) {
	ctx := context.Background()
	store := newMemTokenStore()
	clock := newFakeClock(testT0)
	stations := NewStationCredentials(store, clock)
	secret, err := stations.Issue(ctx, "nuc-1")
	if err != nil {
		t.Fatal(err)
	}
	for name, auth := range map[string]*StationAuth{
		"with devices":    NewStationAuth(stations, NewDeviceCredentials(store, clock), fixedFleet{stationFleet()}),
		"without devices": NewStationAuth(stations, nil, nil),
	} {
		if !auth.AuthenticateTag(ctx, secret, "nuc-1") {
			t.Errorf("%s: the minted credential was refused", name)
		}
		if has, err := auth.HasCredential(ctx, "nuc-1"); err != nil || !has {
			t.Errorf("%s: HasCredential = %v, %v", name, has, err)
		}
		if auth.AuthenticateTag(ctx, secret, "nuc-2") {
			t.Errorf("%s: the minted credential spoke for another station", name)
		}
	}
}
