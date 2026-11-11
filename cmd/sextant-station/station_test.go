package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/discovery"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/imaging"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/api"
)

// The console side is the real station API over in-memory stores, so the
// station is tested against the handlers it will meet, not a mock of them.

type memJobs struct {
	mu sync.Mutex
	m  map[string]imaging.Job
}

func (s *memJobs) Upsert(_ context.Context, _ string, j imaging.Job, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[j.MAC] = j
	return nil
}
func (s *memJobs) ListByStation(_ context.Context, _, station string) ([]imaging.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []imaging.Job
	for _, j := range s.m {
		if j.Station == station {
			out = append(out, j)
		}
	}
	return out, nil
}
func (s *memJobs) ListPending(ctx context.Context, tenant, station string) ([]imaging.Job, error) {
	all, _ := s.ListByStation(ctx, tenant, station)
	var out []imaging.Job
	for _, j := range all {
		if j.Status == imaging.Pending || j.Status == imaging.Imaging {
			out = append(out, j)
		}
	}
	return out, nil
}
func (s *memJobs) Get(_ context.Context, _, _, mac string) (imaging.Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.m[mac]
	return j, ok, nil
}
func (s *memJobs) GetActiveByTag(context.Context, string, string) (imaging.Job, bool, error) {
	return imaging.Job{}, false, nil
}
func (s *memJobs) UpdateProgress(_ context.Context, _, _, mac string, p int, step string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.m[mac]
	j.Progress, j.Step = p, step
	s.m[mac] = j
	return nil
}
func (s *memJobs) TransitionStatus(_ context.Context, _, _, mac string, from, to imaging.Status, msg string, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.m[mac]
	if j.Status != from {
		return false, nil
	}
	j.Status, j.Message = to, msg
	s.m[mac] = j
	return true, nil
}
func (s *memJobs) Delete(_ context.Context, _, _, mac string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, mac)
	return nil
}

type memDisc struct {
	mu  sync.Mutex
	got []discovery.Discovered
}

func (d *memDisc) Report(_ context.Context, _, _ string, devs []discovery.Discovered, _ time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.got = devs
	return nil
}
func (d *memDisc) List(context.Context, string, string) ([]discovery.Discovered, error) {
	return nil, nil
}
func (d *memDisc) Remove(context.Context, string, string, string) error { return nil }

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

type creds struct{ n int }

func (c *creds) Issue(_ context.Context, tag string) (string, error) {
	c.n++
	return "sxt_" + tag + "_" + string(rune('0'+c.n)), nil
}

const (
	targetMAC = "aa:bb:cc:00:00:01"
	targetIP  = "10.66.0.21"
	otherIP   = "10.66.0.22"
)

type rig struct {
	st      *station
	jobs    *memJobs
	disc    *memDisc
	imaging *app.ImagingService
	net     http.Handler
}

func newRig(t *testing.T) rig {
	t.Helper()
	dir := t.TempDir()
	jobs := &memJobs{m: map[string]imaging.Job{}}
	disc := &memDisc{}
	img := app.NewImagingService(jobs, clock{}, "")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	api.NewStation(app.NewDiscoveryService(disc, clock{}, ""), img, &creds{}, nil, "bridge-token", log).Routes(mux)
	consoleSrv := httptest.NewServer(mux)
	t.Cleanup(consoleSrv.Close)

	cred := filepath.Join(dir, "credential")
	write(t, cred, "bridge-token")
	leases := filepath.Join(dir, "dnsmasq.leases")
	write(t, leases, "1900000000 "+targetMAC+" "+targetIP+" target *\n1900000000 AA:BB:CC:00:00:02 "+otherIP+" other *\n")
	st := &station{
		console: &console{url: consoleSrv.URL, tag: "station-1", credential: cred, http: consoleSrv.Client()},
		leases:  leases, state: filepath.Join(dir, "state"), log: log,
	}
	if err := st.load(); err != nil {
		t.Fatal(err)
	}
	return rig{st: st, jobs: jobs, disc: disc, imaging: img, net: st.routes("")}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// from sends a request as the machine at ip on the imaging network.
func (r rig) from(ip, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = ip + ":40000"
	rec := httptest.NewRecorder()
	r.net.ServeHTTP(rec, req)
	return rec
}

func (r rig) approve(t *testing.T) {
	t.Helper()
	if err := r.imaging.Dispatch(context.Background(), imaging.Job{Station: "station-1", MAC: targetMAC, Tag: "laptop-1", Hardware: "hw", Rev: "abc123"}); err != nil {
		t.Fatal(err)
	}
}

func TestATargetGetsItsJobOnceAndOnlyItself(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()

	r.st.tick(ctx)
	if len(r.disc.got) != 2 || r.disc.got[0].MAC != targetMAC {
		t.Fatalf("reported %+v, want both leases", r.disc.got)
	}
	// Before approval the target learns which machine it is, nothing more.
	if rec := r.from(targetIP, "GET", "/job", ""); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), targetMAC) {
		t.Fatalf("before approval: %d %s", rec.Code, rec.Body)
	}

	r.approve(t)
	r.st.tick(ctx)
	if j, _, _ := r.imaging.Get(ctx, "station-1", targetMAC); j.Status != imaging.Imaging {
		t.Fatalf("claimed job is %s, want imaging", j.Status)
	}
	if rec := r.from("10.66.0.99", "GET", "/job", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("an address without a lease got %d", rec.Code)
	}
	if rec := r.from(otherIP, "GET", "/job", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("another machine got %d: %s", rec.Code, rec.Body)
	}
	rec := r.from(targetIP, "GET", "/job", "")
	var h handout
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &h) != nil {
		t.Fatalf("the target got %d %s", rec.Code, rec.Body)
	}
	if h.Tag != "laptop-1" || h.Rev != "abc123" || h.Credential != "sxt_laptop-1_1" || h.Console != r.st.console.url || h.MAC != targetMAC {
		t.Fatalf("handout %+v", h)
	}
	if rec := r.from(targetIP, "GET", "/job", ""); rec.Code != http.StatusGone {
		t.Fatalf("a second ask got %d", rec.Code)
	}
	// A later claim leaves the handed-out credential alone.
	r.st.tick(ctx)
	if r.st.jobs[targetMAC].Credential != "sxt_laptop-1_1" {
		t.Fatalf("credential replaced: %+v", r.st.jobs[targetMAC])
	}
}

func TestTheStationRelaysTheTargetsStepsAndForgetsAFinishedJob(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.approve(t)
	r.st.tick(ctx)

	if rec := r.from(targetIP, "POST", "/status", `{"step":"disk","progress":10}`); rec.Code != http.StatusConflict {
		t.Fatalf("a report before the handout got %d", rec.Code)
	}
	r.from(targetIP, "GET", "/job", "")
	if rec := r.from(otherIP, "POST", "/status", `{"status":"failed"}`); rec.Code != http.StatusConflict {
		t.Fatalf("another machine reported on the job: %d", rec.Code)
	}
	if rec := r.from(targetIP, "POST", "/status", `{"status":"imaging"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a target set imaging: %d", rec.Code)
	}
	if rec := r.from(targetIP, "POST", "/status", `{"step":"Installing the system","progress":40}`); rec.Code != http.StatusNoContent {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body)
	}
	if j, _, _ := r.imaging.Get(ctx, "station-1", targetMAC); j.Step != "Installing the system" || j.Progress != 40 {
		t.Fatalf("the console shows %q %d", j.Step, j.Progress)
	}
	if rec := r.from(targetIP, "POST", "/status", `{"status":"installed"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("installed: %d %s", rec.Code, rec.Body)
	}
	if j, _, _ := r.imaging.Get(ctx, "station-1", targetMAC); j.Status != imaging.Installed {
		t.Fatalf("job is %s, want installed", j.Status)
	}
	if _, held := r.st.jobs[targetMAC]; held {
		t.Fatal("the station still holds a finished job")
	}
	b, _ := os.ReadFile(r.st.jobsFile())
	if strings.Contains(string(b), "sxt_") {
		t.Fatalf("a finished job's credential stays on disk: %s", b)
	}
}

func TestClaimedJobsSurviveARestartAndStayPrivate(t *testing.T) {
	r := newRig(t)
	r.approve(t)
	r.st.tick(context.Background())

	fi, err := os.Stat(r.st.jobsFile())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("jobs.json is %v, want 0600: it holds credentials", fi.Mode().Perm())
	}
	again := &station{console: r.st.console, leases: r.st.leases, state: r.st.state, log: r.st.log}
	if err := again.load(); err != nil {
		t.Fatal(err)
	}
	if again.jobs[targetMAC].Credential != "sxt_laptop-1_1" {
		t.Fatalf("after a restart the station holds %+v", again.jobs)
	}
}

func TestLeases(t *testing.T) {
	dir := t.TempDir()
	if l, err := readLeases(filepath.Join(dir, "none")); err != nil || l != nil {
		t.Fatalf("a missing lease file: %v %v", l, err)
	}
	p := filepath.Join(dir, "l")
	write(t, p, "garbage\n1 AA:BB:CC:00:00:01 10.0.0.1 x *\n2 aa:bb:cc:00:00:02 10.0.0.1 y *\n")
	l, err := readLeases(p)
	if err != nil || len(l) != 2 || l[0].MAC != "aa:bb:cc:00:00:01" {
		t.Fatalf("leases %+v %v", l, err)
	}
	// An address handed out again belongs to whoever got it last.
	if mac, ok := macFor(l, "10.0.0.1"); !ok || mac != "aa:bb:cc:00:00:02" {
		t.Fatalf("10.0.0.1 is %q %v", mac, ok)
	}
}
