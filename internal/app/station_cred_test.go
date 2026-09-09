package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/token"
)

// station_cred_test.go: the station half of the bridge rule.
//
// The device half has been pinned since the credential sweep found H1: a
// shared token must not speak for a subject that holds a credential of its
// own, or issuing credentials means nothing while the bridge is up. The
// station endpoint enforces the same narrowing and says so in its comment,
// but StationCredentials.HasCredential - the thing it asks - had no test at
// all.

// erroringTokenStore fails Get and nothing else, so a caller's fail-closed
// behaviour can be told apart from "no credential".
type erroringTokenStore struct {
	*memTokenStore
	err error
}

func (s *erroringTokenStore) Get(context.Context, string) (token.Token, bool, error) {
	return token.Token{}, false, s.err
}

func TestAStationCredentialAnswersOnlyForItsOwnStation(t *testing.T) {
	ctx := context.Background()
	store := newMemTokenStore()
	sc := NewStationCredentials(store, newFakeClock(testT0))

	secret, err := sc.Issue(ctx, "nuc-1")
	if err != nil {
		t.Fatal(err)
	}
	if !sc.AuthenticateTag(ctx, secret, "nuc-1") {
		t.Error("a station cannot authenticate as itself")
	}
	// The property the endpoint's comment names: a leaked credential cannot
	// report discoveries as a different station.
	if sc.AuthenticateTag(ctx, secret, "nuc-2") {
		t.Error("a station credential authenticated a DIFFERENT station; one leak would speak for the whole inspoelstraat")
	}
	if sc.AuthenticateTag(ctx, "not-the-secret", "nuc-1") {
		t.Error("a wrong secret authenticated")
	}
}

func TestHasCredentialDecidesWhetherTheBridgeMaySpeak(t *testing.T) {
	ctx := context.Background()
	store := newMemTokenStore()
	clock := newFakeClock(testT0)
	sc := NewStationCredentials(store, clock)

	has, err := sc.HasCredential(ctx, "nuc-1")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("a station with no credential reads as having one; the bridge would be refused for a station that needs it")
	}

	if _, err := sc.Issue(ctx, "nuc-1"); err != nil {
		t.Fatal(err)
	}
	if has, err = sc.HasCredential(ctx, "nuc-1"); err != nil || !has {
		t.Errorf("has = %v (%v) after issuing; the bridge token would speak for a credentialed station", has, err)
	}

	// One station's credential is not another's.
	if has, err = sc.HasCredential(ctx, "nuc-2"); err != nil || has {
		t.Errorf("nuc-2 reads as credentialed because nuc-1 is: %v (%v)", has, err)
	}

	// Expired still counts, exactly as on the device side: the remedy is to
	// re-issue, not to let the shared token take over. It reads like a bug
	// and is not.
	clock.Advance(boundCredTTL + time.Hour)
	if has, err = sc.HasCredential(ctx, "nuc-1"); err != nil || !has {
		t.Errorf("an expired station credential reads as absent (%v, %v); the bridge would speak for it", has, err)
	}
	if sc.AuthenticateTag(ctx, "anything", "nuc-1") {
		t.Error("the expired credential still authenticates")
	}

	// Revoked does not count, and then the bridge may speak again.
	if err := sc.Revoke(ctx, "nuc-1"); err != nil {
		t.Fatal(err)
	}
	if has, err = sc.HasCredential(ctx, "nuc-1"); err != nil || has {
		t.Errorf("a revoked credential still reads as present: %v (%v)", has, err)
	}
}

// TestHasCredentialFailsClosedOnAStoreError: the endpoint refuses the bridge
// when this call errors, so returning (false, nil) here would quietly open
// the thing the error was supposed to close.
func TestHasCredentialFailsClosedOnAStoreError(t *testing.T) {
	boom := errors.New("store unreachable")
	sc := NewStationCredentials(&erroringTokenStore{memTokenStore: newMemTokenStore(), err: boom}, newFakeClock(testT0))

	has, err := sc.HasCredential(context.Background(), "nuc-1")
	if err == nil {
		t.Fatal("a store error was reported as an answer; the caller cannot tell 'no credential' from 'cannot tell'")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error %v does not wrap the store's own; the log would not say what broke", err)
	}
	if has {
		t.Error("has = true on an error")
	}
}
