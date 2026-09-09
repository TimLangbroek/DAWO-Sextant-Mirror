package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/token"
)

func TestStationCredentialsBindToStation(t *testing.T) {
	store := newMemTokenStore()
	sc := NewStationCredentials(store, newFakeClock(testT0))
	ctx := context.Background()

	secretA, err := sc.Issue(ctx, "nuc-1")
	if err != nil {
		t.Fatal(err)
	}
	secretB, err := sc.Issue(ctx, "nuc-2")
	if err != nil {
		t.Fatal(err)
	}

	// A station credential proves exactly its own station tag.
	if !sc.AuthenticateTag(ctx, secretA, "nuc-1") {
		t.Fatal("station A rejected for its own tag")
	}
	// THE IMPERSONATION GAP: nuc-1's credential cannot report as nuc-2.
	if sc.AuthenticateTag(ctx, secretA, "nuc-2") {
		t.Fatal("station A reported as station B")
	}
	if sc.AuthenticateTag(ctx, secretB, "nuc-1") {
		t.Fatal("station B reported as station A")
	}
	if sc.AuthenticateTag(ctx, "not-a-token", "nuc-1") {
		t.Fatal("garbage accepted")
	}
	if sc.AuthenticateTag(ctx, secretA+"x", "nuc-1") {
		t.Fatal("tampered secret accepted")
	}
}

func TestStationCredentialReissueRotatesAndRevoke(t *testing.T) {
	store := newMemTokenStore()
	sc := NewStationCredentials(store, newFakeClock(testT0))
	ctx := context.Background()

	old, _ := sc.Issue(ctx, "nuc-1")
	fresh, _ := sc.Issue(ctx, "nuc-1")
	if old == fresh {
		t.Fatal("re-issue produced the same secret")
	}
	if sc.AuthenticateTag(ctx, old, "nuc-1") {
		t.Fatal("old station credential still valid after re-issue")
	}
	if !sc.AuthenticateTag(ctx, fresh, "nuc-1") {
		t.Fatal("fresh station credential rejected")
	}
	if err := sc.Revoke(ctx, "nuc-1"); err != nil {
		t.Fatal(err)
	}
	if sc.AuthenticateTag(ctx, fresh, "nuc-1") {
		t.Fatal("revoked station credential still authenticates")
	}
}

func TestStationAndDeviceCredentialsDoNotCross(t *testing.T) {
	// Station and device credentials share the token store. The kind wall
	// must hold both ways: a device credential must not pass as a station
	// report, nor a station credential as a device check-in - even when the
	// tag string happens to match.
	store := newMemTokenStore()
	sc := NewStationCredentials(store, newFakeClock(testT0))
	dc := NewDeviceCredentials(store, newFakeClock(testT0))
	ctx := context.Background()

	stationSecret, _ := sc.Issue(ctx, "shared-tag")
	deviceSecret, _ := dc.Issue(ctx, "shared-tag")

	if dc.AuthenticateTag(ctx, stationSecret, "shared-tag") {
		t.Error("station credential authenticated a device check-in")
	}
	if sc.AuthenticateTag(ctx, deviceSecret, "shared-tag") {
		t.Error("device credential authenticated a station report")
	}
	// Each still works on its own path.
	if !sc.AuthenticateTag(ctx, stationSecret, "shared-tag") {
		t.Error("station credential rejected on its own path")
	}
	if !dc.AuthenticateTag(ctx, deviceSecret, "shared-tag") {
		t.Error("device credential rejected on its own path")
	}
}

// The bridge rule below is the station twin of the device one that came out
// of H1: a shared token must not speak for a subject that holds its own
// credential. HasCredential is what the station endpoint asks, and it had no
// test.

// erroringTokenStore fails Get and nothing else, so a caller's fail-closed
// behaviour can be told apart from "no credential".
type erroringTokenStore struct {
	*memTokenStore
	err error
}

func (s *erroringTokenStore) Get(context.Context, string) (token.Token, bool, error) {
	return token.Token{}, false, s.err
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
