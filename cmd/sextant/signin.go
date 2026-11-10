package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/oidc"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
)

// signinRetry is how often a console whose identity provider did not answer
// at start asks again. A variable so a test does not wait half a minute.
var signinRetry = 30 * time.Second

// signin is the console's sign-in, also before the identity provider has
// answered. The console used to ask the provider once, at start, and when
// that failed it never mounted at all: the root said "Sextant" in plain
// text until somebody restarted the process. Nothing did that by itself,
// because the liveness and startup probes do not look at sign-in. A cluster
// that came back with its identity provider a minute after Sextant was
// enough to leave the console without a login for good.
//
// Now the console is there from the start, sign-in says in words that it is
// waiting, and the provider is asked again until it answers.
type signin struct {
	issuer string
	ready  atomic.Pointer[signinReady]

	mu  sync.Mutex
	err error
}

type signinReady struct {
	authn  *oidc.Authenticator
	routes http.Handler
}

// SessionUser implements the console's and the API's session source.
// Nobody is signed in until the provider has answered.
func (s *signin) SessionUser(r *http.Request) (identity.User, string, bool) {
	if ready := s.ready.Load(); ready != nil {
		return ready.authn.SessionUser(r)
	}
	return identity.User{}, "", false
}

// ServeHTTP serves the login routes, or says why there are none yet.
func (s *signin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ready := s.ready.Load(); ready != nil {
		ready.routes.ServeHTTP(w, r)
		return
	}
	// The provider's own error stays in the log and on /readyz: this page
	// is for anybody who opens the console, signed in or not.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Retry-After", fmt.Sprint(int(signinRetry.Seconds())))
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = fmt.Fprintf(w, "Sign-in is not available yet: the identity provider at %s has not answered.\n"+
		"The console asks again every %s; try again in a minute.\n", s.issuer, signinRetry)
}

// check is the readiness check: not ready while there is no sign-in.
func (s *signin) check(context.Context) error {
	if s.ready.Load() != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Errorf("waiting for the identity provider: %w", s.err)
}

// start asks the provider once, and when it does not answer keeps asking
// in the background until it does or ctx ends.
func (s *signin) start(ctx context.Context, connect func(context.Context) (*oidc.Authenticator, error),
	routes func(*oidc.Authenticator) http.Handler, log *slog.Logger) {
	try := func() bool {
		a, err := connect(ctx)
		if err != nil {
			s.mu.Lock()
			s.err = err
			s.mu.Unlock()
			return false
		}
		s.ready.Store(&signinReady{authn: a, routes: routes(a)})
		log.Info("oidc session auth mounted", "issuer", s.issuer)
		return true
	}
	if try() {
		return
	}
	log.Error("oidc discovery failed; sign-in waits for the identity provider and asks again",
		"issuer", s.issuer, "every", signinRetry, "err", s.check(ctx))
	go func() {
		t := time.NewTicker(signinRetry)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if try() {
					return
				}
			}
		}
	}()
}
