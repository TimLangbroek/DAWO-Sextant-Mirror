package main

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/imaging"
)

// lease is one line of dnsmasq's lease file: what the station's own DHCP
// server handed out on the imaging network.
type lease struct {
	MAC string
	IP  string
}

// readLeases reads dnsmasq's lease file ("<expiry> <mac> <ip> <name>
// <client-id>" per line). A missing file is an empty network: dnsmasq writes
// it on the first lease.
func readLeases(path string) ([]lease, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []lease
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || strings.Count(fields[1], ":") != 5 {
			continue
		}
		out = append(out, lease{MAC: imaging.NormalizeMAC(fields[1]), IP: fields[2]})
	}
	return out, sc.Err()
}

// macFor finds who holds ip. The station trusts its own DHCP server for this
// and nothing the target says about itself.
func macFor(leases []lease, ip string) (string, bool) {
	for i := len(leases) - 1; i >= 0; i-- {
		if leases[i].IP == ip {
			return leases[i].MAC, true
		}
	}
	return "", false
}
