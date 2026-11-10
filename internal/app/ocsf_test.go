package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
)

// The OCSF export carries every commit and every reveal of the period as API
// Activity events, oldest first: a commit as an Update by its author, a
// reveal as a Read of medium severity by whoever read it.
func TestOCSFCarriesCommitsAndRevealsInOrder(t *testing.T) {
	svc, _ := newService(t, nil)
	store := newMemSecretStore()
	now := time.Now().UTC()
	clock := newFakeClock(now.Add(-30 * time.Minute))
	secrets := NewDeviceSecretsService(store, testSealer(t), clock, "")
	es := NewEvidenceService(svc, nil, SystemClock{}).WithReveals(secrets).WithVersion("1.2.3")
	ctx := context.Background()

	if err := secrets.Store(ctx, "lt-1", secret.LUKS, "phrase", "agent"); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Store(ctx, "lt-2", secret.Admin, "ABCDE-FGHJK-LMNPQ-RSTUV", "agent"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := secrets.Reveal(ctx, "lt-2", secret.Admin, "sub-without-mail"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(-time.Hour) // revealed before the other one
	if _, _, err := secrets.Reveal(ctx, "lt-1", secret.LUKS, "alice@example.com"); err != nil {
		t.Fatal(err)
	}

	events, err := es.OCSF(ctx, now.Add(-2*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("%d events, want two reveals and the seed commit: %+v", len(events), events)
	}
	for i, e := range events {
		if e.ClassUID != 6003 || e.CategoryUID != 6 || e.TypeUID != 6003*100+e.ActivityID ||
			e.StatusID != 1 || e.Metadata.Version != OCSFVersion || e.Metadata.Product.Version != "1.2.3" ||
			e.Metadata.UID == "" {
			t.Errorf("event %d does not hold to API Activity: %+v", i, e)
		}
		if i > 0 && e.Time < events[i-1].Time {
			t.Errorf("event %d is older than the one before it", i)
		}
	}

	luks, admin, commit := events[0], events[1], events[2]
	if luks.ActivityID != 2 || luks.SeverityID != 3 || luks.API.Operation != "reveal" ||
		luks.Actor.User.EmailAddr != "alice@example.com" || luks.Resources[0].UID != "lt-1" ||
		!strings.Contains(luks.Message, "LUKS recovery key of lt-1") {
		t.Errorf("recovery-key reveal: %+v", luks)
	}
	if admin.Actor.User.Name != "sub-without-mail" || admin.Actor.User.EmailAddr != "" ||
		!strings.Contains(admin.Message, "administrator password of lt-2") {
		t.Errorf("a reader without an address got one: %+v", admin.Actor)
	}
	if commit.ActivityID != 3 || commit.SeverityID != 1 || commit.API.Operation != "commit" ||
		commit.Message != "seed" || commit.Actor.User.Name != "t" || commit.Actor.User.EmailAddr != "t@t" ||
		!strings.HasPrefix(commit.Metadata.UID, "commit:") {
		t.Errorf("commit: %+v", commit)
	}

	// A period the reveal log cannot be read for is refused, not exported
	// with a hole in it.
	store.revealsErr = errors.New("database unreachable")
	if _, err := es.OCSF(ctx, now.Add(-2*time.Hour), now.Add(time.Hour)); err == nil {
		t.Fatal("an unreadable reveal log produced events")
	}
}
