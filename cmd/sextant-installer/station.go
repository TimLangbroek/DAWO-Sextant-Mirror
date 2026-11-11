package main

// station.go: the installer on a machine that booted over the network from
// an imaging station (design 0018). Same install as from the USB; the job
// replaces the code, and the steps go to the station, which shows them in
// the console.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// stationClient talks to the station's side of the imaging network
// (cmd/sextant-station).
type stationClient struct {
	url  string
	http *http.Client
	// wait is the pause between asking for the job; a seam for tests.
	wait func(ctx context.Context) error
}

// handout is what the station hands this machine.
type handout struct {
	MAC        string `json:"mac"`
	Tag        string `json:"tag"`
	Rev        string `json:"rev,omitempty"`
	Credential string `json:"credential"`
	Console    string `json:"console"`
}

// stationStatus is what the station relays to the console.
type stationStatus struct {
	Status   string `json:"status,omitempty"`
	Message  string `json:"message,omitempty"`
	Progress *int   `json:"progress,omitempty"`
	Step     string `json:"step,omitempty"`
}

func sleep(d time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
			return nil
		}
	}
}

// errJobTaken: the station handed this machine's job out already.
var errJobTaken = errors.New("the station already handed out this machine's job. If the install was interrupted, send it again from the console's Enrollment page")

// job asks once. waiting is true while no operator approved this machine.
func (c *stationClient) job(ctx context.Context) (h handout, waiting bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/job", nil)
	if err != nil {
		return handout{}, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return handout{}, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNotFound:
		if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
			return handout{}, false, fmt.Errorf("the station's answer: %w", err)
		}
		return h, resp.StatusCode == http.StatusNotFound, nil
	case http.StatusGone:
		return handout{}, false, errJobTaken
	default:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return handout{}, false, fmt.Errorf("the station answered %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
}

func (c *stationClient) report(ctx context.Context, s stationStatus) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/status", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// step tells the console, through the station, what the install is doing.
// From the USB there is nobody to tell; a lost step costs nothing.
func (i *installer) step(ctx context.Context, what string, progress int) {
	if i.station == nil {
		return
	}
	if err := i.station.report(ctx, stationStatus{Step: what, Progress: &progress}); err != nil {
		i.say("(the station did not take the step: %v)\n", err)
	}
}

// runFromStation waits for the operator's approval, then installs without
// asking: the approval in the console is the confirmation.
func (i *installer) runFromStation(ctx context.Context) error {
	i.say("Sextant installation from an imaging station\n")
	h, err := i.awaitJob(ctx)
	if err != nil {
		return err
	}
	j := joined{Tag: h.Tag, URL: h.Console}
	if err := i.stageJoined(j, h.Credential); err != nil {
		return i.failed(ctx, err)
	}
	d, err := i.pickDisk(ctx)
	if err != nil {
		return i.failed(ctx, err)
	}
	i.say("\nThis machine becomes %s.\nEverything on %s is erased.\n", j.Tag, d)
	if err := i.install(ctx, j, d, h.Rev); err != nil {
		return i.failed(ctx, err)
	}
	if err := i.station.report(ctx, stationStatus{Status: "installed"}); err != nil {
		// Installed all the same; the console sees the device check in.
		i.say("(the station did not take the result: %v)\n", err)
	}
	i.say("\nInstalled. The machine restarts and appears in the console as %s.\n", j.Tag)
	i.run(ctx, nil, "systemctl", "reboot")
	return nil
}

// awaitJob asks the station until an operator has approved this machine.
func (i *installer) awaitJob(ctx context.Context) (handout, error) {
	told := false
	for {
		h, waiting, err := i.station.job(ctx)
		switch {
		case errors.Is(err, errJobTaken):
			return handout{}, err
		case err != nil:
			i.say("The station did not answer (%v); trying again.\n", err)
		case !waiting:
			return h, nil
		case !told:
			i.say("\nWaiting for approval in the console.\nThis machine is %s.\n", h.MAC)
			told = true
		}
		if err := i.station.wait(ctx); err != nil {
			return handout{}, err
		}
	}
}

// stageJoined writes what join writes from the USB, so the install is the
// same from here on.
func (i *installer) stageJoined(j joined, credential string) error {
	dir := i.path(joinedDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "credential"), []byte(credential), 0o600); err != nil {
		return err
	}
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "joined.json"), b, 0o644)
}

// failed tells the console why, through the station, and returns err.
func (i *installer) failed(ctx context.Context, err error) error {
	if rerr := i.station.report(ctx, stationStatus{Status: "failed", Message: tail(err.Error())}); rerr != nil {
		i.say("(the station did not take the failure: %v)\n", rerr)
	}
	return err
}
