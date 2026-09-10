package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/notify"
)

// async_detach_test.go: what happens to a gated write that outlives its
// request.
//
// Two properties hold this together and neither was covered. The write must
// not be cancelled when the browser is redirected away, because a
// half-validated commit aborted mid-flight is the worst outcome available.
// And a write that detaches has to report its own ending, because the
// operator has already been told "validating" and will not look again.

// spyNotify records what the console tells the operator.
type spyNotify struct {
	mu   sync.Mutex
	sent []notify.Notification
}

func (s *spyNotify) Add(_ context.Context, n notify.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, n)
	return nil
}

func (s *spyNotify) ListFor(context.Context, string, string, []string, int) ([]notify.Notification, error) {
	return nil, nil
}
func (s *spyNotify) UnreadCount(context.Context, string, string, []string) (int, error) {
	return 0, nil
}
func (s *spyNotify) MarkRead(context.Context, string, string, string) error      { return nil }
func (s *spyNotify) MarkAllRead(context.Context, string, string, []string) error { return nil }

func (s *spyNotify) kinds() []notify.Kind {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]notify.Kind, 0, len(s.sent))
	for _, n := range s.sent {
		out = append(out, n.Kind)
	}
	return out
}

type frozenClock struct{ t time.Time }

func (c frozenClock) Now() time.Time { return c.t }

// detachFixture is a bare Server with a notifier: runGatedDetached touches
// nothing else.
func detachFixture(t *testing.T) (*Server, *spyNotify, view) {
	t.Helper()
	spy := &spyNotify{}
	s := &Server{
		svc: Services{Notify: app.NewNotifyService(spy, frozenClock{time.Unix(1, 0).UTC()}, app.DefaultTenant)},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	v := view{User: identity.User{Subject: "ada"}, L: newLocalizer(identity.Preferences{}, "en", "UTC")}
	return s, spy, v
}

func TestAWriteThatFinishesInTimeDoesNotDetach(t *testing.T) {
	s, spy, v := detachFixture(t)
	r := httptest.NewRequest("POST", "/anything", nil)
	boom := errors.New("the gate said no")

	detached, err := s.runGatedDetached(r, v, "group pilot re-parented",
		func(context.Context) error { return boom })

	if detached {
		t.Error("a write that returned at once reported itself as detached")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the gate's own error: the operator has to see it on the page", err)
	}
	if k := spy.kinds(); len(k) != 0 {
		t.Errorf("notifications sent for a write that never detached: %v", k)
	}
}

// TestARedirectDoesNotCancelTheWrite is the WithoutCancel contract. The
// browser is gone by the time the gate finishes; if the write inherited that
// cancellation it would abort halfway through a commit.
func TestARedirectDoesNotCancelTheWrite(t *testing.T) {
	s, _, v := detachFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("POST", "/anything", nil).WithContext(ctx)
	cancel() // the request is over before the write starts

	var sawErr error
	if _, err := s.runGatedDetached(r, v, "desc", func(c context.Context) error {
		sawErr = c.Err()
		return nil
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if sawErr != nil {
		t.Errorf("the write inherited the request's cancellation: %v", sawErr)
	}
}

// TestADetachedWriteReportsItsOwnEnding: the operator was told "validating"
// and has moved on, so the refusal has to arrive by itself.
func TestADetachedWriteReportsItsOwnEnding(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: this one waits out the grace window on purpose")
	}
	s, spy, v := detachFixture(t)
	r := httptest.NewRequest("POST", "/anything", nil)
	release := make(chan struct{})

	detached, err := s.runGatedDetached(r, v, "group pilot re-parented", func(context.Context) error {
		<-release
		return errors.New("the gate said no")
	})
	if err != nil {
		t.Fatalf("a detached write must not report an error to the page: %v", err)
	}
	if !detached {
		t.Fatal("a write past the grace window did not detach")
	}
	if k := spy.kinds(); len(k) != 1 || k[0] != notify.WritePending {
		t.Fatalf("first notification = %v, want one WritePending: the badge is the only sign the write is still running", k)
	}

	close(release)
	deadline := time.After(2 * time.Second)
	for {
		if k := spy.kinds(); len(k) == 2 {
			if k[1] != notify.GateFailed {
				t.Errorf("second notification = %v, want GateFailed", k[1])
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("a detached write that failed never told anybody: %v", spy.kinds())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
