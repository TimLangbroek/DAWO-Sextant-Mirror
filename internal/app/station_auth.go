package app

import (
	"context"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
)

// StationAuth is what the station API asks to tell whether a report comes
// from the station it names. A station set up from the installation USB
// (design 0018) is a fleet device of class station and holds only that
// device's credential; a station set up by hand holds a minted station
// credential. Either proves the station, and only its own tag.
type StationAuth struct {
	stations *StationCredentials
	devices  *DeviceCredentials
	fleet    interface{ Fleet() *fleet.Fleet }
}

// NewStationAuth wires both credential kinds and the fleet document that
// says which devices are stations. devices or fleet may be nil, which leaves
// only minted station credentials.
func NewStationAuth(stations *StationCredentials, devices *DeviceCredentials, f interface{ Fleet() *fleet.Fleet }) *StationAuth {
	return &StationAuth{stations: stations, devices: devices, fleet: f}
}

// AuthenticateTag accepts a minted credential for the station, or the device
// credential of the device that IS this station: same tag, registered under
// stations, class station, not retired. A laptop's credential never speaks
// for a station, even one that shares its tag by accident of history.
func (a *StationAuth) AuthenticateTag(ctx context.Context, secret, station string) bool {
	if a.stations != nil && a.stations.AuthenticateTag(ctx, secret, station) {
		return true
	}
	return a.isStationDevice(station) && a.devices.AuthenticateTag(ctx, secret, station)
}

// HasCredential reports whether the station holds a credential of either
// kind, so the shared bridge token is refused for it.
func (a *StationAuth) HasCredential(ctx context.Context, station string) (bool, error) {
	if a.stations != nil {
		has, err := a.stations.HasCredential(ctx, station)
		if err != nil || has {
			return has, err
		}
	}
	if !a.isStationDevice(station) {
		return false, nil
	}
	return a.devices.HasCredential(ctx, station)
}

func (a *StationAuth) isStationDevice(tag string) bool {
	if a.devices == nil || a.fleet == nil {
		return false
	}
	f := a.fleet.Fleet()
	if f == nil {
		return false
	}
	if _, ok := f.Stations[tag]; !ok {
		return false
	}
	d, ok := f.Devices[tag]
	return ok && d.Class == "station" && d.State != fleet.DeviceRetired
}
