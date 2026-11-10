package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// remoteartifact.go: ports.ArtifactBuilder against the gate-runner's
// /artifact endpoints (design 0016). The runner builds the image; the
// console only asks and passes the file on.

type artifactRequest struct {
	Rev  string `json:"rev"`
	Name string `json:"name"`
}

type artifactResponse struct {
	Phase  string `json:"phase"`
	Detail string `json:"detail,omitempty"`
	File   string `json:"file,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

// EnsureArtifact implements ports.ArtifactBuilder.
func (b *RemoteBuilder) EnsureArtifact(ctx context.Context, rev, name string) (ports.ArtifactState, error) {
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(artifactRequest{Rev: rev, Name: name})
	if err != nil {
		return ports.ArtifactState{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.URL+"/artifact", bytes.NewReader(body))
	if err != nil {
		return ports.ArtifactState{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	b.authorize(req)
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return ports.ArtifactState{}, fmt.Errorf("build runner unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var ar artifactResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return ports.ArtifactState{}, fmt.Errorf("build runner answered %d: %s", resp.StatusCode, raw)
	}
	if resp.StatusCode != http.StatusOK {
		// The runner says why in its own words (no cache directory, an
		// unknown name); that is the sentence the person needs.
		return ports.ArtifactState{}, fmt.Errorf("build runner: %s", ar.Detail)
	}
	st := ports.ArtifactState{Phase: ports.BuildBuilding, Detail: ar.Detail, File: ar.File, Size: ar.Size}
	switch ar.Phase {
	case "done":
		st.Phase = ports.BuildDone
	case "failed":
		st.Phase = ports.BuildFailed
	}
	return st, nil
}

// OpenArtifact implements ports.ArtifactBuilder. No timeout of its own: an
// image is a gigabyte, and the caller's request bounds the copy.
func (b *RemoteBuilder) OpenArtifact(ctx context.Context, rev, name string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		b.URL+"/artifact/"+url.PathEscape(rev)+"/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}
	b.authorize(req)
	resp, err := b.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("build runner unreachable: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("build runner has no %s for %s (status %d)", name, rev, resp.StatusCode)
	}
	return resp.Body, nil
}

func (b *RemoteBuilder) authorize(req *http.Request) {
	if b.Token != "" {
		req.Header.Set("Authorization", "Bearer "+b.Token)
	}
}

func (b *RemoteBuilder) httpClient() *http.Client {
	if b.client == nil {
		return http.DefaultClient
	}
	return b.client
}
