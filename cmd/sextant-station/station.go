package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/imaging"
)

// job is a claimed job the station holds until its target has finished.
type job struct {
	claimed
	// HandedOut is set once the target fetched it. A job is handed out once:
	// a second machine answering to the same address gets nothing, and a
	// target that has to start over reports failed, after which the operator
	// sends it again from the console.
	HandedOut bool `json:"handedOut,omitempty"`
}

// handout is what a target receives: enough to install itself as the
// device the operator approved, from the revision the console chose.
type handout struct {
	MAC        string `json:"mac"`
	Tag        string `json:"tag"`
	Hardware   string `json:"hardware"`
	Rev        string `json:"rev,omitempty"`
	Credential string `json:"credential"`
	Console    string `json:"console"`
}

// station holds the claimed jobs and serves them to the imaging network.
type station struct {
	console *console
	leases  string // dnsmasq's lease file
	state   string // jobs.json lives here, root-only
	log     *slog.Logger

	mu   sync.Mutex
	jobs map[string]job // by MAC
}

func (s *station) jobsFile() string { return filepath.Join(s.state, "jobs.json") }

// load reads the jobs a previous run held. Credentials survive a restart
// this way: the console hands each out once.
func (s *station) load() error {
	s.jobs = map[string]job{}
	b, err := os.ReadFile(s.jobsFile())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &s.jobs)
}

// save writes the jobs atomically and readable by root only: they hold
// device credentials. Called with mu held.
func (s *station) save() error {
	b, err := json.Marshal(s.jobs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.state, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.state, ".jobs-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.jobsFile())
}

// tick reports the imaging network and claims new jobs. A failed report
// does not stop the claim: the operator may have approved a machine that is
// already known.
func (s *station) tick(ctx context.Context) {
	leases, err := readLeases(s.leases)
	if err != nil {
		s.log.Warn("cannot read the DHCP leases", "file", s.leases, "err", err)
	} else if err := s.console.report(ctx, leases); err != nil {
		s.log.Warn("report to the console failed", "err", err)
	}
	got, err := s.console.claim(ctx)
	if err != nil {
		s.log.Warn("claim from the console failed", "err", err)
		return
	}
	if len(got) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range got {
		c.MAC = imaging.NormalizeMAC(c.MAC)
		s.jobs[c.MAC] = job{claimed: c}
		s.log.Info("job claimed", "mac", c.MAC, "tag", c.Tag)
	}
	if err := s.save(); err != nil {
		s.log.Error("cannot keep the claimed jobs; a restart loses them", "err", err)
	}
}

// targetMAC names the machine asking, by its DHCP lease.
func (s *station) targetMAC(r *http.Request) (string, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", false
	}
	leases, err := readLeases(s.leases)
	if err != nil {
		s.log.Warn("cannot read the DHCP leases", "err", err)
		return "", false
	}
	return macFor(leases, host)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// getJob answers a target asking for its job. 404 with its MAC while the
// operator has not approved it yet, so the target can show which machine it
// is; 410 once the job was handed out.
func (s *station) getJob(w http.ResponseWriter, r *http.Request) {
	mac, ok := s.targetMAC(r)
	if !ok {
		http.Error(w, "this address has no lease from this station", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[mac]
	switch {
	case !ok:
		writeJSON(w, http.StatusNotFound, map[string]string{"mac": mac})
		return
	case j.HandedOut:
		writeJSON(w, http.StatusGone, map[string]string{"mac": mac})
		return
	}
	j.HandedOut = true
	s.jobs[mac] = j
	if err := s.save(); err != nil {
		s.log.Error("cannot record the handout", "mac", mac, "err", err)
	}
	s.log.Info("job handed out", "mac", mac, "tag", j.Tag)
	writeJSON(w, http.StatusOK, handout{MAC: mac, Tag: j.Tag, Hardware: j.Hardware, Rev: j.Rev,
		Credential: j.Credential, Console: s.console.url})
}

// postStatus relays a target's progress to the console. The MAC comes from
// the lease, never from the body, and only a target holding its job may
// report on it. A final status lets the station forget the job.
func (s *station) postStatus(w http.ResponseWriter, r *http.Request) {
	mac, ok := s.targetMAC(r)
	if !ok {
		http.Error(w, "this address has no lease from this station", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	j, held := s.jobs[mac]
	s.mu.Unlock()
	if !held || !j.HandedOut {
		http.Error(w, "no job handed out to this machine", http.StatusConflict)
		return
	}
	var in statusReport
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		http.Error(w, "bad status: "+err.Error(), http.StatusBadRequest)
		return
	}
	if in.Status != "" && in.Status != string(imaging.Installed) && in.Status != string(imaging.Failed) {
		http.Error(w, "a target reports installed or failed, or progress", http.StatusBadRequest)
		return
	}
	if err := s.console.status(r.Context(), mac, in); err != nil {
		s.log.Warn("relay to the console failed", "mac", mac, "err", err)
		http.Error(w, "the console did not take it: "+err.Error(), http.StatusBadGateway)
		return
	}
	if in.Status != "" {
		s.mu.Lock()
		delete(s.jobs, mac)
		if err := s.save(); err != nil {
			s.log.Error("cannot forget the finished job", "mac", mac, "err", err)
		}
		s.mu.Unlock()
		s.log.Info("job finished", "mac", mac, "tag", j.Tag, "status", in.Status)
	}
	w.WriteHeader(http.StatusNoContent)
}

// routes is the imaging network's side: the job, the status, and the
// netboot files when the station serves them itself.
func (s *station) routes(boot string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /job", s.getJob)
	mux.HandleFunc("POST /status", s.postStatus)
	if boot != "" {
		mux.Handle("GET /boot/", http.StripPrefix("/boot/", http.FileServer(http.Dir(boot))))
	}
	return mux
}
