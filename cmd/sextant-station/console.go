package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/discovery"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
)

// console speaks the station API (internal/http/api/station.go) as one
// station. The credential is read from its file on every call, so a rotated
// device credential is picked up without a restart.
type console struct {
	url        string
	tag        string
	credential string // path
	http       *http.Client
}

// claimed is a job as the claim returns it.
type claimed struct {
	MAC        string `json:"mac"`
	Tag        string `json:"tag"`
	Hardware   string `json:"hardware"`
	Rev        string `json:"rev,omitempty"`
	Credential string `json:"credential,omitempty"`
}

// statusReport is what the station relays for a target. It has no hostKey
// and the console seals or refuses a recovery key in the message, so a
// target cannot write anything into the fleet through it.
type statusReport struct {
	Status   string `json:"status,omitempty"`
	Message  string `json:"message,omitempty"`
	Progress *int   `json:"progress,omitempty"`
	Step     string `json:"step,omitempty"`
}

func (c *console) do(ctx context.Context, method, path string, body, into any) error {
	secret, err := os.ReadFile(c.credential)
	if err != nil {
		return fmt.Errorf("read the station's credential: %w", err)
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.url+"/api/station/"+c.tag+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(secret)))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if into == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// report tells the console which machines are on the imaging network.
func (c *console) report(ctx context.Context, leases []lease) error {
	r := discovery.Report{Devices: make([]discovery.Discovered, 0, len(leases))}
	seen := map[string]bool{}
	for _, l := range leases {
		if seen[l.MAC] {
			continue
		}
		seen[l.MAC] = true
		r.Devices = append(r.Devices, discovery.Discovered{MAC: l.MAC, Phase: observed.Discovered})
	}
	return c.do(ctx, http.MethodPost, "/report", r, nil)
}

// claim takes the jobs an operator approved since the last claim, each with
// its device's credential. Only new ones: a credential already handed to a
// target must stay valid.
func (c *console) claim(ctx context.Context) ([]claimed, error) {
	var out []claimed
	err := c.do(ctx, http.MethodPost, "/jobs/claim?only=pending", nil, &out)
	return out, err
}

// status relays a target's report on its job.
func (c *console) status(ctx context.Context, mac string, s statusReport) error {
	return c.do(ctx, http.MethodPost, "/jobs/"+mac+"/status", s, nil)
}
