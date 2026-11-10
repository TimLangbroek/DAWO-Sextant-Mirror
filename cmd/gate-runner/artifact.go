package main

// artifact.go: files the runner builds from the fleet repository for people
// rather than for devices (design 0016). Today one: the organisation's
// installation USB. POST /artifact starts or reports the job, poll-style and
// idempotent per (rev, name) like /build; GET /artifact/{rev}/{name} hands
// the file to the console, which streams it on to the person downloading.
//
// The files stay off /cache/ on purpose. The cache is readable by every
// device; an installer image is for whoever may add a laptop, and the
// console decides who that is.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// artifactAttrs maps an artifact name to the flake output that builds it.
var artifactAttrs = map[string]string{
	"installer": "packages.x86_64-linux.installer",
}

// keepArtifactRevs is how many revisions of each artifact stay on disk. Two:
// the current one, and the one before it for whoever is still downloading.
const keepArtifactRevs = 2

type artifactRequest struct {
	Rev  string `json:"rev"`
	Name string `json:"name"`
}

type artifactResponse struct {
	Phase  string `json:"phase"` // building | done | failed
	Detail string `json:"detail,omitempty"`
	// File is the name to save it under and Size its length in bytes, once
	// done.
	File string `json:"file,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// artifactDir is where finished artifacts live: next to the binary cache,
// never inside it.
func (s *server) artifactDir() string {
	if s.cacheDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(filepath.Clean(s.cacheDir)), "artifacts")
}

func (s *server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="gate-runner"`)
		writeJSON(w, http.StatusUnauthorized, artifactResponse{Phase: "failed", Detail: "unauthorized"})
		return
	}
	if s.artifactDir() == "" {
		writeJSON(w, http.StatusNotImplemented,
			artifactResponse{Phase: "failed", Detail: "no cache directory on this runner, so nowhere to keep an artifact"})
		return
	}
	var req artifactRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, artifactResponse{Phase: "failed", Detail: "bad request body"})
		return
	}
	if !revRe.MatchString(req.Rev) {
		writeJSON(w, http.StatusBadRequest, artifactResponse{Phase: "failed", Detail: "rev must be a commit hash"})
		return
	}
	if _, ok := artifactAttrs[req.Name]; !ok {
		writeJSON(w, http.StatusBadRequest, artifactResponse{Phase: "failed", Detail: fmt.Sprintf("no artifact called %q", req.Name)})
		return
	}

	key := req.Rev + "-" + req.Name
	s.buildMu.Lock()
	job, ok := s.artifacts[key]
	if !ok {
		// Built before this process started: the file is the verdict.
		if path, err := s.artifactFile(req.Rev, req.Name); err == nil {
			job = doneArtifact(path)
		} else {
			job = &artifactResponse{Phase: "building"}
			go s.runArtifact(key, req)
		}
		if s.artifacts == nil {
			s.artifacts = map[string]*artifactResponse{}
		}
		s.artifacts[key] = job
	}
	resp := *job
	s.buildMu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// runArtifact builds one artifact in the background, in the same slot as
// release builds: an image build is as heavy as a closure build.
func (s *server) runArtifact(key string, req artifactRequest) {
	s.buildSlot <- struct{}{}
	defer func() { <-s.buildSlot }()

	ctx := context.Background()
	link := filepath.Join(s.artifactDir(), req.Rev+"-"+req.Name)
	err := s.atRevision(ctx, req.Rev, "artifact", func(scratch string) error {
		if err := os.MkdirAll(s.artifactDir(), 0o750); err != nil {
			return err
		}
		build := s.artifactFn
		if build == nil {
			build = nixBuildArtifact
		}
		return build(ctx, scratch, artifactAttrs[req.Name], link)
	})
	var path string
	if err == nil {
		path, err = s.artifactFile(req.Rev, req.Name)
	}

	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	if err != nil {
		s.log.Error("artifact build failed", "rev", req.Rev, "name", req.Name, "err", err)
		s.artifacts[key] = &artifactResponse{Phase: "failed", Detail: shortDetail(err)}
		return
	}
	s.log.Info("artifact built", "rev", req.Rev, "name", req.Name, "file", filepath.Base(path))
	s.artifacts[key] = doneArtifact(path)
	s.pruneArtifacts(req.Name)
}

func doneArtifact(path string) *artifactResponse {
	resp := &artifactResponse{Phase: "done", File: filepath.Base(path)}
	if fi, err := os.Stat(path); err == nil {
		resp.Size = fi.Size()
	}
	return resp
}

// nixBuildArtifact builds attr from the flake in dir with an out-link at
// link, which also keeps the result from the store's garbage collection
// until the link is pruned.
func nixBuildArtifact(ctx context.Context, dir, attr, link string) error {
	cmd := exec.CommandContext(ctx, "nix", "build", "--no-write-lock-file",
		"--out-link", link, dir+"#"+attr)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nix build %s: %w: %s", attr, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// artifactFile finds the one image a built artifact holds: an ISO under
// iso/, which is where NixOS's image builders put it.
func (s *server) artifactFile(rev, name string) (string, error) {
	link := filepath.Join(s.artifactDir(), rev+"-"+name)
	found, err := filepath.Glob(filepath.Join(link, "iso", "*.iso"))
	if err != nil {
		return "", err
	}
	if len(found) != 1 {
		return "", fmt.Errorf("%s at %s holds %d images, want one", name, rev, len(found))
	}
	return found[0], nil
}

// pruneArtifacts removes all but the newest keepArtifactRevs of an artifact.
// Removing the out-link lets the store collect the image.
func (s *server) pruneArtifacts(name string) {
	links, _ := filepath.Glob(filepath.Join(s.artifactDir(), "*-"+name))
	type built struct {
		link string
		at   int64
	}
	var all []built
	for _, l := range links {
		if fi, err := os.Lstat(l); err == nil {
			all = append(all, built{l, fi.ModTime().UnixNano()})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at > all[j].at })
	for i := keepArtifactRevs; i < len(all); i++ {
		if err := os.RemoveAll(all[i].link); err != nil {
			s.log.Warn("artifact not pruned", "link", all[i].link, "err", err)
			continue
		}
		rev := strings.TrimSuffix(filepath.Base(all[i].link), "-"+name)
		delete(s.artifacts, rev+"-"+name)
	}
}

// handleArtifactFile hands a finished artifact to the console.
func (s *server) handleArtifactFile(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="gate-runner"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rev, name := r.PathValue("rev"), r.PathValue("name")
	if !revRe.MatchString(rev) || artifactAttrs[name] == "" || s.artifactDir() == "" {
		http.NotFound(w, r)
		return
	}
	path, err := s.artifactFile(rev, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path) // #nosec G304 - path is under artifactDir, built from a checked rev and a known name
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "unreadable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
	http.ServeContent(w, r, filepath.Base(path), fi.ModTime(), f)
}
